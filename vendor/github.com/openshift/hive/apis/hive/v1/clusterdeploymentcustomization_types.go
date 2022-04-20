package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LastApplyStatusType indicates the status of the customization on the last
// applied cluster deployment. This is needed to for inventory sorting process to
// avoid using same broken customization
type LastApplyStatusType string

const (
	// LastApplySucceeded indicates that the customization
	// worked properly on the last applied cluster deployment
	LastApplySucceeded LastApplyStatusType = "Succeeded"
	// LastApplyBrokenSyntax indicates that Hive failed to apply
	// customization patches on install-config. More detailes would be found in
	// Valid condition message.
	LastApplyBrokenSyntax LastApplyStatusType = "BrokenBySyntax"
	// LastApplyBrokenCloud indicates that cluser deployment provision has failed
	// when used this customization. More detailes would be found in the Valid condition message.
	LastApplyBrokenCloud LastApplyStatusType = "BrokenByCloud"
	// LastApplyInstallationPending indicates that the customization patches have
	// been successfully applied but provisioning is not completed yet.
	LastApplyInstallationPending LastApplyStatusType = "InstallationPending"
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
	// InstallConfigPatches is a list of patches to be applied to the install-config
	InstallConfigPatches []PatchEntity `json:"installConfigPatches,omitempty"`
}

// PatchEntity represent a json patch (RFC 6902) to be applied to the install-config
type PatchEntity struct {
	// Op is the operation to perform: add, remove, replace, move, copy, test
	// +required
	Op string `json:"op"`
	// Path is the json path to the value to be modified
	// +required
	Path string `json:"path"`
	// Value is the value to be used in the operation
	// +required
	Value string `json:"value"`
}

// ClusterDeploymentCustomizationStatus defines the observed state of ClusterDeploymentCustomization
type ClusterDeploymentCustomizationStatus struct {
	// ClusterDeploymentRef is a reference to the cluster deployment that this customization is applied on
	// +optional
	ClusterDeploymentRef *corev1.LocalObjectReference `json:"clusterDeploymentRef,omitempty"`

	// LastApplyTime indicates the time when the customization was applied on a cluster deployment
	// +optional
	LastApplyTime metav1.Time `json:"lastApplyTime,omitempty"`

	// LastApplyStatus indicates the customization status in the last applied cluster deployment
	// +optional
	LastApplyStatus LastApplyStatusType `json:"lastApplyStatus,omitempty"`

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
	ClusterDeploymentCustomizationAvailableCondition ClusterDeploymentCustomizationConditionType = "Available"
	ClusterDeploymentCustomizationValid              ClusterDeploymentCustomizationConditionType = "Valid"
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
