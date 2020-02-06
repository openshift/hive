/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// A SyncSetInstance represents a single instance of a SyncSet or SelectorSyncSet
// as it applies to a particular ClusterDeployment. Results of the apply operation
// are stored in the status of the SyncSetInstance.
// A SyncSetInstance must contain a reference to either a SyncSet or SelectorSyncSet
// but not both.

const (
	// FinalizerSyncSetInstance is used on SyncSetInstances to ensure we remove
	// resources corresponnding to Sync mode syncset resources.
	FinalizerSyncSetInstance string = "hive.openshift.io/syncsetinstance"
)

// SyncSetInstanceSpec defines the desired state of SyncSetInstance
type SyncSetInstanceSpec struct {
	// ClusterDeployment is a reference to to the clusterdeployment for this syncsetinstance.
	ClusterDeploymentRef corev1.LocalObjectReference `json:"clusterDeploymentRef"`

	// SyncSet is a reference to the syncset for this syncsetinstance.
	// +optional
	SyncSetRef *corev1.LocalObjectReference `json:"syncSetRef,omitempty"`

	// SelectorSyncSetRef is a reference to the selectorsyncset for this syncsetinstance.
	// +optional
	SelectorSyncSetRef *SelectorSyncSetReference `json:"selectorSyncSetRef,omitempty"`

	// ResourceApplyMode indicates if the resource apply mode is "upsert" (default) or "sync".
	// ApplyMode "upsert" indicates create and update.
	// ApplyMode "sync" indicates create, update and delete.
	// +optional
	ResourceApplyMode SyncSetResourceApplyMode `json:"resourceApplyMode,omitempty"`

	// SyncSetHash is a hash of the contents of the syncset or selectorsyncset spec.
	// Its purpose is to cause a syncset instance update whenever there's a change in its
	// source.
	SyncSetHash string `json:"syncSetHash,omitempty"`
}

// SelectorSyncSetReference is a reference to a SelectorSyncSet
type SelectorSyncSetReference struct {
	// Name is the name of the SelectorSyncSet
	Name string `json:"name"`
}

// SyncSetInstanceStatus defines the observed state of SyncSetInstance
type SyncSetInstanceStatus struct {
	// Resources is the list of SyncStatus for objects that have been synced.
	// +optional
	Resources []SyncStatus `json:"resources,omitempty"`

	// Patches is the list of SyncStatus for patches that have been applied.
	// +optional
	Patches []SyncStatus `json:"patches,omitempty"`

	// Secrets is the list of SyncStatus for secrets that have been synced.
	// +optional
	Secrets []SyncStatus `json:"secretReferences,omitempty"`

	// Conditions is the list of SyncConditions used to indicate UnknownObject
	// when a resource type cannot be determined from a SyncSet resource.
	// +optional
	Conditions []SyncCondition `json:"conditions,omitempty"`

	// Applied will be true if all resources, patches, or secrets have successfully been applied on last attempt.
	Applied bool `json:"applied"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SyncSetInstance is the Schema for the syncsetinstances API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=syncsetinstances,shortName=ssi
type SyncSetInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SyncSetInstanceSpec   `json:"spec,omitempty"`
	Status SyncSetInstanceStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SyncSetInstanceList contains a list of SyncSetInstance
type SyncSetInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SyncSetInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SyncSetInstance{}, &SyncSetInstanceList{})
}
