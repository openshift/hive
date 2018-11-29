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

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// Important: Run "make" to regenerate code after modifying this file

const (
	// FinalizerDeprovision is used on ClusterDeployments to ensure we run a successful deprovision
	// job before cleaning up the API object.
	FinalizerDeprovision string = "hive.openshift.io/deprovision"
)

// ClusterDeploymentSpec defines the desired state of ClusterDeployment
type ClusterDeploymentSpec struct {

	// ClusterUUID is a unique identifier for this cluster. Will be generated if none is provided.
	// TODO: omitempty for now so we don't have to specify when creating.
	ClusterUUID string `json:"clusterUUID,omitempty"`

	// Config contains the desired configuration for the cluster.
	Config InstallConfig `json:"config"`

	// PlatformSecrets contains credentials and secrets for the cluster infrastructure.
	PlatformSecrets PlatformSecrets `json:"platformSecrets"`

	// Images allows overriding the default images used to provision and manage the cluster.
	// +optional
	Images ProvisionImages `json:"images"`
}

// ProvisionImages allows overriding the default images used to provision a cluster.
type ProvisionImages struct {
	// InstallerImage is the image containing the openshift-install binary that will be used to install.
	InstallerImage string `json:"installerImage"`
	// InstallerImagePullPolicy is the pull policy for the installer image.
	InstallerImagePullPolicy corev1.PullPolicy `json:"installerImagePullPolicy"`
	// HiveImage is the image used in the sidecar container to manage execution of openshift-install.
	HiveImage string `json:"hiveImage"`
	// HiveImagePullPolicy is the pull policy for the installer image.
	HiveImagePullPolicy corev1.PullPolicy `json:"hiveImagePullPolicy"`
}

// PlatformSecrets defines the secrets to be used by various clouds.
type PlatformSecrets struct {
	// +optional
	AWS *AWSPlatformSecrets `json:"aws,omitempty"`
}

// AWSPlatformSecrets contains secrets for clusters on the AWS platform.
type AWSPlatformSecrets struct {
	// SSH refers to a secret that contains the ssh private key to access
	// EC2 instances in this cluster.
	//SSH corev1.LocalObjectReference `json:"ssh"`

	// Credentials refers to a secret that contains the AWS account access
	// credentials.
	Credentials corev1.LocalObjectReference `json:"credentials"`
}

// ClusterDeploymentStatus defines the observed state of ClusterDeployment
type ClusterDeploymentStatus struct {

	// Installed is true if the installer job has successfully completed for this cluster.
	Installed bool `json:"installed"`

	// AdminKubeconfigSecret references the secret containing the admin kubeconfig for this cluster.
	AdminKubeconfigSecret corev1.LocalObjectReference `json:"adminKubeconfigSecret"`

	// ClusterVersionStatus will hold a copy of the remote cluster's ClusterVersion.Status
	ClusterVersionStatus openshiftapiv1.ClusterVersionStatus `json:"clusterVersionStatus"`

	// APIURL is the URL where the cluster's API can be accessed.
	APIURL string `json:"apiURL"`

	// WebConsoleURL is the URL for the cluster's web console UI.
	WebConsoleURL string `json:"webConsoleURL"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterDeployment is the Schema for the clusterdeployments API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type ClusterDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterDeploymentSpec   `json:"spec,omitempty"`
	Status ClusterDeploymentStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterDeploymentList contains a list of ClusterDeployment
type ClusterDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterDeployment{}, &ClusterDeploymentList{})
}
