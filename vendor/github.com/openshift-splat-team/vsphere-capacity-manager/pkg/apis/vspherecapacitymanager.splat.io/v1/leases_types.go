package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NetworkType string

const (
	LeaseKind               = "Lease"
	APIGroupName            = "vsphere-capacity-manager.splat-team.io"
	LeaseFinalizer          = "vsphere-capacity-manager.splat-team.io/lease-finalizer"
	LeaseNamespace          = "vsphere-capacity-manager.splat-team.io/lease-namespace"
	NetworkTypeDisconnected = NetworkType("disconnected")
	NetworkTypeSingleTenant = NetworkType("single-tenant")
	NetworkTypeMultiTenant  = NetworkType("multi-tenant")
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Lease represents the definition of resources allocated for a resource pool
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:scope=Namespaced
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="vCPUs",type=string,JSONPath=`.spec.vcpus`
// +kubebuilder:printcolumn:name="Memory(GB)",type=string,JSONPath=`.spec.memory`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
type Lease struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec LeaseSpec `json:"spec"`
	// +optional
	Status LeaseStatus `json:"status"`
}

// LeaseSpec defines the specification for a lease
type LeaseSpec struct {
	// VCpus is the number of virtual CPUs allocated for this lease
	VCpus int `json:"vcpus,omitempty"`
	// Memory is the amount of memory in GB allocated for this lease
	Memory int `json:"memory,omitempty"`
	// Storage is the amount of storage in GB allocated for this lease
	// +optional
	Storage int `json:"storage,omitempty"`
	// Networks is the number of networks requested
	Networks int `json:"networks"`
	// RequiredPool when configured, this lease can only be fulfilled by a specific
	// pool
	// +optional
	RequiredPool string `json:"required-pool,omitempty"`

	// NetworkType defines the type of network required by the lease.
	// by default, all networks are treated as single-tenant. single-tenant networks
	// are only used by one CI jobs.  multi-tenant networks reside on a
	// VLAN which may be used by multiple jobs.  disconnected networks aren't yet
	// supported.
	// +kubebuilder:validation:Enum="";disconnected;single-tenant;multi-tenant;nested-multi-tenant;public-ipv6
	// +kubebuilder:default=single-tenant
	// +optional
	NetworkType NetworkType `json:"network-type"`

	// BoskosLeaseID is the ID of the lease in Boskos associated with this lease
	// +optional
	BoskosLeaseID string `json:"boskos-lease-id,omitempty"`
}

// LeaseStatus defines the status for a lease
type LeaseStatus struct {
	FailureDomainSpec `json:",inline"`

	// EnvVars a freeform string which contains bash which is to be sourced
	// by the holder of the lease.
	EnvVars string `json:"envVars,omitempty"`

	// Phase is the current phase of the lease
	// +optional
	Phase Phase `json:"phase,omitempty"`
}

type Leases []*Lease

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type LeaseList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Lease `json:"items"`
}
