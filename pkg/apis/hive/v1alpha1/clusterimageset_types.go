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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterImageSetSpec defines the desired state of ClusterImageSet
type ClusterImageSetSpec struct {
	// HiveImage is the Hive image to use when installing or destroying a cluster.
	// If not present, the default Hive image for the clusterdeployment controller
	// is used.
	// +optional
	HiveImage *string `json:"hiveImage,omitempty"`

	// ReleaseImage is the image that contains the payload to use when installing
	// a cluster. If the installer image is specified, the release image
	// is optional.
	// +optional
	ReleaseImage *string `json:"releaseImage,omitempty"`

	// InstallerImage is the image used to install a cluster. If not specified,
	// the installer image reference is obtained from the release image.
	// +optional
	InstallerImage *string `json:"installerImage,omitempty"`
}

// ClusterImageSetStatus defines the observed state of ClusterImageSet
type ClusterImageSetStatus struct{}

// +genclient:nonNamespaced
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterImageSet is the Schema for the clusterimagesets API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Hive",type="string",JSONPath=".spec.hiveImage"
// +kubebuilder:printcolumn:name="Installer",type="string",JSONPath=".status.installerImage"
// +kubebuilder:printcolumn:name="Release",type="string",JSONPath=".spec.releaseImage"
// +kubebuilder:resource:path=clusterimagesets,shortName=imgset
type ClusterImageSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterImageSetSpec   `json:"spec,omitempty"`
	Status ClusterImageSetStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterImageSetList contains a list of ClusterImageSet
type ClusterImageSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterImageSet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterImageSet{}, &ClusterImageSetList{})
}
