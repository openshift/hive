package nutanix

import (
	machinev1 "github.com/openshift/api/machine/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// MachinePool stores the configuration for a machine pool installed
// on Nutanix.
type MachinePool struct {
	// NumCPUs is the total number of virtual processor cores to assign a vm.
	//
	// +optional
	NumCPUs int64 `json:"cpus,omitempty"`

	// NumCoresPerSocket is the number of cores per socket in a vm. The number
	// of vCPUs on the vm will be NumCPUs times NumCoresPerSocket.
	// For example: 4 CPUs and 4 Cores per socket will result in 16 VPUs.
	// The AHV scheduler treats socket and core allocation exactly the same
	// so there is no benefit to configuring cores over CPUs.
	//
	// +optional
	NumCoresPerSocket int64 `json:"coresPerSocket,omitempty"`

	// Memory is the size of a VM's memory in MiB.
	//
	// +optional
	MemoryMiB int64 `json:"memoryMiB,omitempty"`

	// OSDisk defines the storage for instance.
	//
	// +optional
	OSDisk `json:"osDisk,omitempty"`

	// BootType indicates the boot type (Legacy, UEFI or SecureBoot) the Machine's VM uses to boot.
	// If this field is empty or omitted, the VM will use the default boot type "Legacy" to boot.
	// "SecureBoot" depends on "UEFI" boot, i.e., enabling "SecureBoot" means that "UEFI" boot is also enabled.
	// +kubebuilder:validation:Enum="";Legacy;UEFI;SecureBoot
	// +optional
	BootType machinev1.NutanixBootType `json:"bootType,omitempty"`

	// Project optionally identifies a Prism project for the Machine's VM to associate with.
	// +optional
	Project *machinev1.NutanixResourceIdentifier `json:"project,omitempty"`

	// Categories optionally adds one or more prism categories (each with key and value) for
	// the Machine's VM to associate with. All the category key and value pairs specified must
	// already exist in the prism central.
	// +listType=map
	// +listMapKey=key
	// +optional
	Categories []machinev1.NutanixCategory `json:"categories,omitempty"`

	// GPUs is a list of GPU devices to attach to the machine's VM.
	// +listType=set
	// +optional
	GPUs []machinev1.NutanixGPU `json:"gpus"`

	// DataDisks holds information of the data disks to attach to the Machine's VM
	// +listType=set
	// +optional
	DataDisks []DataDisk `json:"dataDisks"`

	// FailureDomains optionally configures a list of failure domain names
	// that will be applied to the MachinePool
	// +listType=set
	// +optional
	FailureDomains []string `json:"failureDomains,omitempty"`
}

// OSDisk defines the system disk for a Machine VM.
type OSDisk struct {
	// DiskSizeGiB defines the size of disk in GiB.
	//
	// +optional
	DiskSizeGiB int64 `json:"diskSizeGiB,omitempty"`
}

// StorageConfig specifies the storage configuration parameters for VM disks.
type StorageConfig struct {
	// diskMode specifies the disk mode.
	// The valid values are Standard and Flash, and the default is Standard.
	// +kubebuilder:default=Standard
	// +kubebuilder:validation:Enum=Standard;Flash
	DiskMode machinev1.NutanixDiskMode `json:"diskMode"`

	// storageContainer refers to the storage_container used by the VM disk.
	// +optional
	StorageContainer *StorageResourceReference `json:"storageContainer,omitempty"`
}

// DataDisk defines a data disk for a Machine VM.
type DataDisk struct {
	// diskSize is size (in Quantity format) of the disk to attach to the VM.
	// See https://pkg.go.dev/k8s.io/apimachinery/pkg/api/resource#Format for the Quantity format and example documentation.
	// The minimum diskSize is 1GB.
	// +kubebuilder:validation:Required
	DiskSize resource.Quantity `json:"diskSize"`

	// deviceProperties are the properties of the disk device.
	// +optional
	DeviceProperties *machinev1.NutanixVMDiskDeviceProperties `json:"deviceProperties,omitempty"`

	// storageConfig are the storage configuration parameters of the VM disks.
	// +optional
	StorageConfig *StorageConfig `json:"storageConfig,omitempty"`

	// dataSource refers to a data source image for the VM disk.
	// +optional
	DataSourceImage *StorageResourceReference `json:"dataSourceImage,omitempty"`
}
