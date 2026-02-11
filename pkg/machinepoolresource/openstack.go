package machinepoolresource

import (
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1openstack "github.com/openshift/hive/apis/hive/v1/openstack"
)

const (
	// DefaultOpenStackFlavor is the default OpenStack Nova flavor for MachinePool (CLI default).
	DefaultOpenStackFlavor = "m1.medium"
)

// OpenStackOptions holds all OpenStack MachinePool.Spec.Platform.OpenStack fields.
type OpenStackOptions struct {
	Flavor                     string
	RootVolumeSize             int // GiB; 0 means no root volume (ephemeral)
	RootVolumeType             string
	AdditionalSecurityGroupIDs []string
}

// FillPlatform sets mp.Spec.Platform.OpenStack from o.
func (o *OpenStackOptions) FillPlatform(mp *hivev1.MachinePool) {
	if o == nil {
		return
	}
	flavor := o.Flavor
	if flavor == "" {
		flavor = DefaultOpenStackFlavor
	}
	plat := &hivev1openstack.MachinePool{
		Flavor:                     flavor,
		AdditionalSecurityGroupIDs: o.AdditionalSecurityGroupIDs,
	}
	if o.RootVolumeSize > 0 && o.RootVolumeType != "" {
		plat.RootVolume = &hivev1openstack.RootVolume{
			Size: o.RootVolumeSize,
			Type: o.RootVolumeType,
		}
	}
	mp.Spec.Platform.OpenStack = plat
}

var _ PlatformFiller = (*OpenStackOptions)(nil)
