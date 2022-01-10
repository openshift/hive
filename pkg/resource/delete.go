package resource

import (
	"context"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

// DeleteAnyExistingObject will look for any object that exists that matches the passed in 'obj' and will delete it if it exists.
// The first return value is true iff the object was already gone.
func DeleteAnyExistingObject(c client.Client, key client.ObjectKey, obj hivev1.MetaRuntimeObject, logger log.FieldLogger) (bool, error) {
	logger = logger.WithField("object", key)
	switch err := c.Get(context.Background(), key, obj); {
	case apierrors.IsNotFound(err):
		logger.Debug("object does not exist")
		return true, nil
	case err != nil:
		logger.WithError(err).Error("error getting object")
		return false, errors.Wrap(err, "error getting object")
	}
	if obj.GetDeletionTimestamp() != nil {
		logger.Debug("object has already been deleted")
		// BUT the object still exists!
		return false, nil
	}
	logger.Info("deleting existing object")
	if err := c.Delete(context.Background(), obj); err != nil {
		logger.WithError(err).Error("error deleting object")
		return false, errors.Wrap(err, "error deleting object")
	}
	return false, nil
}

func (r *helper) Delete(apiVersion, kind, namespace, name string) error {
	f, err := r.getFactory(namespace)
	if err != nil {
		return errors.Wrap(err, "could not get factory")
	}
	mapper, err := f.ToRESTMapper()
	if err != nil {
		return errors.Wrap(err, "could not get mapper")
	}
	gvk := schema.FromAPIVersionAndKind(apiVersion, kind)
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return errors.Wrap(err, "could not get mapping")
	}
	dynamicClient, err := f.DynamicClient()
	if err != nil {
		return errors.Wrap(err, "could not create dynamic client")
	}
	switch err := dynamicClient.Resource(mapping.Resource).Namespace(namespace).Delete(context.Background(), name, metav1.DeleteOptions{}); {
	case apierrors.IsNotFound(err):
		r.logger.Info("resource has already been deleted")
	case err != nil:
		return errors.Wrap(err, "could not delete resource")
	}
	return nil
}
