package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterSyncLease is a record of the last time that SyncSets and SelectorSyncSets were applied to a cluster.
// +k8s:openapi-gen=true
// +kubebuilder:resource:path=clustersyncleases,shortName=csl,scope=Namespaced
type ClusterSyncLease struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ClusterSyncLeaseSpec `json:"spec,omitempty"`
}

// ClusterSyncLeaseSpec is the specification of a ClusterSyncLease.
type ClusterSyncLeaseSpec struct {
	// RenewTime is the time when SyncSets and SelectorSyncSets were last applied to the cluster.
	RenewTime metav1.MicroTime `json:"renewTime"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterSyncLeaseList contains a list of ClusterSyncLeases.
type ClusterSyncLeaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterSyncLease `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterSyncLease{}, &ClusterSyncLeaseList{})
}
