package hive

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/resource"
)

const (
	managedDomainsConfigMapName = "hive-managed-domains"
	managedDomainsConfigMapKey  = "managed-domains"
	configMapLabel              = "managed-domains"
	configMapMountPath          = "/data/config"
)

// configureManagedDomains will create a new configmap holding the managed domains settings (if necessary), or simply
// use the current configmap of the current deployment if the settings it contains match the desired settings. The
// first return value is a hash (MD5 sum) of the configmap's data, which can be used to indicate whether it changed.
func (r *ReconcileHiveConfig) configureManagedDomains(logger log.FieldLogger, h resource.Helper, instance *hivev1.HiveConfig, namespacesToClean []string) (string, error) {
	// Scrub old target namespaces.
	for _, ns := range namespacesToClean {
		// Cheat: getCurrentConfigMap deletes any configmap that doesn't have the specified data.
		// (Technically this could miss a configmap with no data. That's not a realistic problem
		// as we always create them with managedDomainsConfigMapKey.)
		_, err := r.getCurrentConfigMap(nil, h, ns, logger)
		if err != nil {
			return "", errors.Wrapf(err, "failed to scrub managed domains config maps from old target namespace %s", ns)
		}
	}

	domains, err := json.Marshal(instance.Spec.ManagedDomains)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal managed domains list into the configmap")
	}

	newConfigMapData := map[string]string{managedDomainsConfigMapKey: string(domains)}

	// TODO: The algorithm from here down can be simplified once we're sure we've flushed out any
	// old managed domain configmaps with generated names.

	currentConfigMap, err := r.getCurrentConfigMap(newConfigMapData, h, getHiveNamespace(instance), logger)
	if err != nil {
		return "", err
	}

	var mdConfigMap *corev1.ConfigMap
	if currentConfigMap == nil {
		log.Debug("need a new/updated configmap for managed domains")
		mdConfigMap = &corev1.ConfigMap{}
		mdConfigMap.Kind = "ConfigMap"
		mdConfigMap.APIVersion = "v1"
		mdConfigMap.Name = managedDomainsConfigMapName
		mdConfigMap.Namespace = getHiveNamespace(instance)
		mdConfigMap.Labels = map[string]string{configMapLabel: "true"}

		mdConfigMap.Data = newConfigMapData

		if _, err := h.CreateRuntimeObject(mdConfigMap, r.scheme); err != nil {
			return "", errors.Wrap(err, "failed to save new managed domains configmap")
		}
		log.WithField("configmap", fmt.Sprintf("%s/%s", mdConfigMap.Namespace, mdConfigMap.Name)).
			Debug("saved configmap for managed domains")

	} else {
		// If configmap data was equal keep using current configmap.
		log.WithField("configmap", fmt.Sprintf("%s/%s", currentConfigMap.Namespace, currentConfigMap.Name)).
			Info("using existing manage domains config map")
		mdConfigMap = currentConfigMap
	}

	return computeHash(mdConfigMap.Data), nil
}

// getCurrentConfigMap will see if any existing configmap (for managed domains) already has the necessary
// settings. It will also delete any configmaps (for managed domains) that have out-of-date contents
// (so that the configmaps are not orphaned as config changes happen).
func (r *ReconcileHiveConfig) getCurrentConfigMap(cmData map[string]string, h resource.Helper, hiveNSName string, logger log.FieldLogger) (*corev1.ConfigMap, error) {
	configMapList := &corev1.ConfigMapList{}
	labelSelector := fmt.Sprintf("%s=true", configMapLabel)

	err := r.List(context.TODO(), configMapList, hiveNSName, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list config maps for managed domains")
	}

	currentConfigMap := &corev1.ConfigMap{}

	// find any configmap that has current, correct managed domains data
	for i, cm := range configMapList.Items {
		// NOTE: We used to generate the name of this configmap and find it based on the label. Now
		// we want it to have a static name. So if it doesn't have that static name, consider it
		// "not found" (which will cause it to be deleted and replaced with the proper name) even
		// if the data matches.
		if cm.Name == managedDomainsConfigMapName && reflect.DeepEqual(cmData, cm.Data) {
			currentConfigMap = &configMapList.Items[i]
			break
		}
	}

	// delete all the other configmaps (will delete all if no matches found in the step above)
	for _, cm := range configMapList.Items {
		if cm.Name != currentConfigMap.Name {
			logger.WithFields(log.Fields{"namespace": cm.Namespace, "name": cm.Name}).Infof("deleting out-of-date managed domains configmap")
			if err := h.Delete(cm.APIVersion, cm.Kind, cm.Namespace, cm.Name); err != nil {
				logger.WithError(err).Error("failed to delete out-of-date managed domains configmap")
			}
		}
	}

	if currentConfigMap.Name == "" {
		logger.Debugf("no current managed domains configmap in namespace %s", hiveNSName)
		return nil, nil
	}

	logger.Debugf("found existing managed domains configmap %s in namespace %s", currentConfigMap.Name, hiveNSName)
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
