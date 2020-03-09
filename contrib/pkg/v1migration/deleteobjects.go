package v1migration

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"runtime"
	"sync"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"

	contributils "github.com/openshift/hive/contrib/pkg/utils"
	hivev1alpha1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
)

var removeFinalizersPatch = []byte(`{"metadata":{"finalizers":[]}}`)

// DeleteObjectsOptions is the set of options for the deleting Hive objects.
type DeleteObjectsOptions struct {
	fileName string
}

// NewDeleteObjectsCommand creates a command that executes the migration utility to delete Hive objects stored in a file.
func NewDeleteObjectsCommand() *cobra.Command {
	opt := &DeleteObjectsOptions{}
	cmd := &cobra.Command{
		Use:   "delete-objects JSON_FILE_NAME",
		Short: "delete the Hive objects stored in the JSON file",
		Args:  cobra.ExactArgs(1),
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
	return cmd
}

// Complete finishes parsing arguments for the command
func (o *DeleteObjectsOptions) Complete(cmd *cobra.Command, args []string) error {
	o.fileName = args[0]
	return nil
}

// Validate ensures that option values make sense
func (o *DeleteObjectsOptions) Validate(cmd *cobra.Command) error {
	return nil
}

// Run executes the command
func (o *DeleteObjectsOptions) Run() error {
	clientConfig, err := contributils.GetClientConfig()
	if err != nil {
		return errors.Wrap(err, "could not get the client config")
	}
	client, err := dynamic.NewForConfig(clientConfig)
	if err != nil {
		return errors.Wrap(err, "could not create kube client")
	}
	file, err := os.Open(o.fileName)
	if err != nil {
		errors.Wrap(err, "could not open the JSON file")
	}
	defer file.Close()
	logger := log.StandardLogger()
	objChan := make(chan *unstructured.Unstructured)
	var deleteWG sync.WaitGroup
	for i := 0; i < runtime.NumCPU(); i++ {
		deleteWG.Add(1)
		go deleteObjects(client, objChan, &deleteWG, logger)
	}
	decoder := json.NewDecoder(bufio.NewReader(file))
	for {
		var objFromFile unstructured.Unstructured
		if err := decoder.Decode(&objFromFile); err != nil {
			if err == io.EOF {
				break
			}
			return errors.Wrap(err, "could not decode JSON from file")
		}
		objChan <- &objFromFile
	}
	close(objChan)
	deleteWG.Wait()
	return nil
}

func deleteObjects(client dynamic.Interface, objChan chan *unstructured.Unstructured, wg *sync.WaitGroup, logger log.FieldLogger) {
	defer wg.Done()
	for objFromFile := range objChan {
		apiVersion := objFromFile.GetAPIVersion()
		kind := objFromFile.GetKind()
		namespace := objFromFile.GetNamespace()
		name := objFromFile.GetName()
		logger := logger.WithFields(log.Fields{
			"apiVersion": apiVersion,
			"kind":       kind,
			"name":       name,
		})
		if namespace != "" {
			logger = logger.WithField("namespace", namespace)
		}
		if apiVersion != hivev1alpha1.SchemeGroupVersion.String() {
			logger.Warn("object in JSON file is not a Hive v1alpha1 resource")
			continue
		}
		gvr := hivev1alpha1.SchemeGroupVersion.WithResource(resourceForHiveKind(kind))
		var resourceClient dynamic.ResourceInterface
		if namespace != "" {
			resourceClient = client.Resource(gvr).Namespace(namespace)
		} else {
			resourceClient = client.Resource(gvr)
		}

		if finalizersFromKube := objFromFile.GetFinalizers(); len(finalizersFromKube) > 0 {
			if _, err := resourceClient.Patch(objFromFile.GetName(), types.MergePatchType, []byte(removeFinalizersPatch), metav1.PatchOptions{}); err != nil {
				logger.WithError(err).Error("could not remove finalizers")
				continue
			}
		}
		propagationPolicy := metav1.DeletePropagationOrphan
		deleteOptions := &metav1.DeleteOptions{
			PropagationPolicy: &propagationPolicy,
		}
		if err := resourceClient.Delete(name, deleteOptions); err != nil {
			logger.WithError(err).Error("could not delete object")
			continue
		}
		logger.Info("deleted object")
	}
}
