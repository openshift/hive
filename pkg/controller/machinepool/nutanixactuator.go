package machinepool

import (
	"fmt"
	machinev1 "github.com/openshift/api/machine/v1"
	machineapi "github.com/openshift/api/machine/v1beta1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	installvsphere "github.com/openshift/installer/pkg/asset/machines/vsphere"
	installertypes "github.com/openshift/installer/pkg/types"
	installertypesnutanix "github.com/openshift/installer/pkg/types/nutanix"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NutanixActuator encapsulates the pieces necessary to be able to generate
// a list of MachineSets to sync to the remote cluster
type NutanixActuator struct {
	client client.Client
	logger log.FieldLogger
	scheme *runtime.Scheme
	//nutanixClient *nutanixclient.Client
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

	var disks []installertypesnutanix.DataDisk
	for deviceIndex, disk := range pool.Spec.Platform.Nutanix.Disks {
		disks = append(disks, installertypesnutanix.DataDisk{
			DiskSize: resource.MustParse(fmt.Sprintf("%db", disk.DiskSizeBytes)),
			DeviceProperties: &machinev1.NutanixVMDiskDeviceProperties{
				DeviceType:  disk.DeviceType,
				AdapterType: disk.AdapterType,
				DeviceIndex: int32(deviceIndex),
			},
			StorageConfig:   nil,
			DataSourceImage: nil,
		})
	}

	computePool.Platform.Nutanix = &installertypesnutanix.MachinePool{
		NumCPUs:           pool.Spec.Platform.Nutanix.NumSockets,
		NumCoresPerSocket: pool.Spec.Platform.Nutanix.NumVcpusPerSocket,
		MemoryMiB:         pool.Spec.Platform.Nutanix.MemorySizeMiB,
		BootType:          pool.Spec.Platform.Nutanix.BootType,
		DataDisks:         disks,
	}

	ic := &installertypes.InstallConfig{
		Platform: installertypes.Platform{
			Nutanix: &installertypesnutanix.Platform{
				PrismCentral: installertypesnutanix.PrismCentral{
					Endpoint: installertypesnutanix.PrismEndpoint{
						Address: cd.Spec.Platform.Nutanix.Endpoint,
						Port:    cd.Spec.Platform.Nutanix.Port,
					},
					Username: "",
					Password: "",
				},
				PrismElements:  nil,
				IngressVIPs:    nil,
				SubnetUUIDs:    []string{cd.Spec.Platform.Nutanix.Subnet},
				FailureDomains: nil,
			},
		},
	}

	installerMachineSets, err := installvsphere.MachineSets(
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
