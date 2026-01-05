package hibernation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/openstack/imageservice/v2/images"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/openstackclient"
	"github.com/pkg/errors"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func init() {
	RegisterActuator(&openstackActuator{openstackClientFn: getOpenStackClient})
}

// openstackActuator implements HibernationActuator for OpenStack
type openstackActuator struct {
	openstackClientFn func(*hivev1.ClusterDeployment, client.Client, log.FieldLogger) (openstackclient.Client, error)
}

var _ HibernationActuator = &openstackActuator{}

// OpenStackInstanceConfig stores instance configuration for restoration
type OpenStackInstanceConfig struct {
	Name               string            `json:"name"`
	Flavor             string            `json:"flavor"`
	PortID             string            `json:"portID"`
	SnapshotID         string            `json:"snapshotID"`
	SnapshotName       string            `json:"snapshotName"`
	SecurityGroups     []string          `json:"securityGroups"`
	ClusterID          string            `json:"clusterID"`
	NetworkID          string            `json:"networkID"`
	OpenshiftClusterID string            `json:"openshiftClusterID"`
	Metadata           map[string]string `json:"metadata"`
	Tags               []string          `json:"tags"`
}

// Create API client
func getOpenStackClient(cd *hivev1.ClusterDeployment, c client.Client, logger log.FieldLogger) (openstackclient.Client, error) {
	if cd.Spec.Platform.OpenStack == nil || cd.Spec.Platform.OpenStack.CredentialsSecretRef.Name == "" {
		return nil, errors.New("no OpenStack credentials secret reference found in ClusterDeployment")
	}

	secretName := cd.Spec.Platform.OpenStack.CredentialsSecretRef.Name
	secretNamespace := cd.Namespace

	secret := &corev1.Secret{}
	err := c.Get(context.Background(), types.NamespacedName{
		Name:      secretName,
		Namespace: secretNamespace,
	}, secret)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get credentials secret %s/%s", secretNamespace, secretName)
	}

	return openstackclient.NewClientFromSecret(secret)
}

// Return true if this actuator can handle the given ClusterDeployment
func (a *openstackActuator) CanHandle(cd *hivev1.ClusterDeployment) bool {
	return cd.Spec.Platform.OpenStack != nil
}

// StopMachines
func (a *openstackActuator) StopMachines(cd *hivev1.ClusterDeployment, hiveClient client.Client, logger log.FieldLogger) error {
	logger = logger.WithField("cloud", "openstack")
	logger.Info("stopping machines")

	openstackClient, err := a.openstackClientFn(cd, hiveClient, logger)
	if err != nil {
		return errors.Wrap(err, "failed to create OpenStack client")
	}

	infraID := cd.Spec.ClusterMetadata.InfraID

	// Step 1: Find instances
	matchingServers, err := a.findInstancesByInfraID(openstackClient, infraID)
	if err != nil {
		return errors.Wrap(err, "error finding instances")
	}

	if len(matchingServers) == 0 {
		logger.Info("no instances found - cluster already hibernated")
		return nil
	}

	logger.WithField("count", len(matchingServers)).Info("found instances to hibernate")

	// Step 2: Validate instance states
	if err := a.validateInstanceStates(matchingServers, logger); err != nil {
		return err
	}

	// Step 3: Pause all instances
	if err := a.pauseInstances(openstackClient, matchingServers, logger); err != nil {
		return err
	}

	// Step 4: Create snapshots
	snapshotMapping, err := a.createSnapshots(openstackClient, matchingServers, logger)
	if err != nil {
		return err
	}

	// Step 5: Wait for snapshots to complete
	if err := a.waitForSnapshots(openstackClient, snapshotMapping, logger); err != nil {
		return err
	}

	// Step 6: Save instance configuration
	if err := a.saveInstanceConfiguration(cd, hiveClient, openstackClient, matchingServers, snapshotMapping, logger); err != nil {
		return errors.Wrap(err, "error saving configuration")
	}

	// Step 7: Delete instances
	if err := a.deleteInstances(openstackClient, matchingServers, logger); err != nil {
		return err
	}

	// Step 8: Wait for cleanup
	if err := a.waitForInstanceCleanup(openstackClient, infraID, logger); err != nil {
		return err
	}

	// Step 9: Best-effort cleanup of old snapshots
	if err := a.cleanupOldSnapshots(cd, hiveClient, openstackClient, infraID, snapshotMapping, logger); err != nil {
		logger.Warnf("failed to cleanup old snapshots: %v", err)
	}

	logger.Info("hibernation completed successfully")
	return nil
}

// StartMachines
func (a *openstackActuator) StartMachines(cd *hivev1.ClusterDeployment, hiveClient client.Client, logger log.FieldLogger) error {
	logger = logger.WithField("cloud", "openstack")
	logger.Info("starting machines from snapshots")

	// Only proceed if PowerState is Running
	if cd.Spec.PowerState != hivev1.ClusterPowerStateRunning {
		logger.WithField("power_state", cd.Spec.PowerState).Info("PowerState is not Running - refusing to start machines")
		return nil
	}

	openstackClient, err := a.openstackClientFn(cd, hiveClient, logger)
	if err != nil {
		return errors.Wrap(err, "failed to create OpenStack client")
	}

	// Load hibernation config
	instances, err := a.loadHibernationConfig(cd, hiveClient, logger)
	if err != nil {
		logger.Warnf("no hibernation config found: %v", err)

		// Check if we have existing instances
		infraID := cd.Spec.ClusterMetadata.InfraID
		existingServers, checkErr := a.findInstancesByInfraID(openstackClient, infraID)
		if checkErr != nil {
			logger.Warnf("could not check existing instances: %v", checkErr)
		} else if len(existingServers) > 0 {
			logger.Info("instances exist but no hibernation config - clearing any state")
			_ = a.deleteHibernationConfig(cd, hiveClient, logger)
		} else {
			logger.Warn("cannot recreate instances without hibernation snapshots")
		}
		return nil
	}

	// Check for existing instances
	infraID := cd.Spec.ClusterMetadata.InfraID
	existingServers, err := a.findInstancesByInfraID(openstackClient, infraID)
	if err != nil {
		logger.Warnf("could not check existing instances: %v", err)
		existingServers = []*servers.Server{}
	}

	// Check if we already have all instances
	if len(existingServers) >= len(instances) {
		logger.Info("sufficient instances already exist - clearing hibernation config")
		_ = a.deleteHibernationConfig(cd, hiveClient, logger)
		return nil
	}

	logger.Infof("restoring %d instances from hibernation (currently have %d)", len(instances), len(existingServers))

	// Figure out what needs to be created
	instancesToCreate := a.findMissingInstances(instances, existingServers, logger)

	if len(instancesToCreate) > 0 {
		logger.WithField("count", len(instancesToCreate)).Info("creating missing instances")

		// Create missing instances
		if err := a.createMissingInstances(openstackClient, instancesToCreate, logger); err != nil {
			logger.Errorf("some instance creation failed: %v", err)
		}
	}

	// Wait for all instances to be active
	if err := a.waitForAllInstancesToBeActive(openstackClient, infraID, len(instances), logger); err != nil {
		return errors.Wrap(err, "not all instances are active yet")
	}

	// Restore tags after instances are active
	if err := a.restoreInstanceTags(openstackClient, instances, logger); err != nil {
		logger.Warnf("failed to restore some tags: %v", err)
	}

	// Cleanup hibernation snapshots
	logger.Info("cleaning up hibernation snapshots after successful restoration")
	if err := a.cleanupRestorationSnapshots(openstackClient, instances, logger); err != nil {
		logger.Warnf("failed to cleanup some snapshots: %v", err)
	}

	// Clear hibernation config
	logger.Info("all instances confirmed active - clearing hibernation configuration")
	return a.deleteHibernationConfig(cd, hiveClient, logger)
}

// MachinesRunning checks if machines are running
func (a *openstackActuator) MachinesRunning(cd *hivev1.ClusterDeployment, hiveClient client.Client, logger log.FieldLogger) (bool, []string, error) {
	logger = logger.WithField("cloud", "openstack")
	logger.Info("checking if machines are running")

	openstackClient, err := a.openstackClientFn(cd, hiveClient, logger)
	if err != nil {
		return false, nil, errors.Wrap(err, "failed to create OpenStack client")
	}

	infraID := cd.Spec.ClusterMetadata.InfraID
	matchingServers, err := a.findInstancesByInfraID(openstackClient, infraID)
	if err != nil {
		return false, nil, errors.Wrap(err, "error finding instances")
	}

	logger.Infof("found %d instances with infraID '%s'", len(matchingServers), infraID)

	if len(matchingServers) == 0 {
		logger.Info("no instances found - machines are not running")
		return false, []string{"no instances found"}, nil
	}

	// Check actual instance states
	var notRunningInstances []string
	for _, server := range matchingServers {
		status := strings.ToLower(server.Status)
		if status != "active" && status != "paused" {
			notRunningInstances = append(notRunningInstances, server.Name)
		}
	}

	if len(notRunningInstances) > 0 {
		logger.Infof("found non-running instances: %v", notRunningInstances)
		return false, notRunningInstances, nil
	}

	return true, []string{}, nil
}

// MachinesStopped checks if machines are stopped
func (a *openstackActuator) MachinesStopped(cd *hivev1.ClusterDeployment, hiveClient client.Client, logger log.FieldLogger) (bool, []string, error) {
	logger = logger.WithField("cloud", "openstack")
	logger.Info("checking if machines are stopped")

	openstackClient, err := a.openstackClientFn(cd, hiveClient, logger)
	if err != nil {
		return false, nil, errors.Wrap(err, "failed to create OpenStack client")
	}

	infraID := cd.Spec.ClusterMetadata.InfraID
	matchingServers, err := a.findInstancesByInfraID(openstackClient, infraID)
	if err != nil {
		return false, nil, errors.Wrap(err, "error finding instances")
	}

	if len(matchingServers) == 0 {
		logger.Info("no instances found - machines are stopped")
		return true, nil, nil
	}

	// Instances still exist
	var notStopped []string
	for _, server := range matchingServers {
		notStopped = append(notStopped, server.Name)
	}

	logger.Infof("found %d instances still existing", len(notStopped))
	return false, notStopped, nil
}

// Validate all instances
func (a *openstackActuator) validateInstanceStates(servers []*servers.Server, logger log.FieldLogger) error {
	var errs []error

	for _, server := range servers {
		serverLogger := logger.WithFields(log.Fields{
			"instance": server.Name,
			"status":   server.Status,
		})

		serverLogger.Info("validating instance status")

		// Check for deleting states that would cause conflicts
		if strings.Contains(strings.ToLower(server.Status), "delet") {
			errs = append(errs, errors.Errorf("instance %s is being deleted by another process", server.Name))
		}

		// Warn about non-ACTIVE states
		if server.Status != "ACTIVE" && server.Status != "PAUSED" {
			serverLogger.Warn("instance status is not ACTIVE - operation may proceed but could fail")
		}
	}

	return utilerrors.NewAggregate(errs)
}

// Pause instances
func (a *openstackActuator) pauseInstances(openstackClient openstackclient.Client, servers []*servers.Server, logger log.FieldLogger) error {
	var errs []error

	for _, server := range servers {
		serverLogger := logger.WithField("instance", server.Name)

		// Check current status first
		current, err := openstackClient.GetServer(context.Background(), server.ID)
		if err != nil {
			if isNotFoundError(err) {
				serverLogger.Info("instance not found, skipping pause")
				continue
			}
			errs = append(errs, errors.Wrapf(err, "failed to get status of %s", server.Name))
			continue
		}

		if current.Status == "PAUSED" {
			serverLogger.Info("instance already paused")
			continue
		}

		serverLogger.Info("pausing instance")
		if err := openstackClient.PauseServer(context.Background(), server.ID); err != nil {
			// Check if it's a conflict (already paused)
			if strings.Contains(err.Error(), "409") {
				serverLogger.Info("instance already paused (409)")
				continue
			}
			errs = append(errs, errors.Wrapf(err, "failed to pause %s", server.Name))
		}
	}

	return utilerrors.NewAggregate(errs)
}

// Snapshot creation
func (a *openstackActuator) createSnapshots(openstackClient openstackclient.Client, servers []*servers.Server, logger log.FieldLogger) (map[string]string, error) {
	snapshotMapping := make(map[string]string)
	timestamp := time.Now().UTC().Format("20060102-150405")
	var errs []error

	for _, server := range servers {
		snapshotName := fmt.Sprintf("%s-hibernation-%s", server.Name, timestamp)
		serverLogger := logger.WithFields(log.Fields{
			"instance":      server.Name,
			"snapshot_name": snapshotName,
		})

		// Check if snapshot already exists
		existing, err := a.findSnapshotByName(openstackClient, snapshotName)
		if err == nil && existing != nil {
			serverLogger.WithField("snapshot_id", existing.ID).Info("snapshot already exists")
			snapshotMapping[server.ID] = existing.ID
			continue
		}

		serverLogger.Info("creating snapshot")
		snapshotID, err := openstackClient.CreateServerSnapshot(context.Background(), server.ID, snapshotName)
		if err != nil {
			if strings.Contains(err.Error(), "409") || strings.Contains(err.Error(), "task_state deleting") {
				errs = append(errs, errors.Errorf("conflict creating snapshot for %s - instance being modified", server.Name))
			} else {
				errs = append(errs, errors.Wrapf(err, "failed to create snapshot for %s", server.Name))
			}
			continue
		}

		snapshotMapping[server.ID] = snapshotID
		serverLogger.WithField("snapshot_id", snapshotID).Info("snapshot created")
	}

	if len(errs) > 0 {
		return nil, utilerrors.NewAggregate(errs)
	}
	return snapshotMapping, nil
}

// Wait for snapshots to complete
func (a *openstackActuator) waitForSnapshots(openstackClient openstackclient.Client, snapshotMapping map[string]string, logger log.FieldLogger) error {
	timeout := time.After(30 * time.Minute)
	ticker := time.NewTicker(45 * time.Second)
	defer ticker.Stop()

	pendingSnapshots := make(map[string]string)
	for k, v := range snapshotMapping {
		pendingSnapshots[k] = v
	}

	for len(pendingSnapshots) > 0 {
		select {
		case <-timeout:
			return errors.New("timeout waiting for snapshots to complete after 30 minutes")
		case <-ticker.C:
			var errs []error
			for instanceID, snapshotID := range pendingSnapshots {
				image, err := openstackClient.GetImage(context.Background(), snapshotID)
				if err != nil {
					errs = append(errs, errors.Wrapf(err, "error checking snapshot %s", snapshotID))
					continue
				}

				logger.Debugf("snapshot %s status: %s", snapshotID, image.Status)

				switch image.Status {
				case "active":
					delete(pendingSnapshots, instanceID)
					logger.Infof("snapshot %s completed", snapshotID)
				case "queued", "saving":
					// Still in progress
				case "killed", "deleted", "deactivated":
					errs = append(errs, errors.Errorf("snapshot %s failed with status: %s", snapshotID, image.Status))
					delete(pendingSnapshots, instanceID)
				default:
					logger.Warnf("unknown snapshot status %s for %s", image.Status, snapshotID)
				}
			}

			if len(errs) > 0 {
				return utilerrors.NewAggregate(errs)
			}
		}
	}

	logger.Info("all snapshots completed successfully")
	return nil
}

// Delete instances
func (a *openstackActuator) deleteInstances(openstackClient openstackclient.Client, servers []*servers.Server, logger log.FieldLogger) error {
	var errs []error

	for _, server := range servers {
		logger.WithField("instance", server.Name).Info("deleting instance")
		err := openstackClient.DeleteServer(context.Background(), server.ID)
		if err != nil {
			if isNotFoundError(err) {
				logger.WithField("instance", server.Name).Info("instance already deleted (404)")
				continue
			}
			errs = append(errs, errors.Wrapf(err, "failed to delete instance %s", server.Name))
		}
	}

	return utilerrors.NewAggregate(errs)
}

// Wait for instance cleanup
func (a *openstackActuator) waitForInstanceCleanup(openstackClient openstackclient.Client, infraID string, logger log.FieldLogger) error {
	timeout := time.After(5 * time.Minute)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	logger.Info("waiting for OpenStack to clean up deleted instances...")

	for {
		select {
		case <-timeout:
			return errors.New("timeout waiting for instance cleanup after 5 minutes")
		case <-ticker.C:
			matchingServers, err := a.findInstancesByInfraID(openstackClient, infraID)
			if err != nil {
				logger.Warnf("error checking for remaining instances: %v", err)
				continue // Continue polling despite errors
			}

			if len(matchingServers) == 0 {
				logger.Info("all instances have been cleaned up")
				return nil
			}

			var instanceNames []string
			for _, server := range matchingServers {
				instanceNames = append(instanceNames, server.Name)
			}
			logger.Debugf("still waiting for %d instances to be cleaned up: %v", len(matchingServers), instanceNames)
		}
	}
}

// Find missing instances
func (a *openstackActuator) findMissingInstances(expectedInstances []OpenStackInstanceConfig, existingServers []*servers.Server, logger log.FieldLogger) []OpenStackInstanceConfig {
	existingNames := make(map[string]bool)
	for _, server := range existingServers {
		existingNames[server.Name] = true
	}

	var missingInstances []OpenStackInstanceConfig
	for _, expected := range expectedInstances {
		if !existingNames[expected.Name] {
			logger.WithField("instance", expected.Name).Info("instance is missing")
			missingInstances = append(missingInstances, expected)
		}
	}

	return missingInstances
}

// Create missing instances
func (a *openstackActuator) createMissingInstances(openstackClient openstackclient.Client, instancesToCreate []OpenStackInstanceConfig, logger log.FieldLogger) error {
	var errs []error

	for _, instance := range instancesToCreate {
		instanceLogger := logger.WithField("instance", instance.Name)

		// Verify snapshot still exists
		_, err := openstackClient.GetImage(context.Background(), instance.SnapshotID)
		if err != nil {
			errs = append(errs, errors.Wrapf(err, "snapshot %s not found for instance %s", instance.SnapshotID, instance.Name))
			continue
		}

		// Build server creation options
		createOpts := &servers.CreateOpts{
			Name:      instance.Name,
			ImageRef:  instance.SnapshotID,
			FlavorRef: instance.Flavor,
			Networks: []servers.Network{
				{
					UUID: instance.NetworkID,
					Port: instance.PortID,
				},
			},
			SecurityGroups: instance.SecurityGroups,
			Metadata:       instance.Metadata,
		}

		newServer, err := openstackClient.CreateServerFromOpts(context.Background(), createOpts)
		if err != nil {
			// Check if instance already exists
			if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "409") {
				instanceLogger.Info("instance already exists")
				continue
			}
			errs = append(errs, errors.Wrapf(err, "failed to create instance %s", instance.Name))
			continue
		}

		instanceLogger.WithField("server_id", newServer.ID).Info("created instance")
	}

	return utilerrors.NewAggregate(errs)
}

// Wait for all instances to be active
func (a *openstackActuator) waitForAllInstancesToBeActive(openstackClient openstackclient.Client, infraID string, expectedCount int, logger log.FieldLogger) error {
	timeout := time.After(30 * time.Minute)
	ticker := time.NewTicker(45 * time.Second)
	defer ticker.Stop()

	logger.Infof("waiting for all %d instances to become active", expectedCount)

	for {
		select {
		case <-timeout:
			return errors.New("timeout waiting for instances to become active")
		case <-ticker.C:
			currentServers, err := a.findInstancesByNamePrefix(openstackClient, infraID)
			if err != nil {
				logger.Warnf("error checking instances: %v", err)
				continue
			}

			if len(currentServers) < expectedCount {
				logger.Debugf("have %d/%d instances", len(currentServers), expectedCount)
				continue
			}

			activeCount := 0
			var nonActiveInstances []string
			for _, server := range currentServers {
				if server.Status == "ACTIVE" {
					activeCount++
				} else {
					nonActiveInstances = append(nonActiveInstances, fmt.Sprintf("%s(%s)", server.Name, server.Status))
				}
			}

			if activeCount == expectedCount {
				logger.Info("all instances are active")
				return nil
			}

			logger.Debugf("%d/%d instances active, waiting for: %v", activeCount, expectedCount, nonActiveInstances)
		}
	}
}

// Restore instance tags
func (a *openstackActuator) restoreInstanceTags(openstackClient openstackclient.Client, instances []OpenStackInstanceConfig, logger log.FieldLogger) error {
	var errs []error

	for _, instance := range instances {
		if len(instance.Tags) == 0 {
			continue
		}

		servers, err := a.findInstancesByNamePrefix(openstackClient, instance.ClusterID)
		if err != nil {
			errs = append(errs, errors.Wrapf(err, "failed to find instances for tag restoration"))
			continue
		}

		found := false
		for _, server := range servers {
			if server.Name == instance.Name {
				found = true

				err = openstackClient.SetServerTags(context.Background(), server.ID, instance.Tags)
				if err != nil {
					errs = append(errs, errors.Wrapf(err, "failed to restore tags for %s", instance.Name))
				} else {
					logger.WithField("instance", instance.Name).Info("restored server tags")
				}
				break
			}
		}

		if !found {
			errs = append(errs, errors.Errorf("instance %s not found", instance.Name))
		}
	}

	return utilerrors.NewAggregate(errs)
}

// Save instance configuration
func (a *openstackActuator) saveInstanceConfiguration(cd *hivev1.ClusterDeployment, hiveClient client.Client, openstackClient openstackclient.Client, servers []*servers.Server, snapshotMapping map[string]string, logger log.FieldLogger) error {
	if len(servers) == 0 {
		return nil
	}

	infraID := cd.Spec.ClusterMetadata.InfraID

	// Get shared configuration
	networkID, err := a.getNetworkIDForCluster(openstackClient, infraID)
	if err != nil {
		return errors.Wrap(err, "error getting network ID")
	}

	// Get openshiftClusterID from first instance
	var openshiftClusterID string
	if len(servers) > 0 && servers[0].Metadata != nil {
		if id, exists := servers[0].Metadata["openshiftClusterID"]; exists {
			openshiftClusterID = id
		}
	}

	// Get all ports
	allPorts, err := openstackClient.ListPorts(context.Background())
	if err != nil {
		return errors.Wrap(err, "error listing ports")
	}

	// Build configuration for each instance
	var instanceConfigs []OpenStackInstanceConfig
	var errs []error

	for _, server := range servers {
		// Get flavor ID
		var flavorID string
		if server.Flavor != nil {
			if id, ok := server.Flavor["id"].(string); ok {
				flavorID = id
			} else {
				errs = append(errs, errors.Errorf("could not extract flavor ID for %s", server.Name))
				continue
			}
		} else {
			errs = append(errs, errors.Errorf("no flavor information found for %s", server.Name))
			continue
		}

		// Find matching port
		var portID string
		for _, port := range allPorts {
			if port.Name == server.Name || port.Name == server.Name+"-0" {
				portID = port.ID
				break
			}
		}

		if portID == "" {
			errs = append(errs, errors.Errorf("no port found for instance %s", server.Name))
			continue
		}

		// Get security groups
		secGroups, err := openstackClient.GetServerSecurityGroupNames(context.Background(), server.ID)
		if err != nil {
			errs = append(errs, errors.Wrapf(err, "error getting security groups for %s", server.Name))
			continue
		}

		// Get tags
		serverTags, err := openstackClient.GetServerTags(context.Background(), server.ID)
		if err != nil {
			logger.Warnf("failed to get tags for %s: %v", server.Name, err)
			serverTags = []string{}
		}

		// Get snapshot info
		snapshotID, exists := snapshotMapping[server.ID]
		if !exists {
			errs = append(errs, errors.Errorf("no snapshot found for instance %s", server.Name))
			continue
		}

		// Get snapshot name
		snapshot, err := openstackClient.GetImage(context.Background(), snapshotID)
		if err != nil {
			errs = append(errs, errors.Wrapf(err, "failed to get snapshot details for %s", snapshotID))
			continue
		}

		instanceConfigs = append(instanceConfigs, OpenStackInstanceConfig{
			Name:               server.Name,
			Flavor:             flavorID,
			PortID:             portID,
			SnapshotID:         snapshotID,
			SnapshotName:       snapshot.Name,
			SecurityGroups:     secGroups,
			ClusterID:          infraID,
			NetworkID:          networkID,
			OpenshiftClusterID: openshiftClusterID,
			Metadata:           server.Metadata,
			Tags:               serverTags,
		})

		logger.WithFields(log.Fields{
			"instance":    server.Name,
			"snapshot_id": snapshotID,
		}).Info("saved snapshot info")
	}

	if len(errs) > 0 {
		return utilerrors.NewAggregate(errs)
	}

	return a.saveHibernationConfig(cd, hiveClient, instanceConfigs, logger)
}

// Save hibernation config to secret
func (a *openstackActuator) saveHibernationConfig(cd *hivev1.ClusterDeployment, hiveClient client.Client, instanceConfigs []OpenStackInstanceConfig, logger log.FieldLogger) error {
	configData, err := json.Marshal(instanceConfigs)
	if err != nil {
		return errors.Wrap(err, "failed to marshal hibernation config")
	}

	secretName := fmt.Sprintf("%s-hibernation-config", cd.Name)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: cd.Namespace,
			Labels: map[string]string{
				"hive.openshift.io/cluster-deployment": cd.Name,
				"hive.openshift.io/hibernation-config": "openstack",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: cd.APIVersion,
					Kind:       cd.Kind,
					Name:       cd.Name,
					UID:        cd.UID,
				},
			},
		},
		Data: map[string][]byte{
			"hibernation-config": configData,
		},
	}

	err = hiveClient.Create(context.Background(), secret)
	if err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return errors.Wrap(err, "failed to create hibernation config secret")
		}

		// Update existing secret
		existingSecret := &corev1.Secret{}
		err = hiveClient.Get(context.Background(), types.NamespacedName{
			Name:      secretName,
			Namespace: cd.Namespace,
		}, existingSecret)
		if err != nil {
			return errors.Wrap(err, "failed to get existing hibernation config secret")
		}

		existingSecret.Data = secret.Data
		err = hiveClient.Update(context.Background(), existingSecret)
		if err != nil {
			return errors.Wrap(err, "failed to update hibernation config secret")
		}
	}

	logger.Infof("saved hibernation configuration to secret %s", secretName)
	return nil
}

// Load hibernation config from secret
func (a *openstackActuator) loadHibernationConfig(cd *hivev1.ClusterDeployment, hiveClient client.Client, logger log.FieldLogger) ([]OpenStackInstanceConfig, error) {
	secretName := fmt.Sprintf("%s-hibernation-config", cd.Name)

	secret := &corev1.Secret{}
	err := hiveClient.Get(context.Background(), types.NamespacedName{
		Name:      secretName,
		Namespace: cd.Namespace,
	}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, errors.New("hibernation config secret not found")
		}
		return nil, errors.Wrap(err, "failed to get hibernation config secret")
	}

	configData, exists := secret.Data["hibernation-config"]
	if !exists {
		return nil, errors.New("hibernation config not found in secret")
	}

	var instanceConfigs []OpenStackInstanceConfig
	err = json.Unmarshal(configData, &instanceConfigs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal hibernation config")
	}

	logger.Infof("loaded hibernation configuration from secret %s (%d instances)", secretName, len(instanceConfigs))
	return instanceConfigs, nil
}

// Delete hibernation config secret
func (a *openstackActuator) deleteHibernationConfig(cd *hivev1.ClusterDeployment, hiveClient client.Client, logger log.FieldLogger) error {
	secretName := fmt.Sprintf("%s-hibernation-config", cd.Name)

	secret := &corev1.Secret{}
	err := hiveClient.Get(context.Background(), types.NamespacedName{
		Name:      secretName,
		Namespace: cd.Namespace,
	}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Debug("hibernation config secret already deleted")
			return nil
		}
		return errors.Wrap(err, "failed to get hibernation config secret")
	}

	err = hiveClient.Delete(context.Background(), secret)
	if err != nil {
		return errors.Wrap(err, "failed to delete hibernation config secret")
	}

	logger.Infof("deleted hibernation config secret %s", secretName)
	return nil
}

// Cleanup old snapshots
func (a *openstackActuator) cleanupOldSnapshots(cd *hivev1.ClusterDeployment, hiveClient client.Client, openstackClient openstackclient.Client, infraID string, currentSnapshots map[string]string, logger log.FieldLogger) error {
	// Get all hibernation snapshots for this cluster
	allSnapshots, err := a.findHibernationSnapshots(openstackClient, infraID, logger)
	if err != nil {
		return errors.Wrap(err, "failed to find hibernation snapshots")
	}

	if len(allSnapshots) == 0 {
		logger.Debug("no hibernation snapshots found for cleanup")
		return nil
	}

	// Create set of current snapshot IDs
	currentIDs := sets.NewString()
	for _, snapshotID := range currentSnapshots {
		currentIDs.Insert(snapshotID)
	}

	// Delete old snapshots
	var errs []error
	deleted := 0
	for _, snapshot := range allSnapshots {
		if currentIDs.Has(snapshot.ID) {
			logger.Debugf("keeping current snapshot %s", snapshot.Name)
			continue
		}

		logger.WithField("snapshot", snapshot.Name).Info("deleting old hibernation snapshot")
		err := openstackClient.DeleteImage(context.Background(), snapshot.ID)
		if err != nil {
			if !isNotFoundError(err) {
				errs = append(errs, errors.Wrapf(err, "failed to delete old snapshot %s", snapshot.Name))
			}
		} else {
			deleted++
		}
	}

	if deleted > 0 {
		logger.Infof("deleted %d old hibernation snapshots", deleted)
	}

	return utilerrors.NewAggregate(errs)
}

// Cleanup restoration snapshots
func (a *openstackActuator) cleanupRestorationSnapshots(openstackClient openstackclient.Client, instances []OpenStackInstanceConfig, logger log.FieldLogger) error {
	logger.Info("attempting best-effort cleanup of restoration snapshots")

	var errs []error
	successCount := 0
	for _, instance := range instances {
		if instance.SnapshotID == "" {
			continue
		}

		logger.Debugf("attempting to delete restoration snapshot %s for instance %s", instance.SnapshotID, instance.Name)
		err := openstackClient.DeleteImage(context.Background(), instance.SnapshotID)
		if err != nil {
			if strings.Contains(err.Error(), "in use") || strings.Contains(err.Error(), "409") {
				logger.Debugf("snapshot %s still in use (will be cleaned up next hibernation cycle)", instance.SnapshotID)
			} else if !isNotFoundError(err) {
				errs = append(errs, errors.Wrapf(err, "failed to delete snapshot %s", instance.SnapshotID))
			}
		} else {
			successCount++
		}
	}

	if successCount > 0 {
		logger.Infof("deleted %d restoration snapshots", successCount)
	}

	return utilerrors.NewAggregate(errs)
}

// Using tags to filter the instances
func (a *openstackActuator) findInstancesByInfraID(openstackClient openstackclient.Client, infraID string) ([]*servers.Server, error) {
	taggedServers, err := openstackClient.ListServers(context.Background(), &servers.ListOpts{
		Tags: fmt.Sprintf("openshiftClusterID=%s", infraID),
	})

	if err != nil {
		return nil, errors.Wrap(err, "error listing servers by tag")
	}

	var matchingServers []*servers.Server
	for i := range taggedServers {
		matchingServers = append(matchingServers, &taggedServers[i])
	}
	return matchingServers, nil
}

// Filter instances by the InfraID prefix
func (a *openstackActuator) findInstancesByNamePrefix(openstackClient openstackclient.Client, infraID string) ([]*servers.Server, error) {
	allServers, err := openstackClient.ListServers(context.Background(), nil)
	if err != nil {
		return nil, errors.Wrap(err, "error listing servers")
	}

	var matchingServers []*servers.Server
	for i := range allServers {
		if strings.HasPrefix(allServers[i].Name, infraID) {
			matchingServers = append(matchingServers, &allServers[i])
		}
	}

	return matchingServers, nil
}

// Get network ID for cluster
func (a *openstackActuator) getNetworkIDForCluster(openstackClient openstackclient.Client, infraID string) (string, error) {
	networkName := fmt.Sprintf("%s-openshift", infraID)

	network, err := openstackClient.GetNetworkByName(context.Background(), networkName)
	if err != nil {
		return "", errors.Wrapf(err, "failed to find network '%s'", networkName)
	}

	return network.ID, nil
}

// Find hibernation snapshots by pattern
func (a *openstackActuator) findHibernationSnapshots(openstackClient openstackclient.Client, infraID string, logger log.FieldLogger) ([]images.Image, error) {
	// List all images and filter by pattern
	allImages, err := openstackClient.ListImages(context.Background(), nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list images")
	}

	var hibernationSnapshots []images.Image
	for _, image := range allImages {
		if strings.Contains(image.Name, "-hibernation-") && strings.HasPrefix(image.Name, infraID) {
			hibernationSnapshots = append(hibernationSnapshots, image)
		}
	}

	logger.Debugf("found %d hibernation snapshots for cluster %s", len(hibernationSnapshots), infraID)
	return hibernationSnapshots, nil
}

// Find snapshot by name
func (a *openstackActuator) findSnapshotByName(openstackClient openstackclient.Client, snapshotName string) (*images.Image, error) {
	listOpts := &images.ListOpts{
		Name: snapshotName,
	}

	snapshots, err := openstackClient.ListImages(context.Background(), listOpts)
	if err != nil {
		return nil, err
	}

	for _, snapshot := range snapshots {
		if snapshot.Name == snapshotName {
			return &snapshot, nil
		}
	}

	return nil, errors.New("snapshot not found")
}

// Helper utilities
func isNotFoundError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found"))
}
