package machinepool

import (
	"context"
	"fmt"
	"strings"
	"time"

	azenc "github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute"
	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-12-01/compute"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/to"
	machineapi "github.com/openshift/api/machine/v1beta1"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	capiazure "sigs.k8s.io/cluster-api-provider-azure/api/v1beta1"

	installconfig "github.com/openshift/installer/pkg/asset/installconfig"
	icazure "github.com/openshift/installer/pkg/asset/installconfig/azure"
	installazure "github.com/openshift/installer/pkg/asset/machines/azure"
	installertypes "github.com/openshift/installer/pkg/types"
	installertypesazure "github.com/openshift/installer/pkg/types/azure"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/azureclient"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

// AzureActuator encapsulates the pieces necessary to be able to generate
// a list of MachineSets to sync to the remote cluster.
type AzureActuator struct {
	client azureclient.Client
	logger log.FieldLogger
}

var _ Actuator = &AzureActuator{}

// NewAzureActuator is the constructor for building a AzureActuator
func NewAzureActuator(azureCreds *corev1.Secret, cloudName string, logger log.FieldLogger) (*AzureActuator, error) {
	azureClient, err := azureclient.NewClientFromSecret(azureCreds, cloudName)
	if err != nil {
		logger.WithError(err).Warn("failed to create Azure client with creds in clusterDeployment's secret")
		return nil, err
	}
	actuator := &AzureActuator{
		client: azureClient,
		logger: logger,
	}
	return actuator, nil
}

// GenerateMachineSets satisfies the Actuator interface and will take a clusterDeployment and return a list of MachineSets
// to sync to the remote cluster.
func (a *AzureActuator) GenerateMachineSets(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool, logger log.FieldLogger) ([]*machineapi.MachineSet, bool, error) {
	if cd.Spec.ClusterMetadata == nil {
		return nil, false, errors.New("ClusterDeployment does not have cluster metadata")
	}
	if cd.Spec.Platform.Azure == nil {
		return nil, false, errors.New("ClusterDeployment is not for Azure")
	}
	if pool.Spec.Platform.Azure == nil {
		return nil, false, errors.New("MachinePool is not for Azure")
	}

	// If we haven't discovered the ResourceGroupName yet, fail
	rg, err := controllerutils.AzureResourceGroup(cd)
	if err != nil {
		return nil, false, err
	}

	// NOTE: This is dummied up specifically to facilitate the no-op path through MachineSets()=>provider=>getBootDiagnosticObject.
	// That path relies on ic.Platform.DefaultMachinePlatform and computePool.Platform.Azure.BootDiagnostics both being nil.
	session := &icazure.Session{
		Environment: azure.Environment{},
	}

	ic := &installertypes.InstallConfig{
		Platform: installertypes.Platform{
			Azure: &installertypesazure.Platform{
				Region:                   cd.Spec.Platform.Azure.Region,
				ResourceGroupName:        rg,
				NetworkResourceGroupName: pool.Spec.Platform.Azure.NetworkResourceGroupName,
				VirtualNetwork:           pool.Spec.Platform.Azure.VirtualNetwork,
				// This will be defaulted by the installer if empty
				OutboundType: installertypesazure.OutboundType(pool.Spec.Platform.Azure.OutboundType),
			},
		},
	}

	if pool.Spec.Platform.Azure.ComputeSubnet != "" {
		ic.Platform.Azure.Subnets = []installertypesazure.SubnetSpec{{
			Name: pool.Spec.Platform.Azure.ComputeSubnet,
			Role: capiazure.SubnetNode,
		}}
	}
	// If ic.Platform.Azure.Subnets is NOT empty, this value is never read.
	// It is only used to populate a default subnet when none is provided.
	var subnetZones []string = nil

	computePool := baseMachinePool(pool)
	computePool.Platform.Azure = &installertypesazure.MachinePool{
		Zones: pool.Spec.Platform.Azure.Zones,
		// Currently not supported
		Identity:     &installertypesazure.VMIdentity{},
		InstanceType: pool.Spec.Platform.Azure.InstanceType,
		OSDisk: installertypesazure.OSDisk{
			DiskSizeGB: pool.Spec.Platform.Azure.OSDisk.DiskSizeGB,
		},
		VMNetworkingType: pool.Spec.Platform.Azure.VMNetworkingType,
	}

	if pool.Spec.Platform.Azure.OSDisk.DiskType != "" {
		computePool.Platform.Azure.OSDisk.DiskType = pool.Spec.Platform.Azure.OSDisk.DiskType
	}

	if diskEncryptionSet := pool.Spec.Platform.Azure.OSDisk.DiskEncryptionSet; diskEncryptionSet != nil {
		computePool.Platform.Azure.OSDisk.DiskEncryptionSet = &installertypesazure.DiskEncryptionSet{
			SubscriptionID: diskEncryptionSet.SubscriptionID,
			ResourceGroup:  diskEncryptionSet.ResourceGroup,
			Name:           diskEncryptionSet.Name,
		}
	}

	capabilities, err := a.getVMCapabilities(context.TODO(), computePool.Platform.Azure.InstanceType, ic.Platform.Azure.Region)
	if err != nil {
		return nil, false, errors.Wrap(err, "error retrieving VM capabilities")
	}

	// If we leave the rhcosImg empty, installer supplies a default marketplace image.
	// If we set it to *any value*, installer will use the well-known gallery image URN format that
	// varies only by HyperVGeneration, which we'll compute below. The value of rhcosImg itself is
	// *not used*.
	rhcosImg := ""

	if osImage := pool.Spec.Platform.Azure.OSImage; osImage != nil && osImage.Publisher != "" {
		var plan = installertypesazure.ImageWithPurchasePlan // default value
		if osImage.Plan != "" {
			plan = installertypesazure.ImagePurchasePlan(osImage.Plan)
		}

		computePool.Platform.Azure.OSImage = installertypesazure.OSImage{
			Plan:      plan,
			Publisher: osImage.Publisher,
			Offer:     osImage.Offer,
			SKU:       osImage.SKU,
			Version:   osImage.Version,
		}
	} else {
		// An image was not provided. If a gallery image exists at all, we need to assume the spoke
		// is old and doesn't have marketplace images. If that's the case, we want to use a gen2
		// image if available (and compatible with our VM size). If no gen2 image is available, we
		// dummy down our VM capabilities to force using the gen1 image.
		// If no gallery image exists, we'll trigger the path in MachineSets() that uses a default
		// marketplace image.
		gallery, gen2, err := a.galleryImageExistsAndIsGen2(ic.Platform.Azure.ClusterResourceGroupName(cd.Spec.ClusterMetadata.InfraID))
		if err != nil {
			return nil, false, err
		}
		if gallery {
			// signal that the installer code should use the gallery URN. This is pretty hacky and
			// non-future-proof; include a breadcrumb to flag possible future regressions.
			rhcosImg = "gallery (if you are seeing this string in a Machine[Set], please report a hive bug)"
			if !gen2 {
				// Ensure that a V1 image is chosen by installazure.MachineSets() because a V2
				// image does not exist.
				// A VM size is capable of running V1, V2, or both, as indicated in its capabilities map:
				//
				// capabilities := map[string]string{
				//   "HyperVGenerations": "V1,V2",
				// }
				//
				// An image is V1 xor V2.
				// Installer code will assume the highest version in the capabilities map; so if no
				// V2 image exists, make sure it chooses V1.
				// (There is still a failure mode here if the VM size doesn't support V1. In that
				// case, the Machine will fail with an appropriate message, and the user should
				// choose a VM size that does.)
				capabilities["HyperVGenerations"] = "V1"
			}
		}
	}

	if len(computePool.Platform.Azure.Zones) == 0 {
		zones, err := a.getZones(cd.Spec.Platform.Azure.Region, pool.Spec.Platform.Azure.InstanceType)
		if err != nil {
			return nil, false, errors.Wrap(err, "compute pool not providing list of zones and failed to fetch list of zones")
		}
		if len(zones) == 0 {
			// No zones specified. Upstream will handle defaulting logic. Adding log here for visibility.
			logger.WithField("region", cd.Spec.Platform.Azure.Region).Info("No availability zones detected for region. Using non-zoned deployment.")
		} else {
			computePool.Platform.Azure.Zones = zones
		}
	}

	installerMachineSets, err := installazure.MachineSets(
		cd.Spec.ClusterMetadata.InfraID,
		installconfig.MakeAsset(ic),
		computePool,
		rhcosImg,
		workerRole,
		workerUserDataName,
		capabilities,
		subnetZones,
		session,
		// TODO: support adding userTags? https://issues.redhat.com/browse/HIVE-2143
	)
	return installerMachineSets, err == nil, errors.Wrap(err, "failed to generate machinesets")
}

func (a *AzureActuator) getZones(region string, instanceType string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.TODO(), 2*time.Minute)
	defer cancel()

	var res azureclient.ResourceSKUsPage
	var err error
	for res, err = a.client.ListResourceSKUs(ctx, region); err == nil && res.NotDone(); err = res.NextWithContext(ctx) {
		for _, resSku := range res.Values() {
			if strings.EqualFold(to.String(resSku.Name), instanceType) {
				for _, locationInfo := range *resSku.LocationInfo {
					if strings.EqualFold(to.String(locationInfo.Location), region) {
						return *locationInfo.Zones, nil
					}
				}
			}
		}
	}

	return nil, err
}

func (a *AzureActuator) getImagesByResourceGroup(resourceGroupName string) ([]compute.Image, error) {
	ctx, cancel := context.WithTimeout(context.TODO(), 2*time.Minute)
	defer cancel()

	var images []compute.Image
	var res azureclient.ImageListResultPage
	var err error
	for res, err = a.client.ListImagesByResourceGroup(ctx, resourceGroupName); err == nil && res.NotDone(); err = res.NextWithContext(ctx) {
		images = append(images, res.Values()...)
	}

	return images, err
}

// galleryImageExistsAndIsGen2 looks for gallery images in the specified RG.
// The first return indicates that we found *any* such images. If this is false, the caller should
// use marketplace images.
// The second return, only relevant if the first is true, indicates whether any of the found images
// are gen2. If this is false, the caller must assume gen1, validate compatibility with the VM
// size, and request gen1 accordingly.
func (a *AzureActuator) galleryImageExistsAndIsGen2(resourceGroupName string) (bool, bool, error) {
	images, err := a.getImagesByResourceGroup(resourceGroupName)
	if err != nil {
		// We get 200/[] rather than 404 if there are none, right?
		return false, false, errors.Wrapf(err, "error listing images by resourceGroup: %s", resourceGroupName)
	}
	found := len(images) != 0
	for _, image := range images {
		if image.ImageProperties.HyperVGeneration == compute.HyperVGenerationTypesV2 {
			return found, true, nil
		}
	}
	return found, false, nil
}

// getVMCapabilities retrieves the capabilities of an instance type in a specific region. Returns these values
// in a map with the capability name as the key and the corresponding value.
func (a *AzureActuator) getVMCapabilities(ctx context.Context, instanceType, region string) (map[string]string, error) {
	typeMeta, err := a.getVirtualMachineSku(ctx, instanceType, region)
	if err != nil {
		return nil, fmt.Errorf("error connecting to Azure client: %v", err)
	}
	if typeMeta == nil {
		return nil, fmt.Errorf("SKU %s not found in region %s", instanceType, region)
	}
	if typeMeta.Capabilities == nil {
		return nil, fmt.Errorf("SKU %s has no Capabilities", instanceType)
	}

	capabilities := make(map[string]string)
	for _, capability := range *typeMeta.Capabilities {
		capabilities[to.String(capability.Name)] = to.String(capability.Value)
	}
	return capabilities, nil
}

// getVirtualMachineSku retrieves the resource SKU of a specified virtual machine SKU in the specified region.
func (a *AzureActuator) getVirtualMachineSku(ctx context.Context, name, region string) (*azenc.ResourceSku, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	var page azureclient.ResourceSKUsPage
	var err error
	for page, err = a.client.ListResourceSKUs(ctx, region); err == nil && page.NotDone(); err = page.NextWithContext(ctx) {
		for _, sku := range page.Values() {
			// Filter out resources that are not virtualMachines
			if !strings.EqualFold("virtualMachines", to.String(sku.ResourceType)) {
				continue
			}
			// Filter out resources that do not match the provided name
			if strings.EqualFold(name, to.String(sku.Name)) {
				return &sku, nil
			}
		}
	}
	// err is nil if we didn't find it (page.NotDone() == false above)
	return nil, errors.Wrap(err, "error fetching SKU pages")
}
