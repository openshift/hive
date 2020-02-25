package hive

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/resource"
)

const (
	managedDomainsConfigMapNamePrefix = "managed-domains-"
	managedDomainsConfigMapKey        = "managed-domains"
	configMapLabel                    = "managed-domains"
	configMapMountPath                = "/data/config"
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

func (r *ReconcileHiveConfig) teardownLegacyMangedDomainsConfigMap(hLog log.FieldLogger) error {
	key := client.ObjectKey{Namespace: constants.HiveNamespace, Name: "managed-domains"}
	if err := resource.DeleteAnyExistingObject(r, key, &corev1.ConfigMap{}, hLog); err != nil {
		hLog.WithError(err).Error("failed to delete legacy managed domains configmap")
	}
	return nil
}

// configureManagedDomains will create a new configmap holding the managed domains settings (if necessary), or simply
// return the current configmap of the current deployment if the settings it contains match the desired settings.
func (r *ReconcileHiveConfig) configureManagedDomains(logger log.FieldLogger, instance *hivev1.HiveConfig) (*corev1.ConfigMap, error) {
	domains, err := json.Marshal(instance.Spec.ManagedDomains)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal managed domains list into the configmap")
	}

	newConfigMapData := map[string]string{managedDomainsConfigMapKey: string(domains)}

	currentConfigMap, err := r.getCurrentConfigMap(newConfigMapData, logger)
	if err != nil {
		return nil, err
	}

	var mdConfigMap *corev1.ConfigMap
	if currentConfigMap == nil {
		log.Debug("need a new/updated configmap for managed domains")
		mdConfigMap = &corev1.ConfigMap{}
		mdConfigMap.Kind = "ConfigMap"
		mdConfigMap.APIVersion = "v1"
		mdConfigMap.GenerateName = managedDomainsConfigMapNamePrefix
		mdConfigMap.Namespace = constants.HiveNamespace
		mdConfigMap.Labels = map[string]string{configMapLabel: "true"}

		mdConfigMap.Data = newConfigMapData

		if err := r.Create(context.TODO(), mdConfigMap); err != nil {
			return nil, errors.Wrap(err, "failed to save new managed domains configmap")
		}
		log.WithField("configmap", fmt.Sprintf("%s/%s", mdConfigMap.Namespace, mdConfigMap.Name)).
			Debug("saved configmap for managed domains")

	} else {
		// If configmap data was equal keep using current configmap.
		log.WithField("configmap", fmt.Sprintf("%s/%s", currentConfigMap.Namespace, currentConfigMap.Name)).
			Info("using existing manage domains config map")
		mdConfigMap = currentConfigMap
	}

	return mdConfigMap, nil
}

// getCurrentConfigMap will see if any existing configmap (for managed domains) already has the necessary
// settings. It will also delete any configmaps (for managed domains) that have out-of-date contents
// (so that the configmaps are not orphaned as config changes happen).
func (r *ReconcileHiveConfig) getCurrentConfigMap(cmData map[string]string, logger log.FieldLogger) (*corev1.ConfigMap, error) {
	configMapList := &corev1.ConfigMapList{}
	labelSelector := map[string]string{configMapLabel: "true"}

	err := r.List(context.TODO(), configMapList, client.MatchingLabels(labelSelector))
	if err != nil {
		return nil, errors.Wrap(err, "failed to list config maps for managed domains")
	}

	currentConfigMap := &corev1.ConfigMap{}

	// find any configmap that has current, correct managed domains data
	for i, cm := range configMapList.Items {
		if reflect.DeepEqual(cmData, cm.Data) {
			currentConfigMap = &configMapList.Items[i]
			break
		}
	}

	// delete all the other configmaps (will delete all if no matches found in the step above)
	for _, cm := range configMapList.Items {
		if cm.Name != currentConfigMap.Name {
			if err := r.Delete(context.TODO(), &cm); err != nil {
				logger.WithError(err).Error("failed to delete out-of-date manged domains configmap")
			}
		}
	}

	if currentConfigMap.Name == "" {
		return nil, nil
	}

	return currentConfigMap, nil
}

func addManagedDomainsVolume(podSpec *corev1.PodSpec, configMapName string) {
	volume := corev1.Volume{}
	volume.Name = constants.ManagedDomainsVolumeName
	volume.ConfigMap = &corev1.ConfigMapVolumeSource{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: configMapName,
		},
	}
	volumeMount := corev1.VolumeMount{
		Name:      constants.ManagedDomainsVolumeName,
		MountPath: configMapMountPath,
	}
	envVar := corev1.EnvVar{
		Name:  constants.ManagedDomainsFileEnvVar,
		Value: fmt.Sprintf("%s/%s", configMapMountPath, managedDomainsConfigMapKey),
	}
	podSpec.Volumes = append(podSpec.Volumes, volume)
	podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts, volumeMount)
	podSpec.Containers[0].Env = append(podSpec.Containers[0].Env, envVar)
}
