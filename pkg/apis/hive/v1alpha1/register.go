// NOTE: Boilerplate only.  Ignore this file.

// Package v1alpha1 contains API Schema definitions for the hive v1alpha1 API group
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen=package,register
// +k8s:conversion-gen=github.com/openshift/hive/pkg/apis/hive
// +k8s:defaulter-gen=TypeMeta
// +groupName=hive.openshift.io
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupName is the group name use in this package
const GroupName = "hive.openshift.io"

// SchemeGroupVersion is group version used to register these objects
var SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: "v1alpha1"}

// Resource takes an unqualified resource and returns a Group qualified GroupResource
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

var (
	// SchemeBuilder is used to build a scheme for this API
	SchemeBuilder      = runtime.NewSchemeBuilder(addKnownTypes)
	localSchemeBuilder = &SchemeBuilder
	// AddToScheme is used to add this API to a scheme
	AddToScheme = localSchemeBuilder.AddToScheme
)

// Adds the list of known types to the given scheme.
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&Checkpoint{}, &CheckpointList{},
		&ClusterDeployment{}, &ClusterDeploymentList{},
		&ClusterDeprovisionRequest{}, &ClusterDeprovisionRequestList{},
		&ClusterImageSet{}, &ClusterImageSetList{},
		&ClusterProvision{}, &ClusterProvisionList{},
		&ClusterState{}, &ClusterStateList{},
		&DNSZone{}, &DNSZoneList{},
		&HiveConfig{}, &HiveConfigList{},
		&SyncIdentityProvider{}, &SyncIdentityProviderList{},
		&SelectorSyncIdentityProvider{}, &SelectorSyncIdentityProviderList{},
		&SyncSet{}, &SyncSetList{},
		&SelectorSyncSet{}, &SelectorSyncSetList{},
		&SyncSetInstance{}, &SyncSetInstanceList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}
