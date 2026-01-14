package machinepool

import (
	"context"
	"strings"
	"time"

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

	capabilities, err := a.client.GetVMCapabilities(context.TODO(), computePool.Platform.Azure.InstanceType, ic.Platform.Azure.Region)
	if err != nil {
		return nil, false, errors.Wrap(err, "error retrieving VM capabilities")
	}

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
		// An image was not provided so check if the installer created a "gen2" image
		// to determine if we should allow resultant machinesets to consume a "gen2" image.
		gen2ImageExists, err := a.gen2ImageExists(ic.Platform.Azure.ClusterResourceGroupName(cd.Spec.ClusterMetadata.InfraID))
		if err != nil {
			return nil, false, err
		}
		if !gen2ImageExists {
			// Modify capabilities to ensure that a V1 image is chosen by installazure.MachineSets()
			// because a V2 image does not exist.
			// The HyperVGeneration is germane to the instance/disk type and affects the image used
			// for the instance. "-gen-2" will be appended to the image name by installazure.MachineSets()
			// when the HyperVGenerations capability (comma separated list of HyperVGenerations) includes "V2".
			//
			// capabilities := map[string]string{
			//   "HyperVGenerations": "V1,V2",
			// }
			//
			if _, ok := capabilities["HyperVGenerations"]; ok {
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

	// If we leave the imageID, the image is determined by the installer
	const imageID = ""

	installerMachineSets, err := installazure.MachineSets(
		cd.Spec.ClusterMetadata.InfraID,
		installconfig.MakeAsset(ic),
		computePool,
		imageID,
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
	ctx, cancel := context.WithTimeout(context.TODO(), 30*time.Second)
	defer cancel()

	var res azureclient.ResourceSKUsPage
	var err error
	for res, err = a.client.ListResourceSKUs(ctx, ""); err == nil && res.NotDone(); err = res.NextWithContext(ctx) {
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
	ctx, cancel := context.WithTimeout(context.TODO(), 30*time.Second)
	defer cancel()

	var images []compute.Image
	var res azureclient.ImageListResultPage
	var err error
	for res, err = a.client.ListImagesByResourceGroup(ctx, resourceGroupName); err == nil && res.NotDone(); err = res.NextWithContext(ctx) {
		images = append(images, res.Values()...)
	}

	return images, err
}

func (a *AzureActuator) gen2ImageExists(resourceGroupName string) (bool, error) {
	images, err := a.getImagesByResourceGroup(resourceGroupName)
	if err != nil {
		return false, errors.Wrapf(err, "error listing images by resourceGroup: %s", resourceGroupName)
	}
	for _, image := range images {
		if image.ImageProperties.HyperVGeneration == compute.HyperVGenerationTypesV2 {
			return true, nil
		}
	}
	return false, nil
}
