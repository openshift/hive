package machinepoolresource

import (
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
)

const (
	// DefaultAWSInstanceType is the default AWS instance type for MachinePool (CLI and fill default).
	DefaultAWSInstanceType = "m6a.xlarge"
	defaultRootVolumeSize  = 120
	defaultRootVolumeType  = "gp3"
)

// AWSOptions holds all AWS MachinePool.Spec.Platform.AWS fields.
// All MachinePool platform fields are defined in this package.
type AWSOptions struct {
	Zones                      []string
	Subnets                    []string
	InstanceType               string
	WorkerInstanceType         string
	RootVolumeSize             int
	RootVolumeType             string
	RootVolumeIOPS             int
	RootVolumeKMSKeyARN        string
	SpotMarketOptionsMaxPrice  *string
	EC2MetadataAuthentication  string
	AdditionalSecurityGroupIDs []string
	UserTags                   map[string]string
}

// FillPlatform sets mp.Spec.Platform.AWS from o.
func (o *AWSOptions) FillPlatform(mp *hivev1.MachinePool) {
	if o == nil {
		return
	}
	instanceType := o.InstanceType
	if o.WorkerInstanceType != "" {
		instanceType = o.WorkerInstanceType
	}
	if instanceType == "" {
		instanceType = DefaultAWSInstanceType
	}
	size := o.RootVolumeSize
	if size <= 0 {
		size = defaultRootVolumeSize
	}
	volType := o.RootVolumeType
	if volType == "" {
		volType = defaultRootVolumeType
	}
	rootVol := hivev1aws.EC2RootVolume{
		Size: size,
		Type: volType,
	}
	if o.RootVolumeIOPS > 0 {
		rootVol.IOPS = o.RootVolumeIOPS
	}
	if o.RootVolumeKMSKeyARN != "" {
		rootVol.KMSKeyARN = o.RootVolumeKMSKeyARN
	}

	plat := &hivev1aws.MachinePoolPlatform{
		InstanceType:               instanceType,
		EC2RootVolume:              rootVol,
		Zones:                      o.Zones,
		Subnets:                    o.Subnets,
		UserTags:                   o.UserTags,
		AdditionalSecurityGroupIDs: o.AdditionalSecurityGroupIDs,
	}
	if o.SpotMarketOptionsMaxPrice != nil {
		plat.SpotMarketOptions = &hivev1aws.SpotMarketOptions{MaxPrice: o.SpotMarketOptionsMaxPrice}
	}
	if o.EC2MetadataAuthentication != "" {
		plat.EC2Metadata = &hivev1aws.EC2Metadata{Authentication: o.EC2MetadataAuthentication}
	}
	mp.Spec.Platform.AWS = plat
}

var _ PlatformFiller = (*AWSOptions)(nil)
