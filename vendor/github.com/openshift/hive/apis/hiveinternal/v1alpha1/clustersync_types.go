package v1alpha1

import (
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterSync is the status of all of the SelectorSyncSets and SyncSets that apply to a ClusterDeployment.
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=clustersyncs,shortName=csync,scope=Namespaced
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[0].reason`
// +kubebuilder:printcolumn:name="ControllerReplica",type=string,JSONPath=`.status.controlledByReplica`
// +kubebuilder:printcolumn:name="Message",type=string,priority=1,JSONPath=`.status.conditions[?(@.type=="Failed")].message`
type ClusterSync struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterSyncSpec   `json:"spec,omitempty"`
	Status ClusterSyncStatus `json:"status,omitempty"`
}

// ClusterSyncSpec defines the desired state of ClusterSync
type ClusterSyncSpec struct{}

// ClusterSyncStatus defines the observed state of ClusterSync
type ClusterSyncStatus struct {
	// SyncSets is the sync status of all of the SyncSets for the cluster.
	// +optional
	SyncSets []SyncStatus `json:"syncSets,omitempty"`

	// SelectorSyncSets is the sync status of all of the SelectorSyncSets for the cluster.
	// +optional
	SelectorSyncSets []SyncStatus `json:"selectorSyncSets,omitempty"`

	// Conditions is a list of conditions associated with syncing to the cluster.
	// +optional
	Conditions []ClusterSyncCondition `json:"conditions,omitempty"`

	// FirstSuccessTime is the time we first successfully applied all (selector)syncsets to a cluster.
	// +optional
	FirstSuccessTime *metav1.Time `json:"firstSuccessTime,omitempty"`

	// ControlledByReplica indicates which replica of the hive-clustersync StatefulSet is responsible
	// for (the CD related to) this clustersync. Note that this value indicates the replica that most
	// recently handled the ClusterSync. If the hive-clustersync statefulset is scaled up or down, the
	// controlling replica can change, potentially causing logs to be spread across multiple pods.
	ControlledByReplica *int64 `json:"controlledByReplica,omitempty"`
}

// SyncStatus is the status of applying a specific SyncSet or SelectorSyncSet to the cluster.
type SyncStatus struct {
	// Name is the name of the SyncSet or SelectorSyncSet.
	Name string `json:"name"`

	// ObservedGeneration is the generation of the SyncSet or SelectorSyncSet that was last observed.
	ObservedGeneration int64 `json:"observedGeneration"`

	// ResourcesToDelete is the list of resources in the cluster that should be deleted when the SyncSet or SelectorSyncSet
	// is deleted or is no longer matched to the cluster.
	// +optional
	ResourcesToDelete []SyncResourceReference `json:"resourcesToDelete,omitempty"`

	// Result is the result of the last attempt to apply the SyncSet or SelectorSyncSet to the cluster.
	Result SyncSetResult `json:"result"`

	// FailureMessage is a message describing why the SyncSet or SelectorSyncSet could not be applied. This is only
	// set when Result is Failure.
	// +optional
	FailureMessage string `json:"failureMessage,omitempty"`

	// LastTransitionTime is the time when this status last changed.
	LastTransitionTime metav1.Time `json:"lastTransitionTime"`

	// FirstSuccessTime is the time when the SyncSet or SelectorSyncSet was first successfully applied to the cluster.
	// +optional
	FirstSuccessTime *metav1.Time `json:"firstSuccessTime,omitempty"`
}

// SyncResourceReference is a reference to a resource that is synced to a cluster via a SyncSet or SelectorSyncSet.
type SyncResourceReference struct {
	// APIVersion is the Group and Version of the resource.
	APIVersion string `json:"apiVersion"`

	// Kind is the Kind of the resource.
	// +optional
	Kind string `json:"kind"`

	// Name is the name of the resource.
	Name string `json:"name"`

	// Namespace is the namespace of the resource.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// SyncSetResult is the result of a sync attempt.
// +kubebuilder:validation:Enum=Success;Failure
type SyncSetResult string

const (
	// SuccessSyncSetResult is the result when the SyncSet or SelectorSyncSet was applied successfully to the cluster.
	SuccessSyncSetResult SyncSetResult = "Success"

	// FailureSyncSetResult is the result when there was an error when attempting to apply the SyncSet or SelectorSyncSet
	// to the cluster
	FailureSyncSetResult SyncSetResult = "Failure"
)

// ClusterSyncCondition contains details for the current condition of a ClusterSync
type ClusterSyncCondition struct {
	// Type is the type of the condition.
	Type ClusterSyncConditionType `json:"type"`
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
	// Message is a human-readable message indicating details about the last transition.
	// +optional
	Message string `json:"message,omitempty"`
}

// ClusterSyncConditionType is a valid value for ClusterSyncCondition.Type
type ClusterSyncConditionType string

// ConditionType satisfies the generics.Condition interface
func (c ClusterSyncCondition) ConditionType() hivev1.ConditionType {
	return c.Type
}

// String satisfies the generics.ConditionType interface
func (t ClusterSyncConditionType) String() string {
	return string(t)
}

const (
	// ClusterSyncFailed is the type of condition used to indicate whether there are SyncSets or SelectorSyncSets which
	// have not been applied due to an error.
	ClusterSyncFailed ClusterSyncConditionType = "Failed"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterSyncList contains a list of ClusterSync
type ClusterSyncList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterSync `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterSync{}, &ClusterSyncList{})
}
