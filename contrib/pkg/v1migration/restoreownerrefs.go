package v1migration

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	contributils "github.com/openshift/hive/contrib/pkg/utils"
	hivev1alpha1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
)

// RestoreOwnerRefsOptions is the set of options for the restoring owner references.
type RestoreOwnerRefsOptions struct {
	workDir string
}

// NewRestoreOwnerRefsCommand creates a command that executes the migration utility to restore owner references to Hive resources.
func NewRestoreOwnerRefsCommand() *cobra.Command {
	opt := &RestoreOwnerRefsOptions{}
	cmd := &cobra.Command{
		Use:   "restore-owner-refs",
		Short: "restore from a file the owner references to Hive resources",
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
	flags.StringVar(&opt.workDir, "work-dir", ".", "Directory containing owner references file")
	return cmd
}

// Complete finishes parsing arguments for the command
func (o *RestoreOwnerRefsOptions) Complete(cmd *cobra.Command, args []string) error {
	return nil
}

// Validate ensures that option values make sense
func (o *RestoreOwnerRefsOptions) Validate(cmd *cobra.Command) error {
	return validateWorkDir(o.workDir)
}

// Run executes the command
func (o *RestoreOwnerRefsOptions) Run() error {
	clientConfig, err := contributils.GetClientConfig()
	if err != nil {
		return errors.Wrap(err, "could not get the client config")
	}
	client, err := dynamic.NewForConfig(clientConfig)
	if err != nil {
		return errors.Wrap(err, "could not create kube client")
	}
	file, err := os.Open(filepath.Join(o.workDir, ownerRefsFilename))
	if err != nil {
		return errors.Wrap(err, "could not open owner refs file")
	}
	defer file.Close()
	logger := log.StandardLogger()
	ownerRefChan := make(chan ownerRef)
	var processWG sync.WaitGroup
	for i := 0; i < runtime.NumCPU(); i++ {
		processWG.Add(1)
		go processOwnerRefs(client, ownerRefChan, &processWG, logger)
	}
	decoder := json.NewDecoder(bufio.NewReader(file))
	for {
		var ref ownerRef
		if err := decoder.Decode(&ref); err != nil {
			if err == io.EOF {
				break
			}
			return errors.Wrap(err, "could not decode JSON from file")
		}
		ownerRefChan <- ref
	}
	close(ownerRefChan)
	processWG.Wait()
	return nil
}

func processOwnerRefs(client dynamic.Interface, ownerRefChan chan ownerRef, wg *sync.WaitGroup, logger log.FieldLogger) {
	defer wg.Done()
	for ref := range ownerRefChan {
		logger := log.WithField("resource", ref.Resource).WithField("name", ref.Name)
		if ref.Namespace != "" {
			logger = logger.WithField("namespace", ref.Namespace)
		}
		obj, err := ownedClient(client, ref).Get(ref.Name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				if isRefFromPVCToCD(ref) || isRefFromDNSEndpoint(ref) {
					logger.Info("skipping reference of ClusterDeployment owning deleted PVC")
					continue
				}
			}
			logger.WithError(err).Error("could not get object to restore owner reference")
			continue
		}
		logger = logger.WithField("referencedKind", ref.Ref.Kind).WithField("referencedName", ref.Ref.Name)
		ownerObj, err := ownerClient(client, ref).Get(ref.Ref.Name, metav1.GetOptions{})
		if err != nil {
			logger.WithError(err).Error("could not get owner object")
			continue
		}
		if ownerRefs, changed := restoreOwnerReference(obj.GetOwnerReferences(), ref, ownerObj); changed {
			logger.Info("restoring owner reference")
			obj.SetOwnerReferences(ownerRefs)
			if _, err := ownedClient(client, ref).Update(obj, metav1.UpdateOptions{}); err != nil {
				logger.WithError(err).Error("could not update object")
			}
		} else {
			logger.Info("owner reference already restored")
		}
	}
}

func ownedClient(client dynamic.Interface, ref ownerRef) dynamic.ResourceInterface {
	gvr := schema.GroupVersionResource{
		Group:    ref.Group,
		Version:  ref.Version,
		Resource: ref.Resource,
	}
	if ns := ref.Namespace; ns != "" {
		return client.Resource(gvr).Namespace(ns)
	}
	return client.Resource(gvr)
}

func ownerClient(client dynamic.Interface, ref ownerRef) dynamic.ResourceInterface {
	gvr := hivev1alpha1.SchemeGroupVersion.WithResource(resourceForHiveKind(ref.Ref.Kind))
	if isNamespaceScoped(ref.Ref.Kind) {
		return client.Resource(gvr).Namespace(ref.Namespace)
	}
	return client.Resource(gvr)
}

func restoreOwnerReference(ownerRefs []metav1.OwnerReference, ref ownerRef, ownerObj *unstructured.Unstructured) (newOwnerRefs []metav1.OwnerReference, changed bool) {
	newUID := ownerObj.GetUID()
	for i, ownerRef := range ownerRefs {
		if ownerRef.APIVersion != ref.Ref.APIVersion ||
			ownerRef.Kind != ref.Ref.Kind ||
			ownerRef.Name != ref.Ref.Name {
			continue
		}
		if ownerRef.UID != newUID {
			ownerRefs[i].UID = newUID
			ownerRefs[i].BlockOwnerDeletion = ref.Ref.BlockOwnerDeletion
			ownerRefs[i].Controller = ref.Ref.Controller
			changed = true
		}
		return ownerRefs, changed
	}
	ownerRef := ref.Ref
	ownerRef.UID = newUID
	return append(ownerRefs, ownerRef), true
}

func resourceForHiveKind(kind string) string {
	// We can do this more safely by getting the resource using the discovery client, but we are only dealing with
	// Hive resources, for which we know that the resource name will fit the expected pattern.
	return strings.ToLower(kind) + "s"
}

func isNamespaceScoped(kind string) bool {
	// We can do this more safely by getting the resource using the discovery client, but we are only dealing with
	// Hive resources, for which we know whether the resource is namespace-scoped or cluster-scoped.
	switch kind {
	case "ClusterImageSet", "HiveConfig", "SelectorSyncIdentityProvider", "SelectorSyncSet":
		return false
	default:
		return true
	}
}

func isRefFromPVCToCD(ref ownerRef) bool {
	return ref.Resource == "persistentvolumeclaims" && ref.Group == "" &&
		ref.Ref.Kind == "ClusterDeployment" && ref.Ref.APIVersion == hivev1alpha1.SchemeGroupVersion.String()
}

func isRefFromDNSEndpoint(ref ownerRef) bool {
	return ref.Resource == "dnsendpoints" && ref.Group == hivev1alpha1.GroupName
}
