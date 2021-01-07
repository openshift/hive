package hive

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strconv"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/operator/util"
	"github.com/openshift/hive/pkg/resource"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

const (
	// hiveControllersConfigMapName is the name of the configmap to store the
	// configurations like goroutines, qps, burst etc. for different hive controllers
	hiveControllersConfigMapName = "hive-controllers-config"
)

func (r *ReconcileHiveConfig) deployHiveControllersConfigMap(hLog log.FieldLogger, h resource.Helper, instance *hivev1.HiveConfig) (string, error) {
	hiveControllersConfigMap := &corev1.ConfigMap{}
	hiveControllersConfigMap.Name = hiveControllersConfigMapName
	hiveControllersConfigMap.Namespace = getHiveNamespace(instance)
	hiveControllersConfigMap.Data = make(map[string]string)

	if instance.Spec.ControllersConfig != nil {
		if instance.Spec.ControllersConfig.Default != nil {
			setHiveControllersConfig(instance.Spec.ControllersConfig.Default, hiveControllersConfigMap, "default")
		}
		for _, controller := range instance.Spec.ControllersConfig.Controllers {
			replicasIsSet := controller.Config.Replicas != nil
			replicasShouldBeSet := controllersUsingReplicas.Contains(controller.Name)
			if !replicasShouldBeSet && replicasIsSet {
				hLog.WithField("controller", controller.Name).Warn("hiveconfig.spec.controllersConfig.controllers[].config.replicas shouldn't be set for this controller")
			}

			setHiveControllersConfig(&controller.Config, hiveControllersConfigMap, controller.Name)
		}
	}

	result, err := util.ApplyRuntimeObjectWithGC(h, hiveControllersConfigMap, instance)
	if err != nil {
		hLog.WithError(err).Error("error applying hive-controllers-config configmap")
		return "", err
	}
	hLog.WithField("result", result).Info("hive-controllers-config configmap applied")

	hLog.Info("Hashing hive-controllers-config data onto a hive deployment annotation")
	hiveControllersConfigHash := computeHiveControllersConfigHash(hiveControllersConfigMap)

	return hiveControllersConfigHash, nil
}

func getHiveControllerConfig(controllerName hivev1.ControllerName, controllerConfigs []hivev1.SpecificControllerConfig) (*hivev1.ControllerConfig, bool) {
	for _, controllerConfig := range controllerConfigs {
		if controllerConfig.Name == controllerName {
			return &controllerConfig.Config, true
		}
	}

	return nil, false
}

func setHiveControllersConfig(config *hivev1.ControllerConfig, hiveControllersConfigMap *corev1.ConfigMap, controllerName hivev1.ControllerName) {
	if config.ConcurrentReconciles != nil {
		hiveControllersConfigMap.Data[fmt.Sprintf(utils.ConcurrentReconcilesEnvVariableFormat, controllerName)] = strconv.Itoa(int(*config.ConcurrentReconciles))
	}
	if config.ClientQPS != nil {
		hiveControllersConfigMap.Data[fmt.Sprintf(utils.ClientQPSEnvVariableFormat, controllerName)] = strconv.Itoa(int(*config.ClientQPS))
	}
	if config.ClientBurst != nil {
		hiveControllersConfigMap.Data[fmt.Sprintf(utils.ClientBurstEnvVariableFormat, controllerName)] = strconv.Itoa(int(*config.ClientBurst))
	}
	if config.QueueQPS != nil {
		hiveControllersConfigMap.Data[fmt.Sprintf(utils.QueueQPSEnvVariableFormat, controllerName)] = strconv.Itoa(int(*config.QueueQPS))
	}
	if config.QueueBurst != nil {
		hiveControllersConfigMap.Data[fmt.Sprintf(utils.QueueBurstEnvVariableFormat, controllerName)] = strconv.Itoa(int(*config.QueueBurst))
	}
}

func computeHiveControllersConfigHash(hiveControllersConfigMap *corev1.ConfigMap) string {
	hasher := md5.New()
	hasher.Write([]byte(fmt.Sprintf("%v", hiveControllersConfigMap.Data)))
	return hex.EncodeToString(hasher.Sum(nil))
}
