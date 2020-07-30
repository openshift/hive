package clustersyncset

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	resource "github.com/openshift/hive/pkg/resource"
)

// applier knows how to Apply, Patch and return Info for []byte arrays describing objects and patches.
type applier interface {
	Apply(obj []byte) (resource.ApplyResult, error)
	Info(obj []byte) (*resource.Info, error)
	Patch(name types.NamespacedName, kind, apiVersion string, patch []byte, patchType string) error
	ApplyRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (resource.ApplyResult, error)
	CreateOrUpdate(obj []byte) (resource.ApplyResult, error)
	CreateOrUpdateRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (resource.ApplyResult, error)
	Create(obj []byte) (resource.ApplyResult, error)
	CreateRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (resource.ApplyResult, error)
	Delete(apiVersion, kind, namespace, name string) error
}
