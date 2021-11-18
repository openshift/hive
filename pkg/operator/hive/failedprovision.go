package hive

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/operator/util"
	"github.com/openshift/hive/pkg/resource"
)

const (
	failedProvisionConfigMapName      = "hive-failed-provision-config"
	failedProvisionConfigMapNameKey   = "hive-failed-provision-config"
	failedProvisionConfigMapMountPath = "/data/failed-provision-config"
)

func (r *ReconcileHiveConfig) deployFailedProvisionConfigMap(hLog log.FieldLogger, h resource.Helper, instance *hivev1.HiveConfig, namespacesToClean []string) (string, error) {
	// Delete the configmap from previous target namespaces
	for _, ns := range namespacesToClean {
		hLog.Infof("Deleting configmap/%s from old target namespace %s", failedProvisionConfigMapName, ns)
		// h.Delete already no-ops for IsNotFound
		// TODO: Something better than hardcoding apiVersion and kind.
		if err := h.Delete("v1", "ConfigMap", ns, failedProvisionConfigMapName); err != nil {
			return "", errors.Wrapf(err, "error deleting configmap/%s from old target namespace %s", failedProvisionConfigMapName, ns)
		}
	}

	cm := &corev1.ConfigMap{}
	cm.Name = failedProvisionConfigMapName
	cm.Namespace = getHiveNamespace(instance)
	cm.Data = make(map[string]string)

	data, err := json.Marshal(instance.Spec.FailedProvisionConfig)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal failed provision config")
	}
	cm.Data[failedProvisionConfigMapNameKey] = string(data)

	result, err := util.ApplyRuntimeObjectWithGC(h, cm, instance)
	if err != nil {
		hLog.WithError(err).Error("error applying failed provision configmap")
		return "", err
	}
	hLog.WithField("result", result).Info("failed provision configmap applied")

	hLog.Info("Hashing hive-controllers-config data onto a hive deployment annotation")
	failedProvisionConfigHash := computeHash(cm.Data)

	return failedProvisionConfigHash, nil
}

func addFailedProvisionConfigVolume(podSpec *corev1.PodSpec) {
	optional := true
	volume := corev1.Volume{}
	volume.Name = failedProvisionConfigMapName
	volume.ConfigMap = &corev1.ConfigMapVolumeSource{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: failedProvisionConfigMapName,
		},
		Optional: &optional,
	}
	volumeMount := corev1.VolumeMount{
		Name:      failedProvisionConfigMapName,
		MountPath: failedProvisionConfigMapMountPath,
	}
	envVar := corev1.EnvVar{
		Name:  constants.FailedProvisionConfigFileEnvVar,
		Value: fmt.Sprintf("%s/%s", failedProvisionConfigMapMountPath, failedProvisionConfigMapNameKey),
	}
	podSpec.Volumes = append(podSpec.Volumes, volume)
	podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts, volumeMount)
	podSpec.Containers[0].Env = append(podSpec.Containers[0].Env, envVar)
}
