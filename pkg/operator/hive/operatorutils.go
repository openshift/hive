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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/dynamic"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/util/scheme"
)

var appsCodecs = serializer.NewCodecFactory(scheme.GetScheme())

func GetHiveNamespace(config *hivev1.HiveConfig) string {
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

func computeHash(data any, additionalHashes ...string) string {
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

// applyDeploymentConfig looks in hiveConfig for a spec.deploymentConfig whose deploymentName matches the
// deploymentName parameter. If found, it copies the guts of that deploymentConfig into the container, as
// appropriate.
// NOTE: Currently each deployment/statefulset has only one container. If that changes in the future, we
// will need to expand the DeploymentConfig struct to include a way to specify one or more containers,
// and expand this function's parameter list accordingly.
func applyDeploymentConfig(hiveConfig *hivev1.HiveConfig, deploymentName hivev1.DeploymentName, container *corev1.Container, hLog log.FieldLogger) {
	logger := hLog.WithField("deploymentName", deploymentName)
	if hiveConfig.Spec.DeploymentConfig == nil {
		return
	}
	for _, dc := range *hiveConfig.Spec.DeploymentConfig {
		if dc.DeploymentName != deploymentName {
			continue
		}
		// Currently DeploymentConfig only has Resources. If those are absent, nothing to do.
		if dc.Resources == nil {
			logger.Warn("Ignoring deploymentConfig with no resources")
			continue
		}
		logger.Info("Overriding resources from deploymentConfig")
		container.Resources = *dc.Resources
	}
}

func getImagePullSecretReference(config *hivev1.HiveConfig) *corev1.LocalObjectReference {
	if config.Spec.HiveImagePullSecretRef != nil && config.Spec.HiveImagePullSecretRef.Name != "" {
		return config.Spec.HiveImagePullSecretRef
	}
	return nil
}

func readRuntimeObjectOrDie[T any](sgv schema.GroupVersion, objBytes []byte) T {
	requiredObj, err := runtime.Decode(appsCodecs.UniversalDecoder(sgv), objBytes)
	if err != nil {
		panic(err)
	}
	return requiredObj.(T)

}
