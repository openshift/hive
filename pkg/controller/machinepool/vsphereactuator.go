package machinepool

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/runtime"

	machineapi "github.com/openshift/api/machine/v1beta1"
	installvsphere "github.com/openshift/installer/pkg/asset/machines/vsphere"
	installertypes "github.com/openshift/installer/pkg/types"
	installertypesvsphere "github.com/openshift/installer/pkg/types/vsphere"
	vsphereutil "github.com/openshift/machine-api-operator/pkg/controller/vsphere"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

// VSphereActuator encapsulates the pieces necessary to be able to generate
// a list of MachineSets to sync to the remote cluster
type VSphereActuator struct {
	logger  log.FieldLogger
	osImage string
}

var _ Actuator = &VSphereActuator{}

// NewVSphereActuator is the constructor for building a VSphereActuator
func NewVSphereActuator(masterMachine *machineapi.Machine, scheme *runtime.Scheme, logger log.FieldLogger) (*VSphereActuator, error) {
	osImage, err := getVSphereOSImage(masterMachine, scheme, logger)
	if err != nil {
		logger.WithError(err).Error("error getting os image from master machine")
		return nil, err
	}
	actuator := &VSphereActuator{
		logger:  logger,
		osImage: osImage,
	}
	return actuator, nil
}

// GenerateMachineSets satisfies the Actuator interface and will take a clusterDeployment and return a list of MachineSets
// to sync to the remote cluster.
func (a *VSphereActuator) GenerateMachineSets(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool, logger log.FieldLogger) ([]*machineapi.MachineSet, bool, error) {
	if cd.Spec.ClusterMetadata == nil {
		return nil, false, errors.New("ClusterDeployment does not have cluster metadata")
	}
	if cd.Spec.Platform.VSphere == nil {
		return nil, false, errors.New("ClusterDeployment is not for VSphere")
	}
	if pool.Spec.Platform.VSphere == nil {
		return nil, false, errors.New("MachinePool is not for VSphere")
	}

	computePool := baseMachinePool(pool)
	computePool.Platform.VSphere = &installertypesvsphere.MachinePool{
		NumCPUs:           pool.Spec.Platform.VSphere.NumCPUs,
		NumCoresPerSocket: pool.Spec.Platform.VSphere.NumCoresPerSocket,
		MemoryMiB:         pool.Spec.Platform.VSphere.MemoryMiB,
		OSDisk: installertypesvsphere.OSDisk{
			DiskSizeGB: pool.Spec.Platform.VSphere.DiskSizeGB,
		},
	}

	// Fake an install config as we do with other actuators. We only populate what we know is needed today.
	// WARNING: changes to use more of installconfig in the MachineSets function can break here. Hopefully
	// will be caught by unit tests.
	ic := &installertypes.InstallConfig{
		Platform: installertypes.Platform{
			VSphere: &installertypesvsphere.Platform{
				VCenters: []installertypesvsphere.VCenter{
					{
						Server:      cd.Spec.Platform.VSphere.VCenter,
						Port:        443,
						Username:    "",
						Password:    "",
						Datacenters: []string{cd.Spec.Platform.VSphere.Datacenter},
					},
				},
				FailureDomains: []installertypesvsphere.FailureDomain{
					{
						Name:   "generated-failure-domain",
						Region: "generated-region",
						Zone:   "generated-zone",
						Server: cd.Spec.Platform.VSphere.VCenter,
						Topology: installertypesvsphere.Topology{
							Datacenter:     cd.Spec.Platform.VSphere.Datacenter,
							Datastore:      setDatastorePath(cd.Spec.Platform.VSphere.DefaultDatastore, cd.Spec.Platform.VSphere.Datacenter, logger),
							Folder:         setFolderPath(cd.Spec.Platform.VSphere.Folder, cd.Spec.Platform.VSphere.Datacenter, logger),
							ComputeCluster: setComputeClusterPath(cd.Spec.Platform.VSphere.Cluster, cd.Spec.Platform.VSphere.Datacenter, logger),
							Networks:       []string{cd.Spec.Platform.VSphere.Network},
							Template:       a.osImage,
						},
					},
				},
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

// Get the OS image from an existing master machine.
func getVSphereOSImage(masterMachine *machineapi.Machine, scheme *runtime.Scheme, logger log.FieldLogger) (string, error) {
	providerSpec, err := vsphereutil.ProviderSpecFromRawExtension(masterMachine.Spec.ProviderSpec.Value)
	if err != nil {
		logger.WithError(err).Warn("cannot decode VSphereMachineProviderSpec from master machine")
		return "", errors.Wrap(err, "cannot decode VSphereMachineProviderSpec from master machine")
	}
	osImage := providerSpec.Template
	logger.WithField("image", osImage).Debug("resolved image to use for new machinesets")
	return osImage, nil
}

// Copied from https://github.com/openshift/installer/blob/f7731922a0f17a8339a3e837f72898ac77643611/pkg/types/vsphere/conversion/installconfig.go#L75-L97

func setComputeClusterPath(cluster, datacenter string, logger log.FieldLogger) string {
	if cluster != "" && !strings.HasPrefix(cluster, "/") {
		logger.Warn("computeCluster as a non-path is now depreciated please use the form: /%s/host/%s", datacenter, cluster)
		return fmt.Sprintf("/%s/host/%s", datacenter, cluster)
	}
	return cluster
}

func setDatastorePath(datastore, datacenter string, logger log.FieldLogger) string {
	if datastore != "" && !strings.HasPrefix(datastore, "/") {
		logger.Warn("datastore as a non-path is now depreciated please use the form: /%s/datastore/%s", datacenter, datastore)
		return fmt.Sprintf("/%s/datastore/%s", datacenter, datastore)
	}
	return datastore
}

func setFolderPath(folder, datacenter string, logger log.FieldLogger) string {
	if folder != "" && !strings.HasPrefix(folder, "/") {
		logger.Warn("folder as a non-path is now depreciated please use the form: /%s/vm/%s", datacenter, folder)
		return fmt.Sprintf("/%s/vm/%s", datacenter, folder)
	}
	return folder
}
