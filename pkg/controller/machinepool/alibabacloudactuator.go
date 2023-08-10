package machinepool

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	machineapi "github.com/openshift/api/machine/v1beta1"
	alibabacloudprovider "github.com/openshift/cluster-api-provider-alibaba/pkg/apis/alibabacloudprovider/v1"
	installalibabacloud "github.com/openshift/installer/pkg/asset/machines/alibabacloud"
	installertypes "github.com/openshift/installer/pkg/types"
	installertypesalibabacloud "github.com/openshift/installer/pkg/types/alibabacloud"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/alibabaclient"
)

// AlibabaCloudActuator encapsulates the pieces necessary to be able to generate
// a list of MachineSets to sync to the remote cluster
type AlibabaCloudActuator struct {
	logger        log.FieldLogger
	alibabaClient alibabaclient.API
	imageID       string
}

var _ Actuator = &AlibabaCloudActuator{}

// NewAlibabaCloudActuator is the constructor for building an AlibabaCloudActuator
func NewAlibabaCloudActuator(alibabaCreds *corev1.Secret, region string, masterMachine *machineapi.Machine, logger log.FieldLogger) (*AlibabaCloudActuator, error) {
	alibabaClient, err := alibabaclient.NewClientFromSecret(alibabaCreds, region)
	if err != nil {
		logger.WithError(err).Warn("failed to create Alibaba cloud client with creds in clusterDeployment's secret")
		return nil, err
	}
	imageID, err := getAlibabaCloudImageID(masterMachine, logger)
	if err != nil {
		logger.WithError(err).Error("error getting image ID from master machine")
		return nil, err
	}
	actuator := &AlibabaCloudActuator{
		logger:        logger,
		alibabaClient: alibabaClient,
		imageID:       imageID,
	}
	return actuator, nil
}

// GenerateMachineSets satisfies the Actuator interface and will take a clusterDeployment and return a list of MachineSets
// to sync to the remote cluster.
func (a *AlibabaCloudActuator) GenerateMachineSets(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool, logger log.FieldLogger) ([]*machineapi.MachineSet, bool, error) {
	if cd.Spec.ClusterMetadata == nil {
		return nil, false, errors.New("ClusterDeployment does not have cluster metadata")
	}
	if cd.Spec.Platform.AlibabaCloud == nil {
		return nil, false, errors.New("ClusterDeployment is not for Alibaba Cloud")
	}
	if pool.Spec.Platform.AlibabaCloud == nil {
		return nil, false, errors.New("MachinePool is not for Alibaba Cloud")
	}

	computePool := baseMachinePool(pool)
	computePool.Platform.AlibabaCloud = &installertypesalibabacloud.MachinePool{
		ImageID:      a.imageID,
		InstanceType: pool.Spec.Platform.AlibabaCloud.InstanceType,
		Zones:        pool.Spec.Platform.AlibabaCloud.Zones,
	}

	if pool.Spec.Platform.AlibabaCloud.SystemDiskSize != 0 {
		computePool.Platform.AlibabaCloud.SystemDiskSize = pool.Spec.Platform.AlibabaCloud.SystemDiskSize
	}
	if pool.Spec.Platform.AlibabaCloud.SystemDiskCategory != "" {
		computePool.Platform.AlibabaCloud.SystemDiskCategory = installertypesalibabacloud.DiskCategory(pool.Spec.Platform.AlibabaCloud.SystemDiskCategory)
	}
	if pool.Spec.Platform.AlibabaCloud.ImageID != "" {
		computePool.Platform.AlibabaCloud.ImageID = pool.Spec.Platform.AlibabaCloud.ImageID
	}

	if len(computePool.Platform.AlibabaCloud.Zones) == 0 {
		zones, err := a.alibabaClient.GetAvailableZonesByInstanceType(pool.Spec.Platform.AlibabaCloud.InstanceType)
		if err != nil {
			return nil, false, errors.Wrap(err, "compute pool not providing list of zones and failed to fetch list of zones")
		}
		if len(zones) == 0 {
			return nil, false, fmt.Errorf("zero zones returned for region %s", cd.Spec.Platform.AlibabaCloud.Region)
		}
		computePool.Platform.AlibabaCloud.Zones = zones
	}

	// Fake an install config as we do with other actuators. We only populate what we know is needed today.
	// WARNING: changes to use more of installconfig in the MachineSets function can break here. Hopefully
	// will be caught by unit tests.
	ic := &installertypes.InstallConfig{
		Platform: installertypes.Platform{
			AlibabaCloud: &installertypesalibabacloud.Platform{
				Region: cd.Spec.Platform.AlibabaCloud.Region,
			},
		},
	}

	// resourceTags are settings available in the installconfig that we are choosing
	// to ignore for the time being. These empty settings should be updated to feed
	// from the machinepool / installconfig in the future.
	resourceTags := map[string]string{}

	// VSwitchMaps is the VSwitch and availability zone metadata indexed by VSwitch ID.
	// VSwitchIDs is the ID list of already existing VSwitches where cluster resources
	// will be created. The existing VSwitches can only be used when also using existing
	// VPC. Currently, Hive does not support installing in the existing VPC and hence
	// we keep it empty.
	vswitchMaps := map[string]string{}

	installerMachineSets, err := installalibabacloud.MachineSets(
		cd.Spec.ClusterMetadata.InfraID,
		ic,
		computePool,
		workerRole,
		workerUserDataName,
		resourceTags,
		vswitchMaps,
	)
	if err != nil {
		return nil, false, errors.Wrap(err, "failed to generate machinesets")
	}

	return installerMachineSets, true, nil
}

// Get the image ID from an existing master machine.
func getAlibabaCloudImageID(masterMachine *machineapi.Machine, logger log.FieldLogger) (string, error) {
	providerSpec, err := alibabacloudprovider.ProviderSpecFromRawExtension(masterMachine.Spec.ProviderSpec.Value)
	if err != nil {
		logger.WithError(err).Warn("cannot decode AlibabaCloudMachineProviderSpec from master machine")
		return "", errors.Wrap(err, "cannot decode AlibabaCloudMachineProviderSpec from master machine")
	}
	if providerSpec.ImageID == "" {
		logger.Warn("master machine does not have image ID set")
		return "", errors.New("master machine does not have image ID set")
	}
	imageID := providerSpec.ImageID
	logger.WithField("image", imageID).Debug("resolved image to use for new machinesets")
	return imageID, nil
}
