package hive

import (
	"crypto/md5"
	"encoding/hex"
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
	awsPrivateLinkConfigMapName      = "aws-private-link"
	awsPrivateLinkConfigMapNameKey   = "aws-private-link"
	awsPrivateLinkConfigMapMountPath = "/data/aws-private-link-config"
)

func (r *ReconcileHiveConfig) deployAWSPrivateLinkConfigMap(hLog log.FieldLogger, h resource.Helper, instance *hivev1.HiveConfig, namespacesToClean []string) (string, error) {
	// Delete the configmap from previous target namespaces
	for _, ns := range namespacesToClean {
		hLog.Infof("Deleting configmap/%s from old target namespace %s", awsPrivateLinkConfigMapName, ns)
		// h.Delete already no-ops for IsNotFound
		// TODO: Something better than hardcoding apiVersion and kind.
		if err := h.Delete("v1", "ConfigMap", ns, awsPrivateLinkConfigMapName); err != nil {
			return "", errors.Wrapf(err, "error deleting configmap/%s from old target namespace %s", awsPrivateLinkConfigMapName, ns)
		}
	}

	cm := &corev1.ConfigMap{}
	cm.Name = awsPrivateLinkConfigMapName
	cm.Namespace = getHiveNamespace(instance)
	cm.Data = make(map[string]string)

	if instance.Spec.AWSPrivateLink != nil {
		data, err := json.Marshal(instance.Spec.AWSPrivateLink)
		if err != nil {
			return "", errors.Wrap(err, "failed to marshal aws privatelink controller config")
		}
		cm.Data[awsPrivateLinkConfigMapNameKey] = string(data)
	}

	result, err := util.ApplyRuntimeObjectWithGC(h, cm, instance)
	if err != nil {
		hLog.WithError(err).Error("error applying aws-private-link configmap")
		return "", err
	}
	hLog.WithField("result", result).Info("aws-private-link configmap applied")

	hLog.Info("Hashing hive-controllers-config data onto a hive deployment annotation")
	awsPrivateLinkConfigHash := computeAWSPrivateLinkConfigHash(cm)

	return awsPrivateLinkConfigHash, nil
}

func computeAWSPrivateLinkConfigHash(cm *corev1.ConfigMap) string {
	hasher := md5.New()
	hasher.Write([]byte(fmt.Sprintf("%v", cm.Data)))
	return hex.EncodeToString(hasher.Sum(nil))
}

func addAWSPrivateLinkConfigVolume(podSpec *corev1.PodSpec) {
	optional := true
	volume := corev1.Volume{}
	volume.Name = awsPrivateLinkConfigMapName
	volume.ConfigMap = &corev1.ConfigMapVolumeSource{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: awsPrivateLinkConfigMapName,
		},
		Optional: &optional,
	}
	volumeMount := corev1.VolumeMount{
		Name:      awsPrivateLinkConfigMapName,
		MountPath: awsPrivateLinkConfigMapMountPath,
	}
	envVar := corev1.EnvVar{
		Name:  constants.AWSPrivateLinkControllerConfigFileEnvVar,
		Value: fmt.Sprintf("%s/%s", awsPrivateLinkConfigMapMountPath, awsPrivateLinkConfigMapNameKey),
	}
	podSpec.Volumes = append(podSpec.Volumes, volume)
	podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts, volumeMount)
	podSpec.Containers[0].Env = append(podSpec.Containers[0].Env, envVar)
}
