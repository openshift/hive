package v1alpha1

import (
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FakeClusterInstallSpec defines the desired state of the FakeClusterInstall.
type FakeClusterInstallSpec struct {

	// ImageSetRef is a reference to a ClusterImageSet. The release image specified in the ClusterImageSet will be used
	// to install the cluster.
	ImageSetRef hivev1.ClusterImageSetReference `json:"imageSetRef"`

	// ClusterDeploymentRef is a reference to the ClusterDeployment associated with this AgentClusterInstall.
	ClusterDeploymentRef corev1.LocalObjectReference `json:"clusterDeploymentRef"`

	// ClusterMetadata contains metadata information about the installed cluster. It should be populated once the cluster install is completed. (it can be populated sooner if desired, but Hive will not copy back to ClusterDeployment until the Installed condition goes True.
	ClusterMetadata *hivev1.ClusterMetadata `json:"clusterMetadata,omitempty"`
}

// FakeClusterInstallStatus defines the observed state of the FakeClusterInstall.
type FakeClusterInstallStatus struct {
	// Conditions includes more detailed status for the cluster install.
	// +optional
	Conditions []hivev1.ClusterInstallCondition `json:"conditions,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// FakeClusterInstall represents a fake request to provision an agent based cluster.
//
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type FakeClusterInstall struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FakeClusterInstallSpec   `json:"spec"`
	Status FakeClusterInstallStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// FakeClusterInstallList contains a list of FakeClusterInstall
type FakeClusterInstallList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FakeClusterInstall `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FakeClusterInstall{}, &FakeClusterInstallList{})
}
