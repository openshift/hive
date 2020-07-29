package resource

import (
	"context"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
)

// DeleteAnyExistingObject will look for any object that exists that matches the passed in 'obj' and will delete it if it exists
func DeleteAnyExistingObject(c client.Client, key client.ObjectKey, obj hivev1.MetaRuntimeObject, logger log.FieldLogger) error {
	logger = logger.WithField("object", key)
	switch err := c.Get(context.Background(), key, obj); {
	case apierrors.IsNotFound(err):
		logger.Debug("object does not exist")
		return nil
	case err != nil:
		logger.WithError(err).Error("error getting object")
		return errors.Wrap(err, "error getting object")
	}
	if obj.GetDeletionTimestamp() != nil {
		logger.Debug("object has already been deleted")
		return nil
	}
	logger.Info("deleting existing object")
	if err := c.Delete(context.Background(), obj); err != nil {
		logger.WithError(err).Error("error deleting object")
		return errors.Wrap(err, "error deleting object")
	}
	return nil
}
