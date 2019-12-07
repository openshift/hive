package remotemachineset

import (
	"fmt"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	machineapi "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"

	installgcp "github.com/openshift/installer/pkg/asset/machines/gcp"
	installertypes "github.com/openshift/installer/pkg/types"
	installertypesgcp "github.com/openshift/installer/pkg/types/gcp"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/gcpclient"
)

// GCPActuator encapsulates the pieces necessary to be able to generate
// a list of MachineSets to sync to the remote cluster.
type GCPActuator struct {
	client gcpclient.Client
	logger log.FieldLogger
}

var _ Actuator = &GCPActuator{}

// NewGCPActuator is the constructor for building a GCPActuator
func NewGCPActuator(gcpCreds *corev1.Secret, logger log.FieldLogger) (*GCPActuator, error) {
	gcpClient, err := gcpclient.NewClientFromSecret(gcpCreds)
	if err != nil {
		logger.WithError(err).Warn("failed to create GCP client with creds in clusterDeployment's secret")
		return nil, err
	}
	actuator := &GCPActuator{
		client: gcpClient,
		logger: logger,
	}
	return actuator, nil
}

// GenerateMachineSets satisfies the Actuator interface and will take a clusterDeployment and return a list of MachineSets
// to sync to the remote cluster.
func (a *GCPActuator) GenerateMachineSets(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool, logger log.FieldLogger) ([]*machineapi.MachineSet, error) {
	if cd.Spec.ClusterMetadata == nil {
		return nil, errors.New("ClusterDeployment does not have cluster metadata")
	}
	if cd.Spec.Platform.GCP == nil {
		return nil, errors.New("ClusterDeployment is not for GCP")
	}
	if pool.Spec.Platform.GCP == nil {
		return nil, errors.New("MachinePool is not for GCP")
	}

	ic := &installertypes.InstallConfig{
		Platform: installertypes.Platform{
			GCP: &installertypesgcp.Platform{
				Region: cd.Spec.Platform.GCP.Region,
			},
		},
	}

	computePool := baseMachinePool(pool)
	computePool.Platform.GCP = &installertypesgcp.MachinePool{
		Zones:        pool.Spec.Platform.GCP.Zones,
		InstanceType: pool.Spec.Platform.GCP.InstanceType,
	}

	// get image ID for the generated machine sets
	imageID, err := a.getImageID(cd, logger)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find image ID for the machine sets")
	}

	if len(computePool.Platform.GCP.Zones) == 0 {
		zones, err := a.getZones(cd.Spec.Platform.GCP.Region)
		if err != nil {
			return nil, errors.Wrap(err, "compute pool not providing list of zones and failed to fetch list of zones")
		}
		if len(zones) == 0 {
			return nil, fmt.Errorf("zero zones returned for region %s", cd.Spec.Platform.GCP.Region)
		}
		computePool.Platform.GCP.Zones = zones
	}

	installerMachineSets, err := installgcp.MachineSets(cd.Spec.ClusterMetadata.InfraID, ic, computePool, imageID, pool.Spec.Name, "worker-user-data")
	return installerMachineSets, errors.Wrap(err, "failed to generate machinesets")
}

func (a *GCPActuator) getZones(region string) ([]string, error) {
	zones := []string{}

	// Filter to regions matching '.*<region>.*' (where the zone is actually UP)
	zoneFilter := fmt.Sprintf("(region eq '.*%s.*') (status eq UP)", region)

	pageToken := ""

	for {
		zoneList, err := a.client.ListComputeZones(gcpclient.ListComputeZonesOptions{
			Filter:    zoneFilter,
			PageToken: pageToken,
		})
		if err != nil {
			return zones, err
		}

		for _, zone := range zoneList.Items {
			zones = append(zones, zone.Name)
		}

		if zoneList.NextPageToken == "" {
			break
		}
		pageToken = zoneList.NextPageToken
	}

	return zones, nil
}

func (a *GCPActuator) getImageID(cd *hivev1.ClusterDeployment, logger log.FieldLogger) (string, error) {
	infra := cd.Spec.ClusterMetadata.InfraID

	// find names of the form '<infra>-.*'
	filter := fmt.Sprintf("name eq \"%s-.*\"", infra)
	result, err := a.client.ListComputeImages(gcpclient.ListComputeImagesOptions{Filter: filter})
	if err != nil {
		logger.WithError(err).Warnf("failed to find a GCP image starting with name: %s", infra)
		return "", err
	}
	switch len(result.Items) {
	case 0:
		msg := fmt.Sprintf("found 0 results searching for GCP image starting with name: %s", infra)
		logger.Warnf(msg)
		return "", errors.New(msg)
	case 1:
		logger.Debugf("using image with name %s for machine sets", result.Items[0].Name)
		return result.Items[0].Name, nil
	default:
		msg := fmt.Sprintf("unexpected number of results when looking for GCP image with name starting with %s", infra)
		logger.Warnf(msg)
		return "", errors.New(msg)
	}
}
