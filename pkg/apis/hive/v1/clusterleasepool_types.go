package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	//"github.com/openshift/hive/pkg/apis/hive/v1/aws"
)

// ClusterLeasePoolSpec defines the desired state of the ClusterLeasePool
type ClusterLeasePoolSpec struct {
	// Platform encompasses the desired platform for the cluster.
	Platform Platform `json:"platform"`

	// DesiredHotClusterCount is the default number of clusters that we should keep provisioned and waiting for use
	// if we're not in any of the DesiredHostClusterWindows.
	DesiredHotClusterCount int `json:"desiredHotClusterCount"`

	// TODO implement windows of time

	// DeleteAfter is a duration of time after the ClusterDeployment's creationTimestamp when we should delete the
	// cluster. Stored as an annotation on the ClusterDeployment and maybe adjusted and thus overridden by users who
	// obtain a lease to the cluster.
	// +optional
	DeleteAfter *metav1.Time `json:"deleteAfter,omitempty"`
}

// ClusterLeasePoolStatus defines the observed state of ClusterLeasePool
type ClusterLeasePoolStatus struct {
	// TODO: current number of clusters? installing, ready
}

// +genclient:nonNamespaced
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterLeasePool represents a pool of clusters that should be kept ready to be leased out to users.
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=clusterleasepool,shortName=clp
type ClusterLeasePool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterLeasePoolSpec   `json:"spec"`
	Status ClusterLeasePoolStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterLeasePoolList contains a list of ClusterLeasePools
type ClusterLeasePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterLeasePool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterLeasePool{}, &ClusterLeasePoolList{})
}
