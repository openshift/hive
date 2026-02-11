package machinepoolresource

import (
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1azure "github.com/openshift/hive/apis/hive/v1/azure"
)

const (
	// DefaultAzureInstanceType is the default Azure instance type for MachinePool.
	DefaultAzureInstanceType = "Standard_D4s_v3"
	defaultAzureDiskSizeGB   = 128
	defaultAzureDiskType     = "Premium_LRS"
)

// AzureOptions holds all Azure MachinePool.Spec.Platform.Azure fields.
type AzureOptions struct {
	Zones                    []string
	InstanceType             string
	OSDiskSizeGB             int
	OSDiskType               string
	DiskEncryptionSetSubID   string
	DiskEncryptionSetRG      string
	DiskEncryptionSetName    string
	OSImagePlan              string
	OSImagePublisher         string
	OSImageOffer             string
	OSImageSKU               string
	OSImageVersion           string
	NetworkResourceGroupName string
	ComputeSubnet            string
	VirtualNetwork           string
	VMNetworkingType         string
	OutboundType             string
}

// FillPlatform sets mp.Spec.Platform.Azure from o.
func (o *AzureOptions) FillPlatform(mp *hivev1.MachinePool) {
	if o == nil {
		return
	}
	instanceType := o.InstanceType
	if instanceType == "" {
		instanceType = DefaultAzureInstanceType
	}
	diskSize := o.OSDiskSizeGB
	if diskSize <= 0 {
		diskSize = defaultAzureDiskSizeGB
	}
	diskType := o.OSDiskType
	if diskType == "" {
		diskType = defaultAzureDiskType
	}
	osDisk := hivev1azure.OSDisk{
		DiskSizeGB: int32(diskSize),
		DiskType:   diskType,
	}
	if o.DiskEncryptionSetName != "" && o.DiskEncryptionSetRG != "" {
		osDisk.DiskEncryptionSet = &hivev1azure.DiskEncryptionSet{
			SubscriptionID: o.DiskEncryptionSetSubID,
			ResourceGroup:  o.DiskEncryptionSetRG,
			Name:           o.DiskEncryptionSetName,
		}
	}

	plat := &hivev1azure.MachinePool{
		Zones:                    o.Zones,
		InstanceType:             instanceType,
		OSDisk:                   osDisk,
		NetworkResourceGroupName: o.NetworkResourceGroupName,
		ComputeSubnet:            o.ComputeSubnet,
		VirtualNetwork:           o.VirtualNetwork,
		VMNetworkingType:         o.VMNetworkingType,
		OutboundType:             o.OutboundType,
	}
	if o.OSImagePublisher != "" && o.OSImageOffer != "" && o.OSImageSKU != "" && o.OSImageVersion != "" {
		plan := hivev1azure.ImageWithPurchasePlan
		if o.OSImagePlan == "NoPurchasePlan" {
			plan = hivev1azure.ImageNoPurchasePlan
		}
		plat.OSImage = &hivev1azure.OSImage{
			Plan:      plan,
			Publisher: o.OSImagePublisher,
			Offer:     o.OSImageOffer,
			SKU:       o.OSImageSKU,
			Version:   o.OSImageVersion,
		}
	}
	mp.Spec.Platform.Azure = plat
}

var _ PlatformFiller = (*AzureOptions)(nil)
