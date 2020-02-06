package hive

import (
	"encoding/json"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/resource"
)

const (
	managedDomainsConfigMapName = "managed-domains"
)

func (r *ReconcileHiveConfig) teardownLegacyExternalDNS(hLog log.FieldLogger) error {
	key := client.ObjectKey{Namespace: constants.HiveNamespace, Name: "external-dns"}
	for _, obj := range []runtime.Object{
		&appsv1.Deployment{},
		&corev1.ServiceAccount{},
		&rbacv1.ClusterRoleBinding{},
		&rbacv1.ClusterRole{},
	} {
		if err := resource.DeleteAnyExistingObject(r, key, obj, hLog); err != nil {
			return err
		}
	}
	return nil
}

func deployManagedDomainsConfigMap(h *resource.Helper, instance *hivev1.HiveConfig) error {
	cm := &corev1.ConfigMap{}
	cm.Kind = "ConfigMap"
	cm.APIVersion = "v1"
	cm.Name = managedDomainsConfigMapName
	cm.Namespace = constants.HiveNamespace

	domains, err := json.Marshal(instance.Spec.ManagedDomains)
	if err != nil {
		return errors.Wrap(err, "failed to marshal managed domains list into the configmap")
	}

	cm.Data = map[string]string{"managed-domains": string(domains)}
	_, err = h.ApplyRuntimeObject(cm, scheme.Scheme)
	return errors.Wrap(err, "error applying managed domains configmap")
}

func addManagedDomainsVolume(podSpec *corev1.PodSpec) {
	volume := corev1.Volume{}
	volume.Name = "managed-domains"
	volume.ConfigMap = &corev1.ConfigMapVolumeSource{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: managedDomainsConfigMapName,
		},
	}
	volumeMount := corev1.VolumeMount{
		Name:      "managed-domains",
		MountPath: "/data/config",
	}
	envVar := corev1.EnvVar{
		Name:  constants.ManagedDomainsFileEnvVar,
		Value: "/data/config/managed-domains",
	}
	podSpec.Volumes = append(podSpec.Volumes, volume)
	podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts, volumeMount)
	podSpec.Containers[0].Env = append(podSpec.Containers[0].Env, envVar)
}
