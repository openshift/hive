package machinepool

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	configv1 "github.com/openshift/api/config/v1"
	machineapi "github.com/openshift/api/machine/v1beta1"
	installibmcloud "github.com/openshift/installer/pkg/asset/machines/ibmcloud"
	installertypes "github.com/openshift/installer/pkg/types"
	installertypesibmcloud "github.com/openshift/installer/pkg/types/ibmcloud"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	msop "github.com/openshift/hive/pkg/controller/machinesetwithopflags"
	"github.com/openshift/hive/pkg/ibmclient"
)

// IBMCloudActuator encapsulates the pieces necessary to be able to generate
// a list of MachineSets to sync to the remote cluster
type IBMCloudActuator struct {
	logger    log.FieldLogger
	ibmClient ibmclient.API
}

var _ Actuator = &IBMCloudActuator{}

// NewIBMCloudActuator is the constructor for building an IBMCloudActuator
func NewIBMCloudActuator(ibmCreds *corev1.Secret, scheme *runtime.Scheme, logger log.FieldLogger) (*IBMCloudActuator, error) {
	ibmClient, err := ibmclient.NewClientFromSecret(ibmCreds)
	if err != nil {
		logger.WithError(err).Warn("failed to create IBM client with creds in clusterDeployment's secret")
		return nil, err
	}
	actuator := &IBMCloudActuator{
		logger:    logger,
		ibmClient: ibmClient,
	}
	return actuator, nil
}

func (a *IBMCloudActuator) GetRemoteMachineSetsWithOpFlags(pool *hivev1.MachinePool, remoteMachineSets *machineapi.MachineSetList, infrastructure *configv1.Infrastructure, logger log.FieldLogger) ([]msop.MachineSetWithOpFlags, error) {
	remotes_to_update := []msop.MachineSetWithOpFlags{}
	for _, rMS := range remoteMachineSets.Items {
		remotes_to_update = append(remotes_to_update, msop.MachineSetWithOpFlags{&rMS, false, false})
	}
	return remotes_to_update, nil
}

// GenerateMachineSets satisfies the Actuator interface and will take a clusterDeployment and return a list of MachineSets
// to sync to the remote cluster.
func (a *IBMCloudActuator) GenerateMachineSets(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool, logger log.FieldLogger) ([]*machineapi.MachineSet, bool, error) {
	if cd.Spec.ClusterMetadata == nil {
		return nil, false, errors.New("ClusterDeployment does not have cluster metadata")
	}
	if cd.Spec.Platform.IBMCloud == nil {
		return nil, false, errors.New("ClusterDeployment is not for IBMCloud")
	}
	if pool.Spec.Platform.IBMCloud == nil {
		return nil, false, errors.New("MachinePool is not for IBMCloud")
	}

	computePool := baseMachinePool(pool)
	computePool.Platform.IBMCloud = &installertypesibmcloud.MachinePool{
		InstanceType: pool.Spec.Platform.IBMCloud.InstanceType,
		Zones:        pool.Spec.Platform.IBMCloud.Zones,
	}

	if pool.Spec.Platform.IBMCloud.DedicatedHosts != nil {
		dedicatedHosts := []installertypesibmcloud.DedicatedHost{}
		for _, host := range pool.Spec.Platform.IBMCloud.DedicatedHosts {
			dedicatedHosts = append(dedicatedHosts, installertypesibmcloud.DedicatedHost{
				Name:    host.Name,
				Profile: host.Profile,
			})
		}
		computePool.Platform.IBMCloud.DedicatedHosts = dedicatedHosts
	}

	if pool.Spec.Platform.IBMCloud.BootVolume != nil {
		computePool.Platform.IBMCloud.BootVolume = &installertypesibmcloud.BootVolume{
			EncryptionKey: pool.Spec.Platform.IBMCloud.BootVolume.EncryptionKey,
		}
	}

	if len(computePool.Platform.IBMCloud.Zones) == 0 {
		zones, err := a.ibmClient.GetVPCZonesForRegion(context.TODO(), cd.Spec.Platform.IBMCloud.Region)
		if err != nil {
			return nil, false, errors.Wrap(err, "compute pool not providing list of zones and failed to fetch list of zones")
		}
		if len(zones) == 0 {
			return nil, false, fmt.Errorf("zero zones returned for region %s", cd.Spec.Platform.IBMCloud.Region)
		}
		computePool.Platform.IBMCloud.Zones = zones
	}

	// Fake an install config as we do with other actuators. We only populate what we know is needed today.
	// WARNING: changes to use more of installconfig in the MachineSets function can break here. Hopefully
	// will be caught by unit tests.
	ic := &installertypes.InstallConfig{
		Platform: installertypes.Platform{
			IBMCloud: &installertypesibmcloud.Platform{
				Region: cd.Spec.Platform.IBMCloud.Region,
			},
		},
	}

	installerMachineSets, err := installibmcloud.MachineSets(
		cd.Spec.ClusterMetadata.InfraID,
		ic,

		// A default empty map will work fine here, see:
		// vendor/github.com/openshift/installer/pkg/asset/machines/ibmcloud/machines.go:148
		make(map[string]string),

		computePool,
		workerRole,
		workerUserDataName,
	)
	if err != nil {
		return nil, false, errors.Wrap(err, "failed to generate machinesets")
	}

	return installerMachineSets, true, nil
}
