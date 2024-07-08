// NOTE: Boilerplate only.  Ignore this file.

// Package v1alpha1 contains API Schema definitions for the hiveinternal v1alpha1 API group
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen=package
// +groupName=hiveinternal.openshift.io
package v1alpha1

import (
	"github.com/openshift/hive/apis/scheme"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// HiveInternalAPIGroup is the group that all hiveinternal objects belong to in the API server.
	HiveInternalAPIGroup = "hiveinternal.openshift.io"

	// HiveInternalAPIVersion is the api version that all hiveinternal objects are currently at.
	HiveInternalAPIVersion = "v1alpha1"

	// SchemeGroupVersion is group version used to register these objects
	SchemeGroupVersion = schema.GroupVersion{Group: HiveInternalAPIGroup, Version: HiveInternalAPIVersion}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = &scheme.Builder{GroupVersion: SchemeGroupVersion}

	// AddToScheme is a shortcut for SchemeBuilder.AddToScheme
	AddToScheme = SchemeBuilder.AddToScheme
)

// Resource takes an unqualified resource and returns a Group qualified GroupResource
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}
