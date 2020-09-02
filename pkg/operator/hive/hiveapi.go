package hive

import (
	log "github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes/scheme"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/library-go/pkg/operator/resource/resourceread"

	"github.com/openshift/hive/pkg/operator/assets"
	"github.com/openshift/hive/pkg/operator/util"
	"github.com/openshift/hive/pkg/resource"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
)

func (r *ReconcileHiveConfig) deployHiveAPI(hLog log.FieldLogger, h resource.Helper, hiveConfig *hivev1.HiveConfig) error {

	hiveNSName := getHiveNamespace(hiveConfig)

	if !hiveConfig.Spec.HiveAPIEnabled {
		return r.tearDownHiveAPI(hLog, hiveNSName)
	}

	// Load namespaced assets, decode them, set to our target namespace, and apply:
	namespacedAssets := []string{
		"config/apiserver/service.yaml",
		"config/apiserver/service-account.yaml",
	}
	for _, assetPath := range namespacedAssets {
		if err := util.ApplyAssetWithNSOverrideAndGC(h, assetPath, hiveNSName, hiveConfig); err != nil {
			hLog.WithError(err).Error("error applying object with namespace override")
			return err
		}
		hLog.WithField("asset", assetPath).Info("applied asset with namespace override")
	}

	// Apply global non-namespaced assets:
	applyAssets := []string{
		"config/apiserver/hiveapi_rbac_role.yaml",
	}
	for _, a := range applyAssets {
		if err := util.ApplyAssetWithGC(h, a, hiveConfig, hLog); err != nil {
			return err
		}
	}

	// Apply global ClusterRoleBindings which may need Subject namespace overrides for their ServiceAccounts.
	clusterRoleBindingAssets := []string{
		"config/apiserver/hiveapi_rbac_role_binding.yaml",
	}
	for _, crbAsset := range clusterRoleBindingAssets {
		if err := util.ApplyClusterRoleBindingAssetWithSubjectNSOverrideAndGC(h, crbAsset, hiveNSName, hiveConfig); err != nil {
			hLog.WithError(err).Error("error applying ClusterRoleBinding with namespace override")
			return err
		}
		hLog.WithField("asset", crbAsset).Info("applied ClusterRoleRoleBinding asset with namespace override")
	}

	if err := r.createHiveAPIDeployment(hLog, h, hiveNSName, hiveConfig); err != nil {
		return err
	}

	if err := r.createAPIServerAPIService(hLog, h, hiveNSName, hiveConfig); err != nil {
		return err
	}

	return nil
}

func (r *ReconcileHiveConfig) createAPIServerAPIService(hLog log.FieldLogger, h resource.Helper, hiveNSName string, hiveConfig *hivev1.HiveConfig) error {
	hLog.Debug("reading apiservice")
	asset := assets.MustAsset("config/apiserver/apiservice.yaml")
	apiService := util.ReadAPIServiceV1Beta1OrDie(asset, scheme.Scheme)
	apiService.Spec.Service.Namespace = hiveNSName

	// If on 3.11 we need to set the service CA on the apiservice
	is311, err := r.is311(hLog)
	if err != nil {
		hLog.Error("error detecting 3.11 cluster")
		return err
	}
	// If we're running on vanilla Kube (mostly devs using kind), or OpenShift 3.x, we
	// will not have access to the service cert injection we normally use. Lookup
	// the cluster CA and inject into the APIServer.
	// NOTE: If this is vanilla kube, you will also need to manually create a certificate
	// secret, see hack/hiveapi-dev-cert.sh.
	isOpenShift, err := r.runningOnOpenShift(hLog)
	if err != nil {
		return err
	}

	if !isOpenShift || is311 {
		hLog.Debug("non-OpenShift 4.x cluster detected, modifying apiservice")
		serviceCA, _, err := r.getCACerts(hLog, hiveNSName)
		if err != nil {
			return err
		}
		apiService.Spec.CABundle = serviceCA
	}

	result, err := util.ApplyRuntimeObjectWithGC(h, apiService, hiveConfig)
	if err != nil {
		hLog.WithError(err).Error("error applying apiservice")
		return err
	}

	hLog.Infof("apiservice applied (%s)", result)
	return nil
}

func (r *ReconcileHiveConfig) createHiveAPIDeployment(hLog log.FieldLogger, h resource.Helper, hiveNSName string, hiveConfig *hivev1.HiveConfig) error {
	asset := assets.MustAsset("config/apiserver/deployment.yaml")
	hLog.Debug("reading deployment")
	hiveAPIDeployment := resourceread.ReadDeploymentV1OrDie(asset)
	hiveAPIDeployment.Namespace = hiveNSName

	if r.hiveImage != "" {
		hiveAPIDeployment.Spec.Template.Spec.Containers[0].Image = r.hiveImage
	}

	if r.hiveImagePullPolicy != "" {
		hiveAPIDeployment.Spec.Template.Spec.Containers[0].ImagePullPolicy = r.hiveImagePullPolicy
	}

	result, err := util.ApplyRuntimeObjectWithGC(h, hiveAPIDeployment, hiveConfig)
	if err != nil {
		hLog.WithError(err).Error("error applying deployment")
		return err
	}
	hLog.Infof("hiveapi deployment applied (%s)", result)

	return nil
}

func (r *ReconcileHiveConfig) tearDownHiveAPI(hLog log.FieldLogger, hiveNSName string) error {

	objects := []struct {
		key    client.ObjectKey
		object hivev1.MetaRuntimeObject
	}{
		{
			key:    client.ObjectKey{Namespace: hiveNSName, Name: "hiveapi"},
			object: &appsv1.Deployment{},
		},
		{
			key:    client.ObjectKey{Name: "v1alpha1.hive.openshift.io"},
			object: &apiregistrationv1.APIService{},
		},
		{
			key:    client.ObjectKey{Namespace: hiveNSName, Name: "hiveapi"},
			object: &corev1.Service{},
		},
		{
			key:    client.ObjectKey{Namespace: hiveNSName, Name: "hiveapi-sa"},
			object: &corev1.ServiceAccount{},
		},
	}

	var errorList []error
	for _, obj := range objects {
		if err := resource.DeleteAnyExistingObject(r, obj.key, obj.object, hLog); err != nil {
			errorList = append(errorList, err)
			hLog.WithError(err).Warn("failed to clean up old aggregated API server object")
		}
	}

	return errors.NewAggregate(errorList)
}
