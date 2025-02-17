package machinepool

import (
	"fmt"

	machinev1 "github.com/openshift/api/machine/v1"
	machineapi "github.com/openshift/api/machine/v1beta1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/apis/hive/v1/nutanix"
	installnutanix "github.com/openshift/installer/pkg/asset/machines/nutanix"
	installertypes "github.com/openshift/installer/pkg/types"
	installertypesnutanix "github.com/openshift/installer/pkg/types/nutanix"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NutanixActuator encapsulates the pieces necessary to be able to generate
// a list of MachineSets to sync to the remote cluster
type NutanixActuator struct {
	client client.Client
	logger log.FieldLogger
	scheme *runtime.Scheme
	//nutanixClient *nutanixclient.Client // TODO need nutanix client?
}

var _ Actuator = &NutanixActuator{}

// NewNutanixActuator is the constructor for building a NutanixActuator
func NewNutanixActuator(client client.Client, scheme *runtime.Scheme, logger log.FieldLogger) (*NutanixActuator, error) {
	//nutanixClient, err := nutanixclient.NewClient()
	//if err != nil {
	//	return nil, errors.Wrap(err, "failed to create Nutanix session")
	//}

	actuator := &NutanixActuator{
		client: client,
		scheme: scheme,
		//nutanixClient: nutanixClient,
		logger: logger,
	}

	return actuator, nil
}

func (a *NutanixActuator) getNutanixPlatformInstallConfig(cd *hivev1.ClusterDeployment) *installertypesnutanix.Platform {
	var platform installertypesnutanix.Platform
	platform.PrismCentral = installertypesnutanix.PrismCentral{
		Endpoint: installertypesnutanix.PrismEndpoint{
			Address: cd.Spec.Platform.Nutanix.PrismCentral.Endpoint.Address,
			Port:    cd.Spec.Platform.Nutanix.PrismCentral.Endpoint.Port,
		},
		Username: "",
		Password: "",
	}

	for _, pe := range cd.Spec.Platform.Nutanix.PrismElements {
		platform.PrismElements = append(platform.PrismElements, installertypesnutanix.PrismElement{
			UUID: pe.UUID,
			Endpoint: installertypesnutanix.PrismEndpoint{
				Address: pe.Endpoint.Address,
				Port:    pe.Endpoint.Port,
			},
			Name: pe.Name,
		})
	}

	platform.SubnetUUIDs = cd.Spec.Platform.Nutanix.SubnetUUIDs
	platform.FailureDomains = getNutanixInstallConfigFailureDomains(cd.Spec.Platform.Nutanix)

	return &platform
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
	dataDisks, err := a.getNutanixDataDisks(pool)
	if err != nil {
		return nil, false, err
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

	installerMachineSets, err := installnutanix.MachineSets(
		cd.Spec.ClusterMetadata.InfraID,
		ic,
		computePool,
		// With FailureDomains[0].Topology.Template set, this param is ignored.
		"HIVE_BUG!",
		workerRole,
		workerUserDataName,
	)
	if err != nil {
		return nil, false, errors.Wrap(err, "failed to generate machinesets")
	}

	return installerMachineSets, true, nil
}

func getNutanixFailureDomainsStorageResourceReference(failureDomain nutanix.FailureDomain) []installertypesnutanix.StorageResourceReference {
	var storageResourceReference []installertypesnutanix.StorageResourceReference
	for _, storageContainer := range failureDomain.StorageContainers {
		storageResourceReference = append(storageResourceReference, installertypesnutanix.StorageResourceReference{
			ReferenceName: storageContainer.ReferenceName,
			UUID:          storageContainer.UUID,
			Name:          storageContainer.Name,
		})
	}

	return storageResourceReference
}

func getNutanixFailureDomainsDataSourceImages(failureDomain nutanix.FailureDomain) []installertypesnutanix.StorageResourceReference {
	var dataSourceImages []installertypesnutanix.StorageResourceReference
	for _, dataSourceImage := range failureDomain.StorageContainers {
		dataSourceImages = append(dataSourceImages, installertypesnutanix.StorageResourceReference{
			ReferenceName: dataSourceImage.ReferenceName,
			UUID:          dataSourceImage.UUID,
			Name:          dataSourceImage.Name,
		})
	}

	return dataSourceImages
}

func getNutanixInstallConfigFailureDomains(platform *nutanix.Platform) []installertypesnutanix.FailureDomain {
	var failureDomains []installertypesnutanix.FailureDomain
	for _, failureDomain := range platform.FailureDomains {

		storageResourceReference := getNutanixFailureDomainsStorageResourceReference(failureDomain)
		dataSourceImages := getNutanixFailureDomainsDataSourceImages(failureDomain)

		failureDomains = append(failureDomains, installertypesnutanix.FailureDomain{
			Name: failureDomain.Name,
			PrismElement: installertypesnutanix.PrismElement{
				UUID: failureDomain.PrismElement.UUID,
				Endpoint: installertypesnutanix.PrismEndpoint{
					Address: failureDomain.PrismElement.Endpoint.Address,
					Port:    failureDomain.PrismElement.Endpoint.Port,
				},
				Name: failureDomain.Name,
			},
			SubnetUUIDs:       failureDomain.SubnetUUIDs,
			StorageContainers: storageResourceReference,
			DataSourceImages:  dataSourceImages,
		})
	}

	return failureDomains
}

func (a *NutanixActuator) getNutanixDataDisksStorageConfig(dataDisk machinev1.NutanixVMDisk) (*installertypesnutanix.StorageConfig, error) {
	if dataDisk.StorageConfig == nil {
		return nil, nil
	}
	storageConfig := &installertypesnutanix.StorageConfig{
		DiskMode: dataDisk.StorageConfig.DiskMode,
	}

	if dataDisk.StorageConfig.StorageContainer == nil {
		return storageConfig, nil
	}

	if dataDisk.StorageConfig.StorageContainer.Type == machinev1.NutanixIdentifierUUID {
		storageConfig.StorageContainer.UUID = *dataDisk.StorageConfig.StorageContainer.UUID
	}

	switch dataDisk.StorageConfig.StorageContainer.Type {
	case machinev1.NutanixIdentifierUUID:
		if dataDisk.StorageConfig.StorageContainer.UUID == nil {
			return nil, fmt.Errorf("no UUID found for data source type %s", dataDisk.StorageConfig.StorageContainer.Type)
		}
		storageConfig.StorageContainer.UUID = *dataDisk.StorageConfig.StorageContainer.UUID

	default:
		return nil, fmt.Errorf("unknown storage type %s", dataDisk.StorageConfig.StorageContainer.Type)
	}

	return storageConfig, nil
}

func (a *NutanixActuator) getNutanixDataDisksDataSource(dataDisk machinev1.NutanixVMDisk) (*installertypesnutanix.StorageResourceReference, error) {
	if dataDisk.DataSource == nil {
		return nil, nil
	}

	dataSource := &installertypesnutanix.StorageResourceReference{}
	switch dataDisk.DataSource.Type {
	case machinev1.NutanixIdentifierUUID:
		if dataDisk.DataSource.UUID == nil {
			return nil, fmt.Errorf("no UUID found for data source type %s", dataDisk.DataSource.Type)
		}
		dataSource.UUID = *dataDisk.DataSource.UUID

	case machinev1.NutanixIdentifierName:
		if dataDisk.DataSource.Name == nil {
			return nil, fmt.Errorf("no name found for data source type %s", dataDisk.DataSource.Type)
		}
		dataSource.Name = *dataDisk.DataSource.Name

	default:
		return nil, fmt.Errorf("unknown data type %s", dataDisk.DataSource.Type)
	}

	// TODO what to do with dataSource.ReferenceName?
	return dataSource, nil
}

func (a *NutanixActuator) getNutanixDataDisks(pool *hivev1.MachinePool) ([]installertypesnutanix.DataDisk, error) {
	var dataDisks []installertypesnutanix.DataDisk
	for _, dataDisk := range pool.Spec.Platform.Nutanix.DataDisks {
		storageConfig, err := a.getNutanixDataDisksStorageConfig(dataDisk)
		if err != nil {
			return nil, err
		}
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

	return dataDisks, nil
}
