package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterDeprovisionRequestSpec defines the desired state of ClusterDeprovisionRequest
type ClusterDeprovisionRequestSpec struct {
	// InfraID is the identifier generated during installation for a cluster. It is used for tagging/naming resources in cloud providers.
	InfraID string `json:"infraID"`

	// ClusterID is a globally unique identifier for the cluster to deprovision. It will be used if specified.
	ClusterID string `json:"clusterID,omitempty"`

	// Platform contains platform-specific configuration for a ClusterDeprovisionRequest
	Platform ClusterDeprovisionRequestPlatform `json:"platform,omitempty"`
}

// ClusterDeprovisionRequestStatus defines the observed state of ClusterDeprovisionRequest
type ClusterDeprovisionRequestStatus struct {
	// Completed is true when the uninstall has completed successfully
	Completed bool `json:"completed,omitempty"`
}

// ClusterDeprovisionRequestPlatform contains platform-specific configuration for the
// deprovision request
type ClusterDeprovisionRequestPlatform struct {
	// AWS contains AWS-specific deprovision request settings
	AWS *AWSClusterDeprovisionRequest `json:"aws,omitempty"`
	// Azure contains Azure-specific deprovision request settings
	Azure *AzureClusterDeprovisionRequest `json:"azure,omitempty"`
	// GCP contains GCP-specific deprovision request settings
	GCP *GCPClusterDeprovisionRequest `json:"gcp,omitempty"`
}

// AWSClusterDeprovisionRequest contains AWS-specific configuration for a ClusterDeprovisionRequest
type AWSClusterDeprovisionRequest struct {
	// Region is the AWS region for this deprovisioning request
	Region string `json:"region"`

	// Credentials is the AWS account credentials to use for deprovisioning the cluster
	Credentials *corev1.LocalObjectReference `json:"credentials,omitempty"`
}

// AzureClusterDeprovisionRequest contains Azure-specific configuration for a ClusterDeprovisionRequest
type AzureClusterDeprovisionRequest struct {
	// Credentials is the Azure account credentials to use for deprovisioning the cluster
	Credentials *corev1.LocalObjectReference `json:"credentials,omitempty"`
}

// GCPClusterDeprovisionRequest contains GCP-specific configuration for a ClusterDeprovisionRequest
type GCPClusterDeprovisionRequest struct {
	// Region is the GCP region for this deprovisioning request
	Region string `json:"region"`
	// ProjectID is the ID of the GCP project in which the cluster exists
	ProjectID string `json:"projectID"`
	// Credentials is the GCP account credentials to use for deprovisioning the cluster
	Credentials *corev1.LocalObjectReference `json:"credentials,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterDeprovisionRequest is the Schema for the clusterdeprovisionrequests API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="InfraID",type="string",JSONPath=".spec.infraID"
// +kubebuilder:printcolumn:name="ClusterID",type="string",JSONPath=".spec.clusterID"
// +kubebuilder:printcolumn:name="Completed",type="boolean",JSONPath=".status.completed"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:path=clusterdeprovisionrequests,shortName=cdr
type ClusterDeprovisionRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterDeprovisionRequestSpec   `json:"spec,omitempty"`
	Status ClusterDeprovisionRequestStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterDeprovisionRequestList contains a list of ClusterDeprovisionRequest
type ClusterDeprovisionRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterDeprovisionRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterDeprovisionRequest{}, &ClusterDeprovisionRequestList{})
}
