package hive

import (
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/controller/images"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/operator/assets"
	"github.com/openshift/hive/pkg/operator/util"
	"github.com/openshift/hive/pkg/resource"
)

const (
	defaultClustersyncReplicas = 1
)

func (r *ReconcileHiveConfig) deployClusterSync(hLog log.FieldLogger, h resource.Helper, hiveconfig *hivev1.HiveConfig, hiveControllersConfigHash string) error {
	asset := assets.MustAsset("config/clustersync/statefulset.yaml")
	hLog.Debug("reading statefulset")
	clusterSyncStatefulSet := controllerutils.ReadStatefulsetOrDie(asset)
	hiveContainer := &clusterSyncStatefulSet.Spec.Template.Spec.Containers[0]

	hLog.Infof("hive image: %s", r.hiveImage)
	if r.hiveImage != "" {
		hiveContainer.Image = r.hiveImage
		hiveImageEnvVar := corev1.EnvVar{
			Name:  images.HiveImageEnvVar,
			Value: r.hiveImage,
		}

		hiveContainer.Env = append(hiveContainer.Env, hiveImageEnvVar)
	}

	if r.hiveImagePullPolicy != "" {
		hiveContainer.ImagePullPolicy = r.hiveImagePullPolicy

		hiveContainer.Env = append(
			hiveContainer.Env,
			corev1.EnvVar{
				Name:  images.HiveImagePullPolicyEnvVar,
				Value: string(r.hiveImagePullPolicy),
			},
		)
	}

	if level := hiveconfig.Spec.LogLevel; level != "" {
		hiveContainer.Args = append(hiveContainer.Args, "--log-level", level)
	}

	if syncSetReapplyInterval := hiveconfig.Spec.SyncSetReapplyInterval; syncSetReapplyInterval != "" {
		syncsetReapplyIntervalEnvVar := corev1.EnvVar{
			Name:  "SYNCSET_REAPPLY_INTERVAL",
			Value: syncSetReapplyInterval,
		}

		hiveContainer.Env = append(hiveContainer.Env, syncsetReapplyIntervalEnvVar)
	}

	hiveNSName := getHiveNamespace(hiveconfig)

	if clusterSyncStatefulSet.Spec.Template.Annotations == nil {
		clusterSyncStatefulSet.Spec.Template.Annotations = make(map[string]string, 1)
	}
	clusterSyncStatefulSet.Spec.Template.Annotations[hiveConfigHashAnnotation] = hiveControllersConfigHash

	// Load namespaced assets, decode them, set to our target namespace, and apply:
	namespacedAssets := []string{
		"config/clustersync/service.yaml",
	}
	for _, assetPath := range namespacedAssets {
		if err := util.ApplyAssetWithNSOverrideAndGC(h, assetPath, hiveNSName, hiveconfig); err != nil {
			hLog.WithError(err).Error("error applying object with namespace override")
			return err
		}
		hLog.WithField("asset", assetPath).Info("applied asset with namespace override")
	}

	if hiveconfig.Spec.MaintenanceMode != nil && *hiveconfig.Spec.MaintenanceMode {
		hLog.Warn("maintenanceMode enabled in HiveConfig, setting hive-clustersync replicas to 0")
		replicas := int32(0)
		clusterSyncStatefulSet.Spec.Replicas = &replicas
	} else {
		// Default replicas to defaultClustersyncReplicas and only change if they specify in hiveconfig.
		clusterSyncStatefulSet.Spec.Replicas = pointer.Int32Ptr(defaultClustersyncReplicas)

		if hiveconfig.Spec.ControllersConfig != nil {
			// Set the number of replicas that was given in the hiveconfig
			clusterSyncControllerConfig, found := getHiveControllerConfig(hivev1.ClustersyncControllerName, hiveconfig.Spec.ControllersConfig.Controllers)
			if found && clusterSyncControllerConfig.Replicas != nil {
				clusterSyncStatefulSet.Spec.Replicas = clusterSyncControllerConfig.Replicas
			}
		}
	}

	clusterSyncStatefulSet.Namespace = hiveNSName
	result, err := util.ApplyRuntimeObjectWithGC(h, clusterSyncStatefulSet, hiveconfig)
	if err != nil {
		hLog.WithError(err).Error("error applying statefulset")
		return err
	}
	hLog.Infof("clustersync statefulset applied (%s)", result)

	hLog.Info("all clustersync components successfully reconciled")
	return nil
}
