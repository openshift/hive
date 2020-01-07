package install

import (
	"k8s.io/apimachinery/pkg/runtime"

	hivev1alpha1 "github.com/openshift/hive/pkg/hive/apis/hive/install"
)

// InternalHive install the Hive API for internal consumption
func InternalHive(scheme *runtime.Scheme) {
	hivev1alpha1.Install(scheme)
}
