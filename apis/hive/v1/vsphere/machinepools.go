package vsphere

import "github.com/openshift/installer/pkg/types/vsphere"

// MachinePool stores the configuration for a machine pool installed
// on vSphere.
type MachinePool struct {
	vsphere.MachinePool `json:",inline"`

	// ResourcePool is the name of the resource pool that will be used for virtual machines.
	// If it is not present, a default value will be used.
	// Deprecated: use Topology instead
	// +optional
	DeprecatedResourcePool string `json:"resourcePool,omitempty"`

	// TagIDs is a list of up to 10 tags to add to the VMs that this machine set provisions in vSphere.
	// Deprecated: use Topology instead
	// +kubebuilder:validation:MaxItems:=10
	DeprecatedTagIDs []string `json:"tagIDs,omitempty"`

	// Topology is the vSphere topology that will be used for virtual machines.
	// If it is not present, a default value will be used.
	// +optional
	Topology *vsphere.Topology `json:"topology,omitempty"`
}
