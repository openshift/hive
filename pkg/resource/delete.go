package resource

import (
	"context"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DeleteAnyExistingObject will look for any object that exists that matches the passed in 'obj' and will delete it if it exists
func DeleteAnyExistingObject(c client.Client, key client.ObjectKey, obj runtime.Object, logger log.FieldLogger) error {
	logger = logger.WithField("object", key)
	switch err := c.Get(context.Background(), key, obj); {
	case apierrors.IsNotFound(err):
		logger.Debug("object does not exist")
		return nil
	case err != nil:
		logger.WithError(err).Error("error getting object")
		return errors.Wrap(err, "error getting object")
	}
	logger.Info("deleting existing object")
	if err := c.Delete(context.Background(), obj); err != nil {
		logger.WithError(err).Error("error deleting object")
		return errors.Wrap(err, "error deleting object")
	}
	return nil
}

func (r *Helper) Delete(apiVersion, kind, namespace, name string) error {
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
