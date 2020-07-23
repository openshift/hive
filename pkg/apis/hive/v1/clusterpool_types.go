package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterPoolSpec defines the desired state of the ClusterPool.
type ClusterPoolSpec struct {

	// Platform encompasses the desired platform for the cluster.
	// +required
	Platform Platform `json:"platform"`

	// PullSecretRef is the reference to the secret to use when pulling images.
	// +optional
	PullSecretRef *corev1.LocalObjectReference `json:"pullSecretRef,omitempty"`

	// Size is the default number of clusters that we should keep provisioned and waiting for use.
	// +kubebuilder:validation:Minimum=0
	// +required
	Size int32 `json:"size"`

	// BaseDomain is the base domain to use for all clusters created in this pool.
	// +required
	BaseDomain string `json:"baseDomain"`

	// ImageSetRef is a reference to a ClusterImageSet. The release image specified in the ClusterImageSet will be used
	// by clusters created for this cluster pool.
	ImageSetRef ClusterImageSetReference `json:"imageSetRef"`
}

// ClusterPoolStatus defines the observed state of ClusterPool
type ClusterPoolStatus struct {
	// Size is the number of unclaimed clusters that have been created for the pool.
	Size int32 `json:"size"`

	// Ready is the number of unclaimed clusters that have been installed and are ready to be claimed.
	Ready int32 `json:"ready"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterPool represents a pool of clusters that should be kept ready to be given out to users. Clusters are removed
// from the pool once claimed and then automatically replaced with a new one.
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.size,statuspath=.status.size
// +kubebuilder:resource:path=clusterpools,shortName=cp
type ClusterPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterPoolSpec   `json:"spec"`
	Status ClusterPoolStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterPoolList contains a list of ClusterPools
type ClusterPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterPool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterPool{}, &ClusterPoolList{})
}
