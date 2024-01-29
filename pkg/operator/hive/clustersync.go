package hive

import (
	"context"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/controller/images"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/operator/assets"
	"github.com/openshift/hive/pkg/operator/util"
	"github.com/openshift/hive/pkg/resource"
)

const (
	defaultClustersyncReplicas = 1
)

func (r *ReconcileHiveConfig) deployClusterSync(hLog log.FieldLogger, h resource.Helper, hiveconfig *hivev1.HiveConfig, hiveControllersConfigHash string, namespacesToClean []string) error {
	ssAsset := "config/clustersync/statefulset.yaml"
	namespacedAssets := []string{
		"config/clustersync/service.yaml",
	}
	// Delete the assets from previous target namespaces
	assetsToClean := append(namespacedAssets, ssAsset)
	for _, ns := range namespacesToClean {
		for _, asset := range assetsToClean {
			hLog.Infof("Deleting asset %s from old target namespace %s", asset, ns)
			// DeleteAssetWithNSOverride already no-ops for IsNotFound
			if err := util.DeleteAssetWithNSOverride(h, asset, ns, hiveconfig); err != nil {
				return errors.Wrapf(err, "error deleting asset %s from old target namespace %s", asset, ns)
			}
		}
	}

	asset := assets.MustAsset(ssAsset)
	hLog.Debug("reading statefulset")
	newClusterSyncStatefulSet := controllerutils.ReadStatefulsetOrDie(asset)
	clusterSyncContainer, err := containerByName(&newClusterSyncStatefulSet.Spec.Template.Spec, "clustersync")
	if err != nil {
		return err
	}
	applyDeploymentConfig(hiveconfig, hivev1.DeploymentNameClustersync, clusterSyncContainer, hLog)

	hLog.Infof("hive image: %s", r.hiveImage)
	if r.hiveImage != "" {
		clusterSyncContainer.Image = r.hiveImage
		hiveImageEnvVar := corev1.EnvVar{
			Name:  images.HiveImageEnvVar,
			Value: r.hiveImage,
		}

		clusterSyncContainer.Env = append(clusterSyncContainer.Env, hiveImageEnvVar)
	}

	if r.hiveImagePullPolicy != "" {
		clusterSyncContainer.ImagePullPolicy = r.hiveImagePullPolicy

		clusterSyncContainer.Env = append(
			clusterSyncContainer.Env,
			corev1.EnvVar{
				Name:  images.HiveImagePullPolicyEnvVar,
				Value: string(r.hiveImagePullPolicy),
			},
		)
	}

	if level := hiveconfig.Spec.LogLevel; level != "" {
		clusterSyncContainer.Args = append(clusterSyncContainer.Args, "--log-level", level)
	}

	if syncSetReapplyInterval := hiveconfig.Spec.SyncSetReapplyInterval; syncSetReapplyInterval != "" {
		syncsetReapplyIntervalEnvVar := corev1.EnvVar{
			Name:  "SYNCSET_REAPPLY_INTERVAL",
			Value: syncSetReapplyInterval,
		}

		clusterSyncContainer.Env = append(clusterSyncContainer.Env, syncsetReapplyIntervalEnvVar)
	}

	hiveNSName := GetHiveNamespace(hiveconfig)

	// Load namespaced assets, decode them, set to our target namespace, and apply:
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
		newClusterSyncStatefulSet.Spec.Replicas = &replicas
	} else {
		// Default replicas to defaultClustersyncReplicas and only change if they specify in hiveconfig.
		newClusterSyncStatefulSet.Spec.Replicas = pointer.Int32Ptr(defaultClustersyncReplicas)

		if hiveconfig.Spec.ControllersConfig != nil {
			// Set the number of replicas that was given in the hiveconfig
			clusterSyncControllerConfig, found := getHiveControllerConfig(hivev1.ClustersyncControllerName, hiveconfig.Spec.ControllersConfig.Controllers)
			if found && clusterSyncControllerConfig.Replicas != nil {
				newClusterSyncStatefulSet.Spec.Replicas = clusterSyncControllerConfig.Replicas
			}

			if found && clusterSyncControllerConfig.Resources != nil {
				clusterSyncContainer.Resources = *clusterSyncControllerConfig.Resources
			}
		}
	}

	newClusterSyncStatefulSetSpecHash, err := controllerutils.CalculateStatefulSetSpecHash(newClusterSyncStatefulSet)
	if err != nil {
		hLog.WithError(err).Error("error calculating new statefulset hash")
		return err
	}

	if newClusterSyncStatefulSet.Annotations == nil {
		newClusterSyncStatefulSet.Annotations = make(map[string]string, 1)
	}
	newClusterSyncStatefulSet.Annotations[hiveClusterSyncStatefulSetSpecHashAnnotation] = newClusterSyncStatefulSetSpecHash

	httpProxy, httpsProxy, noProxy, err := r.discoverProxyVars()
	if err != nil {
		return err
	}
	controllerutils.SetProxyEnvVars(&newClusterSyncStatefulSet.Spec.Template.Spec, httpProxy, httpsProxy, noProxy)

	if newClusterSyncStatefulSet.Spec.Template.Annotations == nil {
		newClusterSyncStatefulSet.Spec.Template.Annotations = make(map[string]string, 1)
	}
	// Include the proxy vars in the hash so we redeploy if they change
	newClusterSyncStatefulSet.Spec.Template.Annotations[hiveConfigHashAnnotation] = computeHash(
		httpProxy+httpsProxy+noProxy, hiveControllersConfigHash)

	existingClusterSyncStatefulSet := &appsv1.StatefulSet{}
	existingClusterSyncStatefulSetNamespacedName := apitypes.NamespacedName{Name: newClusterSyncStatefulSet.Name, Namespace: newClusterSyncStatefulSet.Namespace}
	err = r.Get(context.TODO(), existingClusterSyncStatefulSetNamespacedName, existingClusterSyncStatefulSet)
	if err == nil {
		if existingClusterSyncStatefulSet.DeletionTimestamp != nil {
			hLog.Info("clustersync statefulset is in a deleting state. Not reconciling the statefulset")
			return nil
		}

		if hasStatefulSetSpecChanged(existingClusterSyncStatefulSet, newClusterSyncStatefulSet, hLog) {
			// The statefulset spec.selector changed. Trying to apply this new statefulset will error with:
			//     invalid: spec: Forbidden: updates to statefulset spec for fields other than 'replicas', 'template', and 'updateStrategy' are forbidden"
			// The only fix is to delete the statefulset and have the apply below recreate it.
			hLog.Info("deleting the existing clustersync statefulset because spec has changed")
			err := h.Delete(existingClusterSyncStatefulSet.APIVersion, existingClusterSyncStatefulSet.Kind, existingClusterSyncStatefulSet.Namespace, existingClusterSyncStatefulSet.Name)
			if err != nil {
				hLog.WithError(err).Error("error deleting statefulset")
			}

			// No matter what, we want to return:
			// * On success deleting the sts, we want to return as the delete will trigger another reconcile.
			// * On error deleting the sts, we don't want to attempt the apply
			return err
		}
	}

	// Apply nodeSelector and tolerations passed through from the operator deployment
	newClusterSyncStatefulSet.Spec.Template.Spec.NodeSelector = r.nodeSelector
	newClusterSyncStatefulSet.Spec.Template.Spec.Tolerations = r.tolerations

	newClusterSyncStatefulSet.Namespace = hiveNSName
	result, err := util.ApplyRuntimeObjectWithGC(h, newClusterSyncStatefulSet, hiveconfig)
	if err != nil {
		hLog.WithError(err).Error("error applying statefulset")
		return err
	}
	hLog.Infof("clustersync statefulset applied (%s)", result)

	hLog.Info("all clustersync components successfully reconciled")
	return nil
}

func hasStatefulSetSpecChanged(existingClusterSyncStatefulSet, newClusterSyncStatefulSet *appsv1.StatefulSet, hLog log.FieldLogger) bool {
	// hash doesn't exist, assume the spec has changed.
	if existingClusterSyncStatefulSet == nil {
		hLog.Debug("existing clustersync statefulset is nil, so the spec didn't change.")
		return false
	}

	if existingClusterSyncStatefulSet.Annotations == nil {
		hLog.Debug("existing clustersync statefulset Annotations is nil, assuming spec changed.")
		return true
	}

	existingClusterSyncStatefulSetSpecHash, found := existingClusterSyncStatefulSet.Annotations[hiveClusterSyncStatefulSetSpecHashAnnotation]
	if !found {
		hLog.Debug("existing clustersync statefulset Annotations doesn't contain the spec hash, assuming spec changed.")
		return true
	}

	newClusterSyncStatefulSetSpecHash := newClusterSyncStatefulSet.Annotations[hiveClusterSyncStatefulSetSpecHashAnnotation]
	if existingClusterSyncStatefulSetSpecHash != newClusterSyncStatefulSetSpecHash {
		hLog.Debug("existing clustersync statefulset Spec changed")
		return true
	}

	hLog.Debug("existing clustersync statefulset Spec hasn't changed")
	return false
}
