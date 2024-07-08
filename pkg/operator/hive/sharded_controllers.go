package hive

import (
	"context"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/controller/images"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/operator/assets"
	"github.com/openshift/hive/pkg/operator/util"
	"github.com/openshift/hive/pkg/resource"
)

type ssCfg struct {
	name                   hivev1.ControllerName
	deploymentName         hivev1.DeploymentName
	defaultReplicas        int32
	hashAnnotation         string
	containerCustomization func(*hivev1.HiveConfig, *corev1.Container)
}

var (
	clusterSyncCfg = ssCfg{
		name:            hivev1.ClustersyncControllerName,
		deploymentName:  hivev1.DeploymentNameClustersync,
		defaultReplicas: 1,
		hashAnnotation:  "hive.openshift.io/clustersync-statefulset-spec-hash",
		containerCustomization: func(hiveconfig *hivev1.HiveConfig, container *corev1.Container) {
			if syncSetReapplyInterval := hiveconfig.Spec.SyncSetReapplyInterval; syncSetReapplyInterval != "" {
				syncsetReapplyIntervalEnvVar := corev1.EnvVar{
					Name:  constants.SyncSetReapplyIntervalEnvVar,
					Value: syncSetReapplyInterval,
				}

				container.Env = append(container.Env, syncsetReapplyIntervalEnvVar)
			}
		},
	}

	machinePoolCfg = ssCfg{
		name:            hivev1.MachinePoolControllerName,
		deploymentName:  hivev1.DeploymentNameMachinepool,
		defaultReplicas: 1,
		hashAnnotation:  "hive.openshift.io/machinepool-statefulset-spec-hash",
	}
)

type asset struct {
	path      string
	processed []byte
}

func newAsset(path string, values map[string]string) (asset, error) {
	var err error
	ret := asset{
		path: path,
	}
	ret.processed, err = controllerutils.ProcessAssetTemplate(assets.MustAsset(ret.path), values)
	return ret, errors.Wrapf(err, "unable to process asset template %s", path)
}

func (r *ReconcileHiveConfig) deployStatefulSet(c ssCfg, hLog log.FieldLogger, h resource.Helper, hiveconfig *hivev1.HiveConfig, hiveControllersConfigHash string, namespacesToClean []string) error {
	templateValues := map[string]string{
		"ControllerName": string(c.name),
	}
	ssAsset, err := newAsset("config/sharded_controllers/statefulset.yaml", templateValues)
	if err != nil {
		return err
	}
	namespacedAssets := []asset{}
	for _, path := range []string{"config/sharded_controllers/service.yaml"} {
		a, err := newAsset(path, templateValues)
		if err != nil {
			return err
		}
		namespacedAssets = append(namespacedAssets, a)
	}

	// Delete the assets from previous target namespaces
	assetsToClean := append(namespacedAssets, ssAsset)
	for _, ns := range namespacesToClean {
		for _, a := range assetsToClean {
			hLog.Infof("Deleting asset %s from old target namespace %s", a.path, ns)
			// DeleteAsset*WithNSOverride already no-ops for IsNotFound
			if err := util.DeleteAssetBytesWithNSOverride(h, a.processed, ns, hiveconfig); err != nil {
				return errors.Wrapf(err, "error deleting asset %s from old target namespace %s", a.path, ns)
			}
		}
	}

	hLog.Debug("reading statefulset")
	// Safe to ignore error because we processed this asset above
	newStatefulSet := controllerutils.ReadStatefulsetOrDie(ssAsset.processed)
	container, err := containerByName(&newStatefulSet.Spec.Template.Spec, string(c.name))
	if err != nil {
		return err
	}
	applyDeploymentConfig(hiveconfig, c.deploymentName, container, hLog)

	hLog.Infof("hive image: %s", r.hiveImage)
	if r.hiveImage != "" {
		container.Image = r.hiveImage
		hiveImageEnvVar := corev1.EnvVar{
			Name:  images.HiveImageEnvVar,
			Value: r.hiveImage,
		}

		container.Env = append(container.Env, hiveImageEnvVar)
	}

	if r.hiveImagePullPolicy != "" {
		container.ImagePullPolicy = r.hiveImagePullPolicy

		container.Env = append(
			container.Env,
			corev1.EnvVar{
				Name:  images.HiveImagePullPolicyEnvVar,
				Value: string(r.hiveImagePullPolicy),
			},
		)
	}

	if level := hiveconfig.Spec.LogLevel; level != "" {
		container.Args = append(container.Args, "--log-level", level)
	}

	if c.containerCustomization != nil {
		c.containerCustomization(hiveconfig, container)
	}

	hiveNSName := GetHiveNamespace(hiveconfig)

	// Load namespaced assets, decode them, set to our target namespace, and apply:
	for _, a := range namespacedAssets {
		if err := util.ApplyAssetBytesWithNSOverrideAndGC(h, a.processed, hiveNSName, hiveconfig); err != nil {
			hLog.WithError(err).WithField("asset", a.path).Error("error applying object with namespace override")
			return err
		}
		hLog.WithField("asset", a.path).Info("applied asset with namespace override")
	}

	if hiveconfig.Spec.MaintenanceMode != nil && *hiveconfig.Spec.MaintenanceMode {
		hLog.Warnf("maintenanceMode enabled in HiveConfig, setting %s replicas to 0", c.deploymentName)
		replicas := int32(0)
		newStatefulSet.Spec.Replicas = &replicas
	} else {
		// Default replicas and only change if they specify in hiveconfig.
		newStatefulSet.Spec.Replicas = ptr.To(c.defaultReplicas)

		if hiveconfig.Spec.ControllersConfig != nil {
			// Set the number of replicas that was given in the hiveconfig
			controllerConfig, found := getHiveControllerConfig(c.name, hiveconfig.Spec.ControllersConfig.Controllers)
			if found && controllerConfig.Replicas != nil {
				newStatefulSet.Spec.Replicas = controllerConfig.Replicas
			}

			if found && controllerConfig.Resources != nil {
				container.Resources = *controllerConfig.Resources
			}
		}
	}

	newStatefulSetSpecHash, err := controllerutils.CalculateStatefulSetSpecHash(newStatefulSet)
	if err != nil {
		hLog.WithError(err).Error("error calculating new statefulset hash")
		return err
	}

	if newStatefulSet.Annotations == nil {
		newStatefulSet.Annotations = make(map[string]string, 1)
	}
	newStatefulSet.Annotations[c.hashAnnotation] = newStatefulSetSpecHash

	httpProxy, httpsProxy, noProxy, err := r.discoverProxyVars()
	if err != nil {
		return err
	}
	controllerutils.SetProxyEnvVars(&newStatefulSet.Spec.Template.Spec, httpProxy, httpsProxy, noProxy)

	if newStatefulSet.Spec.Template.Annotations == nil {
		newStatefulSet.Spec.Template.Annotations = make(map[string]string, 1)
	}
	// Include the proxy vars in the hash so we redeploy if they change
	newStatefulSet.Spec.Template.Annotations[hiveConfigHashAnnotation] = computeHash(
		httpProxy+httpsProxy+noProxy, hiveControllersConfigHash)

	existingStatefulSet := &appsv1.StatefulSet{}
	existingStatefulSetNamespacedName := apitypes.NamespacedName{Name: newStatefulSet.Name, Namespace: newStatefulSet.Namespace}
	err = r.Get(context.TODO(), existingStatefulSetNamespacedName, existingStatefulSet)
	if err == nil {
		if existingStatefulSet.DeletionTimestamp != nil {
			hLog.Infof("%s statefulset is in a deleting state. Not reconciling the statefulset", c.name)
			return nil
		}

		if hasStatefulSetSpecChanged(c, existingStatefulSet, newStatefulSet, hLog) {
			// The statefulset spec.selector changed. Trying to apply this new statefulset will error with:
			//     invalid: spec: Forbidden: updates to statefulset spec for fields other than 'replicas', 'template', and 'updateStrategy' are forbidden"
			// The only fix is to delete the statefulset and have the apply below recreate it.
			hLog.Infof("deleting the existing %s statefulset because spec has changed", c.name)
			err := h.Delete(existingStatefulSet.APIVersion, existingStatefulSet.Kind, existingStatefulSet.Namespace, existingStatefulSet.Name)
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
	newStatefulSet.Spec.Template.Spec.NodeSelector = r.nodeSelector
	newStatefulSet.Spec.Template.Spec.Tolerations = r.tolerations

	newStatefulSet.Namespace = hiveNSName
	result, err := util.ApplyRuntimeObjectWithGC(h, newStatefulSet, hiveconfig)
	if err != nil {
		hLog.WithError(err).Error("error applying statefulset")
		return err
	}
	hLog.Infof("%s statefulset applied (%s)", c.name, result)

	hLog.Infof("all %s components successfully reconciled", c.name)
	return nil
}

func hasStatefulSetSpecChanged(c ssCfg, existingStatefulSet, newStatefulSet *appsv1.StatefulSet, hLog log.FieldLogger) bool {
	// hash doesn't exist, assume the spec has changed.
	if existingStatefulSet == nil {
		hLog.Debugf("existing %s statefulset is nil, so the spec didn't change.", c.name)
		return false
	}

	if existingStatefulSet.Annotations == nil {
		hLog.Debugf("existing %s statefulset Annotations is nil, assuming spec changed.", c.name)
		return true
	}

	existingStatefulSetSpecHash, found := existingStatefulSet.Annotations[c.hashAnnotation]
	if !found {
		hLog.Debugf("existing %s statefulset Annotations doesn't contain the spec hash, assuming spec changed.", c.name)
		return true
	}

	newStatefulSetSpecHash := newStatefulSet.Annotations[c.hashAnnotation]
	if existingStatefulSetSpecHash != newStatefulSetSpecHash {
		hLog.Debugf("existing %s statefulset Spec changed", c.name)
		return true
	}

	hLog.Debugf("existing %s statefulset Spec hasn't changed", c.name)
	return false
}
