package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	//"github.com/openshift/hive/pkg/apis/hive/v1/aws"
)

// ClusterLeaseRequestSpec represents a request for a cluster we will attempt to fille with
// one of the clusters in the pool.
type ClusterLeaseRequestSpec struct {

	// ClusterLeasePoolRef is a reference the to cluster pool the requester would
	// like a cluster from.
	// +required
	ClusterLeasePoolRef corev1.LocalObjectReference `json:"clusterLeasePoolRef"`

	// User is the person requesting a cluster from the pool.
	// +required
	User string `json:"user"`

	// ClusterDeploymentRef will be set when a cluster has been allocated to fill this request.
	// This field cannot be set at creation time, it must be populated by the controllers.
	// +optional
	ClusterDeploymentRef *corev1.ObjectReference `json:"clusterDeploymentRef"`
}

// ClusterLeaseRequestStatus defines the observed state of ClusterLeaseRequest
type ClusterLeaseRequestStatus struct {
	// Conditions includes more detailed status for the cluster deployment
	// +optional
	Conditions []ClusterDeploymentCondition `json:"conditions,omitempty"`
}

// +genclient:nonNamespaced
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterLeaseRequest represents a users request to lease a cluster from the pool.
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=clusterleaserequests,shortName=clr
type ClusterLeaseRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterLeaseRequestSpec   `json:"spec"`
	Status ClusterLeaseRequestStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterLeaseRequestList contains a list of ClusterLeaseRequests
type ClusterLeaseRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterLeaseRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterLeaseRequest{}, &ClusterLeaseRequestList{})
}
