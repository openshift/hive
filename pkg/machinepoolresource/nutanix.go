package machinepoolresource

import (
	machinev1 "github.com/openshift/api/machine/v1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1nutanix "github.com/openshift/hive/apis/hive/v1/nutanix"
)

const (
	defaultNutanixNumCPUs           = 4
	defaultNutanixNumCoresPerSocket = 2
	defaultNutanixMemoryMiB         = 16348
	defaultNutanixOSDiskSizeGiB     = 120
)

// NutanixOptions holds Nutanix MachinePool.Spec.Platform.Nutanix fields (subset used by CLI).
type NutanixOptions struct {
	NumCPUs           int64
	NumCoresPerSocket int64
	MemoryMiB         int64
	OSDiskSizeGiB     int64
	BootType          string // Legacy, UEFI, SecureBoot
	FailureDomains    []string
}

// FillPlatform sets mp.Spec.Platform.Nutanix from o.
func (o *NutanixOptions) FillPlatform(mp *hivev1.MachinePool) {
	if o == nil {
		return
	}
	cpus := o.NumCPUs
	if cpus <= 0 {
		cpus = defaultNutanixNumCPUs
	}
	coresPerSocket := o.NumCoresPerSocket
	if coresPerSocket <= 0 {
		coresPerSocket = defaultNutanixNumCoresPerSocket
	}
	mem := o.MemoryMiB
	if mem <= 0 {
		mem = defaultNutanixMemoryMiB
	}
	diskSize := o.OSDiskSizeGiB
	if diskSize <= 0 {
		diskSize = defaultNutanixOSDiskSizeGiB
	}
	bootType := parseNutanixBootType(o.BootType)
	mp.Spec.Platform.Nutanix = &hivev1nutanix.MachinePool{
		NumCPUs:           cpus,
		NumCoresPerSocket: coresPerSocket,
		MemoryMiB:         mem,
		OSDisk:            hivev1nutanix.OSDisk{DiskSizeGiB: diskSize},
		BootType:          bootType,
		FailureDomains:    o.FailureDomains,
	}
}

func parseNutanixBootType(s string) machinev1.NutanixBootType {
	switch s {
	case "UEFI":
		return machinev1.NutanixUEFIBoot
	case "SecureBoot":
		return machinev1.NutanixSecureBoot
	case "Legacy", "":
		return machinev1.NutanixLegacyBoot
	default:
		return machinev1.NutanixLegacyBoot
	}
}

var _ PlatformFiller = (*NutanixOptions)(nil)
