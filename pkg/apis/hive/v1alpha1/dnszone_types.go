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

const (
	// FinalizerDNSZone is used on DNSZones to ensure we successfuly deprovision
	// the cloud objects before cleaning up the API object.
	FinalizerDNSZone string = "hive.openshift.io/dnszone"
)

// DNSZoneSpec defines the desired state of DNSZone
type DNSZoneSpec struct {
	// Zone is the DNS zoneto host
	Zone string `json:"zone"`

	// AWS specifies AWS-specific cloud configuration
	// +optional
	AWS *AWSDNSZoneSpec `json:"aws,omitempty"`
}

// AWSDNSZoneSpec contains AWS-specific DNSZone specifications
type AWSDNSZoneSpec struct {
	// AccountSecret contains a reference to a secret that contains AWS credentials
	// for CRUD operations
	AccountSecret corev1.LocalObjectReference `json:"accountSecret"`

	// Region specifies the region-specific API endpoint to use
	Region string `json:"region"`
}

// DNSZoneStatus defines the observed state of DNSZone
type DNSZoneStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// LastSyncTimestamp is the time that the zone was last sync'd.
	// +optional
	LastSyncTimestamp *metav1.Time `json:"lastSyncTimestamp,omitempty"`

	// LastSyncGeneration is the generation of the zone resource that was last sync'd. This is used to know
	// if the Object has changed and we should sync immediately.
	LastSyncGeneration int64 `json:"lastSyncGeneration"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DNSZone is the Schema for the dnszones API
// +k8s:openapi-gen=true
type DNSZone struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DNSZoneSpec   `json:"spec,omitempty"`
	Status DNSZoneStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DNSZoneList contains a list of DNSZone
type DNSZoneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DNSZone `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DNSZone{}, &DNSZoneList{})
}
