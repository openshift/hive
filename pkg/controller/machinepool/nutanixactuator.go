package machinepool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	machinev1 "github.com/openshift/api/machine/v1"
	machineapi "github.com/openshift/api/machine/v1beta1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/controller/utils/nutanixutils"
	installnutanix "github.com/openshift/installer/pkg/asset/machines/nutanix"
	installertypes "github.com/openshift/installer/pkg/types"
	installertypesnutanix "github.com/openshift/installer/pkg/types/nutanix"
)

// NutanixActuator encapsulates the pieces necessary to be able to generate
// a list of MachineSets to sync to the remote cluster
type NutanixActuator struct {
	client        client.Client
	masterMachine *machineapi.Machine
}

var _ Actuator = &NutanixActuator{}

// NewNutanixActuator is the constructor for building a NutanixActuator
func NewNutanixActuator(client client.Client, masterMachine *machineapi.Machine) (*NutanixActuator, error) {
	actuator := &NutanixActuator{
		client:        client,
		masterMachine: masterMachine,
	}
	return actuator, nil
}

func decodeNutanixMachineProviderSpec(rawExtension *runtime.RawExtension) (*machinev1.NutanixMachineProviderConfig, error) {
	if rawExtension == nil {
		return &machinev1.NutanixMachineProviderConfig{}, nil
	}

	spec := new(machinev1.NutanixMachineProviderConfig)
	if err := json.Unmarshal(rawExtension.Raw, &spec); err != nil {
		return nil, fmt.Errorf("error unmarshalling providerSpec: %v", err)
	}

	return spec, nil
}

// getRHCOSImageNameFromMasterMachine retrieves the RHCOS image name from the master machine provider spec.
// Note: The machine image name is reliably set only in IPI installations. In UPI installations, this field may not be populated.
func getRHCOSImageNameFromMasterMachine(masterMachine *machineapi.Machine, cd *hivev1.ClusterDeployment, logger log.FieldLogger) (string, error) {
	providerSpec, err := decodeNutanixMachineProviderSpec(masterMachine.Spec.ProviderSpec.Value)
	if err != nil {
		return "", errors.Wrap(err, "failed to decode provider spec while retrieving RHCOS image name from master machine")
	}

	if providerSpec.Image.Name != nil && *providerSpec.Image.Name != "" {
		logger.Infof("using RHCOS image name from master machine providerSpec: %s", *providerSpec.Image.Name)
		return *providerSpec.Image.Name, nil
	}

	return "", errors.Errorf("no RHCOS image name found in provider spec for ClusterDeployment %s/%s ", cd.Namespace, cd.Name)
}

// GenerateMachineSets satisfies the Actuator interface and will take a clusterDeployment and return a list of MachineSets
// to sync to the remote cluster.
func (a *NutanixActuator) GenerateMachineSets(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool, logger log.FieldLogger) ([]*machineapi.MachineSet, bool, error) {
	if cd.Spec.ClusterMetadata == nil {
		return nil, false, errors.New("ClusterDeployment does not have cluster metadata")
	}

	if cd.Spec.Platform.Nutanix == nil {
		return nil, false, errors.New("ClusterDeployment is not for Nutanix")
	}
	if pool.Spec.Platform.Nutanix == nil {
		return nil, false, errors.New("MachinePool is not for Nutanix")
	}

	computePool := baseMachinePool(pool)

	// Handle autoscaling case: when autoscaling is enabled, pool.Spec.Replicas is nil
	// but the installer's MachineSets function needs a non-nil value to generate MachineSets
	// for each failure domain. We set it to the minimum required replicas to ensure
	// MachineSets are created, and the MachinePool controller will handle setting
	// the correct replicas later.
	if pool.Spec.Autoscaling != nil {
		numFailureDomains := len(pool.Spec.Platform.Nutanix.FailureDomains)
		if numFailureDomains == 0 {
			numFailureDomains = 1 // Default to 1 MachineSet if no failure domains
		}
		// Set replicas to ensure at least one MachineSet per failure domain is created
		minReplicas := int64(numFailureDomains)
		computePool.Replicas = &minReplicas
		logger.WithField("minReplicas", minReplicas).WithField("failureDomains", numFailureDomains).
			Info("autoscaling enabled, setting temporary replicas for MachineSet generation")
	}

	dataDisks, err := a.getNutanixDataDisks(pool, logger)
	if err != nil {
		if updateErr := a.setUnsupportedConfigurationCondition(pool, logger, "InvalidDataDisk", "data source must specify a UUID"); updateErr != nil {
			return nil, false, updateErr
		}
		return nil, false, nil
	}

	computePool.Platform.Nutanix = &installertypesnutanix.MachinePool{
		NumCPUs:           pool.Spec.Platform.Nutanix.NumCPUs,
		NumCoresPerSocket: pool.Spec.Platform.Nutanix.NumCoresPerSocket,
		MemoryMiB:         pool.Spec.Platform.Nutanix.MemoryMiB,
		OSDisk: installertypesnutanix.OSDisk{
			DiskSizeGiB: pool.Spec.Platform.Nutanix.OSDisk.DiskSizeGiB,
		},
		BootType:       pool.Spec.Platform.Nutanix.BootType,
		Project:        pool.Spec.Platform.Nutanix.Project,
		Categories:     pool.Spec.Platform.Nutanix.Categories,
		FailureDomains: pool.Spec.Platform.Nutanix.FailureDomains,
		DataDisks:      dataDisks,
		GPUs:           pool.Spec.Platform.Nutanix.GPUs,
	}

	ic := &installertypes.InstallConfig{
		Platform: installertypes.Platform{
			Nutanix: a.getNutanixPlatformInstallConfig(cd),
		},
	}

	osImage, err := getRHCOSImageNameFromMasterMachine(a.masterMachine, cd, logger)
	if err != nil {
		if updateErr := a.setUnsupportedConfigurationCondition(pool, logger, "MissingRHCOSImage", "no RHCOS image found in master machine provider spec"); updateErr != nil {
			return nil, false, updateErr
		}
		return nil, false, nil
	}

	installerMachineSets, err := installnutanix.MachineSets(
		cd.Spec.ClusterMetadata.InfraID,
		ic,
		computePool,
		osImage,
		workerRole,
		workerUserDataName,
	)
	if err != nil {
		return nil, false, errors.Wrap(err, "failed to generate machinesets")
	}

	if pool.Spec.Autoscaling != nil {
		logger.WithField("numMachineSets", len(installerMachineSets)).
			Info("generated worker machine sets for autoscaling")
	} else {
		logger.WithField("numMachineSets", len(installerMachineSets)).
			Info("generated worker machine sets")
	}

	return installerMachineSets, true, nil
}

// getNutanixPlatformInstallConfig constructs a Nutanix Platform configuration for installation.
// It gathers necessary Prism elements and subnet UUIDs from the ClusterDeployment platform.
func (a *NutanixActuator) getNutanixPlatformInstallConfig(cd *hivev1.ClusterDeployment) *installertypesnutanix.Platform {
	var platform installertypesnutanix.Platform
	platform.PrismCentral = installertypesnutanix.PrismCentral{
		Endpoint: installertypesnutanix.PrismEndpoint{
			Address: cd.Spec.Platform.Nutanix.PrismCentral.Address,
			Port:    cd.Spec.Platform.Nutanix.PrismCentral.Port,
		},
		Username: "",
		Password: "",
	}

	platform.FailureDomains, platform.PrismElements, platform.SubnetUUIDs = nutanixutils.ConvertHiveFailureDomains(
		cd.Spec.Platform.Nutanix.FailureDomains)

	return &platform
}

// getNutanixDataDisksStorageConfig retrieves the storage configuration for a given Nutanix VM data disk.
func (a *NutanixActuator) getNutanixDataDisksStorageConfig(dataDisk machinev1.NutanixVMDisk) *installertypesnutanix.StorageConfig {
	if dataDisk.StorageConfig == nil {
		return nil
	}

	storageConfig := &installertypesnutanix.StorageConfig{
		DiskMode: dataDisk.StorageConfig.DiskMode,
	}

	if dataDisk.StorageConfig.StorageContainer == nil || dataDisk.StorageConfig.StorageContainer.UUID == nil {
		return storageConfig
	}

	storageConfig.StorageContainer.UUID = *dataDisk.StorageConfig.StorageContainer.UUID
	return storageConfig
}

// getNutanixDataDisksDataSource extracts the data source reference for a Nutanix VM data disk.
func (a *NutanixActuator) getNutanixDataDisksDataSource(dataDisk machinev1.NutanixVMDisk) (*installertypesnutanix.StorageResourceReference, error) {
	if dataDisk.DataSource == nil || dataDisk.DataSource.UUID == nil {
		return nil, fmt.Errorf("data source must specify a UUID")
	}

	// Pass through UUID blindly, as MachineSets() only cares about UUID.
	return &installertypesnutanix.StorageResourceReference{
		UUID: *dataDisk.DataSource.UUID,
	}, nil
}

// getNutanixDataDisks retrieves and constructs a list of Nutanix data disks from a MachinePool specification.
func (a *NutanixActuator) getNutanixDataDisks(pool *hivev1.MachinePool, logger log.FieldLogger) ([]installertypesnutanix.DataDisk, error) {
	var dataDisks []installertypesnutanix.DataDisk
	for _, dataDisk := range pool.Spec.Platform.Nutanix.DataDisks {
		storageConfig := a.getNutanixDataDisksStorageConfig(dataDisk)
		dataSource, err := a.getNutanixDataDisksDataSource(dataDisk)
		if err != nil {
			return nil, err
		}

		disk := &installertypesnutanix.DataDisk{
			DiskSize:         dataDisk.DiskSize,
			DeviceProperties: dataDisk.DeviceProperties,
			StorageConfig:    storageConfig,
			DataSourceImage:  dataSource,
		}
		dataDisks = append(dataDisks, *disk)
	}

	logger.WithField("numDisks", len(dataDisks)).Info("found Nutanix data disks")
	return dataDisks, nil
}

func (a *NutanixActuator) setUnsupportedConfigurationCondition(pool *hivev1.MachinePool, logger log.FieldLogger, reason, message string) error {
	logger.WithField("reason", reason).Error(message)

	conds, changed := controllerutils.SetMachinePoolConditionWithChangeCheck(
		pool.Status.Conditions,
		hivev1.UnsupportedConfigurationMachinePoolCondition,
		corev1.ConditionFalse,
		reason,
		message,
		controllerutils.UpdateConditionIfReasonOrMessageChange,
	)

	if changed {
		pool.Status.Conditions = conds
		if updateErr := a.client.Status().Update(context.Background(), pool); updateErr != nil {
			logger.WithError(updateErr).Error("failed to update MachinePool status with UnsupportedConfiguration condition")
			return updateErr
		}
	}
	return nil
}
