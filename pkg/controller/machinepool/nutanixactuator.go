package machinepool

import (
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

func getNutanixInstallConfigFailureDomains(platform *nutanix.Platform) []installertypesnutanix.FailureDomain {
	var domains []installertypesnutanix.FailureDomain

	var failureDomains []nutanix.FailureDomain
	for _, pe := range platform.PrismElements {
		failureDomains = append(failureDomains, nutanix.FailureDomain{
			Name: "generated-failure-domain",
			PrismElement: nutanix.PrismElement{
				UUID: pe.UUID,
				Endpoint: nutanix.PrismEndpoint{
					Address: pe.Endpoint.Address,
					Port:    pe.Endpoint.Port,
				},
				Name: pe.Name,
			},
			SubnetUUIDs: platform.SubnetUUIDs,
			//StorageContainers: platform,
			//DataSourceImages:  platform.,
		})
	}

	for _, fd := range failureDomains {
		domain := installertypesnutanix.FailureDomain{
			Name: fd.Name,
			PrismElement: installertypesnutanix.PrismElement{
				UUID: fd.PrismElement.UUID,
				Endpoint: installertypesnutanix.PrismEndpoint{
					Address: fd.PrismElement.Endpoint.Address,
					Port:    fd.PrismElement.Endpoint.Port,
				},
				Name: fd.PrismElement.Name,
			},
			SubnetUUIDs:       fd.SubnetUUIDs,
			StorageContainers: nil,
			DataSourceImages:  nil,
		}

		for _, storage := range fd.StorageContainers {
			domain.StorageContainers = append(domain.StorageContainers, installertypesnutanix.StorageResourceReference{
				ReferenceName: storage.ReferenceName,
				UUID:          storage.UUID,
				Name:          storage.Name,
			})
		}

		for _, dsImages := range fd.DataSourceImages {
			domain.DataSourceImages = append(domain.DataSourceImages, installertypesnutanix.StorageResourceReference{
				ReferenceName: dsImages.ReferenceName,
				UUID:          dsImages.UUID,
				Name:          dsImages.Name,
			})
		}
	}

	return domains
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

	//var dataDisks []installertypesnutanix.DataDisk
	//for _, dataDisk := range pool.Spec.Platform.Nutanix.DataDisks {
	//	disk := &installertypesnutanix.DataDisk{
	//		DiskSize:         dataDisk.DiskSize,
	//		DeviceProperties: dataDisk.DeviceProperties,
	//		StorageConfig: &installertypesnutanix.StorageConfig{
	//			DiskMode: dataDisk.StorageConfig.DiskMode,
	//			StorageContainer: &installertypesnutanix.StorageResourceReference{
	//				ReferenceName: dataDisk.StorageConfig.StorageContainer.ReferenceName,
	//				UUID:          dataDisk.StorageConfig.StorageContainer.UUID,
	//				Name:          dataDisk.StorageConfig.StorageContainer.Name,
	//			},
	//		},
	//		DataSourceImage: &installertypesnutanix.StorageResourceReference{
	//			ReferenceName: dataDisk.DataSourceImage.ReferenceName,
	//			UUID:          dataDisk.DataSourceImage.UUID,
	//			Name:          dataDisk.DataSourceImage.Name,
	//		},
	//	}
	//	dataDisks = append(dataDisks, *disk)
	//}

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
		FailureDomains: []string{}, // TODO
	}
	//GPUs:           pool.Spec.Platform.Nutanix.GPUs,
	//DataDisks:      dataDisks,
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
