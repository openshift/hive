package hive

import (
	log "github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes/scheme"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/library-go/pkg/operator/resource/resourceread"

	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/operator/assets"
	"github.com/openshift/hive/pkg/operator/util"
	"github.com/openshift/hive/pkg/resource"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
)

func (r *ReconcileHiveConfig) deployHiveAPI(hLog log.FieldLogger, h *resource.Helper, hiveConfig *hivev1.HiveConfig) error {

	if !hiveConfig.Spec.HiveAPIEnabled {
		return r.tearDownHiveAPI(hLog)
	}

	err := util.ApplyAsset(h, "config/apiserver/hiveapi-cluster-role-binding.yaml", hLog)
	if err != nil {
		return err
	}

	err = util.ApplyAsset(h, "config/apiserver/service.yaml", hLog)
	if err != nil {
		return err
	}

	err = util.ApplyAsset(h, "config/apiserver/service-account.yaml", hLog)
	if err != nil {
		return err
	}

	if err := r.createHiveAPIDeployment(hLog, h); err != nil {
		return err
	}

	if err := r.createAPIServerAPIService(hLog, h); err != nil {
		return err
	}

	return nil
}

func (r *ReconcileHiveConfig) createAPIServerAPIService(hLog log.FieldLogger, h *resource.Helper) error {
	hLog.Debug("reading apiservice")
	asset := assets.MustAsset("config/apiserver/apiservice.yaml")
	apiService := util.ReadAPIServiceV1Beta1OrDie(asset, scheme.Scheme)

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
	if !r.runningOnOpenShift(hLog) || is311 {
		hLog.Debug("non-OpenShift 4.x cluster detected, modifying apiservice")
		serviceCA, _, err := r.getCACerts(hLog)
		if err != nil {
			return err
		}
		apiService.Spec.CABundle = serviceCA
	}

	result, err := h.ApplyRuntimeObject(apiService, scheme.Scheme)
	if err != nil {
		hLog.WithError(err).Error("error applying apiservice")
		return err
	}

	hLog.Infof("apiservice applied (%s)", result)
	return nil
}

func (r *ReconcileHiveConfig) createHiveAPIDeployment(hLog log.FieldLogger, h *resource.Helper) error {
	asset := assets.MustAsset("config/apiserver/deployment.yaml")
	hLog.Debug("reading deployment")
	hiveAPIDeployment := resourceread.ReadDeploymentV1OrDie(asset)

	if r.hiveImage != "" {
		hiveAPIDeployment.Spec.Template.Spec.Containers[0].Image = r.hiveImage
	}

	if r.hiveImagePullPolicy != "" {
		hiveAPIDeployment.Spec.Template.Spec.Containers[0].ImagePullPolicy = r.hiveImagePullPolicy
	}

	result, err := h.ApplyRuntimeObject(hiveAPIDeployment, scheme.Scheme)
	if err != nil {
		hLog.WithError(err).Error("error applying deployment")
		return err
	}
	hLog.Infof("hiveapi deployment applied (%s)", result)

	return nil
}

func (r *ReconcileHiveConfig) tearDownHiveAPI(hLog log.FieldLogger) error {

	objects := []struct {
		key    client.ObjectKey
		object runtime.Object
	}{
		{
			key:    client.ObjectKey{Namespace: constants.HiveNamespace, Name: "hiveapi"},
			object: &appsv1.Deployment{},
		},
		{
			key:    client.ObjectKey{Name: "v1alpha1.hive.openshift.io"},
			object: &apiregistrationv1.APIService{},
		},
		{
			key:    client.ObjectKey{Namespace: constants.HiveNamespace, Name: "hiveapi"},
			object: &corev1.Service{},
		},
		{
			key:    client.ObjectKey{Namespace: constants.HiveNamespace, Name: "hiveapi-sa"},
			object: &corev1.ServiceAccount{},
		},
	}

	errorList := []error{}
	for _, obj := range objects {
		if err := resource.DeleteAnyExistingObject(r, obj.key, obj.object, hLog); err != nil {
			errorList = append(errorList, err)
			hLog.WithError(err).Warn("failed to clean up old aggregated API server object")
		}
	}

	return errors.NewAggregate(errorList)
}
