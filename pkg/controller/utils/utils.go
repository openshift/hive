package utils

import (
	"crypto/md5"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"

	machineapi "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openshiftapiv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"

	apihelpers "github.com/openshift/hive/pkg/apis/helpers"
)

// BuildClusterAPIClientFromKubeconfig will return a kubeclient with metrics using the provided kubeconfig.
// Controller name is required for metrics purposes.
func BuildClusterAPIClientFromKubeconfig(kubeconfigData, controllerName string) (client.Client, error) {
	config, err := clientcmd.Load([]byte(kubeconfigData))
	if err != nil {
		return nil, err
	}
	kubeConfig := clientcmd.NewDefaultClientConfig(*config, &clientcmd.ConfigOverrides{})
	cfg, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	AddControllerMetricsTransportWrapper(cfg, controllerName, true)

	scheme, err := machineapi.SchemeBuilder.Build()
	if err != nil {
		return nil, err
	}

	if err := openshiftapiv1.Install(scheme); err != nil {
		return nil, err
	}

	if err := routev1.Install(scheme); err != nil {
		return nil, err
	}

	return client.New(cfg, client.Options{
		Scheme: scheme,
	})
}

// HasUnreachableCondition returns true if the cluster deployment has the unreachable condition set to true.
func HasUnreachableCondition(cd *hivev1.ClusterDeployment) bool {
	condition := FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.UnreachableCondition)
	if condition != nil {
		return condition.Status == corev1.ConditionTrue
	}
	return false
}

// BuildDynamicClientFromKubeconfig returns a dynamic client with metrics, using the provided kubeconfig.
// Controller name is required for metrics purposes.
func BuildDynamicClientFromKubeconfig(kubeconfigData, controllerName string) (dynamic.Interface, error) {
	config, err := clientcmd.Load([]byte(kubeconfigData))
	if err != nil {
		return nil, err
	}
	kubeConfig := clientcmd.NewDefaultClientConfig(*config, &clientcmd.ConfigOverrides{})
	cfg, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	AddControllerMetricsTransportWrapper(cfg, controllerName, true)

	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// FixupEmptyClusterVersionFields will un-'nil' fields that would fail validation in the ClusterVersion.Status
func FixupEmptyClusterVersionFields(clusterVersionStatus *openshiftapiv1.ClusterVersionStatus) {

	// Fetching clusterVersion object can result in nil clusterVersion.Status.AvailableUpdates
	// Place an empty list if needed to satisfy the object validation.

	if clusterVersionStatus.AvailableUpdates == nil {
		clusterVersionStatus.AvailableUpdates = []openshiftapiv1.Update{}
	}
}

// HasFinalizer returns true if the given object has the given finalizer
func HasFinalizer(object metav1.Object, finalizer string) bool {
	for _, f := range object.GetFinalizers() {
		if f == finalizer {
			return true
		}
	}
	return false
}

// AddFinalizer adds a finalizer to the given object
func AddFinalizer(object metav1.Object, finalizer string) {
	finalizers := sets.NewString(object.GetFinalizers()...)
	finalizers.Insert(finalizer)
	object.SetFinalizers(finalizers.List())
}

// DeleteFinalizer removes a finalizer from the given object
func DeleteFinalizer(object metav1.Object, finalizer string) {
	finalizers := sets.NewString(object.GetFinalizers()...)
	finalizers.Delete(finalizer)
	object.SetFinalizers(finalizers.List())
}

// GetKubeClient creates a new Kubernetes dynamic client.
func GetKubeClient(scheme *runtime.Scheme) (client.Client, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{})
	cfg, err := kubeconfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	dynamicClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}

	return dynamicClient, nil
}

const (
	concurrentControllerReconciles = 5
)

// GetConcurrentReconciles returns the number of goroutines each controller should
// use for parallel processing of their queue. For now this is a static value of 5.
// In future this may be read from an env var set by the operator, and driven by HiveConfig.
func GetConcurrentReconciles() int {
	return concurrentControllerReconciles
}

// MergeJsons will merge the global and local pull secret and return it
func MergeJsons(globalPullSecret string, localPullSecret string, cdLog log.FieldLogger) (string, error) {

	type dockerConfig map[string]interface{}
	type dockerConfigJSON struct {
		Auths dockerConfig `json:"auths"`
	}

	var mGlobal, mLocal dockerConfigJSON
	jGlobal := []byte(globalPullSecret)
	err := json.Unmarshal(jGlobal, &mGlobal)
	if err != nil {
		return "", err
	}

	jLocal := []byte(localPullSecret)
	err = json.Unmarshal(jLocal, &mLocal)
	if err != nil {
		return "", err
	}

	for k, v := range mLocal.Auths {
		if _, ok := mGlobal.Auths[k]; ok {
			cdLog.Infof("The auth for %s from cluster deployment pull secret is used instead of global pull secret", k)
		}
		mGlobal.Auths[k] = v
	}
	jMerged, err := json.Marshal(mGlobal)
	if err != nil {
		return "", err
	}
	return string(jMerged), nil
}

// GetChecksumOfObject returns the md5sum hash of the object passed in.
func GetChecksumOfObject(object interface{}) (string, error) {
	b, err := json.Marshal(object)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", md5.Sum(b)), nil
}

// GetChecksumOfObjects returns the md5sum hash of the objects passed in.
func GetChecksumOfObjects(objects ...interface{}) (string, error) {
	return GetChecksumOfObject(objects)
}

// DNSZoneName returns the predictable name for a DNSZone for the given ClusterDeployment.
func DNSZoneName(cdName string) string {
	return apihelpers.GetResourceName(cdName, "zone")
}

// LogLevel returns the log level to use to log the specified error.
func LogLevel(err error) log.Level {
	if err == nil {
		return log.ErrorLevel
	}
	for {
		switch {
		case apierrors.IsAlreadyExists(err),
			apierrors.IsConflict(err),
			apierrors.IsNotFound(err):
			return log.InfoLevel
		}
		cause := errors.Cause(err)
		if cause == err {
			return log.ErrorLevel
		}
		err = cause
	}
}
