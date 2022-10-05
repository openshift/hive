package hive

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

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

func computeHash(data interface{}, additionalHashes ...string) string {
	hasher := md5.New()
	hasher.Write([]byte(fmt.Sprintf("%v", data)))
	for _, h := range additionalHashes {
		hasher.Write([]byte(h))
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func containerByName(template *corev1.PodSpec, name string) (*corev1.Container, error) {
	for i, container := range template.Containers {
		if container.Name == name {
			return &template.Containers[i], nil
		}
	}
	return nil, fmt.Errorf("no container named %s", name)
}

func isScaleMode(instance *hivev1.HiveConfig) bool {
	return instance.Spec.ScaleMode != nil && instance.Spec.ScaleMode.Enabled
}
