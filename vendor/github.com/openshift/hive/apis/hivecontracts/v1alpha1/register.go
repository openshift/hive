// NOTE: Boilerplate only.  Ignore this file.

// Package v1alpha1 contains API Schema definitions for the hivecontracts v1alpha1 API group
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen=package
// +groupName=hivecontracts.openshift.io
package v1alpha1

import (
	"github.com/openshift/hive/apis/scheme"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// HiveContractsAPIGroup is the group that all hiveinternal objects belong to in the API server.
	HiveContractsAPIGroup = "hivecontracts.openshift.io"

	// HiveContractsAPIVersion is the api version that all hiveinternal objects are currently at.
	HiveContractsAPIVersion = "v1alpha1"

	// SchemeGroupVersion is group version used to register these objects
	SchemeGroupVersion = schema.GroupVersion{Group: HiveContractsAPIGroup, Version: HiveContractsAPIVersion}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme
	SchemeBuilder = &scheme.Builder{GroupVersion: SchemeGroupVersion}

	// AddToScheme is a shortcut for SchemeBuilder.AddToScheme
	AddToScheme = SchemeBuilder.AddToScheme
)

// Resource takes an unqualified resource and returns a Group qualified GroupResource
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}
