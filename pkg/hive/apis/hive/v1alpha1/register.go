package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
	corev1conversions "k8s.io/kubernetes/pkg/apis/core/v1"

	hivev1alpah1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	hiveapi "github.com/openshift/hive/pkg/hive/apis/hive"
)

var (
	localSchemeBuilder = runtime.NewSchemeBuilder(
		hiveapi.Install,
		hivev1alpah1.AddToScheme,
		corev1conversions.AddToScheme,
		RegisterDefaults,
	)
	// Install installs the Hive v1alpha1 API
	Install = localSchemeBuilder.AddToScheme
)
