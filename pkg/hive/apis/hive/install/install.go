package install

import (
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/kubernetes/pkg/api/legacyscheme"

	hivev1alpha1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	hiveapi "github.com/openshift/hive/pkg/hive/apis/hive"
	"github.com/openshift/hive/pkg/hive/apis/hive/hiveconversion"
	hiveapiv1alpha1 "github.com/openshift/hive/pkg/hive/apis/hive/v1alpha1"
)

func init() {
	Install(legacyscheme.Scheme)
}

// Install registers the API group and adds types to a scheme
func Install(scheme *runtime.Scheme) {
	utilruntime.Must(hiveapi.Install(scheme))
	utilruntime.Must(hiveconversion.AddToScheme(scheme))
	utilruntime.Must(hiveapiv1alpha1.Install(scheme))
	utilruntime.Must(scheme.SetVersionPriority(hivev1alpha1.SchemeGroupVersion))
}
