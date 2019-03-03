/*
Copyright 2018 The Kubernetes Authors.

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

package v1alpha1

import (
	openshiftapiv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SyncIdentityProviderCommonSpec defines the identity providers to sync
type SyncIdentityProviderCommonSpec struct {
	//IdentityProviders is an ordered list of ways for a user to identify themselves
	IdentityProviders []openshiftapiv1.IdentityProvider `json:"identityProviders"`
}

// SelectorSyncIdentityProviderSpec defines the SyncIdentityProviderCommonSpec to sync to
// ClusterDeploymentSelector indicating which clusters the SelectorSyncIdentityProvider applies
// to in any namespace.
type SelectorSyncIdentityProviderSpec struct {
	SyncIdentityProviderCommonSpec `json:",inline"`

	// ClusterDeploymentSelector is a LabelSelector indicating which clusters the SelectorIdentityProvider
	// applies to in any namespace.
	// +optional
	ClusterDeploymentSelector metav1.LabelSelector `json:"clusterDeploymentSelector,omitempty"`
}

// SyncIdentityProviderSpec defines the SyncIdentityProviderCommonSpec identity providers to sync along with
// ClusterDeploymentRefs indicating which clusters the SyncIdentityProvider applies to in the
// SyncIdentityProvider's namespace.
type SyncIdentityProviderSpec struct {
	SyncIdentityProviderCommonSpec `json:",inline"`

	// ClusterDeploymentRefs is the list of LocalObjectReference indicating which clusters the
	// SyncSet applies to in the SyncSet's namespace.
	ClusterDeploymentRefs []corev1.LocalObjectReference `json:"clusterDeploymentRefs"`
}

// IdentityProviderStatus defines the observed state of SyncSet
type IdentityProviderStatus struct {
}

// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SelectorSyncIdentityProvider is the Schema for the SelectorSyncSet API
// +k8s:openapi-gen=true
type SelectorSyncIdentityProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SelectorSyncIdentityProviderSpec `json:"spec,omitempty"`
	Status IdentityProviderStatus           `json:"status,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SyncIdentityProvider is the Schema for the SyncIdentityProvider API
// +k8s:openapi-gen=true
type SyncIdentityProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SyncIdentityProviderSpec `json:"spec,omitempty"`
	Status IdentityProviderStatus   `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SelectorSyncIdentityProviderList contains a list of SelectorSyncIdentityProviders
type SelectorSyncIdentityProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SelectorSyncIdentityProvider `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SyncIdentityProviderList contains a list of SyncIdentityProviders
type SyncIdentityProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SyncIdentityProvider `json:"items"`
}

func init() {
	SchemeBuilder.Register(
		&SyncIdentityProvider{},
		&SyncIdentityProviderList{},
		&SelectorSyncIdentityProvider{},
		&SelectorSyncIdentityProviderList{},
	)
}
