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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HiveConfigSpec defines the desired state of Hive
type HiveConfigSpec struct {
	// ManagedDomains is the list of DNS domains that are managed by the Hive cluster
	// When specifying 'managedDNS: true' in a ClusterDeployment, the ClusterDeployment's
	// baseDomain should be a direct child of one of these domains, otherwise the
	// ClusterDeployment creation will result in a validation error.
	// +optional
	ManagedDomains []string `json:"managedDomains,omitempty"`

	// ExternalDNS specifies configuration for external-dns if it is to be deployed by
	// Hive. If absent, external-dns will not be deployed.
	// +optional
	ExternalDNS *ExternalDNSConfig `json:"externalDNS,omitempty"`
}

// HiveConfigStatus defines the observed state of Hive
type HiveConfigStatus struct {
	// AggregatorClientCAHash keeps an md5 hash of the aggregator client CA
	// configmap data from the openshift-config-managed namespace. When the configmap changes,
	// admission is redeployed.
	AggregatorClientCAHash string `json:"aggregatorClientCAHash,omitempty"`
}

// ExternalDNSConfig contains settings for running external-dns in a Hive
// environment.
type ExternalDNSConfig struct {

	// Image is a reference to the image that will run the external-dns controller.
	// If not specified, a default image will be used.
	Image string `json:"image,omitempty"`

	// AWS contains AWS-specific settings for external DNS
	// +optional
	AWS *ExternalDNSAWSConfig `json:"aws,omitempty"`

	// As other cloud providers are supported, additional fields will be
	// added for each of those cloud providers. Only a single cloud provider
	// may be configured at a time.
}

// ExternalDNSAWSConfig contains AWS-specific settings for external DNS
type ExternalDNSAWSConfig struct {
	// Credentials references a secret that will be used to authenticate with
	// AWS Route53. It will need permission to manage entries in each of the
	// managed domains for this cluster.
	// +optional
	Credentials corev1.LocalObjectReference `json:"credentials,omitempty"`
}

// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// HiveConfig is the Schema for the hives API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type HiveConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HiveConfigSpec   `json:"spec,omitempty"`
	Status HiveConfigStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// HiveConfigList contains a list of Hive
type HiveConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HiveConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HiveConfig{}, &HiveConfigList{})
}
