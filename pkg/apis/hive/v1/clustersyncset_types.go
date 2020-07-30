package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// A ClusterSyncSet is all of the SelectorSyncSets and SyncSets that apply to a ClusterDeployment.

// ClusterSyncSetSpec defines the desired state of ClusterSyncSet
type ClusterSyncSetSpec struct {
	// ClusterDeploymentName is the name of the ClusterDeployment for this ClusterSyncSet.
	ClusterDeploymentName string `json:"clusterDeploymentName"`
}

// ClusterSyncSetStatus defines the observed state of ClusterSyncSet
type ClusterSyncSetStatus struct {
	// +optional
	SyncSets []MatchedSyncSetStatus `json:"syncSets,omitempty"`

	// +optional
	SelectorSyncSets []MatchedSyncSetStatus `json:"selectorSyncSets,omitempty"`

	LastFullApplyTime metav1.Time `json:"lastFullApplyTime"`

	// +optional
	Conditions []ClusterSyncSetCondition `json:"conditions,omitempty"`
}

type MatchedSyncSetStatus struct {
	Name string `json:"name"`

	ObservedGeneration int64 `json:"observedGeneration"`

	// +optional
	ResourcesToDelete []ResourceIdentification `json:"resourcesToDelete,omitempty"`

	Result SyncSetResult `json:"result"`
}

type ResourceIdentification struct {
	// APIVersion is the Group and Version of the resource.
	// This is omitted for secret mappings.
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`

	// Kind is the Kind of the resource
	// This is omitted for secret mappings.
	// +optional
	Kind string `json:"kind,omitempty"`

	// Name is the name of the resource.
	Name string `json:"name"`

	// Namespace is the namespace of the resource.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

type SyncSetResult string

const (
	SuccessSyncSetResult SyncSetResult = "Success"

	FailureSyncSetResult SyncSetResult = "Failure"
)

// ClusterSyncSetCondition contains details for the current condition of a ClusterSyncSet
type ClusterSyncSetCondition struct {
	// Type is the type of the condition.
	Type ClusterSyncSetConditionType `json:"type"`
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

// ClusterSyncSetConditionType is a valid value for ClusterSyncSetCondition.Type
type ClusterSyncSetConditionType string

const (
	ClusterSyncSetFailed ClusterSyncSetConditionType = "Failed"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterSyncSet is the Schema for the ClusterSyncSets API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=clustersyncsets,scope=Namespaced
type ClusterSyncSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterSyncSetSpec   `json:"spec,omitempty"`
	Status ClusterSyncSetStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterSyncSetList contains a list of ClusterSyncSet
type ClusterSyncSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterSyncSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterSyncSet{}, &ClusterSyncSetList{})
}
