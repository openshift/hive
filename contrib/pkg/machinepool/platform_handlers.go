package machinepool

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	cpms "github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/providerconfig"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	hivev1azure "github.com/openshift/hive/apis/hive/v1/azure"
	hivev1gcp "github.com/openshift/hive/apis/hive/v1/gcp"
	hivev1ibmcloud "github.com/openshift/hive/apis/hive/v1/ibmcloud"
	hivev1nutanix "github.com/openshift/hive/apis/hive/v1/nutanix"
	hivev1openstack "github.com/openshift/hive/apis/hive/v1/openstack"
	hivev1vsphere "github.com/openshift/hive/apis/hive/v1/vsphere"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/util/logrus"
)

var (
	kubernetesNamespaceRegex = regexp.MustCompile(`^([^/]*\.)?kubernetes.io/`)
	openshiftNamespaceRegex  = regexp.MustCompile(`^([^/]*\.)?openshift.io/`)
	k8sNamespaceRegex        = regexp.MustCompile(`^([^/]*\.)?k8s.io/`)
	sigsK8sNamespaceRegex    = regexp.MustCompile(`^sigs\.k8s\.io/`)
)

func isKubernetesManagedTag(tagKey string) bool {
	return kubernetesNamespaceRegex.MatchString(tagKey) ||
		openshiftNamespaceRegex.MatchString(tagKey) ||
		k8sNamespaceRegex.MatchString(tagKey) ||
		sigsK8sNamespaceRegex.MatchString(tagKey)
}

func providerConfigToMap(providerConfig cpms.ProviderConfig) (map[string]interface{}, error) {
	rawConfig, err := providerConfig.RawConfig()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get raw config from provider config")
	}
	var specMap map[string]interface{}
	if err := json.Unmarshal(rawConfig, &specMap); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal raw config to map")
	}
	return specMap, nil
}

func extractSubnetsFromAnalysis(zones []string, subnetsByZone map[string]string) []string {
	if len(subnetsByZone) == 0 || len(zones) == 0 {
		return nil
	}
	subnets := make([]string, 0, len(zones))
	for _, zone := range zones {
		if subnet, ok := subnetsByZone[zone]; ok && subnet != "" {
			subnets = append(subnets, subnet)
		}
	}
	return subnets
}

type PlatformHandler interface {
	Platform() string
	ExtractZone(providerConfig cpms.ProviderConfig) (string, error)
	ExtractInstanceType(providerConfig cpms.ProviderConfig) (string, error)
	ExtractSubnet(providerConfig cpms.ProviderConfig, zone string, analysis *MachineSetAnalysis) error
	ExtractPlatformAggregatedData(analysis *MachineSetAnalysis) error
	ValidateFailureDomain(fd string, cd *hivev1.ClusterDeployment) error
	SupportsZones() bool
	BuildPlatformStruct(opts *CreateOptions, mp *hivev1.MachinePool) error
	BuildPlatformStructFromProviderConfig(providerConfig cpms.ProviderConfig, analysis *MachineSetAnalysis, mp *hivev1.MachinePool) error
	ValidatePlatformSpecificFields(opts *CreateOptions) error
}

var (
	platformRegistry     = make(map[string]PlatformHandler)
	platformRegistryOnce sync.Once
	platformRegistryMu   sync.RWMutex
)

func initPlatformRegistry() {
	platformRegistryMu.Lock()
	defer platformRegistryMu.Unlock()
	platformRegistry[constants.PlatformAWS] = &awsHandler{basePlatformHandler: newPlatformHandler(constants.PlatformAWS)}
	platformRegistry[constants.PlatformGCP] = &gcpHandler{basePlatformHandler: newPlatformHandler(constants.PlatformGCP)}
	platformRegistry[constants.PlatformAzure] = &azureHandler{basePlatformHandler: newPlatformHandler(constants.PlatformAzure)}
	platformRegistry[constants.PlatformVSphere] = &vSphereHandler{basePlatformHandler: newPlatformHandler(constants.PlatformVSphere)}
	platformRegistry[constants.PlatformNutanix] = &nutanixHandler{basePlatformHandler: newPlatformHandler(constants.PlatformNutanix)}
	platformRegistry[constants.PlatformIBMCloud] = &ibmCloudHandler{basePlatformHandler: newPlatformHandler(constants.PlatformIBMCloud)}
	platformRegistry[constants.PlatformOpenStack] = &openStackHandler{basePlatformHandler: newPlatformHandler(constants.PlatformOpenStack)}
}

func GetPlatformHandler(platform string) (PlatformHandler, error) {
	platformRegistryOnce.Do(initPlatformRegistry)
	platformRegistryMu.RLock()
	defer platformRegistryMu.RUnlock()
	handler, ok := platformRegistry[platform]
	if !ok {
		return nil, errors.Errorf("unsupported platform: %s", platform)
	}
	return handler, nil
}

type basePlatformHandler struct {
	platform string
	logger   log.FieldLogger
}

func (b *basePlatformHandler) Platform() string { return b.platform }

func (b *basePlatformHandler) SupportsZones() bool { return true }

func (b *basePlatformHandler) ExtractPlatformAggregatedData(analysis *MachineSetAnalysis) error {
	return nil
}

func (b *basePlatformHandler) ValidateFailureDomain(fd string, cd *hivev1.ClusterDeployment) error {
	return nil
}

func (b *basePlatformHandler) BuildPlatformStruct(opts *CreateOptions, mp *hivev1.MachinePool) error {
	return errors.Errorf("BuildPlatformStruct is not implemented for platform %s", b.platform)
}

func (b *basePlatformHandler) BuildPlatformStructFromProviderConfig(providerConfig cpms.ProviderConfig, analysis *MachineSetAnalysis, mp *hivev1.MachinePool) error {
	return errors.Errorf("BuildPlatformStructFromProviderConfig is not implemented for platform %s", b.platform)
}

func newPlatformHandler(platform string) basePlatformHandler {
	return basePlatformHandler{
		platform: platform,
		logger:   log.WithField("platform", platform),
	}
}

type awsHandler struct {
	basePlatformHandler
}

func (h *awsHandler) ExtractZone(providerConfig cpms.ProviderConfig) (string, error) {
	fd := providerConfig.ExtractFailureDomain()
	if fd == nil {
		return "", nil
	}
	return fd.AWS().Placement.AvailabilityZone, nil
}

func (h *awsHandler) ExtractInstanceType(providerConfig cpms.ProviderConfig) (string, error) {
	instanceType := providerConfig.AWS().Config().InstanceType
	if instanceType == "" {
		return "", errors.New("AWS MachineSet has empty instanceType")
	}
	return instanceType, nil
}

func (h *awsHandler) ExtractSubnet(providerConfig cpms.ProviderConfig, zone string, analysis *MachineSetAnalysis) error {
	if zone == "" {
		return nil
	}
	if subnetID := providerConfig.AWS().Config().Subnet.ID; subnetID != nil && *subnetID != "" {
		analysis.Platform.SubnetsByZone[zone] = *subnetID
	}
	return nil
}

func (h *awsHandler) BuildPlatformStruct(opts *CreateOptions, mp *hivev1.MachinePool) error {
	if opts.AWS == nil {
		return errors.New("AWS platform config is nil")
	}
	mp.Spec.Platform.AWS = opts.AWS
	return nil
}

func (h *awsHandler) ValidatePlatformSpecificFields(opts *CreateOptions) error {
	if opts.AWS == nil {
		return nil
	}
	if opts.AWS.InstanceType == "" {
		return errors.New("AWS instance type is required (--aws-type)")
	}
	if opts.AWS.EC2RootVolume.IOPS > 0 {
		switch opts.AWS.EC2RootVolume.Type {
		case "gp3", "io1", "io2":
		default:
			return errors.Errorf("AWS root volume IOPS can only be specified for gp3, io1, or io2 volume types, but volume type is %s", opts.AWS.EC2RootVolume.Type)
		}
	}
	if opts.AWS.EC2Metadata != nil && opts.AWS.EC2Metadata.Authentication != "" {
		switch strings.ToLower(opts.AWS.EC2Metadata.Authentication) {
		case "optional", "required":
		default:
			return errors.Errorf("invalid AWS metadata authentication value: %s. Valid values: optional, required", opts.AWS.EC2Metadata.Authentication)
		}
	}
	return nil
}

func (h *awsHandler) BuildPlatformStructFromProviderConfig(providerConfig cpms.ProviderConfig, analysis *MachineSetAnalysis, mp *hivev1.MachinePool) error {
	awsConfig := providerConfig.AWS().Config()

	platform := &hivev1aws.MachinePoolPlatform{
		InstanceType: awsConfig.InstanceType,
		Zones:        analysis.Summary.Zones,
	}

	if len(awsConfig.BlockDevices) > 0 {
		if ebs := awsConfig.BlockDevices[0].EBS; ebs != nil {
			platform.EC2RootVolume = hivev1aws.EC2RootVolume{}
			if ebs.VolumeSize != nil {
				platform.EC2RootVolume.Size = int(*ebs.VolumeSize)
			}
			if ebs.VolumeType != nil {
				platform.EC2RootVolume.Type = *ebs.VolumeType
			}
			if ebs.Iops != nil {
				platform.EC2RootVolume.IOPS = int(*ebs.Iops)
			}
			switch {
			case ebs.KMSKey.ID != nil && *ebs.KMSKey.ID != "":
				platform.EC2RootVolume.KMSKeyARN = *ebs.KMSKey.ID
			case ebs.KMSKey.ARN != nil && *ebs.KMSKey.ARN != "":
				platform.EC2RootVolume.KMSKeyARN = *ebs.KMSKey.ARN
			}
		}
	}

	if awsConfig.SpotMarketOptions != nil && awsConfig.SpotMarketOptions.MaxPrice != nil {
		price := *awsConfig.SpotMarketOptions.MaxPrice
		platform.SpotMarketOptions = &hivev1aws.SpotMarketOptions{MaxPrice: &price}
	}

	if awsConfig.MetadataServiceOptions.Authentication != "" {
		platform.EC2Metadata = &hivev1aws.EC2Metadata{Authentication: string(awsConfig.MetadataServiceOptions.Authentication)}
	}

	var securityGroupIDs []string
	for _, sg := range awsConfig.SecurityGroups {
		if sg.ID != nil && *sg.ID != "" && len(sg.Filters) == 0 {
			securityGroupIDs = append(securityGroupIDs, *sg.ID)
		}
	}
	if len(securityGroupIDs) > 0 {
		platform.AdditionalSecurityGroupIDs = securityGroupIDs
	}

	userTags := make(map[string]string)
	for _, tag := range awsConfig.Tags {
		if !isKubernetesManagedTag(tag.Name) {
			userTags[tag.Name] = tag.Value
		}
	}
	if len(userTags) > 0 {
		platform.UserTags = userTags
	}

	if subnets := extractSubnetsFromAnalysis(platform.Zones, analysis.Platform.SubnetsByZone); len(subnets) > 0 {
		platform.Subnets = subnets
	}

	mp.Spec.Platform.AWS = platform
	return nil
}

type gcpHandler struct {
	basePlatformHandler
}

func (h *gcpHandler) ExtractZone(providerConfig cpms.ProviderConfig) (string, error) {
	fd := providerConfig.ExtractFailureDomain()
	if fd == nil {
		return "", nil
	}
	return fd.GCP().Zone, nil
}

func (h *gcpHandler) ExtractInstanceType(providerConfig cpms.ProviderConfig) (string, error) {
	machineType := providerConfig.GCP().Config().MachineType
	if machineType == "" {
		return "", errors.New("GCP MachineSet has empty machineType")
	}
	return machineType, nil
}

func (h *gcpHandler) ExtractSubnet(providerConfig cpms.ProviderConfig, zone string, analysis *MachineSetAnalysis) error {
	if zone == "" {
		return nil
	}
	gcpConfig := providerConfig.GCP().Config()
	if len(gcpConfig.NetworkInterfaces) > 0 && gcpConfig.NetworkInterfaces[0] != nil {
		if subnetwork := gcpConfig.NetworkInterfaces[0].Subnetwork; subnetwork != "" {
			analysis.Platform.SubnetsByZone[zone] = subnetwork
		}
	}
	return nil
}

func (h *gcpHandler) ValidatePlatformSpecificFields(opts *CreateOptions) error {
	if opts.GCP == nil {
		return nil
	}
	if opts.GCP.InstanceType == "" {
		return errors.New("GCP instance type is required (--gcp-type)")
	}
	if opts.GCP.SecureBoot != "" {
		switch strings.ToLower(opts.GCP.SecureBoot) {
		case "enabled", "disabled":
		default:
			return errors.Errorf("invalid GCP secure boot value: %s. Valid values: Enabled, Disabled", opts.GCP.SecureBoot)
		}
	}
	if opts.GCP.OnHostMaintenance != "" {
		switch strings.ToLower(opts.GCP.OnHostMaintenance) {
		case "migrate", "terminate":
		default:
			return errors.Errorf("invalid GCP on host maintenance value: %s. Valid values: Migrate, Terminate", opts.GCP.OnHostMaintenance)
		}
	}
	return nil
}

func (h *gcpHandler) BuildPlatformStruct(opts *CreateOptions, mp *hivev1.MachinePool) error {
	if opts.GCP == nil {
		return errors.New("GCP platform config is nil")
	}
	mp.Spec.Platform.GCP = opts.GCP
	return nil
}

func (h *gcpHandler) BuildPlatformStructFromProviderConfig(providerConfig cpms.ProviderConfig, analysis *MachineSetAnalysis, mp *hivev1.MachinePool) error {
	gcpConfig := providerConfig.GCP().Config()

	platform := &hivev1gcp.MachinePool{
		InstanceType: gcpConfig.MachineType,
		Zones:        analysis.Summary.Zones,
	}

	if len(gcpConfig.Disks) > 0 {
		for _, disk := range gcpConfig.Disks {
			if disk != nil && disk.Boot {
				platform.OSDisk = hivev1gcp.OSDisk{
					DiskSizeGB: disk.SizeGB,
					DiskType:   disk.Type,
				}
				if disk.EncryptionKey != nil && disk.EncryptionKey.KMSKey != nil {
					platform.OSDisk.EncryptionKey = &hivev1gcp.EncryptionKeyReference{
						KMSKey: &hivev1gcp.KMSKeyReference{
							Name:      disk.EncryptionKey.KMSKey.Name,
							KeyRing:   disk.EncryptionKey.KMSKey.KeyRing,
							Location:  disk.EncryptionKey.KMSKey.Location,
							ProjectID: disk.EncryptionKey.KMSKey.ProjectID,
						},
					}
					if disk.EncryptionKey.KMSKeyServiceAccount != "" {
						platform.OSDisk.EncryptionKey.KMSKeyServiceAccount = disk.EncryptionKey.KMSKeyServiceAccount
					}
				}
				break
			}
		}
	}

	if len(gcpConfig.NetworkInterfaces) > 0 && gcpConfig.NetworkInterfaces[0] != nil {
		if projectID := gcpConfig.NetworkInterfaces[0].ProjectID; projectID != "" {
			platform.NetworkProjectID = projectID
		}
	}

	if platform.NetworkProjectID == "" && analysis.Input.ClusterDeployment != nil &&
		analysis.Input.ClusterDeployment.Spec.ClusterMetadata != nil &&
		analysis.Input.ClusterDeployment.Spec.ClusterMetadata.Platform != nil &&
		analysis.Input.ClusterDeployment.Spec.ClusterMetadata.Platform.GCP != nil &&
		analysis.Input.ClusterDeployment.Spec.ClusterMetadata.Platform.GCP.NetworkProjectID != nil {
		platform.NetworkProjectID = *analysis.Input.ClusterDeployment.Spec.ClusterMetadata.Platform.GCP.NetworkProjectID
	}

	if gcpConfig.ShieldedInstanceConfig.SecureBoot != "" {
		platform.SecureBoot = string(gcpConfig.ShieldedInstanceConfig.SecureBoot)
	}

	if gcpConfig.OnHostMaintenance != "" {
		platform.OnHostMaintenance = string(gcpConfig.OnHostMaintenance)
	}

	if len(gcpConfig.ServiceAccounts) > 0 {
		platform.ServiceAccount = gcpConfig.ServiceAccounts[0].Email
	}

	if len(gcpConfig.ResourceManagerTags) > 0 {
		platform.UserTags = make([]hivev1gcp.UserTag, len(gcpConfig.ResourceManagerTags))
		for i, tag := range gcpConfig.ResourceManagerTags {
			platform.UserTags[i] = hivev1gcp.UserTag{
				ParentID: tag.ParentID,
				Key:      tag.Key,
				Value:    tag.Value,
			}
		}
	}

	mp.Spec.Platform.GCP = platform
	return nil
}

type azureHandler struct {
	basePlatformHandler
}

func (h *azureHandler) ExtractZone(providerConfig cpms.ProviderConfig) (string, error) {
	fd := providerConfig.ExtractFailureDomain()
	if fd == nil {
		return "", nil
	}
	return fd.Azure().Zone, nil
}

func (h *azureHandler) ExtractInstanceType(providerConfig cpms.ProviderConfig) (string, error) {
	vmSize := providerConfig.Azure().Config().VMSize
	if vmSize == "" {
		return "", errors.New("Azure MachineSet has empty vmSize")
	}
	return vmSize, nil
}

func (h *azureHandler) ExtractSubnet(providerConfig cpms.ProviderConfig, zone string, analysis *MachineSetAnalysis) error {
	if zone == "" {
		return nil
	}
	azureConfig := providerConfig.Azure().Config()
	if azureConfig.Subnet != "" {
		analysis.Platform.SubnetsByZone[zone] = azureConfig.Subnet
	}
	return nil
}

// ExtractPlatformAggregatedData aggregates platform-specific data from multiple MachineSets.
// For Azure, it determines the ComputeSubnet by analyzing subnets across all zones:
//   - If all MachineSets use the same subnet, that subnet is set as ComputeSubnet
//   - If no subnets are found, ComputeSubnet remains empty
//   - If multiple different subnets are found, a warning is logged and ComputeSubnet is not set
//     (this allows the user to manually specify the subnet if needed)
func (h *azureHandler) ExtractPlatformAggregatedData(analysis *MachineSetAnalysis) error {
	subnetsSet := make(map[string]bool)
	for zone, subnet := range analysis.Platform.SubnetsByZone {
		if zone != "" && subnet != "" {
			subnetsSet[subnet] = true
		}
	}
	switch len(subnetsSet) {
	case 1:
		// All MachineSets use the same subnet - use it as ComputeSubnet
		for subnet := range subnetsSet {
			analysis.Platform.ComputeSubnet = subnet
			break
		}
	case 0:
		// No subnets found - ComputeSubnet remains empty
	default:
		// Multiple different subnets found - cannot auto-determine ComputeSubnet
		h.logger.Warnf("Multiple different subnets found across MachineSets, cannot determine ComputeSubnet. Found: %v", subnetsSet)
	}
	return nil
}

func (h *azureHandler) ValidatePlatformSpecificFields(opts *CreateOptions) error {
	if opts.Azure == nil {
		return nil
	}
	if opts.Azure.InstanceType == "" {
		return errors.New("Azure instance type is required (--azure-type)")
	}
	if opts.Azure.VMNetworkingType != "" {
		switch strings.ToLower(opts.Azure.VMNetworkingType) {
		case "accelerated", "basic":
		default:
			return errors.Errorf("invalid Azure VM networking type: %s. Valid values: Accelerated, Basic", opts.Azure.VMNetworkingType)
		}
	}
	return nil
}

func (h *azureHandler) BuildPlatformStruct(opts *CreateOptions, mp *hivev1.MachinePool) error {
	if opts.Azure == nil {
		return errors.New("Azure platform config is nil")
	}
	if opts.Azure.OSImage != nil && opts.Azure.OSImage.Plan == "" {
		opts.Azure.OSImage.Plan = hivev1azure.ImageWithPurchasePlan
	}
	mp.Spec.Platform.Azure = opts.Azure
	return nil
}

func (h *azureHandler) BuildPlatformStructFromProviderConfig(providerConfig cpms.ProviderConfig, analysis *MachineSetAnalysis, mp *hivev1.MachinePool) error {
	azureConfig := providerConfig.Azure().Config()

	platform := &hivev1azure.MachinePool{
		InstanceType: azureConfig.VMSize,
		Zones:        analysis.Summary.Zones,
	}

	platform.OSDisk = hivev1azure.OSDisk{
		DiskSizeGB: azureConfig.OSDisk.DiskSizeGB,
		DiskType:   azureConfig.OSDisk.ManagedDisk.StorageAccountType,
	}

	switch azureConfig.Image.Type {
	case machinev1beta1.AzureImageTypeMarketplaceWithPlan, machinev1beta1.AzureImageTypeMarketplaceNoPlan:
		plan := hivev1azure.ImageWithPurchasePlan
		if azureConfig.Image.Type == machinev1beta1.AzureImageTypeMarketplaceNoPlan {
			plan = hivev1azure.ImageNoPurchasePlan
		}
		platform.OSImage = &hivev1azure.OSImage{
			Publisher: azureConfig.Image.Publisher,
			Offer:     azureConfig.Image.Offer,
			SKU:       azureConfig.Image.SKU,
			Version:   azureConfig.Image.Version,
			Plan:      plan,
		}
	}

	if azureConfig.NetworkResourceGroup != "" {
		platform.NetworkResourceGroupName = azureConfig.NetworkResourceGroup
	}
	if azureConfig.Subnet != "" {
		platform.ComputeSubnet = azureConfig.Subnet
	}
	if azureConfig.Vnet != "" {
		platform.VirtualNetwork = azureConfig.Vnet
	}
	mp.Spec.Platform.Azure = platform
	return nil
}

type vSphereHandler struct {
	basePlatformHandler
}

func (h *vSphereHandler) ExtractZone(providerConfig cpms.ProviderConfig) (string, error) {
	fd := providerConfig.ExtractFailureDomain()
	if fd == nil {
		return "", nil
	}
	vsphereFD := fd.VSphere()
	return vsphereFD.Name, nil
}

func (h *vSphereHandler) ExtractInstanceType(providerConfig cpms.ProviderConfig) (string, error) {
	cfg := providerConfig.VSphere().Config()
	cpus := float64(cfg.NumCPUs)
	coresPerSocket := float64(cfg.NumCoresPerSocket)
	memoryMB := float64(cfg.MemoryMiB)
	if coresPerSocket > 0 {
		return fmt.Sprintf("%.0fcpu-%.0fcores-%.0fmb", cpus, coresPerSocket, memoryMB), nil
	}
	return fmt.Sprintf("%.0fcpu-%.0fmb", cpus, memoryMB), nil
}

func (h *vSphereHandler) ExtractSubnet(providerConfig cpms.ProviderConfig, zone string, analysis *MachineSetAnalysis) error {
	return nil
}

func (h *vSphereHandler) ExtractPlatformAggregatedData(analysis *MachineSetAnalysis) error {
	if len(analysis.Input.MachineSets) == 0 {
		return nil
	}

	logr := logrus.NewLogr(h.logger)
	resourcePoolsByFD := make(map[string]map[string][]string, len(analysis.Summary.Zones))
	resourcePools := make(map[string][]string)

	var parseErrors []string
	for i := range analysis.Input.MachineSets {
		ms := &analysis.Input.MachineSets[i]
		providerConfig, err := cpms.NewProviderConfigFromMachineSpec(logr, ms.Spec.Template.Spec, analysis.Input.Infrastructure)
		if err != nil {
			parseErrors = append(parseErrors, fmt.Sprintf("MachineSet %s: %v", ms.Name, err))
			continue
		}

		fd := providerConfig.ExtractFailureDomain()
		fdName := ""
		if fd != nil {
			fdName = fd.VSphere().Name
		}

		vsphereConfig := providerConfig.VSphere().Config()
		resourcePool := ""
		if vsphereConfig.Workspace != nil {
			resourcePool = vsphereConfig.Workspace.ResourcePool
		}
		if resourcePoolsByFD[fdName] == nil {
			resourcePoolsByFD[fdName] = make(map[string][]string)
		}
		resourcePoolsByFD[fdName][resourcePool] = append(resourcePoolsByFD[fdName][resourcePool], ms.Name)

		resourcePools[resourcePool] = append(resourcePools[resourcePool], ms.Name)
	}

	if len(parseErrors) > 0 {
		return errors.Errorf("failed to parse provider config for %d MachineSet(s): %s", len(parseErrors), strings.Join(parseErrors, "; "))
	}

	if len(resourcePools) > 1 {
		if len(resourcePoolsByFD) > 1 {
			var fdDetails []string
			for fdName, rpMap := range resourcePoolsByFD {
				if fdName == "" {
					fdName = "<no failure domain>"
				}
				for rp, msNames := range rpMap {
					rpDisplay := rp
					if rpDisplay == "" {
						rpDisplay = "<empty>"
					}
					fdDetails = append(fdDetails, fmt.Sprintf("  FailureDomain %q -> ResourcePool %q: MachineSets %v", fdName, rpDisplay, msNames))
				}
			}
			h.logger.Warnf("vSphere MachineSets use different ResourcePools across multiple failure domains. "+
				"This is normal for vSphere clusters with multiple failure domains. "+
				"Hive MachinePool will use the ResourcePool from the first MachineSet. "+
				"Found %d different ResourcePools across %d failure domains:\n%s",
				len(resourcePools), len(resourcePoolsByFD), strings.Join(fdDetails, "\n"))
		} else {
			var details []string
			for rp, msNames := range resourcePools {
				rpDisplay := rp
				if rpDisplay == "" {
					rpDisplay = "<empty>"
				}
				details = append(details, fmt.Sprintf("ResourcePool %q: MachineSets %v", rpDisplay, msNames))
			}
			h.logger.Warnf("vSphere MachineSets use different ResourcePools. "+
				"Hive MachinePool will use the ResourcePool from the first MachineSet. "+
				"Found %d different ResourcePools:\n%s",
				len(resourcePools), strings.Join(details, "\n"))
		}
	}

	return nil
}

func (h *vSphereHandler) ValidatePlatformSpecificFields(opts *CreateOptions) error {
	return nil
}

func (h *vSphereHandler) SupportsZones() bool {
	return false
}

func (h *vSphereHandler) BuildPlatformStruct(opts *CreateOptions, mp *hivev1.MachinePool) error {
	if opts.VSphere == nil {
		return errors.New("VSphere platform config is nil")
	}
	mp.Spec.Platform.VSphere = opts.VSphere
	return nil
}

func (h *vSphereHandler) BuildPlatformStructFromProviderConfig(providerConfig cpms.ProviderConfig, analysis *MachineSetAnalysis, mp *hivev1.MachinePool) error {
	vsphereConfig := providerConfig.VSphere().Config()

	platform := &hivev1vsphere.MachinePool{
		NumCPUs:           vsphereConfig.NumCPUs,
		NumCoresPerSocket: vsphereConfig.NumCoresPerSocket,
		MemoryMiB:         vsphereConfig.MemoryMiB,
	}

	if vsphereConfig.DiskGiB > 0 {
		platform.OSDisk = hivev1vsphere.OSDisk{
			DiskSizeGB: vsphereConfig.DiskGiB,
		}
	}

	if vsphereConfig.Workspace != nil && vsphereConfig.Workspace.ResourcePool != "" {
		platform.ResourcePool = vsphereConfig.Workspace.ResourcePool
	}

	if len(vsphereConfig.TagIDs) > 0 {
		platform.TagIDs = vsphereConfig.TagIDs
	}

	mp.Spec.Platform.VSphere = platform
	return nil
}

type nutanixHandler struct {
	basePlatformHandler
}

func (h *nutanixHandler) ExtractZone(providerConfig cpms.ProviderConfig) (string, error) {
	fd := providerConfig.ExtractFailureDomain()
	if fd == nil {
		return "", nil
	}
	nutanixFD := fd.Nutanix()
	return nutanixFD.Name, nil
}

func (h *nutanixHandler) ExtractInstanceType(providerConfig cpms.ProviderConfig) (string, error) {
	nutanixConfig := providerConfig.Nutanix().Config()
	vcpuSockets := float64(nutanixConfig.VCPUSockets)
	vcpusPerSocket := float64(nutanixConfig.VCPUsPerSocket)
	memoryBytes := nutanixConfig.MemorySize.Value()
	var memoryMB float64
	if memoryBytes > 0 {
		memoryMB = float64(memoryBytes) / (1024 * 1024)
	}
	return fmt.Sprintf("%.0fsockets-%.0fcores-%.0fmb", vcpuSockets, vcpusPerSocket, memoryMB), nil
}

func (h *nutanixHandler) ExtractSubnet(providerConfig cpms.ProviderConfig, zone string, analysis *MachineSetAnalysis) error {
	if zone == "" {
		return nil
	}
	nutanixConfig := providerConfig.Nutanix().Config()
	if len(nutanixConfig.Subnets) > 0 {
		subnet := nutanixConfig.Subnets[0]
		switch {
		case subnet.UUID != nil && *subnet.UUID != "":
			analysis.Platform.SubnetsByZone[zone] = *subnet.UUID
		case subnet.Name != nil && *subnet.Name != "":
			analysis.Platform.SubnetsByZone[zone] = *subnet.Name
		}
	}
	return nil
}

func (h *nutanixHandler) ValidateFailureDomain(fd string, cd *hivev1.ClusterDeployment) error {
	if fd == "" || cd.Spec.Platform.Nutanix == nil || len(cd.Spec.Platform.Nutanix.FailureDomains) == 0 {
		return nil
	}
	fdNames := make([]string, 0, len(cd.Spec.Platform.Nutanix.FailureDomains))
	fdNameSet := make(map[string]bool, len(cd.Spec.Platform.Nutanix.FailureDomains))
	for _, domain := range cd.Spec.Platform.Nutanix.FailureDomains {
		name := domain.Name
		fdNames = append(fdNames, name)
		fdNameSet[name] = true
	}
	if !fdNameSet[fd] {
		return errors.Errorf("failure domain %q is not defined in ClusterDeployment.spec.platform.nutanix.failureDomains (available: %v)", fd, fdNames)
	}
	return nil
}

func (h *nutanixHandler) ValidatePlatformSpecificFields(opts *CreateOptions) error {
	if opts.Nutanix == nil {
		return nil
	}
	if opts.Nutanix.BootType != "" {
		switch strings.ToLower(string(opts.Nutanix.BootType)) {
		case "legacy", "uefi", "secureboot":
		default:
			return errors.Errorf("invalid Nutanix boot type: %s. Valid values: Legacy, UEFI, SecureBoot", opts.Nutanix.BootType)
		}
	}
	return nil
}

func (h *nutanixHandler) BuildPlatformStruct(opts *CreateOptions, mp *hivev1.MachinePool) error {
	if opts.Nutanix == nil {
		return errors.New("Nutanix platform config is nil")
	}
	mp.Spec.Platform.Nutanix = opts.Nutanix
	return nil
}

func (h *nutanixHandler) BuildPlatformStructFromProviderConfig(providerConfig cpms.ProviderConfig, analysis *MachineSetAnalysis, mp *hivev1.MachinePool) error {
	nutanixConfig := providerConfig.Nutanix().Config()

	platform := &hivev1nutanix.MachinePool{}

	if nutanixConfig.VCPUSockets > 0 {
		platform.NumCPUs = int64(nutanixConfig.VCPUSockets)
	}
	if nutanixConfig.VCPUsPerSocket > 0 {
		platform.NumCoresPerSocket = int64(nutanixConfig.VCPUsPerSocket)
	}

	if !nutanixConfig.MemorySize.IsZero() {
		memoryBytes := nutanixConfig.MemorySize.Value()
		platform.MemoryMiB = memoryBytes / (1024 * 1024)
	}

	if !nutanixConfig.SystemDiskSize.IsZero() {
		diskBytes := nutanixConfig.SystemDiskSize.Value()
		platform.OSDisk = hivev1nutanix.OSDisk{
			DiskSizeGiB: diskBytes / (1024 * 1024 * 1024),
		}
	}

	if nutanixConfig.BootType != "" {
		platform.BootType = nutanixConfig.BootType
	}

	mp.Spec.Platform.Nutanix = platform
	return nil
}

type ibmCloudHandler struct {
	basePlatformHandler
}

func (h *ibmCloudHandler) getIBMCloudSpecMap(providerConfig cpms.ProviderConfig) (map[string]interface{}, error) {
	return providerConfigToMap(providerConfig)
}

func (h *ibmCloudHandler) ExtractZone(providerConfig cpms.ProviderConfig) (string, error) {
	specMap, err := h.getIBMCloudSpecMap(providerConfig)
	if err != nil {
		return "", errors.Wrap(err, "failed to convert IBM Cloud provider config to map")
	}
	if zone, ok := specMap["zone"].(string); ok && zone != "" {
		return zone, nil
	}
	return "", nil
}

func (h *ibmCloudHandler) ExtractInstanceType(providerConfig cpms.ProviderConfig) (string, error) {
	specMap, err := h.getIBMCloudSpecMap(providerConfig)
	if err != nil {
		return "", errors.Wrap(err, "failed to convert IBM Cloud provider config to map")
	}
	profile, ok := specMap["profile"].(string)
	if !ok || profile == "" {
		return "", errors.Errorf("IBM Cloud MachineSet has empty profile")
	}
	return profile, nil
}

func (h *ibmCloudHandler) ExtractSubnet(providerConfig cpms.ProviderConfig, zone string, analysis *MachineSetAnalysis) error {
	if zone == "" {
		return nil
	}
	specMap, err := h.getIBMCloudSpecMap(providerConfig)
	if err != nil {
		return errors.Wrap(err, "failed to convert IBM Cloud provider config to map")
	}
	if subnet, ok := specMap["subnet"].(string); ok && subnet != "" {
		analysis.Platform.SubnetsByZone[zone] = subnet
	} else if subnetID, ok := specMap["subnetID"].(string); ok && subnetID != "" {
		analysis.Platform.SubnetsByZone[zone] = subnetID
	}
	return nil
}

func (h *ibmCloudHandler) ValidatePlatformSpecificFields(opts *CreateOptions) error {
	if opts.IBMCloud == nil {
		return nil
	}
	if opts.IBMCloud.InstanceType == "" {
		return errors.New("IBM Cloud instance type is required (--ibmcloud-type)")
	}
	return nil
}

func (h *ibmCloudHandler) BuildPlatformStruct(opts *CreateOptions, mp *hivev1.MachinePool) error {
	if opts.IBMCloud == nil {
		return errors.New("IBMCloud platform config is nil")
	}
	mp.Spec.Platform.IBMCloud = opts.IBMCloud
	return nil
}

func (h *ibmCloudHandler) BuildPlatformStructFromProviderConfig(providerConfig cpms.ProviderConfig, analysis *MachineSetAnalysis, mp *hivev1.MachinePool) error {
	specMap, err := h.getIBMCloudSpecMap(providerConfig)
	if err != nil {
		return errors.Wrap(err, "failed to convert IBM Cloud provider config to map")
	}

	platform := &hivev1ibmcloud.MachinePool{
		Zones: analysis.Summary.Zones,
	}

	if profile, ok := specMap["profile"].(string); ok && profile != "" {
		platform.InstanceType = profile
	}

	if bootVolumeMap, ok := specMap["bootVolume"].(map[string]interface{}); ok {
		if encryptionKey, ok := bootVolumeMap["encryptionKey"].(string); ok && encryptionKey != "" {
			platform.BootVolume = &hivev1ibmcloud.BootVolume{
				EncryptionKey: encryptionKey,
			}
		}
	}

	if dedicatedHostsSlice, ok := specMap["dedicatedHosts"].([]interface{}); ok && len(dedicatedHostsSlice) > 0 {
		dedicatedHosts := make([]hivev1ibmcloud.DedicatedHost, 0, len(dedicatedHostsSlice))
		for _, hostInterface := range dedicatedHostsSlice {
			if hostMap, ok := hostInterface.(map[string]interface{}); ok {
				host := hivev1ibmcloud.DedicatedHost{}
				if name, ok := hostMap["name"].(string); ok {
					host.Name = name
				}
				if profile, ok := hostMap["profile"].(string); ok {
					host.Profile = profile
				}
				if host.Name != "" {
					dedicatedHosts = append(dedicatedHosts, host)
				}
			}
		}
		if len(dedicatedHosts) > 0 {
			platform.DedicatedHosts = dedicatedHosts
		}
	}

	mp.Spec.Platform.IBMCloud = platform
	return nil
}

type openStackHandler struct {
	basePlatformHandler
}

func (h *openStackHandler) ExtractZone(providerConfig cpms.ProviderConfig) (string, error) {
	fd := providerConfig.ExtractFailureDomain()
	if fd == nil {
		return "", nil
	}
	openstackFD := fd.OpenStack()
	return openstackFD.AvailabilityZone, nil
}

func (h *openStackHandler) ExtractInstanceType(providerConfig cpms.ProviderConfig) (string, error) {
	openstackConfig := providerConfig.OpenStack().Config()
	if openstackConfig.Flavor == "" {
		return "", errors.Errorf("OpenStack MachineSet has empty flavor")
	}
	return openstackConfig.Flavor, nil
}

func (h *openStackHandler) ExtractSubnet(providerConfig cpms.ProviderConfig, zone string, analysis *MachineSetAnalysis) error {
	return nil
}

func (h *openStackHandler) ValidatePlatformSpecificFields(opts *CreateOptions) error {
	if opts.OpenStack == nil {
		return nil
	}
	if opts.OpenStack.Flavor == "" {
		return errors.New("OpenStack flavor is required (--openstack-flavor)")
	}
	return nil
}

func (h *openStackHandler) BuildPlatformStruct(opts *CreateOptions, mp *hivev1.MachinePool) error {
	if opts.OpenStack == nil {
		return errors.New("OpenStack platform config is nil")
	}
	mp.Spec.Platform.OpenStack = opts.OpenStack
	return nil
}

func (h *openStackHandler) BuildPlatformStructFromProviderConfig(providerConfig cpms.ProviderConfig, analysis *MachineSetAnalysis, mp *hivev1.MachinePool) error {
	openstackConfig := providerConfig.OpenStack().Config()

	platform := &hivev1openstack.MachinePool{Flavor: openstackConfig.Flavor}

	if openstackConfig.RootVolume != nil {
		platform.RootVolume = &hivev1openstack.RootVolume{
			Size: openstackConfig.RootVolume.Size,
			Type: openstackConfig.RootVolume.VolumeType,
		}
	}

	var securityGroupIDs []string
	for _, sg := range openstackConfig.SecurityGroups {
		if sg.UUID != "" && (sg.Filter.ID == "" && sg.Filter.Name == "") {
			securityGroupIDs = append(securityGroupIDs, sg.UUID)
		}
	}
	if len(securityGroupIDs) > 0 {
		platform.AdditionalSecurityGroupIDs = securityGroupIDs
	}

	mp.Spec.Platform.OpenStack = platform
	return nil
}
