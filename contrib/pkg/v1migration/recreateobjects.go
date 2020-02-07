package v1migration

import (
	"bufio"
	"encoding/json"
	"io"
	"os"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	contributils "github.com/openshift/hive/contrib/pkg/utils"
	hivev1alpha1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
)

// RecreateObjectsOptions is the set of options for the re-creating Hive objects.
type RecreateObjectsOptions struct {
	fileName string
}

// NewRecreateObjectsCommand creates a command that executes the migration utility to re-create Hive objects stored in a file.
func NewRecreateObjectsCommand() *cobra.Command {
	opt := &RecreateObjectsOptions{}
	cmd := &cobra.Command{
		Use:   "recreate-objects JSON_FILE_NAME",
		Short: "re-create the Hive objects stored in the JSON file",
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
func (o *RecreateObjectsOptions) Complete(cmd *cobra.Command, args []string) error {
	o.fileName = args[0]
	return nil
}

// Validate ensures that option values make sense
func (o *RecreateObjectsOptions) Validate(cmd *cobra.Command) error {
	return nil
}

// Run executes the command
func (o *RecreateObjectsOptions) Run() error {
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
		o.recreateObject(client, &objFromFile)
	}
	return nil
}

func (o *RecreateObjectsOptions) recreateObject(client dynamic.Interface, objFromFile *unstructured.Unstructured) {
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
	switch kind {
	case "DNSEndpoint":
		logger.Info("re-creation skipped since not used in v1")
		return
	case "HiveConfig":
		logger.Info("re-creation skipped since HiveConfig has already been restored")
		return
	}
	gvr := hivev1alpha1.SchemeGroupVersion.WithResource(resourceForHiveKind(kind))
	var resourceClient dynamic.ResourceInterface
	if namespace != "" {
		resourceClient = client.Resource(gvr).Namespace(namespace)
	} else {
		resourceClient = client.Resource(gvr)
	}
	clearResourceVersion(objFromFile)
	removeHiveOwnerReferences(objFromFile)
	removeKubectlLastAppliedAnnotation(objFromFile)
	newObj, err := resourceClient.Create(objFromFile, metav1.CreateOptions{})
	if err != nil {
		logger.WithError(err).Error("could not create object")
		return
	}
	objFromFile.SetUID(newObj.GetUID())
	objFromFile.SetResourceVersion(newObj.GetResourceVersion())
	if _, err := resourceClient.Update(objFromFile, metav1.UpdateOptions{}); err != nil {
		logger.WithError(err).Error("could not update re-created object with status")
		return
	}
	logger.Info("created object")
}

func clearResourceVersion(obj *unstructured.Unstructured) {
	obj.SetResourceVersion("")
}

func removeHiveOwnerReferences(obj *unstructured.Unstructured) {
	var ownerReferences []metav1.OwnerReference
	for _, r := range obj.GetOwnerReferences() {
		if r.APIVersion == hivev1alpha1.SchemeGroupVersion.String() {
			continue
		}
		ownerReferences = append(ownerReferences, r)
	}
	obj.SetOwnerReferences(ownerReferences)
}

func removeKubectlLastAppliedAnnotation(obj *unstructured.Unstructured) {
	annotations := obj.GetAnnotations()
	delete(annotations, "kubectl.kubernetes.io/last-applied-configuration")
	obj.SetAnnotations(annotations)
}
