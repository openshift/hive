package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// SyncSetResourceApplyMode is a string representing the mode with which to
// apply SyncSet Resources.
type SyncSetResourceApplyMode string

const (
	// UpsertResourceApplyMode indicates that objects will be updated
	// or inserted (created).
	UpsertResourceApplyMode SyncSetResourceApplyMode = "Upsert"

	// SyncResourceApplyMode inherits the create or update functionality
	// of Upsert but also indicates that objects will be deleted if created
	// previously and detected missing from defined Resources in the SyncSet.
	SyncResourceApplyMode SyncSetResourceApplyMode = "Sync"
)

// SyncSetPatchApplyMode is a string representing the mode with which to apply
// SyncSet Patches.
type SyncSetPatchApplyMode string

const (
	// ApplyOncePatchApplyMode indicates that the patch should be applied
	// only once.
	ApplyOncePatchApplyMode SyncSetPatchApplyMode = "ApplyOnce"

	// AlwaysApplyPatchApplyMode indicates that the patch should be
	// continuously applied.
	AlwaysApplyPatchApplyMode SyncSetPatchApplyMode = "AlwaysApply"
)

// SyncObjectPatch represents a patch to be applied to a specific object
type SyncObjectPatch struct {
	// APIVersion is the Group and Version of the object to be patched.
	APIVersion string `json:"apiVersion"`

	// Kind is the Kind of the object to be patched.
	Kind string `json:"kind"`

	// Name is the name of the object to be patched.
	Name string `json:"name"`

	// Namespace is the Namespace in which the object to patch exists.
	// Defaults to the SyncSet's Namespace.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Patch is the patch to apply.
	Patch string `json:"patch"`

	// PatchType indicates the PatchType as "strategic" (default), "json", or "merge".
	// +optional
	PatchType string `json:"patchType,omitempty"`
}

// SecretReference represents a reference to an existing secret object to be synced
type SecretReference struct {
	Source corev1.ObjectReference `json:"source"`
	Target corev1.ObjectReference `json:"target"`
}

// SyncConditionType is a valid value for SyncCondition.Type
type SyncConditionType string

const (
	// ApplySuccessSyncCondition indicates whether the resource or patch has been applied.
	ApplySuccessSyncCondition SyncConditionType = "ApplySuccess"

	// ApplyFailureSyncCondition indicates that a resource or patch has failed to apply.
	// It should include a reason and message for the failure.
	ApplyFailureSyncCondition SyncConditionType = "ApplyFailure"

	// DeletionFailedSyncCondition indicates that resource deletion has failed.
	// It should include a reason and message for the failure.
	DeletionFailedSyncCondition SyncConditionType = "DeletionFailed"

	// UnknownObjectSyncCondition indicates that the resource type cannot be determined.
	// It should include a reason and message for the failure.
	UnknownObjectSyncCondition SyncConditionType = "UnknownObject"
)

// SyncCondition is a condition in a SyncStatus
type SyncCondition struct {
	// Type is the type of the condition.
	Type SyncConditionType `json:"type"`
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

// SyncSetObjectStatus describes the status of resources created or patches that have
// been applied from a SyncSet or SelectorSyncSet.
type SyncSetObjectStatus struct {
	// Name is the name of the SyncSet.
	Name string `json:"name"`

	// Resources is the list of SyncStatus for objects that have been synced.
	// +optional
	Resources []SyncStatus `json:"resources,omitempty"`

	// ResourceApplyMode indicates if the Resource apply mode is "upsert" (default) or "sync".
	// ApplyMode "upsert" indicates create and update.
	// ApplyMode "sync" indicates create, update and delete.
	// +optional
	ResourceApplyMode SyncSetResourceApplyMode `json:"resourceApplyMode,omitempty"`

	// Patches is the list of SyncStatus for patches that have been applied.
	// +optional
	Patches []SyncStatus `json:"patches,omitempty"`

	// SecretReferences is the list of SyncStatus for secrets that have been synced.
	// +optional
	SecretReferences []SyncStatus `json:"secretReferences,omitempty"`

	// Conditions is the list of SyncConditions used to indicate UnknownObject
	// when a resource type cannot be determined from a SyncSet resource.
	// +optional
	Conditions []SyncCondition `json:"conditions,omitempty"`
}

// SyncStatus describes objects that have been created or patches that
// have been applied using the unique md5 sum of the object or patch.
type SyncStatus struct {
	// APIVersion is the Group and Version of the object that was synced or
	// patched.
	APIVersion string `json:"apiVersion"`

	// Kind is the Kind of the object that was synced or patched.
	Kind string `json:"kind"`

	// Resource is the resource name for the object that was synced.
	// This will be populated for resources, but not patches
	// +optional
	Resource string `json:"resource,omitempty"`

	// Name is the name of the object that was synced or patched.
	Name string `json:"name"`

	// Namespace is the Namespace of the object that was synced or patched.
	Namespace string `json:"namespace"`

	// Hash is the unique md5 hash of the resource or patch.
	Hash string `json:"hash"`

	// Conditions is the list of conditions indicating success or failure of object
	// create, update and delete as well as patch application.
	Conditions []SyncCondition `json:"conditions"`
}

// SyncSetCommonSpec defines the resources and patches to sync
type SyncSetCommonSpec struct {
	// Resources is the list of objects to sync from RawExtension definitions.
	// +optional
	Resources []runtime.RawExtension `json:"resources,omitempty"`

	// ResourceApplyMode indicates if the Resource apply mode is "upsert" (default) or "sync".
	// ApplyMode "upsert" indicates create and update.
	// ApplyMode "sync" indicates create, update and delete.
	// +optional
	ResourceApplyMode SyncSetResourceApplyMode `json:"resourceApplyMode,omitempty"`

	// Patches is the list of patches to apply.
	// +optional
	Patches []SyncObjectPatch `json:"patches,omitempty"`

	// SecretReferences is the list of secrets to sync from existing resources.
	// +optional
	SecretReferences []SecretReference `json:"secretReferences,omitempty"`
}

// SelectorSyncSetSpec defines the SyncSetCommonSpec resources and patches to sync along
// with a ClusterDeploymentSelector indicating which clusters the SelectorSyncSet applies
// to in any namespace.
type SelectorSyncSetSpec struct {
	SyncSetCommonSpec `json:",inline"`

	// ClusterDeploymentSelector is a LabelSelector indicating which clusters the SelectorSyncSet
	// applies to in any namespace.
	// +optional
	ClusterDeploymentSelector metav1.LabelSelector `json:"clusterDeploymentSelector,omitempty"`
}

// SyncSetSpec defines the SyncSetCommonSpec resources and patches to sync along with
// ClusterDeploymentRefs indicating which clusters the SyncSet applies to in the
// SyncSet's namespace.
type SyncSetSpec struct {
	SyncSetCommonSpec `json:",inline"`

	// ClusterDeploymentRefs is the list of LocalObjectReference indicating which clusters the
	// SyncSet applies to in the SyncSet's namespace.
	// +required
	ClusterDeploymentRefs []corev1.LocalObjectReference `json:"clusterDeploymentRefs"`
}

// SyncSetStatus defines the observed state of a SyncSet
type SyncSetStatus struct {
}

// SelectorSyncSetStatus defines the observed state of a SelectorSyncSet
type SelectorSyncSetStatus struct {
}

// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SelectorSyncSet is the Schema for the SelectorSyncSet API
// +k8s:openapi-gen=true
// +kubebuilder:resource:path=selectorsyncsets,shortName=sss
type SelectorSyncSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SelectorSyncSetSpec   `json:"spec,omitempty"`
	Status SelectorSyncSetStatus `json:"status,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SyncSet is the Schema for the SyncSet API
// +k8s:openapi-gen=true
// +kubebuilder:resource:path=syncsets,shortName=ss
type SyncSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SyncSetSpec   `json:"spec,omitempty"`
	Status SyncSetStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SelectorSyncSetList contains a list of SyncSets
type SelectorSyncSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SelectorSyncSet `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SyncSetList contains a list of SyncSets
type SyncSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SyncSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(
		&SyncSet{},
		&SyncSetList{},
		&SelectorSyncSet{},
		&SelectorSyncSetList{},
	)
}
