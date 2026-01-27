package machinepoolresource

import (
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1vsphere "github.com/openshift/hive/apis/hive/v1/vsphere"
)

const (
	defaultVSphereNumCPUs           = 2
	defaultVSphereNumCoresPerSocket = 1
	defaultVSphereMemoryMiB         = 8192
	defaultVSphereOSDiskSizeGB      = 120
	defaultVSphereTagIDsMaxItems    = 10
)

// VSphereOptions holds all vSphere MachinePool.Spec.Platform.VSphere fields.
type VSphereOptions struct {
	ResourcePool      string
	NumCPUs           int32
	NumCoresPerSocket int32
	MemoryMiB         int64
	OSDiskSizeGB      int32
	TagIDs            []string
}

// FillPlatform sets mp.Spec.Platform.VSphere from o.
func (o *VSphereOptions) FillPlatform(mp *hivev1.MachinePool) {
	if o == nil {
		return
	}
	cpus := o.NumCPUs
	if cpus <= 0 {
		cpus = defaultVSphereNumCPUs
	}
	coresPerSocket := o.NumCoresPerSocket
	if coresPerSocket <= 0 {
		coresPerSocket = defaultVSphereNumCoresPerSocket
	}
	mem := o.MemoryMiB
	if mem <= 0 {
		mem = defaultVSphereMemoryMiB
	}
	diskSize := o.OSDiskSizeGB
	if diskSize <= 0 {
		diskSize = defaultVSphereOSDiskSizeGB
	}
	tagIDs := o.TagIDs
	if len(tagIDs) > defaultVSphereTagIDsMaxItems {
		tagIDs = tagIDs[:defaultVSphereTagIDsMaxItems]
	}
	mp.Spec.Platform.VSphere = &hivev1vsphere.MachinePool{
		ResourcePool:      o.ResourcePool,
		NumCPUs:           cpus,
		NumCoresPerSocket: coresPerSocket,
		MemoryMiB:         mem,
		OSDisk:            hivev1vsphere.OSDisk{DiskSizeGB: diskSize},
		TagIDs:            tagIDs,
	}
}

var _ PlatformFiller = (*VSphereOptions)(nil)
