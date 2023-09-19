package machinepool

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/blang/semver/v4"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-12-01/compute"
	"github.com/Azure/go-autorest/autorest/to"
	machineapi "github.com/openshift/api/machine/v1beta1"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"

	installazure "github.com/openshift/installer/pkg/asset/machines/azure"
	installertypes "github.com/openshift/installer/pkg/types"
	installertypesazure "github.com/openshift/installer/pkg/types/azure"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/azureclient"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

var versionsSupportingAzureImageGallery = semver.MustParseRange(">=4.12.0")

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

	ic := &installertypes.InstallConfig{
		Platform: installertypes.Platform{
			Azure: &installertypesazure.Platform{
				Region:            cd.Spec.Platform.Azure.Region,
				ResourceGroupName: rg,
			},
		},
	}

	computePool := baseMachinePool(pool)
	computePool.Platform.Azure = &installertypesazure.MachinePool{
		Zones:        pool.Spec.Platform.Azure.Zones,
		InstanceType: pool.Spec.Platform.Azure.InstanceType,
		OSDisk: installertypesazure.OSDisk{
			DiskSizeGB: pool.Spec.Platform.Azure.OSDisk.DiskSizeGB,
		},
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
		computePool.Platform.Azure.OSImage = installertypesazure.OSImage{
			Publisher: osImage.Publisher,
			Offer:     osImage.Offer,
			SKU:       osImage.SKU,
			Version:   osImage.Version,
		}
	} else {
		// An image was not provided so check if the installer created a "gen2" image
		// to determine if we should allow resultant machinesets to consume a "gen2" image.
		gen2ImageExists, err := a.gen2ImageExists(cd.Spec.ClusterMetadata.InfraID, ic.Platform.Azure.ClusterResourceGroupName(cd.Spec.ClusterMetadata.InfraID))
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
			return nil, false, fmt.Errorf("zero zones returned for region %s", cd.Spec.Platform.Azure.Region)
		}
		computePool.Platform.Azure.Zones = zones
	}
	// The imageID parameter is not used. The image is determined by the infraID.
	const imageID = ""

	useImageGallery, err := shouldUseImageGallery(cd)
	if err != nil {
		return nil, false, err
	}

	installerMachineSets, err := installazure.MachineSets(
		cd.Spec.ClusterMetadata.InfraID,
		ic,
		computePool,
		imageID,
		workerRole,
		workerUserDataName,
		capabilities,
		useImageGallery,
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

func (a *AzureActuator) gen2ImageExists(infraID, resourceGroupName string) (bool, error) {
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

func shouldUseImageGallery(cd *hivev1.ClusterDeployment) (bool, error) {
	versionString, err := getClusterVersion(cd)
	if err != nil {
		return true, fmt.Errorf("failed to get cluster semver: %w", err)
	}

	version, err := semver.ParseTolerant(versionString)
	if err != nil {
		return true, fmt.Errorf("failed to parse cluster semver: %w", err)
	}

	return versionsSupportingAzureImageGallery(version), nil
}
