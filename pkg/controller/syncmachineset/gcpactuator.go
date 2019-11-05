package syncmachineset

import (
	"fmt"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	machineapi "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"

	installgcp "github.com/openshift/installer/pkg/asset/machines/gcp"
	installtypes "github.com/openshift/installer/pkg/types"
	installtypesgcp "github.com/openshift/installer/pkg/types/gcp"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
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
func NewGCPActuator(gcpCreds []byte, gcpProjectID string, logger log.FieldLogger) (*GCPActuator, error) {
	gcpClient, err := gcpclient.NewClient(gcpProjectID, gcpCreds)
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
func (a *GCPActuator) GenerateMachineSets(cd *hivev1.ClusterDeployment, ic *installtypes.InstallConfig, logger log.FieldLogger) ([]*machineapi.MachineSet, error) {
	machineSets := []*machineapi.MachineSet{}

	// get image ID for the generated machine sets
	imageID, err := a.getImageID(cd, logger)
	if err != nil {
		a.logger.WithError(err).Warn("failed to find image ID for the machine sets")
		return nil, err
	}

	regionZones := []string{}

	// create the machinesets
	for _, computePool := range ic.Compute {
		if computePool.Platform.GCP == nil {
			computePool.Platform.GCP = &installtypesgcp.MachinePool{}
		}
		if len(computePool.Platform.GCP.Zones) == 0 {
			if len(regionZones) == 0 {
				regionZones, err = a.getGCPZones(cd.Spec.Platform.GCP.Region)
				if err != nil {
					msg := "compute pool not providing list of zones and failed to fetch list of zones"
					logger.WithError(err).Error(msg)
					return nil, errors.Wrap(err, msg)
				}
				if len(regionZones) == 0 {
					msg := fmt.Sprintf("zero zones returned for region %s", cd.Spec.Platform.GCP.Region)
					logger.Error(msg)
					return nil, errors.New(msg)
				}
			}
			computePool.Platform.GCP.Zones = regionZones
		}

		installerMachineSets, err := installgcp.MachineSets(cd.Status.InfraID, ic, &computePool, imageID, computePool.Name, "worker-user-data")
		if err != nil {
			logger.WithError(err).Error("failed to generate machinesets")
			return nil, err
		}

		for _, ms := range installerMachineSets {
			machineSets = append(machineSets, ms)
		}
	}

	return machineSets, nil
}

func (a *GCPActuator) getGCPZones(region string) ([]string, error) {
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
	infra := cd.Status.InfraID

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
