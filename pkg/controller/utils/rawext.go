/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
