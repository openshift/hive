package v1migration

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	contributils "github.com/openshift/hive/contrib/pkg/utils"
	hivev1alpha1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/imageset"
)

// fill this out with the resources discovered using the following command:
//    hiveutil v1migration discover-owned
var defaultResources = []schema.GroupVersionResource{
	hivev1alpha1.SchemeGroupVersion.WithResource("clusterdeprovisionrequests"),
	hivev1alpha1.SchemeGroupVersion.WithResource("clusterprovisions"),
	hivev1alpha1.SchemeGroupVersion.WithResource("clusterstates"),
	hivev1alpha1.SchemeGroupVersion.WithResource("dnszones"),
	hivev1alpha1.SchemeGroupVersion.WithResource("syncidentityproviders"),
	hivev1alpha1.SchemeGroupVersion.WithResource("syncsetinstances"),
	hivev1alpha1.SchemeGroupVersion.WithResource("syncsets"),
	corev1.SchemeGroupVersion.WithResource("secrets"),
	corev1.SchemeGroupVersion.WithResource("persistentvolumeclaims"),
	batchv1.SchemeGroupVersion.WithResource("jobs"),
	{
		Group:    "certman.managed.openshift.io",
		Version:  "v1alpha1",
		Resource: "certificaterequests",
	},
}

// SaveOwnerRefsOptions is the set of options for the saving owner references.
type SaveOwnerRefsOptions struct {
	workDir         string
	resourcesFile   string
	listLimit       int64
	outputResources bool
}

// NewSaveOwnerRefsCommand creates a command that executes the migration utility to save owner references to Hive resources.
func NewSaveOwnerRefsCommand() *cobra.Command {
	opt := &SaveOwnerRefsOptions{}
	cmd := &cobra.Command{
		Use:   "save-owner-refs",
		Short: "save to a file the owner references to Hive resources",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			log.SetLevel(log.InfoLevel)
			if err := opt.Complete(cmd, args); err != nil {
				log.WithError(err).Fatal("Error")
			}

			if err := opt.Validate(cmd); err != nil {
				log.WithError(err).Fatal("Error")
			}

			if err := opt.Run(); err != nil {
				log.WithError(err).Fatal("Error")
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opt.workDir, "work-dir", ".", "Directory in which store owner references file")
	flags.StringVar(&opt.resourcesFile, "resources", "", "File containing the resources to look at for owner references")
	flags.Int64Var(&opt.listLimit, "list-limit", 500, "Number of resources to fetch at a time")
	flags.BoolVar(&opt.outputResources, "output-resources", false, "Output a file to use as input with the --resources flag")
	return cmd
}

// Complete finishes parsing arguments for the command
func (o *SaveOwnerRefsOptions) Complete(cmd *cobra.Command, args []string) error {
	return nil
}

// Validate ensures that option values make sense
func (o *SaveOwnerRefsOptions) Validate(cmd *cobra.Command) error {
	if o.outputResources {
		return nil
	}
	return validateWorkDir(o.workDir)
}

// Run executes the command
func (o *SaveOwnerRefsOptions) Run() error {
	resources, err := resourcesToSearch(o.resourcesFile)
	if err != nil {
		return errors.Wrap(err, "could not determine the resources to search")
	}
	if o.outputResources {
		for _, r := range resources {
			fmt.Println(r)
		}
		return nil
	}
	clientConfig, err := contributils.GetClientConfig()
	if err != nil {
		return errors.Wrap(err, "could not get the client config")
	}
	client, err := dynamic.NewForConfig(clientConfig)
	if err != nil {
		return errors.Wrap(err, "could not create kube client")
	}
	logger := log.StandardLogger()
	objChan := make(chan objToProcess)
	ownerRefChan := make(chan ownerRef)
	var processWG sync.WaitGroup
	for i := 0; i < runtime.NumCPU(); i++ {
		processWG.Add(1)
		go processObjects(objChan, ownerRefChan, &processWG, logger)
	}
	var writeWG sync.WaitGroup
	writeWG.Add(1)
	go writeOwnerRefsToFile(filepath.Join(o.workDir, ownerRefsFilename), ownerRefChan, &writeWG, logger)
	for _, r := range resources {
		continueValue := ""
		for {
			objs, err := client.Resource(r).List(metav1.ListOptions{
				Limit:    o.listLimit,
				Continue: continueValue,
			})
			if err != nil {
				logger.WithError(err).WithField("resource", r).Fatal("could not list resource")
			}
			for i := range objs.Items {
				objChan <- objToProcess{
					gvr: r,
					obj: &objs.Items[i],
				}
			}
			continueValue = objs.GetContinue()
			if continueValue == "" {
				break
			}
		}
	}
	close(objChan)
	processWG.Wait()
	close(ownerRefChan)
	writeWG.Wait()
	return nil
}

type objToProcess struct {
	gvr schema.GroupVersionResource
	obj *unstructured.Unstructured
}

func processObjects(objChan chan objToProcess, ownerRefChan chan ownerRef, wg *sync.WaitGroup, logger log.FieldLogger) {
	defer wg.Done()
	for obj := range objChan {
		if isInstallOrImagesetJob(&obj) {
			logger.Debug("ignoring install or imageset job")
			continue
		}
		for _, ref := range obj.obj.GetOwnerReferences() {
			if ref.APIVersion != hivev1alpha1.SchemeGroupVersion.String() {
				continue
			}
			logger = logger.WithField("resource", obj.gvr.Resource).
				WithField("name", obj.obj.GetName()).
				WithField("ownerKind", ref.Kind).
				WithField("ownerName", ref.Name)
			if ns := obj.obj.GetNamespace(); ns != "" {
				logger = logger.WithField("namespace", ns)
			}
			logger.Info("found reference to Hive resource")
			ownerRefChan <- ownerRef{
				Group:     obj.gvr.Group,
				Version:   obj.gvr.Version,
				Resource:  obj.gvr.Resource,
				Namespace: obj.obj.GetNamespace(),
				Name:      obj.obj.GetName(),
				Ref:       ref,
			}
		}
	}
}

func writeOwnerRefsToFile(filename string, ownerRefChan chan ownerRef, wg *sync.WaitGroup, logger log.FieldLogger) {
	defer wg.Done()
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		logger.WithError(err).WithField("filename", filename).Fatal("could not create file")
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	defer writer.Flush()
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	for ownerRef := range ownerRefChan {
		if err := encoder.Encode(ownerRef); err != nil {
			logger.WithError(err).Fatal("could not marshal owner ref")
		}
	}
}

func isInstallOrImagesetJob(obj *objToProcess) bool {
	if obj.gvr.Group != batchv1.GroupName || obj.gvr.Resource != "jobs" {
		return false
	}
	labels := obj.obj.GetLabels()
	return labels[constants.InstallJobLabel] == "true" ||
		labels[imageset.ImagesetJobLabel] == "true"
}

func resourcesToSearch(filename string) ([]schema.GroupVersionResource, error) {
	if filename == "" {
		return defaultResources, nil
	}
	var resources []schema.GroupVersionResource
	file, err := os.Open(filename)
	if err != nil {
		return nil, errors.Wrap(err, "could not read the resources file")
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	lineNumber := 1
	lineExp := regexp.MustCompile("^\\s*([^\\/]*)\\/([^\\/]*),\\s*Resource=([^\\s]*)$")
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		submatches := lineExp.FindStringSubmatch(line)
		if submatches == nil {
			return nil, errors.Errorf("bad line (%d) in resources file: %s", lineNumber, line)
		}
		resources = append(resources, schema.GroupVersionResource{
			Group:    submatches[1],
			Version:  submatches[2],
			Resource: submatches[3],
		})
		lineNumber++
	}
	return resources, nil
}
