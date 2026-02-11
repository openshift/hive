package machinepool

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/pkg/errors"

	machinev1 "github.com/openshift/api/machine/v1"
	machinev1alpha1 "github.com/openshift/api/machine/v1alpha1"
	machineapi "github.com/openshift/api/machine/v1beta1"
	hivev1ibmcloud "github.com/openshift/hive/apis/hive/v1/ibmcloud"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/machinepoolresource"
	ibmcloudprovider "github.com/openshift/machine-api-provider-ibmcloud/pkg/apis/ibmcloudprovider/v1"
)

const (
	machineAPINamespace = "openshift-machine-api"
)

// ExtractResult holds the extracted platform config and total replicas from MachineSet(s).
type ExtractResult struct {
	Platform      string
	Filler        machinepoolresource.PlatformFiller
	TotalReplicas int64
}

// providerKind is used to decode only the "kind" field from providerSpec.Value.Raw.
type providerKind struct {
	Kind string `json:"kind"`
}

// ExtractFromMachineSets builds platform options and total replicas from the given MachineSets.
// All MachineSets must be for the same platform. Zones/subnets are merged from all sets.
func ExtractFromMachineSets(machineSets []*machineapi.MachineSet) (ExtractResult, error) {
	if len(machineSets) == 0 {
		return ExtractResult{}, fmt.Errorf("no MachineSets provided")
	}
	first := machineSets[0]
	if first.Spec.Template.Spec.ProviderSpec.Value == nil || len(first.Spec.Template.Spec.ProviderSpec.Value.Raw) == 0 {
		return ExtractResult{}, fmt.Errorf("MachineSet %s/%s has no providerSpec", first.Namespace, first.Name)
	}
	var kind providerKind
	if err := json.Unmarshal(first.Spec.Template.Spec.ProviderSpec.Value.Raw, &kind); err != nil {
		return ExtractResult{}, errors.Wrap(err, "decode providerSpec kind")
	}
	var totalReplicas int64
	for _, ms := range machineSets {
		if ms.Spec.Replicas != nil {
			totalReplicas += int64(*ms.Spec.Replicas)
		}
	}
	var filler machinepoolresource.PlatformFiller
	var platform string
	switch kind.Kind {
	case "AWSMachineProviderConfig":
		platform = constants.PlatformAWS
		opts, err := extractAWSOptions(machineSets)
		if err != nil {
			return ExtractResult{}, err
		}
		filler = opts
	case "AzureMachineProviderSpec":
		platform = constants.PlatformAzure
		opts, err := extractAzureOptions(machineSets)
		if err != nil {
			return ExtractResult{}, err
		}
		filler = opts
	case "GCPMachineProviderSpec":
		platform = constants.PlatformGCP
		opts, err := extractGCPOptions(machineSets)
		if err != nil {
			return ExtractResult{}, err
		}
		filler = opts
	case "VSphereMachineProviderSpec":
		platform = constants.PlatformVSphere
		opts, err := extractVSphereOptions(machineSets)
		if err != nil {
			return ExtractResult{}, err
		}
		filler = opts
	case "IBMCloudMachineProviderSpec":
		platform = constants.PlatformIBMCloud
		opts, err := extractIBMOptions(machineSets)
		if err != nil {
			return ExtractResult{}, err
		}
		filler = opts
	case "NutanixMachineProviderConfig":
		platform = constants.PlatformNutanix
		opts, err := extractNutanixOptions(machineSets)
		if err != nil {
			return ExtractResult{}, err
		}
		filler = opts
	case "OpenStackProviderSpec", "OpenstackProviderSpec":
		platform = constants.PlatformOpenStack
		opts, err := extractOpenStackOptions(machineSets)
		if err != nil {
			return ExtractResult{}, err
		}
		filler = opts
	default:
		return ExtractResult{}, fmt.Errorf("unsupported provider kind %q", kind.Kind)
	}
	return ExtractResult{Platform: platform, Filler: filler, TotalReplicas: totalReplicas}, nil
}

// zoneReplica holds zone and replicas for sorting
type zoneReplica struct {
	zone     string
	replicas int32
}

func extractAWSOptions(machineSets []*machineapi.MachineSet) (*machinepoolresource.AWSOptions, error) {
	first := machineSets[0]
	cfg := &machineapi.AWSMachineProviderConfig{}
	if err := json.Unmarshal(first.Spec.Template.Spec.ProviderSpec.Value.Raw, cfg); err != nil {
		return nil, errors.Wrap(err, "decode AWS providerSpec")
	}
	// Collect zone replicas to infer original order
	// Hive assigns remainder replicas to first zones, so higher replicas = earlier in order
	zoneMap := make(map[string]int32)
	subnetSet := make(map[string]struct{})
	for _, ms := range machineSets {
		c := &machineapi.AWSMachineProviderConfig{}
		if err := json.Unmarshal(ms.Spec.Template.Spec.ProviderSpec.Value.Raw, c); err != nil {
			continue
		}
		zone := c.Placement.AvailabilityZone
		if zone != "" {
			replicas := int32(0)
			if ms.Spec.Replicas != nil {
				replicas = *ms.Spec.Replicas
			}
			zoneMap[zone] += replicas
		}
		// Subnets order doesn't matter - Hive uses AWS API to map subnet→zone
		if c.Subnet.ID != nil && *c.Subnet.ID != "" {
			subnetSet[*c.Subnet.ID] = struct{}{}
		}
	}
	// Sort zones: replicas descending, zone name ascending (tie-breaker)
	zoneList := make([]zoneReplica, 0, len(zoneMap))
	for z, r := range zoneMap {
		zoneList = append(zoneList, zoneReplica{z, r})
	}
	sort.Slice(zoneList, func(i, j int) bool {
		if zoneList[i].replicas != zoneList[j].replicas {
			return zoneList[i].replicas > zoneList[j].replicas
		}
		return zoneList[i].zone < zoneList[j].zone
	})
	var zones []string
	for _, zr := range zoneList {
		zones = append(zones, zr.zone)
	}
	var subnets []string
	for s := range subnetSet {
		subnets = append(subnets, s)
	}
	rootSize, rootType, rootIOPS, kmsARN := 0, "", 0, ""
	for _, bd := range cfg.BlockDevices {
		if bd.EBS != nil && (bd.DeviceName == nil || *bd.DeviceName == "" || strings.HasSuffix(*bd.DeviceName, "xvda")) {
			if bd.EBS.VolumeSize != nil {
				rootSize = int(*bd.EBS.VolumeSize)
			}
			if bd.EBS.VolumeType != nil {
				rootType = *bd.EBS.VolumeType
			}
			if bd.EBS.Iops != nil {
				rootIOPS = int(*bd.EBS.Iops)
			}
			if bd.EBS.KMSKey.ARN != nil {
				kmsARN = *bd.EBS.KMSKey.ARN
			}
			break
		}
	}
	userTags := make(map[string]string)
	for _, t := range cfg.Tags {
		if t.Name != "" {
			userTags[t.Name] = t.Value
		}
	}
	var spotMaxPrice *string
	if cfg.SpotMarketOptions != nil && cfg.SpotMarketOptions.MaxPrice != nil {
		spotMaxPrice = cfg.SpotMarketOptions.MaxPrice
	}
	ec2Metadata := ""
	if cfg.MetadataServiceOptions.Authentication != "" {
		ec2Metadata = string(cfg.MetadataServiceOptions.Authentication)
	}
	var extraSGs []string
	for _, sg := range cfg.SecurityGroups {
		if sg.ID != nil && *sg.ID != "" {
			extraSGs = append(extraSGs, *sg.ID)
		}
	}
	return &machinepoolresource.AWSOptions{
		InstanceType:               cfg.InstanceType,
		Zones:                      zones,
		Subnets:                    subnets,
		RootVolumeSize:             rootSize,
		RootVolumeType:             rootType,
		RootVolumeIOPS:             rootIOPS,
		RootVolumeKMSKeyARN:        kmsARN,
		UserTags:                   userTags,
		SpotMarketOptionsMaxPrice:  spotMaxPrice,
		EC2MetadataAuthentication:  ec2Metadata,
		AdditionalSecurityGroupIDs: extraSGs,
	}, nil
}

func extractAzureOptions(machineSets []*machineapi.MachineSet) (*machinepoolresource.AzureOptions, error) {
	first := machineSets[0]
	cfg := &machineapi.AzureMachineProviderSpec{}
	if err := json.Unmarshal(first.Spec.Template.Spec.ProviderSpec.Value.Raw, cfg); err != nil {
		return nil, errors.Wrap(err, "decode Azure providerSpec")
	}
	// Collect zone info with replicas to infer original order
	zoneReplicas := make(map[string]int32)
	for _, ms := range machineSets {
		c := &machineapi.AzureMachineProviderSpec{}
		if err := json.Unmarshal(ms.Spec.Template.Spec.ProviderSpec.Value.Raw, c); err != nil {
			continue
		}
		if c.Zone == "" {
			continue
		}
		replicas := int32(0)
		if ms.Spec.Replicas != nil {
			replicas = *ms.Spec.Replicas
		}
		zoneReplicas[c.Zone] += replicas
	}
	// Sort: replicas descending, zone name ascending (tie-breaker)
	type zr struct {
		zone     string
		replicas int32
	}
	zrList := make([]zr, 0, len(zoneReplicas))
	for z, r := range zoneReplicas {
		zrList = append(zrList, zr{z, r})
	}
	sort.Slice(zrList, func(i, j int) bool {
		if zrList[i].replicas != zrList[j].replicas {
			return zrList[i].replicas > zrList[j].replicas
		}
		return zrList[i].zone < zrList[j].zone
	})
	zones := make([]string, 0, len(zrList))
	for _, z := range zrList {
		zones = append(zones, z.zone)
	}
	diskSize := int(cfg.OSDisk.DiskSizeGB)
	diskType := cfg.OSDisk.ManagedDisk.StorageAccountType
	return &machinepoolresource.AzureOptions{
		Zones:                    zones,
		InstanceType:             cfg.VMSize,
		OSDiskSizeGB:             diskSize,
		OSDiskType:               diskType,
		ComputeSubnet:            cfg.Subnet,
		VirtualNetwork:           cfg.Vnet,
		NetworkResourceGroupName: cfg.NetworkResourceGroup,
	}, nil
}

func extractGCPOptions(machineSets []*machineapi.MachineSet) (*machinepoolresource.GCPOptions, error) {
	first := machineSets[0]
	cfg := &machineapi.GCPMachineProviderSpec{}
	if err := json.Unmarshal(first.Spec.Template.Spec.ProviderSpec.Value.Raw, cfg); err != nil {
		return nil, errors.Wrap(err, "decode GCP providerSpec")
	}
	// Collect zone info with replicas to infer original order
	zoneReplicas := make(map[string]int32)
	for _, ms := range machineSets {
		c := &machineapi.GCPMachineProviderSpec{}
		if err := json.Unmarshal(ms.Spec.Template.Spec.ProviderSpec.Value.Raw, c); err != nil {
			continue
		}
		if c.Zone == "" {
			continue
		}
		replicas := int32(0)
		if ms.Spec.Replicas != nil {
			replicas = *ms.Spec.Replicas
		}
		zoneReplicas[c.Zone] += replicas
	}
	// Sort: replicas descending, zone name ascending (tie-breaker)
	type zr struct {
		zone     string
		replicas int32
	}
	zrList := make([]zr, 0, len(zoneReplicas))
	for z, r := range zoneReplicas {
		zrList = append(zrList, zr{z, r})
	}
	sort.Slice(zrList, func(i, j int) bool {
		if zrList[i].replicas != zrList[j].replicas {
			return zrList[i].replicas > zrList[j].replicas
		}
		return zrList[i].zone < zrList[j].zone
	})
	zones := make([]string, 0, len(zrList))
	for _, z := range zrList {
		zones = append(zones, z.zone)
	}
	var diskSize int64
	diskType := ""
	if len(cfg.Disks) > 0 {
		diskSize = cfg.Disks[0].SizeGB
		diskType = cfg.Disks[0].Type
	}
	return &machinepoolresource.GCPOptions{
		Zones:        zones,
		InstanceType: cfg.MachineType,
		OSDiskSizeGB: diskSize,
		OSDiskType:   diskType,
	}, nil
}

func extractVSphereOptions(machineSets []*machineapi.MachineSet) (*machinepoolresource.VSphereOptions, error) {
	first := machineSets[0]
	cfg := &machineapi.VSphereMachineProviderSpec{}
	if err := json.Unmarshal(first.Spec.Template.Spec.ProviderSpec.Value.Raw, cfg); err != nil {
		return nil, errors.Wrap(err, "decode vSphere providerSpec")
	}
	resPool := ""
	if cfg.Workspace != nil {
		resPool = cfg.Workspace.ResourcePool
	}
	return &machinepoolresource.VSphereOptions{
		ResourcePool:      resPool,
		NumCPUs:           cfg.NumCPUs,
		NumCoresPerSocket: cfg.NumCoresPerSocket,
		MemoryMiB:         cfg.MemoryMiB,
		OSDiskSizeGB:      cfg.DiskGiB,
	}, nil
}

func extractIBMOptions(machineSets []*machineapi.MachineSet) (*machinepoolresource.IBMCloudOptions, error) {
	first := machineSets[0]
	cfg := &ibmcloudprovider.IBMCloudMachineProviderSpec{}
	if err := json.Unmarshal(first.Spec.Template.Spec.ProviderSpec.Value.Raw, cfg); err != nil {
		return nil, errors.Wrap(err, "decode IBM Cloud providerSpec")
	}
	zoneReplicas := make(map[string]int32)
	dedicatedHostSet := make(map[string]struct{})
	for _, ms := range machineSets {
		c := &ibmcloudprovider.IBMCloudMachineProviderSpec{}
		if err := json.Unmarshal(ms.Spec.Template.Spec.ProviderSpec.Value.Raw, c); err != nil {
			continue
		}
		if c.Zone != "" {
			replicas := int32(0)
			if ms.Spec.Replicas != nil {
				replicas = *ms.Spec.Replicas
			}
			zoneReplicas[c.Zone] += replicas
		}
		if c.DedicatedHost != "" {
			dedicatedHostSet[c.DedicatedHost] = struct{}{}
		}
	}
	type zr struct {
		zone     string
		replicas int32
	}
	zrList := make([]zr, 0, len(zoneReplicas))
	for z, r := range zoneReplicas {
		zrList = append(zrList, zr{z, r})
	}
	sort.Slice(zrList, func(i, j int) bool {
		if zrList[i].replicas != zrList[j].replicas {
			return zrList[i].replicas > zrList[j].replicas
		}
		return zrList[i].zone < zrList[j].zone
	})
	zones := make([]string, 0, len(zrList))
	for _, z := range zrList {
		zones = append(zones, z.zone)
	}
	var dedicatedHosts []hivev1ibmcloud.DedicatedHost
	for name := range dedicatedHostSet {
		dedicatedHosts = append(dedicatedHosts, hivev1ibmcloud.DedicatedHost{Name: name, Profile: cfg.Profile})
	}
	bootKey := ""
	if cfg.BootVolume.EncryptionKey != "" {
		bootKey = cfg.BootVolume.EncryptionKey
	}
	return &machinepoolresource.IBMCloudOptions{
		InstanceType:   cfg.Profile,
		Zones:          zones,
		BootVolumeKey:  bootKey,
		DedicatedHosts: dedicatedHosts,
	}, nil
}

func extractNutanixOptions(machineSets []*machineapi.MachineSet) (*machinepoolresource.NutanixOptions, error) {
	first := machineSets[0]
	cfg := &machinev1.NutanixMachineProviderConfig{}
	if err := json.Unmarshal(first.Spec.Template.Spec.ProviderSpec.Value.Raw, cfg); err != nil {
		return nil, errors.Wrap(err, "decode Nutanix providerSpec")
	}
	fdReplicas := make(map[string]int32)
	for _, ms := range machineSets {
		c := &machinev1.NutanixMachineProviderConfig{}
		if err := json.Unmarshal(ms.Spec.Template.Spec.ProviderSpec.Value.Raw, c); err != nil {
			continue
		}
		fdName := ""
		if c.FailureDomain != nil && c.FailureDomain.Name != "" {
			fdName = c.FailureDomain.Name
		}
		replicas := int32(0)
		if ms.Spec.Replicas != nil {
			replicas = *ms.Spec.Replicas
		}
		fdReplicas[fdName] += replicas
	}
	type fr struct {
		fd       string
		replicas int32
	}
	frList := make([]fr, 0, len(fdReplicas))
	for f, r := range fdReplicas {
		frList = append(frList, fr{f, r})
	}
	sort.Slice(frList, func(i, j int) bool {
		if frList[i].replicas != frList[j].replicas {
			return frList[i].replicas > frList[j].replicas
		}
		return frList[i].fd < frList[j].fd
	})
	var failureDomains []string
	for _, f := range frList {
		if f.fd != "" {
			failureDomains = append(failureDomains, f.fd)
		}
	}
	numCPUs := int64(cfg.VCPUSockets) * int64(cfg.VCPUsPerSocket)
	numCoresPerSocket := int64(cfg.VCPUsPerSocket)
	memMiB := cfg.MemorySize.Value() / (1024 * 1024)
	diskGiB := cfg.SystemDiskSize.Value() / (1024 * 1024 * 1024)
	bootType := "Legacy"
	switch cfg.BootType {
	case machinev1.NutanixUEFIBoot:
		bootType = "UEFI"
	case machinev1.NutanixSecureBoot:
		bootType = "SecureBoot"
	case machinev1.NutanixLegacyBoot, "":
		bootType = "Legacy"
	}
	return &machinepoolresource.NutanixOptions{
		NumCPUs:           numCPUs,
		NumCoresPerSocket: numCoresPerSocket,
		MemoryMiB:         memMiB,
		OSDiskSizeGiB:     diskGiB,
		BootType:          bootType,
		FailureDomains:    failureDomains,
	}, nil
}

func extractOpenStackOptions(machineSets []*machineapi.MachineSet) (*machinepoolresource.OpenStackOptions, error) {
	first := machineSets[0]
	cfg := &machinev1alpha1.OpenstackProviderSpec{}
	if err := json.Unmarshal(first.Spec.Template.Spec.ProviderSpec.Value.Raw, cfg); err != nil {
		return nil, errors.Wrap(err, "decode OpenStack providerSpec")
	}
	rootSize := 0
	rootType := ""
	if cfg.RootVolume != nil {
		rootSize = cfg.RootVolume.Size
		rootType = cfg.RootVolume.VolumeType
	}
	var sgIDs []string
	for _, sg := range cfg.SecurityGroups {
		if sg.UUID != "" {
			sgIDs = append(sgIDs, sg.UUID)
		} else if sg.Name != "" {
			sgIDs = append(sgIDs, sg.Name)
		}
	}
	return &machinepoolresource.OpenStackOptions{
		Flavor:                     cfg.Flavor,
		RootVolumeSize:             rootSize,
		RootVolumeType:             rootType,
		AdditionalSecurityGroupIDs: sgIDs,
	}, nil
}
