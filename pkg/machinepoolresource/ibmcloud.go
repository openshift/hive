package machinepoolresource

import (
	"strings"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1ibmcloud "github.com/openshift/hive/apis/hive/v1/ibmcloud"
)

const (
	// DefaultIBMCloudInstanceType is the default IBM Cloud VSI profile for MachinePool (e.g. worker).
	DefaultIBMCloudInstanceType = "bx2-4x16"
)

// IBMCloudOptions holds all IBM Cloud MachinePool.Spec.Platform.IBMCloud fields.
type IBMCloudOptions struct {
	InstanceType   string
	Zones          []string
	BootVolumeKey  string // BootVolume.EncryptionKey CRN
	DedicatedHosts []hivev1ibmcloud.DedicatedHost
}

// FillPlatform sets mp.Spec.Platform.IBMCloud from o.
func (o *IBMCloudOptions) FillPlatform(mp *hivev1.MachinePool) {
	if o == nil {
		return
	}
	instanceType := o.InstanceType
	if instanceType == "" {
		instanceType = DefaultIBMCloudInstanceType
	}
	plat := &hivev1ibmcloud.MachinePool{
		InstanceType:   instanceType,
		Zones:          o.Zones,
		DedicatedHosts: o.DedicatedHosts,
	}
	if o.BootVolumeKey != "" {
		plat.BootVolume = &hivev1ibmcloud.BootVolume{EncryptionKey: o.BootVolumeKey}
	}
	mp.Spec.Platform.IBMCloud = plat
}

var _ PlatformFiller = (*IBMCloudOptions)(nil)

// ParseIBMCloudDedicatedHosts converts "name=xxx,profile=yyy" strings into DedicatedHost slice.
// Malformed entries are skipped.
func ParseIBMCloudDedicatedHosts(ss []string) []hivev1ibmcloud.DedicatedHost {
	if len(ss) == 0 {
		return nil
	}
	out := make([]hivev1ibmcloud.DedicatedHost, 0, len(ss))
	for _, s := range ss {
		var name, profile string
		for _, part := range strings.Split(s, ",") {
			kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
			if len(kv) != 2 {
				continue
			}
			k, v := strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])
			switch k {
			case "name":
				name = v
			case "profile":
				profile = v
			}
		}
		out = append(out, hivev1ibmcloud.DedicatedHost{Name: name, Profile: profile})
	}
	return out
}
