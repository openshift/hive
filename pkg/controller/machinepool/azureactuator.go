package machinepool

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/go-autorest/autorest/to"
	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	capiv1 "sigs.k8s.io/cluster-api/api/v1alpha4"

	installazure "github.com/openshift/installer/pkg/asset/machines/azure"
	installertypes "github.com/openshift/installer/pkg/types"
	installertypesazure "github.com/openshift/installer/pkg/types/azure"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/azureclient"
)

// AzureActuator encapsulates the pieces necessary to be able to generate
// a list of MachineSets to sync to the remote cluster.
type AzureActuator struct {
	client azureclient.Client
	logger log.FieldLogger
}

var _ Actuator = &AzureActuator{}

// NewAzureActuator is the constructor for building a AzureActuator
func NewAzureActuator(azureCreds *corev1.Secret, logger log.FieldLogger) (*AzureActuator, error) {
	azureClient, err := azureclient.NewClientFromSecret(azureCreds)
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

// GenerateCAPIMachineSets takes a clusterDeployment and returns a list of upstream CAPI MachineSets
func (a *AzureActuator) GenerateCAPIMachineSets(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool, logger log.FieldLogger) ([]*capiv1.MachineSet, bool, error) {
	machinesets := []*capiv1.MachineSet{}
	return machinesets, true, nil
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

	ic := &installertypes.InstallConfig{
		Platform: installertypes.Platform{
			Azure: &installertypesazure.Platform{
				Region: cd.Spec.Platform.Azure.Region,
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

	installerMachineSets, err := installazure.MachineSets(
		cd.Spec.ClusterMetadata.InfraID,
		ic,
		computePool,
		imageID,
		workerRole,
		workerUserDataName,
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
