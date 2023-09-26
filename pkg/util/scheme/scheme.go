// Package provides singleton scheme to the codebase.
// It is not advised to use a global scheme due to the potential
// for collision with other packages.
// The hive specific scheme is registered once, with all necessary
// packages, and used throughout the codebase, removing the need to specify
// unique schemes for each use case, and decreasing the overall number of
// imports.

package scheme

import (
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	velerov1 "github.com/heptio/velero/pkg/apis/velero/v1"
	oappsv1 "github.com/openshift/api/apps/v1"
	orbacv1 "github.com/openshift/api/authorization/v1"
	configv1 "github.com/openshift/api/config/v1"
	machinev1alpha1 "github.com/openshift/api/machine/v1alpha1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	ingresscontroller "github.com/openshift/api/operator/v1"
	routev1 "github.com/openshift/api/route/v1"
	autoscalingv1 "github.com/openshift/cluster-autoscaler-operator/pkg/apis/autoscaling/v1"
	autoscalingv1beta1 "github.com/openshift/cluster-autoscaler-operator/pkg/apis/autoscaling/v1beta1"
	"github.com/openshift/hive/apis"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	crv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"

	ovirtprovider "github.com/openshift/cluster-api-provider-ovirt/pkg/apis"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivecontractsv1alpha1 "github.com/openshift/hive/apis/hivecontracts/v1alpha1"
	hiveintv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var hive_scheme *runtime.Scheme

func init() {
	hive_scheme = runtime.NewScheme()
	admissionregistrationv1.AddToScheme(hive_scheme)
	admissionv1beta1.AddToScheme(hive_scheme)
	apis.AddToScheme(hive_scheme)
	apiextv1.AddToScheme(hive_scheme)
	appsv1.AddToScheme(hive_scheme)
	apiregistrationv1.AddToScheme(hive_scheme)
	autoscalingv1.SchemeBuilder.AddToScheme(hive_scheme)
	autoscalingv1beta1.SchemeBuilder.AddToScheme(hive_scheme)
	batchv1.AddToScheme(hive_scheme)
	configv1.AddToScheme(hive_scheme)
	corev1.AddToScheme(hive_scheme)
	crv1alpha1.AddToScheme(hive_scheme)
	hivecontractsv1alpha1.AddToScheme(hive_scheme)
	hiveintv1alpha1.AddToScheme(hive_scheme)
	hivev1.AddToScheme(hive_scheme)
	ingresscontroller.AddToScheme(hive_scheme)
	// For some reason machinev1alpha1.Install() doesn't add any types.
	// Mimic how installer registers OpenstackProviderSpec:
	hive_scheme.AddKnownTypes(machinev1alpha1.GroupVersion,
		&machinev1alpha1.OpenstackProviderSpec{},
	)
	machinev1beta1.AddToScheme(hive_scheme)
	monitoringv1.AddToScheme(hive_scheme)
	oappsv1.Install(hive_scheme)
	orbacv1.Install(hive_scheme)
	ovirtprovider.AddToScheme(hive_scheme)
	rbacv1.AddToScheme(hive_scheme)
	routev1.AddToScheme(hive_scheme)
	velerov1.AddToScheme(hive_scheme)

}

func GetScheme() *runtime.Scheme {
	return hive_scheme
}
