package nutanix

import (
	machinev1 "github.com/openshift/api/machine/v1"
)

// MachinePool stores the configuration for a machine pool installed
type MachinePool struct {

	// NumVcpusPerSocket is the total number of virtual processor cores (per socket) to assign a vm.
	NumVcpusPerSocket int64 `json:"vcpus"`

	// NumSockets is the number of sockets in a vm.
	// The number of vCPUs on the vm will be NumCPUs/NumSockets.
	NumSockets int64 `json:"sockets"`

	// ClusterUUID is the UUID of the Nutanix cluster to use for provisioning volumes.
	ClusterUUID string `json:"clusterUUID"`

	// MemorySizeMiB is the size of a VM's memory in MiB.
	MemorySizeMiB int64 `json:"memorySizeMiB"`

	// BootType Indicates whether the VM should use Secure boot, UEFI boot or Legacy boot.
	BootType machinev1.NutanixBootType `json:"bootType,omitempty"`

	// Disks is the list of disks attached to the VM.
	Disks []Disk `json:"disks"`

	// NicList is the list of subnets attached to the VM.
	NicList []string `json:"subnetsUUIDs"`
}

type Disk struct {
	// DiskSizeBytes defines the size of disk in bytes.
	DiskSizeBytes int64 `json:"diskSizeBytes"`

	// DeviceType defines the type of disk.
	DeviceType machinev1.NutanixDiskDeviceType `json:"deviceType"`

	// AdapterType
	AdapterType machinev1.NutanixDiskAdapterType `json:"adapterType"`
}

//type BootType string

//const (
//	// VMs Disk Types
//	DiskDeviceTypeCDROM DiskDeviceType = "CDROM"
//	DiskDeviceTypeDISK  DiskDeviceType = "DISK"
//
//	// VMs Boot Types
//	NutanixBootTypeUEFI       BootType = "UEFI"
//	NutanixBootTypeLegacy     BootType = "LEGACY"
//	NutanixBootTypeSecureBoot BootType = "SECURE_BOOT"
//)
