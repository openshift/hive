package v1migration

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/client-go/dynamic"
	"k8s.io/utils/pointer"

	contributils "github.com/openshift/hive/contrib/pkg/utils"
	hivev1alpha1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
)

// DeleteObjectsOptions is the set of options for the deleting Hive objects.
type DeleteObjectsOptions struct {
	fileName string
	dryRun   bool
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
	flags := cmd.Flags()
	flags.BoolVar(&opt.dryRun, "dry-run", false, "Whether this is a dry run")
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
	decoder := json.NewDecoder(bufio.NewReader(file))
	for {
		var objFromFile unstructured.Unstructured
		if err := decoder.Decode(&objFromFile); err != nil {
			if err == io.EOF {
				break
			}
			return errors.Wrap(err, "could not decode JSON from file")
		}
		o.deleteObject(client, &objFromFile)
	}
	return nil
}

func (o *DeleteObjectsOptions) deleteObject(client dynamic.Interface, objFromFile *unstructured.Unstructured) {
	apiVersion := objFromFile.GetAPIVersion()
	kind := objFromFile.GetKind()
	namespace := objFromFile.GetNamespace()
	name := objFromFile.GetName()
	logger := log.WithFields(log.Fields{
		"apiVersion": apiVersion,
		"kind":       kind,
		"name":       name,
	})
	if namespace != "" {
		logger = logger.WithField("namespace", namespace)
	}
	if apiVersion != hivev1alpha1.SchemeGroupVersion.String() {
		logger.Warn("object in JSON file is not a Hive v1alpha1 resource")
		return
	}
	gvr := hivev1alpha1.SchemeGroupVersion.WithResource(resourceForHiveKind(kind))
	var resourceClient dynamic.ResourceInterface
	if namespace != "" {
		resourceClient = client.Resource(gvr).Namespace(namespace)
	} else {
		resourceClient = client.Resource(gvr)
	}
	objFromKube, err := resourceClient.Get(name, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		logger.Warn("object not found")
		return
	case err != nil:
		logger.WithError(err).Error("failed to get object")
		return
	}

	// Ignore owner refs and resource version when looking for diffs as some Hive objects may have been orphaned.
	// TODO: Check changes to owner refs to ensure that the only changes are removals of references to Hive objects.
	objFromFile.SetOwnerReferences(objFromKube.GetOwnerReferences())
	objFromFile.SetResourceVersion(objFromKube.GetResourceVersion())

	if !reflect.DeepEqual(objFromFile, objFromKube) {
		logger.Warn("object in JSON file differs from object in the cluster")
		fmt.Printf("Diff in %s/%s (%s)\n%s\n", kind, name, namespace, diff.ObjectReflectDiff(objFromFile, objFromKube))
	}
	if !o.dryRun {
		if finalizersFromKube := objFromKube.GetFinalizers(); len(finalizersFromKube) > 0 {
			objFromKube.SetFinalizers(nil)
			objFromKube, err = resourceClient.Update(objFromKube, metav1.UpdateOptions{})
			if err != nil {
				logger.WithError(err).Error("could not remove finalizers")
				return
			}
		}
		uid := objFromKube.GetUID()
		propagationPolicy := metav1.DeletePropagationOrphan
		deleteOptions := &metav1.DeleteOptions{
			Preconditions: &metav1.Preconditions{
				UID:             &uid,
				ResourceVersion: pointer.StringPtr(objFromKube.GetResourceVersion()),
			},
			PropagationPolicy: &propagationPolicy,
		}
		if err := resourceClient.Delete(name, deleteOptions); err != nil {
			logger.WithError(err).Error("could not delete object")
			return
		}
		logger.Info("deleted object")
	}
}
