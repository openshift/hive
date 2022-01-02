package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterDeploymentCustomization is the Schema for clusterdeploymentcustomizations API
// +kubebuilder:subresource:status
// +k8s:openapi-gen=true
// +kubebuilder:resource:scope=Namespaced
type ClusterDeploymentCustomization struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterDeploymentCustomizationSpec   `json:"spec"`
	Status ClusterDeploymentCustomizationStatus `json:"status,omitempty"`
}

// ClusterDeploymentCustomizationSpec defines the desired state of ClusterDeploymentCustomization
type ClusterDeploymentCustomizationSpec struct {
	// TODO: documentation
	InstallConfigPatches []PatchEntity `json:"installConfigPatches,omitempty"`
}

// TODO: documentation
type PatchEntity struct {
	// +required
	Op string `json:"op"`
	// +required
	Path string `json:"path"`
	// +required
	Value string `json:"value"`
}

// ClusterDeploymentCustomizationStatus defines the observed state of ClusterDeploymentCustomization
type ClusterDeploymentCustomizationStatus struct {
	// TODO: documentation
	// +optional
	ClusterDeploymentRef *corev1.ObjectReference `json:"clusterDeploymentRef,omitempty"`

	// +optional
	LastApplyTime metav1.Time `json:"lastApplyTime,omitempty"`

	// +optional
	LastApplyStatus string `json:"lastApplyStatus,omitempty"`

	// Conditions includes more detailed status for the cluster deployment customization status.
	// +optional
	Conditions []ClusterDeploymentCustomizationCondition `json:"conditions,omitempty"`
}

type ClusterDeploymentCustomizationCondition struct {
	// Type is the type of the condition.
	Type ClusterDeploymentCustomizationConditionType `json:"type"`
	// Status is the status of the condition.
	Status corev1.ConditionStatus `json:"status"`
	// LastProbeTime is the last time we probed the condition.
	// +optional
	LastProbeTime metav1.Time `json:"lastProbeTime,omitempty"`
	// LastTransitionTime is the last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// Reason is a unique, one-word, CamelCase reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`
	// Message is a human-readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty"`
}

// ClusterDeploymentCustomizationConditionType is a valid value for ClusterDeploymentCustomizationCondition.Type
type ClusterDeploymentCustomizationConditionType string

const (
	// TODO: add more types
	// TODO: shorter name?
	ClusterDeploymentCustomizationAvailableCondition ClusterDeploymentCustomizationConditionType = "Available"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterDeploymentCustomizationLis contains the list of ClusterDeploymentCustomization
type ClusterDeploymentCustomizationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []ClusterDeploymentCustomization `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterDeploymentCustomization{}, &ClusterDeploymentCustomizationList{})
}
