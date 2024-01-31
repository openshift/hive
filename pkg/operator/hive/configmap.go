package hive

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/operator/util"
	"github.com/openshift/hive/pkg/resource"
	"github.com/openshift/hive/pkg/util/contracts"
)

type configMapInfo struct {
	// name is the name of the configmap
	name string
	// nameKey is the key within the configmap's data field where the configmap data is to be stored.
	// This will also be the basename of the file in which the data can be found when the configmap
	// is mounted in a controller pod.
	nameKey string
	// mountPath is the directory (not including the file name, above) at which the configmap volume
	// should be mounted in a controller pod.
	mountPath string
	// envVar is the name of the environment variable to be added to the pod spec. The value will be the
	// path to the file containing the configmap data ({mountPath}/{nameKey}).
	envVar string
	// volumeSourceOptional, if true, will cause addConfigVolume to include Volume.ConfigMap.Optional=&true.
	// If false, that field will be omitted (as opposed to being set to &false).
	volumeSourceOptional bool
	// getData returns a *pointer* to an object to be marshalled into the getData field of the configmap
	// under the nameKey key. If the pointer is nil, the getData field will be left empty. Ignored if
	// setData is specified.
	getData func(*hivev1.HiveConfig) (interface{}, error)
	// setData is for more complex uses, e.g. where multiple fields need to be set. The second parameter
	// is a pointer to the ConfigMap.Data field; the function can fill it in however it sees fit. If
	// specified, getData is ignored.
	setData func(*hivev1.HiveConfig, *map[string]string) error
}

var managedDomainsConfigMapInfo = configMapInfo{
	name:      "hive-managed-domains",
	nameKey:   "managed-domains",
	mountPath: "/data/config",
	envVar:    constants.ManagedDomainsFileEnvVar,
	getData: func(instance *hivev1.HiveConfig) (interface{}, error) {
		return &instance.Spec.ManagedDomains, nil
	},
}

func (r *ReconcileHiveConfig) scrubOldManagedDomainsConfigMaps(h resource.Helper, logger log.FieldLogger, namespaces ...string) error {
	for _, ns := range namespaces {
		cmLog := logger.WithField("namespace", ns)

		configMapList := &corev1.ConfigMapList{}

		err := r.List(context.TODO(), configMapList, ns, metav1.ListOptions{LabelSelector: "managed-domains=true"})
		if err != nil {
			return errors.Wrap(err, "failed to list config maps for managed domains")
		}

		for _, cm := range configMapList.Items {
			cmLog = cmLog.WithField("name", cm.Name)
			cmLog.Info("deleting out-of-date managed domains configmap")
			if err := h.Delete(cm.APIVersion, cm.Kind, cm.Namespace, cm.Name); err != nil {
				cmLog.WithError(err).Error("failed to delete out-of-date managed domains configmap")
			}
		}
	}
	return nil
}

var awsPrivateLinkConfigMapInfo = configMapInfo{
	name:                 "aws-private-link",
	nameKey:              "aws-private-link",
	mountPath:            "/data/aws-private-link-config",
	envVar:               constants.AWSPrivateLinkControllerConfigFileEnvVar,
	volumeSourceOptional: true,
	getData: func(instance *hivev1.HiveConfig) (interface{}, error) {
		return instance.Spec.AWSPrivateLink, nil
	},
}

var failedProvisionConfigMapInfo = configMapInfo{
	name:                 "hive-failed-provision-config",
	nameKey:              "hive-failed-provision-config",
	mountPath:            "/data/failed-provision-config",
	envVar:               constants.FailedProvisionConfigFileEnvVar,
	volumeSourceOptional: true,
	getData: func(instance *hivev1.HiveConfig) (interface{}, error) {
		return &instance.Spec.FailedProvisionConfig, nil
	},
}

var metricsConfigConfigMapInfo = configMapInfo{
	name:                 "hive-metrics-config",
	nameKey:              "hive-metrics-config",
	mountPath:            "/data/metrics-config",
	envVar:               constants.MetricsConfigFileEnvVar,
	volumeSourceOptional: true,
	getData: func(instance *hivev1.HiveConfig) (interface{}, error) {
		return &instance.Spec.MetricsConfig, nil
	},
}

func (r *ReconcileHiveConfig) supportedContractsConfigMapInfo() configMapInfo {
	f := func(instance *hivev1.HiveConfig) (interface{}, error) {
		supported := map[string][]contracts.ContractImplementation{}
		for _, k := range knowContracts {
			supported[k.Name] = k.Supported
		}

		crdList := &apiextv1.CustomResourceDefinitionList{}
		if err := r.List(context.TODO(), crdList, "", metav1.ListOptions{}); err != nil {
			return nil, errors.Wrap(err, "error getting crds for collect contract implementations")
		}
		for _, crd := range crdList.Items {
			// collect all the possible implementations from this crd
			var impls []contracts.ContractImplementation
			for _, version := range crd.Spec.Versions {
				impls = append(impls, contracts.ContractImplementation{
					Group:   crd.Spec.Group,
					Version: version.Name,
					Kind:    crd.Spec.Names.Kind,
				})
			}

			for label, val := range crd.ObjectMeta.Labels {
				if strings.HasPrefix(label, "contracts.hive.openshift.io") &&
					val == "true" &&
					allowedContracts.Has(label) { // lets store impls for this contract
					contract := strings.TrimPrefix(label, "contracts.hive.openshift.io/")
					curr := supported[contract]
					curr = append(curr, impls...)
					supported[contract] = curr
				}
			}
		}

		if len(supported) > 0 {
			var keys []string
			for k := range supported {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			var config contracts.SupportedContractImplementationsList
			for _, c := range keys {
				config = append(config, contracts.SupportedContractImplementations{
					Name:      c,
					Supported: supported[c],
				})
			}

			return &config, nil
		}
		return nil, nil
	}
	return configMapInfo{
		name:                 "hive-supported-contracts",
		nameKey:              "supported-contracts",
		mountPath:            "/data/supported-contracts-config",
		envVar:               constants.SupportedContractImplementationsFileEnvVar,
		getData:              f,
		volumeSourceOptional: true,
	}
}

var hiveControllersConfigMapInfo = configMapInfo{
	name: "hive-controllers-config",
	setData: func(instance *hivev1.HiveConfig, datap *map[string]string) error {
		if instance.Spec.ControllersConfig != nil {
			if instance.Spec.ControllersConfig.Default != nil {
				setHiveControllersConfig(instance.Spec.ControllersConfig.Default, datap, "default")
			}
			for _, controller := range instance.Spec.ControllersConfig.Controllers {
				replicasIsSet := controller.Config.Replicas != nil
				resourcesIsSet := controller.Config.Resources != nil
				isolatedController := controllersInTheirOwnIsolatedPods.Contains(controller.Name)
				if !isolatedController && replicasIsSet {
					log.WithField("controller", controller.Name).Warn("hiveconfig.spec.controllersConfig.controllers[].config.replicas shouldn't be set for this controller")
				}
				if !isolatedController && resourcesIsSet {
					log.WithField("controller", controller.Name).Warn("hiveconfig.spec.controllersConfig.controllers[].config.resources shouldn't be set for this controller")
				}

				setHiveControllersConfig(&controller.Config, datap, controller.Name)
			}
		}
		return nil
	},
}

func getHiveControllerConfig(controllerName hivev1.ControllerName, controllerConfigs []hivev1.SpecificControllerConfig) (*hivev1.ControllerConfig, bool) {
	for _, controllerConfig := range controllerConfigs {
		if controllerConfig.Name == controllerName {
			return &controllerConfig.Config, true
		}
	}

	return nil, false
}

func setHiveControllersConfig(config *hivev1.ControllerConfig, cmData *map[string]string, controllerName hivev1.ControllerName) {
	if config.ConcurrentReconciles != nil {
		(*cmData)[fmt.Sprintf(utils.ConcurrentReconcilesEnvVariableFormat, controllerName)] = strconv.Itoa(int(*config.ConcurrentReconciles))
	}
	if config.ClientQPS != nil {
		(*cmData)[fmt.Sprintf(utils.ClientQPSEnvVariableFormat, controllerName)] = strconv.Itoa(int(*config.ClientQPS))
	}
	if config.ClientBurst != nil {
		(*cmData)[fmt.Sprintf(utils.ClientBurstEnvVariableFormat, controllerName)] = strconv.Itoa(int(*config.ClientBurst))
	}
	if config.QueueQPS != nil {
		(*cmData)[fmt.Sprintf(utils.QueueQPSEnvVariableFormat, controllerName)] = strconv.Itoa(int(*config.QueueQPS))
	}
	if config.QueueBurst != nil {
		(*cmData)[fmt.Sprintf(utils.QueueBurstEnvVariableFormat, controllerName)] = strconv.Itoa(int(*config.QueueBurst))
	}
}

var featureGatesConfigMapInfo = configMapInfo{
	name:    "hive-feature-gates",
	nameKey: constants.HiveFeatureGatesEnabledEnvVar,
	setData: func(instance *hivev1.HiveConfig, data *map[string]string) error {
		var val string = ""
		if fg := instance.Spec.FeatureGates; fg != nil {
			s, ok := hivev1.FeatureSets[fg.FeatureSet]
			if ok && s != nil {
				val = strings.Join(s.Enabled, ",")
			}
			if fg.FeatureSet == hivev1.CustomFeatureSet && fg.Custom != nil {
				val = strings.Join(fg.Custom.Enabled, ",")
			}
		}
		(*data)[constants.HiveFeatureGatesEnabledEnvVar] = val
		return nil
	},
}

// deployConfigMap deploys a configmap into hive's target namespace for use by one or more controllers.
// cmInfo is a configMapInfo containing the string constants to be used for the configmap's name
// and the key therein that will correspond to the base filename when the configmap is mounted
// into the controller's container; and the data to be stored in the configmap.
//
// namespacesToClean is a list of strings indicating former target namespaces from which this configmap
// is to be deleted.
func (r *ReconcileHiveConfig) deployConfigMap(hLog log.FieldLogger, h resource.Helper, instance *hivev1.HiveConfig, cmInfo configMapInfo, namespacesToClean []string) (string, error) {
	cmLog := hLog.WithField("configMap.name", cmInfo.name)

	// Delete the configmap from previous target namespaces
	for _, ns := range namespacesToClean {
		cmLog.WithField("namespace", ns).Info("Deleting configmap from old target namespace")
		// h.Delete already no-ops for IsNotFound
		// TODO: Something better than hardcoding apiVersion and kind.
		if err := h.Delete("v1", "ConfigMap", ns, cmInfo.name); err != nil {
			return "", errors.Wrapf(err, "error deleting configmap/%s from old target namespace %s", cmInfo.name, ns)
		}
	}

	cm := &corev1.ConfigMap{}
	cm.Name = cmInfo.name
	cm.Namespace = GetHiveNamespace(instance)
	cm.Data = make(map[string]string)

	if cmInfo.setData != nil {
		if err := cmInfo.setData(instance, &cm.Data); err != nil {
			return "", err
		}
	} else {
		data, err := cmInfo.getData(instance)
		if err != nil {
			return "", err
		}
		if reflect.TypeOf(data).Kind() != reflect.Ptr {
			panic("Non-pointer value returned from configMapInfo.data(). This is a bug.")
		}
		if !reflect.ValueOf(data).IsNil() {
			data, err := json.Marshal(data)
			if err != nil {
				return "", errors.Wrapf(err, "failed to marshal %s config", cmInfo.name)
			}
			cm.Data[cmInfo.nameKey] = string(data)
		}
	}

	result, err := util.ApplyRuntimeObjectWithGC(h, cm, instance)
	if err != nil {
		cmLog.WithError(err).Error("error applying configmap")
		return "", err
	}
	cmLog.WithField("result", result).Info("configmap applied")

	cmLog.Info("Hashing configmap data onto a hive deployment annotation")

	return computeHash(cm.Data), nil
}

func addConfigVolume(podSpec *corev1.PodSpec, cmInfo configMapInfo, container *corev1.Container) {
	volume := corev1.Volume{}
	volume.Name = cmInfo.name
	volume.ConfigMap = &corev1.ConfigMapVolumeSource{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: cmInfo.name,
		},
	}
	if cmInfo.volumeSourceOptional {
		optional := true
		volume.ConfigMap.Optional = &optional
	}
	volumeMount := corev1.VolumeMount{
		Name:      cmInfo.name,
		MountPath: cmInfo.mountPath,
	}
	envVar := corev1.EnvVar{
		Name:  cmInfo.envVar,
		Value: fmt.Sprintf("%s/%s", cmInfo.mountPath, cmInfo.nameKey),
	}
	podSpec.Volumes = append(podSpec.Volumes, volume)
	container.VolumeMounts = append(container.VolumeMounts, volumeMount)
	container.Env = append(container.Env, envVar)
}
