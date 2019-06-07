package utils

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
)

// AddTypeMeta adds type metadata to objects in a list of RawExtension
// TypeMeta is needed for proper serialization/deserialization
func AddTypeMeta(objects []runtime.RawExtension, scheme *runtime.Scheme) ([]runtime.RawExtension, error) {
	result := []runtime.RawExtension{}
	for i := range objects {
		object := objects[i].Object
		gvks, _, err := scheme.ObjectKinds(object)
		if err != nil {
			return nil, err
		}
		accessor, err := meta.TypeAccessor(object)
		if err != nil {
			return nil, err
		}
		apiVersion, kind := gvks[0].ToAPIVersionAndKind()
		accessor.SetAPIVersion(apiVersion)
		accessor.SetKind(kind)
		result = append(result, runtime.RawExtension{Object: object})
	}
	return result, nil
}
