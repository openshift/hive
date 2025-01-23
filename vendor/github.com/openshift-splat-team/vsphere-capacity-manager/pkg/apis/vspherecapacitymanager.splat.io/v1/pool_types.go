package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	POOLS_LAST_LEASE_UPDATE_ANNOTATION = "vspherecapacitymanager.splat.io/last-pool-update"
	PoolFinalizer                      = "vsphere-capacity-manager.splat-team.io/pool-finalizer"
	PoolKind                           = "Pool"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Pool defines a pool of resources defined available for a given vCenter, cluster, and datacenter
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:scope=Namespaced
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="vCPUs",type=string,JSONPath=`.status.vcpus-available`
// +kubebuilder:printcolumn:name="Memory(GB)",type=string,JSONPath=`.status.memory-available`
// +kubebuilder:printcolumn:name="Networks",type=string,JSONPath=`.status.network-available`
// +kubebuilder:printcolumn:name="Disabled",type=string,JSONPath=`.spec.noSchedule`
// +kubebuilder:printcolumn:name="Excluded",type=string,JSONPath=`.spec.exclude`
type Pool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec PoolSpec `json:"spec"`
	// +optional
	Status PoolStatus `json:"status"`
}

type IBMPoolSpec struct {
	// Pod the pod in the datacenter where the vCenter resides
	Pod string `json:"pod"`
	// Pod the pod in the datacenter where the vCenter resides
	Datacenter string `json:"datacenter"`
}

// PoolSpec defines the specification for a pool
type PoolSpec struct {
	FailureDomainSpec `json:",inline"`
	// IBMPoolSpec topology information associated with this pool
	IBMPoolSpec IBMPoolSpec `json:"ibmPoolSpec,omitempty"`
	// VCpus is the number of virtual CPUs
	VCpus int `json:"vcpus"`
	// Memory is the amount of memory in GB
	Memory int `json:"memory"`
	// Storage is the amount of storage in GB
	Storage int `json:"storage"`
	// Exclude when true, this pool is excluded from the default pools.
	// This is useful if a job must be scheduled to a specific pool and that
	// pool only has limited capacity.
	Exclude bool `json:"exclude"`
	// NoSchedule when true, new leases for this pool will not be allocated.
	// any in progress leases will remain active until they are destroyed.
	// +optional
	NoSchedule bool `json:"noSchedule"`
}

// PoolStatus defines the status for a pool
type PoolStatus struct {
	// VCPUsAvailable is the number of vCPUs available in the pool
	// +optional
	VCpusAvailable int `json:"vcpus-available"`
	// MemoryAvailable is the amount of memory in GB available in the pool
	// +optional
	MemoryAvailable int `json:"memory-available"`
	// StorageAvailable is the amount of storage in GB available in the pool
	// +optional
	DatastoreAvailable int `json:"datastore-available"`
	// Networks is the number of networks available in the pool
	// +optional
	NetworkAvailable int `json:"network-available"`

	// Initialized when true, the status fields have been initialized
	// +optional
	Initialized bool `json:"initialized"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PoolList is a list of pools
type PoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Pool `json:"items"`
}

type Pools []*Pool
