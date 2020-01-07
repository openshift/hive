package hive

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
)

const (
	// GroupName is the name of the Hive API group
	GroupName = "hive.openshift.io"
)

var (
	schemeBuilder = runtime.NewSchemeBuilder(
		addKnownTypes,
		hivev1.AddToScheme,
	)
	// Install installs the internal Hive API
	Install = schemeBuilder.AddToScheme

	// SchemeGroupVersion kept for generated code
	// DEPRECATED
	SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: runtime.APIVersionInternal}
	// AddToScheme kept for generated code
	// DEPRECATED
	AddToScheme = schemeBuilder.AddToScheme
)

// Resource kept for generated code
// DEPRECATED
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

// TODO: Add more types to the internal API scheme

// Adds the list of known types to api.Scheme.
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
	return nil
}
