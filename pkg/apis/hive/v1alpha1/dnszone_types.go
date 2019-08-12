package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// FinalizerDNSZone is used on DNSZones to ensure we successfully deprovision
	// the cloud objects before cleaning up the API object.
	FinalizerDNSZone string = "hive.openshift.io/dnszone"
)

// DNSZoneSpec defines the desired state of DNSZone
type DNSZoneSpec struct {
	// Zone is the DNS zone to host
	Zone string `json:"zone"`

	// LinkToParentDomain specifies whether DNS records should
	// be automatically created to link this DNSZone with a
	// parent domain.
	// +optional
	LinkToParentDomain bool `json:"linkToParentDomain,omitempty"`

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

	// AdditionalTags is a set of additional tags to set on the DNS hosted zone. In addition
	// to these tags,the DNS Zone controller will set a hive.openhsift.io/hostedzone tag
	// identifying the HostedZone record that it belongs to.
	AdditionalTags []AWSResourceTag `json:"additionalTags,omitempty"`
}

// AWSResourceTag represents a tag that is applied to an AWS cloud resource
type AWSResourceTag struct {
	// Key is the key for the tag
	Key string `json:"key"`
	// Value is the value for the tag
	Value string `json:"value"`
}

// DNSZoneStatus defines the observed state of DNSZone
type DNSZoneStatus struct {
	// LastSyncTimestamp is the time that the zone was last sync'd.
	// +optional
	LastSyncTimestamp *metav1.Time `json:"lastSyncTimestamp,omitempty"`

	// LastSyncGeneration is the generation of the zone resource that was last sync'd. This is used to know
	// if the Object has changed and we should sync immediately.
	LastSyncGeneration int64 `json:"lastSyncGeneration"`

	// NameServers is a list of nameservers for this DNS zone
	// +optional
	NameServers []string `json:"nameServers,omitempty"`

	// AWSDNSZoneStatus contains status information specific to AWS
	// +optional
	AWS *AWSDNSZoneStatus `json:"aws,omitempty"`

	// Conditions includes more detailed status for the DNSZone
	// +optional
	Conditions []DNSZoneCondition `json:"conditions,omitempty"`
}

// AWSDNSZoneStatus contains status information specific to AWS DNS zones
type AWSDNSZoneStatus struct {
	// ZoneID is the ID of the zone in AWS
	// +optional
	ZoneID *string `json:"zoneID,omitempty"`
}

// DNSZoneCondition contains details for the current condition of a DNSZone
type DNSZoneCondition struct {
	// Type is the type of the condition.
	Type DNSZoneConditionType `json:"type"`
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

// DNSZoneConditionType is a valid value for DNSZoneCondition.Type
type DNSZoneConditionType string

const (
	// ZoneAvailableDNSZoneCondition is true if the DNSZone is responding to DNS queries
	ZoneAvailableDNSZoneCondition DNSZoneConditionType = "ZoneAvailable"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DNSZone is the Schema for the dnszones API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
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
