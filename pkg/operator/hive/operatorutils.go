package hive

import (
	"context"

	log "github.com/sirupsen/logrus"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

func getHiveNamespace(config *hivev1.HiveConfig) string {
	if config.Spec.TargetNamespace == "" {
		return constants.DefaultHiveNamespace
	}

	return config.Spec.TargetNamespace
}

func dynamicDelete(dynamicClient dynamic.Interface, gvrnsn gvrNSName, hLog log.FieldLogger) error {
	rLog := hLog.WithField("resource", gvrnsn)
	gvr := schema.GroupVersionResource{Group: gvrnsn.group, Version: gvrnsn.version, Resource: gvrnsn.resource}
	if err := dynamicClient.Resource(gvr).Namespace(gvrnsn.namespace).Delete(context.Background(), gvrnsn.name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		rLog.WithError(err).Log(controllerutils.LogLevel(err), "error deleting resource")
		return err
	}
	rLog.Info("resource deleted")
	return nil
}

type gvrNSName struct {
	group     string
	version   string
	resource  string
	namespace string // empty for global resources
	name      string
}
