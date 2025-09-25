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

// Create API client
func getOpenStackClient(cd *hivev1.ClusterDeployment, c client.Client, logger log.FieldLogger) (openstackclient.Client, error) {
	ctx := context.Background()

	if cd.Spec.Platform.OpenStack == nil || cd.Spec.Platform.OpenStack.CredentialsSecretRef.Name == "" {
		return nil, fmt.Errorf("no OpenStack credentials secret reference found in ClusterDeployment")
	}

	secretName := cd.Spec.Platform.OpenStack.CredentialsSecretRef.Name
	secretNamespace := cd.Namespace

	secret := &corev1.Secret{}
	err := c.Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: secretNamespace,
	}, secret)
	if err != nil {
		return nil, fmt.Errorf("failed to get credentials secret %s/%s: %v", secretNamespace, secretName, err)
	}

	return openstackclient.NewClientFromSecret(secret)
}

// Return true if this actuator can handle the given ClusterDeployment
func (a *openstackActuator) CanHandle(cd *hivev1.ClusterDeployment) bool {
	return cd.Spec.Platform.OpenStack != nil
}

func (a *openstackActuator) StopMachines(cd *hivev1.ClusterDeployment, hiveClient client.Client, logger log.FieldLogger) error {
	logger = logger.WithField("cloud", "openstack")
	logger.Info("stopping machines and creating snapshots")

	openstackClient, err := a.openstackClientFn(cd, hiveClient, logger)
	if err != nil {
		return fmt.Errorf("failed to create OpenStack client: %v", err)
	}

	infraID := cd.Spec.ClusterMetadata.InfraID

	// 1. Find instances
	matchingServers, err := a.findInstancesByInfraID(openstackClient, infraID)
	if err != nil {
		return fmt.Errorf("error finding instances: %v", err)
	}

	if len(matchingServers) == 0 {
		logger.Info("no instances found - cluster already hibernated")
		return nil
	}

	logger.WithField("count", len(matchingServers)).Info("found instances to hibernate")

	// 2. Validate instance states
	if err := a.validateInstanceStates(openstackClient, matchingServers, logger); err != nil {
		return err
	}

	// 3. Pause all instances for data consistency
	if err := a.pauseInstances(openstackClient, matchingServers, logger); err != nil {
		return fmt.Errorf("failed to pause instances: %v", err)
	}

	// 4. Create snapshots
	snapshotIDs, snapshotNames, err := a.createSnapshots(openstackClient, matchingServers, logger)
	if err != nil {
		// If snapshot creation fails, unpause
		logger.Warn("snapshot creation failed - unpausing instances")
		a.unpauseInstances(openstackClient, matchingServers, logger)
		return err
	}

	// 5. Wait for snapshots to complete
	if err := a.waitForSnapshots(openstackClient, snapshotIDs, snapshotNames, matchingServers, logger); err != nil {
		// If snapshot wait fails, unpause
		logger.Warn("snapshot wait failed - unpausing instances")
		a.unpauseInstances(openstackClient, matchingServers, logger)
		return err
	}

	// 6. Save instance configuration
	if err := a.saveInstanceConfigurationToSecret(cd, hiveClient, openstackClient, matchingServers, snapshotIDs, snapshotNames, logger); err != nil {
		// If config save fails, unpause instances before returning
		logger.Warn("config save failed - unpausing instances")
		a.unpauseInstances(openstackClient, matchingServers, logger)
		return fmt.Errorf("error saving configuration: %v", err)
	}

	// 7. Delete instances
	if err := a.deleteInstances(openstackClient, matchingServers, logger); err != nil {
		return err
	}

	// 8. Wait for instance cleanup
	if err := a.waitForInstanceCleanup(openstackClient, infraID, logger); err != nil {
		return err
	}

	// 9. Cleanup old snapshots
	logger.Info("cleaning up old hibernation snapshots after instance deletion")
	err = a.cleanupOldSnapshotsAfterDeletion(cd, hiveClient, openstackClient, infraID, snapshotIDs, logger)
	if err != nil {
		logger.WithField("error", err).Warn("some old snapshots couldn't be cleaned up")
	}

	logger.Info("hibernation completed successfully")
	return nil
}

// Find which instances are missing
func (a *openstackActuator) findMissingInstances(expectedInstances []OpenStackInstanceConfig, existingServers []*servers.Server, logger log.FieldLogger) []OpenStackInstanceConfig {
	var missingInstances []OpenStackInstanceConfig

	// Create a map of existing instance names for quick lookup
	existingNames := make(map[string]bool)
	for _, server := range existingServers {
		existingNames[server.Name] = true
	}

	// Check which expected instances are missing
	for _, expected := range expectedInstances {
		instanceLogger := logger.WithField("instance", expected.Name)
		if !existingNames[expected.Name] {
			instanceLogger.Info("instance is missing - needs to be created")
			missingInstances = append(missingInstances, expected)
		} else {
			instanceLogger.Info("instance already exists")
		}
	}

	return missingInstances
}

// Check if instances are in valid states for hibernation
func (a *openstackActuator) validateInstanceStates(openstackClient openstackclient.Client, servers []*servers.Server, logger log.FieldLogger) error {
	for _, server := range servers {
		serverLogger := logger.WithFields(log.Fields{
			"instance": server.Name,
			"status":   server.Status,
		})

		serverLogger.Info("instance status")

		// Check for deleting states that would cause conflicts
		if strings.Contains(strings.ToLower(server.Status), "delet") {
			return fmt.Errorf("cannot hibernate: instance %s is being deleted by another process", server.Name)
		}

		if server.Status != "ACTIVE" {
			serverLogger.Warn("instance status is not ACTIVE - snapshot may fail")
		}
	}
	return nil
}

// Helper function to generate hibernation snapshot names
func (a *openstackActuator) generateHibernationSnapshotName(serverName string, timestamp string) string {
	return fmt.Sprintf("%s-hibernation-%s", serverName, timestamp)
}

// Create snapshots for all instances
func (a *openstackActuator) createSnapshots(openstackClient openstackclient.Client, servers []*servers.Server, logger log.FieldLogger) ([]string, []string, error) {
	ctx := context.Background()
	snapshotIDs := make([]string, 0, len(servers))
	snapshotNames := make([]string, 0, len(servers))

	timestamp := time.Now().UTC().Format("20060102-150405")
	for i, server := range servers {
		progressLogger := logger.WithFields(log.Fields{
			"current":  i + 1,
			"total":    len(servers),
			"instance": server.Name,
		})

		progressLogger.Info("creating snapshot")

		snapshotName := a.generateHibernationSnapshotName(server.Name, timestamp)

		snapshotID, err := openstackClient.CreateServerSnapshot(ctx, server.ID, snapshotName)
		if err != nil {
			if strings.Contains(err.Error(), "task_state deleting") || strings.Contains(err.Error(), "409") {
				return nil, nil, fmt.Errorf("hibernation conflict: instance %s is being modified by another process", server.Name)
			}
			return nil, nil, fmt.Errorf("failed to create snapshot for %s: %v", server.Name, err)
		}

		snapshotIDs = append(snapshotIDs, snapshotID)
		snapshotNames = append(snapshotNames, snapshotName)
		progressLogger.WithFields(log.Fields{
			"snapshot_id":   snapshotID,
			"snapshot_name": snapshotName,
		}).Info("snapshot created")
	}
	return snapshotIDs, snapshotNames, nil
}

// Wait for all snapshots to complete
func (a *openstackActuator) waitForSnapshots(openstackClient openstackclient.Client, snapshotIDs []string, snapshotNames []string, servers []*servers.Server, logger log.FieldLogger) error {
	for i, snapshotID := range snapshotIDs {
		serverName := servers[i].Name
		snapshotName := snapshotNames[i]
		logger.WithFields(log.Fields{
			"snapshot_id":   snapshotID,
			"snapshot_name": snapshotName,
			"server":        serverName,
		}).Info("waiting for snapshot to complete")

		err := a.waitForSnapshotCompletion(openstackClient, snapshotID, serverName, logger)
		if err != nil {
			return fmt.Errorf("failed to wait for snapshot %s: %v", snapshotID, err)
		}
	}
	return nil
}

// Delete all instances
func (a *openstackActuator) deleteInstances(openstackClient openstackclient.Client, servers []*servers.Server, logger log.FieldLogger) error {
	ctx := context.Background()

	for i, server := range servers {
		logger.WithFields(log.Fields{
			"current":  i + 1,
			"total":    len(servers),
			"instance": server.Name,
		}).Info("deleting instance")

		err := openstackClient.DeleteServer(ctx, server.ID)
		if err != nil {
			return fmt.Errorf("failed to delete %s: %v", server.Name, err)
		}
	}
	return nil
}

// Wait for snapshot to reach ACTIVE state
func (a *openstackActuator) waitForSnapshotCompletion(openstackClient openstackclient.Client, snapshotID, serverName string, logger log.FieldLogger) error {
	ctx := context.Background()
	timeout := time.After(30 * time.Minute)
	ticker := time.NewTicker(45 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for snapshot %s to complete after %v", snapshotID, timeout)
		case <-ticker.C:
			image, err := openstackClient.GetImage(ctx, snapshotID)
			if err != nil {
				logger.Warnf("error checking snapshot %s status: %v", snapshotID, err)
				continue
			}

			logger.Infof("snapshot %s for %s status: %s", snapshotID, serverName, image.Status)

			switch image.Status {
			case "active":
				return nil
			case "queued", "saving":
				continue
			case "killed", "deleted", "deactivated":
				return fmt.Errorf("snapshot %s failed with status: %s", snapshotID, image.Status)
			default:
				logger.Warnf("unknown snapshot status %s for %s, continuing to wait", image.Status, snapshotID)
				continue
			}
		}
	}
}

// Recreate instances from snapshots using saved configuration
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
		return fmt.Errorf("failed to create OpenStack client: %v", err)
	}

	// Load hibernation config first to know how many instances we should have
	instances, err := a.loadHibernationConfigFromSecret(cd, hiveClient, logger)
	if err != nil {
		logger.Warnf("no hibernation config found: %v", err)

		// Check if we have existing instances but no hibernation config
		infraID := cd.Spec.ClusterMetadata.InfraID
		existingServers, checkErr := a.findInstancesByInfraID(openstackClient, infraID)
		if checkErr != nil {
			logger.Warnf("could not check existing instances: %v", checkErr)
		} else if len(existingServers) > 0 {
			logger.Info("instances exist but no hibernation config - clearing any hibernation state")
			_ = a.deleteHibernationConfigSecret(cd, hiveClient, logger)
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
	}

	// Check if we already have all the instances we need and clear hibernation config
	if len(existingServers) >= len(instances) {
		logger.Info("sufficient instances already exist - clearing hibernation config")
		_ = a.deleteHibernationConfigSecret(cd, hiveClient, logger)
		return nil
	}

	logger.Infof("restoring %d instances from hibernation snapshots (currently have %d)", len(instances), len(existingServers))

	return a.restoreFromHibernationConfig(cd, hiveClient, openstackClient, instances, logger)
}

// Recreate instances from hibernation configuration
func (a *openstackActuator) restoreFromHibernationConfig(cd *hivev1.ClusterDeployment, hiveClient client.Client, openstackClient openstackclient.Client, instances []OpenStackInstanceConfig, logger log.FieldLogger) error {
	infraID := cd.Spec.ClusterMetadata.InfraID

	logger.WithField("count", len(instances)).Info("restoring instances from hibernation")

	// Check what instances already exist
	existingServers, err := a.findInstancesByInfraID(openstackClient, infraID)
	if err != nil {
		logger.Warnf("could not check existing instances: %v", err)
		existingServers = []*servers.Server{} // Assume none exist
	}

	// Figure out what we need to create
	instancesToCreate := a.findMissingInstances(instances, existingServers, logger)

	if len(instancesToCreate) == 0 {
		logger.Info("all instances already exist - continuing to verify they're active")
	} else {
		logger.WithField("count", len(instancesToCreate)).Info("need to create missing instances")

		// Create missing instances
		err = a.createMissingInstances(openstackClient, instancesToCreate, logger)
		if err != nil {
			logger.Errorf("some instance creation failed: %v", err)
		}
	}

	// ALWAYS wait for instances to be active (regardless of whether we created new ones)
	err = a.waitForAllInstancesToBeActive(openstackClient, infraID, len(instances), logger)
	if err != nil {
		return fmt.Errorf("not all instances are active yet: %v", err)
	}

	// Tags have to be added separately AFTER the instances are ACTIVE
	a.restoreInstanceTags(openstackClient, instances, logger)

	// ALWAYS clean up hibernation snapshots after successful restoration
	logger.Info("cleaning up hibernation snapshots after successful restoration")
	err = a.cleanupRestorationSnapshots(openstackClient, instances, logger)
	if err != nil {
		// Log but don't fail - restoration succeeded, cleanup is best-effort
		logger.Warnf("failed to cleanup some snapshots: %v", err)
	}

	// ALWAYS clear hibernation config when we have confirmed instances are running
	logger.Info("all instances confirmed active - clearing hibernation configuration")
	return a.deleteHibernationConfigSecret(cd, hiveClient, logger)
}

// Create missing instances during restoration
func (a *openstackActuator) createMissingInstances(openstackClient openstackclient.Client, instancesToCreate []OpenStackInstanceConfig, logger log.FieldLogger) error {
	ctx := context.Background()
	var errs []error

	for i, instance := range instancesToCreate {
		instanceLogger := logger.WithFields(log.Fields{
			"current":  i + 1,
			"total":    len(instancesToCreate),
			"instance": instance.Name,
		})

		instanceLogger.Info("creating missing instance")

		// Validate snapshot still exists
		_, err := openstackClient.GetImage(ctx, instance.SnapshotID)
		if err != nil {
			err := fmt.Errorf("snapshot %s not found for %s: %w", instance.SnapshotID, instance.Name, err)
			errs = append(errs, err)
			logger.WithField("error", err).Error("snapshot validation failed")
			continue
		}

		// Build server creation options with complete metadata
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

		newServer, err := openstackClient.CreateServerFromOpts(ctx, createOpts)
		if err != nil {
			err := fmt.Errorf("failed to create instance %s: %w", instance.Name, err)
			errs = append(errs, err)
			logger.WithField("error", err).Error("instance creation failed")
			continue
		}

		instanceLogger.WithField("server_id", newServer.ID).Info("created instance")

	}

	return utilerrors.NewAggregate(errs)
}

// Wait for ALL instances to be active
func (a *openstackActuator) waitForAllInstancesToBeActive(openstackClient openstackclient.Client, infraID string, expectedCount int, logger log.FieldLogger) error {
	timeout := time.After(30 * time.Minute)
	ticker := time.NewTicker(45 * time.Second)
	defer ticker.Stop()

	logger.Infof("waiting for all %d instances to become active", expectedCount)

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for all instances to become active after %v", timeout)
		case <-ticker.C:
			// Get current instances
			currentServers, err := a.findInstancesByInfraID(openstackClient, infraID)
			if err != nil {
				logger.Warnf("error checking instance status: %v", err)
				continue
			}

			if len(currentServers) != expectedCount {
				logger.Infof("have %d instances, expecting %d - still waiting", len(currentServers), expectedCount)
				continue
			}

			// Check if all instances are ACTIVE
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
				logger.Info("all instances are active!")
				return nil
			}

			logger.Infof("%d/%d instances active, waiting for: %v", activeCount, expectedCount, nonActiveInstances)
		}
	}
}

// MachinesRunning checks if machines are running
func (a *openstackActuator) MachinesRunning(cd *hivev1.ClusterDeployment, hiveClient client.Client, logger log.FieldLogger) (bool, []string, error) {
	logger = logger.WithField("cloud", "openstack")
	logger.Info("checking if machines are running")

	openstackClient, err := a.openstackClientFn(cd, hiveClient, logger)
	if err != nil {
		return false, nil, fmt.Errorf("failed to create OpenStack client: %v", err)
	}

	infraID := cd.Spec.ClusterMetadata.InfraID
	matchingServers, err := a.findInstancesByInfraID(openstackClient, infraID)
	if err != nil {
		return false, nil, fmt.Errorf("error finding instances: %v", err)
	}

	logger.Infof("found %d instances with prefix '%s'", len(matchingServers), infraID)

	if len(matchingServers) == 0 {
		logger.Info("no instances found - machines are not running")
		return false, []string{"no instances found"}, nil
	}

	// Check actual instance states
	runningCount, deletingInstances := a.categorizeInstanceStates(openstackClient, matchingServers, logger)

	// If instances are being deleted, hibernation is in progress
	if len(deletingInstances) > 0 && runningCount == 0 {
		logger.Infof("all instances are being deleted (%v) - hibernation in progress", deletingInstances)
		return false, []string{"instances-being-deleted"}, nil
	}

	return runningCount > 0, []string{}, nil
}

// Check the actual state of instances in OpenStack
func (a *openstackActuator) categorizeInstanceStates(openstackClient openstackclient.Client, servers []*servers.Server, logger log.FieldLogger) (int, []string) {
	runningCount := 0
	var deletingInstances []string

	for _, server := range servers {
		status := strings.ToLower(server.Status)
		if strings.Contains(status, "delet") || status == "shutoff" || status == "error" {
			logger.Infof("instance %s is being deleted/stopped (status: %s)", server.Name, server.Status)
			deletingInstances = append(deletingInstances, server.Name)
		} else {
			logger.Infof("instance %s is running (status: %s)", server.Name, server.Status)
			runningCount++
		}
	}

	return runningCount, deletingInstances
}

// Check if machines are stopped
func (a *openstackActuator) MachinesStopped(cd *hivev1.ClusterDeployment, hiveClient client.Client, logger log.FieldLogger) (bool, []string, error) {
	logger = logger.WithField("cloud", "openstack")
	logger.Info("checking if machines are stopped")

	openstackClient, err := a.openstackClientFn(cd, hiveClient, logger)
	if err != nil {
		return false, nil, fmt.Errorf("failed to create OpenStack client: %v", err)
	}

	infraID := cd.Spec.ClusterMetadata.InfraID
	matchingServers, err := a.findInstancesByInfraID(openstackClient, infraID)
	if err != nil {
		return false, nil, fmt.Errorf("error finding instances: %v", err)
	}

	if len(matchingServers) == 0 {
		logger.Info("no instances found - machines are stopped")
		return true, nil, nil
	}

	var notStopped []string
	for _, server := range matchingServers {
		notStopped = append(notStopped, server.Name)
	}

	logger.Infof("found %d instances still running", len(notStopped))
	return false, notStopped, nil
}

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

// Return servers that match the infraID prefix
func (a *openstackActuator) findInstancesByInfraID(openstackClient openstackclient.Client, prefix string) ([]*servers.Server, error) {
	ctx := context.Background()

	allServers, err := openstackClient.ListServers(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("error listing servers: %v", err)
	}

	var matchingServers []*servers.Server
	for _, server := range allServers {
		if strings.HasPrefix(server.Name, prefix) {
			matchingServers = append(matchingServers, &server)
		}
	}

	return matchingServers, nil
}

// Configuration persistence methods
func (a *openstackActuator) saveInstanceConfigurationToSecret(cd *hivev1.ClusterDeployment, hiveClient client.Client, openstackClient openstackclient.Client, servers []*servers.Server, snapshotIDs []string, snapshotNames []string, logger log.FieldLogger) error {
	ctx := context.Background()

	if len(servers) == 0 {
		return nil
	}

	if len(snapshotIDs) != len(servers) {
		return fmt.Errorf("mismatch between servers (%d) and snapshot IDs (%d)", len(servers), len(snapshotIDs))
	}

	if len(snapshotNames) != len(servers) {
		return fmt.Errorf("mismatch between servers (%d) and snapshot names (%d)", len(servers), len(snapshotNames))
	}

	// Get InfraID
	infraID := cd.Spec.ClusterMetadata.InfraID

	// Get shared configuration
	networkID, err := a.getNetworkIDForCluster(openstackClient, infraID)
	if err != nil {
		return fmt.Errorf("error getting network ID: %v", err)
	}

	// Get openshiftClusterID from first instance
	server := servers[0]
	var openshiftClusterID string
	if server.Metadata != nil {
		if id, exists := server.Metadata["openshiftClusterID"]; exists {
			openshiftClusterID = id
		}
	}

	// Get all ports
	allPorts, err := openstackClient.ListPorts(ctx)
	if err != nil {
		return fmt.Errorf("error listing ports: %v", err)
	}

	// Build configuration for each instance
	var instanceConfigs []OpenStackInstanceConfig
	for i, server := range servers {
		// Get flavor ID
		var flavorID string
		if server.Flavor != nil {
			if id, ok := server.Flavor["id"].(string); ok {
				flavorID = id
			} else {
				return fmt.Errorf("could not extract flavor ID for %s", server.Name)
			}
		} else {
			return fmt.Errorf("no flavor information found for %s", server.Name)
		}

		// Find maching port
		var portID string
		for _, port := range allPorts {
			if port.Name == server.Name || port.Name == server.Name+"-0" {
				portID = port.ID
				break
			}
		}

		if portID == "" {
			return fmt.Errorf("no port found for instance %s", server.Name)
		}

		// Get security groups
		secGroups, err := openstackClient.GetServerSecurityGroupNames(ctx, server.ID)
		if err != nil {
			return fmt.Errorf("error getting security groups for %s: %v", server.Name, err)
		}

		// Get tags
		serverTags, err := openstackClient.GetServerTags(ctx, server.ID)
		if err != nil {
			logger.Warnf("failed to get tags for %s: %v", server.Name, err)
			serverTags = []string{}
		}

		// Use the snapshot name directly
		snapshotName := snapshotNames[i]

		instanceConfigs = append(instanceConfigs, OpenStackInstanceConfig{
			Name:               server.Name,
			Flavor:             flavorID,
			PortID:             portID,
			SnapshotID:         snapshotIDs[i],
			SnapshotName:       snapshotName,
			SecurityGroups:     secGroups,
			ClusterID:          infraID,
			NetworkID:          networkID,
			OpenshiftClusterID: openshiftClusterID,
			Metadata:           server.Metadata,
			Tags:               serverTags,
		})

		logger.WithFields(log.Fields{
			"instance":      server.Name,
			"snapshot_id":   snapshotIDs[i],
			"snapshot_name": snapshotName,
		}).Info("saved snapshot info")
	}

	return a.saveHibernationConfigToSecret(cd, hiveClient, instanceConfigs, logger)
}

// Store hibernation information to secrets
func (a *openstackActuator) saveHibernationConfigToSecret(cd *hivev1.ClusterDeployment, hiveClient client.Client, instanceConfigs []OpenStackInstanceConfig, logger log.FieldLogger) error {
	ctx := context.Background()

	configData, err := json.Marshal(instanceConfigs)
	if err != nil {
		return fmt.Errorf("failed to marshal hibernation config: %v", err)
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

	err = hiveClient.Create(ctx, secret)
	if err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create hibernation config secret: %v", err)
		}

		// Update existing secret
		existingSecret := &corev1.Secret{}
		err = hiveClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: cd.Namespace}, existingSecret)
		if err != nil {
			return fmt.Errorf("failed to get existing hibernation config secret: %v", err)
		}

		existingSecret.Data = secret.Data
		err = hiveClient.Update(ctx, existingSecret)
		if err != nil {
			return fmt.Errorf("failed to update hibernation config secret: %v", err)
		}
	}

	logger.Infof("saved hibernation configuration to secret %s", secretName)
	return nil
}

// Waits for OpenStack to fully remove an instance
func (a *openstackActuator) waitForInstanceCleanup(openstackClient openstackclient.Client, infraID string, logger log.FieldLogger) error {
	timeout := time.After(5 * time.Minute)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	logger.Info("waiting for OpenStack to clean up deleted instances...")

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for instance cleanup after %v", timeout)
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

			// Log remaining instances
			var instanceNames []string
			for _, server := range matchingServers {
				instanceNames = append(instanceNames, server.Name)
			}
			logger.Infof("still waiting for %d instances to be cleaned up: %v", len(matchingServers), instanceNames)
		}
	}
}

func (a *openstackActuator) restoreInstanceTags(openstackClient openstackclient.Client, instances []OpenStackInstanceConfig, logger log.FieldLogger) {
	ctx := context.Background()

	for _, instance := range instances {
		if len(instance.Tags) > 0 {
			// Find the active instance by name
			servers, err := a.findInstancesByInfraID(openstackClient, instance.ClusterID)
			if err != nil {
				continue
			}

			for _, server := range servers {
				if server.Name == instance.Name {
					err = openstackClient.SetServerTags(ctx, server.ID, instance.Tags)
					if err != nil {
						logger.WithFields(log.Fields{
							"instance": instance.Name,
							"error":    err,
							"tags":     instance.Tags,
						}).Warn("failed to restore server tags")
					} else {
						logger.WithFields(log.Fields{
							"instance":  instance.Name,
							"tag_count": len(instance.Tags),
						}).Info("restored server tags")
					}
					break
				}
			}
		}
	}
}

// Get stored hibernation information from secret
func (a *openstackActuator) loadHibernationConfigFromSecret(cd *hivev1.ClusterDeployment, hiveClient client.Client, logger log.FieldLogger) ([]OpenStackInstanceConfig, error) {
	ctx := context.Background()
	secretName := fmt.Sprintf("%s-hibernation-config", cd.Name)

	secret := &corev1.Secret{}
	err := hiveClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: cd.Namespace}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("hibernation config secret not found")
		}
		return nil, fmt.Errorf("failed to get hibernation config secret: %v", err)
	}

	configData, exists := secret.Data["hibernation-config"]
	if !exists {
		return nil, fmt.Errorf("hibernation config not found in secret")
	}

	var instanceConfigs []OpenStackInstanceConfig
	err = json.Unmarshal(configData, &instanceConfigs)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal hibernation config: %v", err)
	}

	logger.Infof("loaded hibernation configuration from secret %s (%d instances)", secretName, len(instanceConfigs))
	return instanceConfigs, nil
}

// Delete stored hibernation information from secret
func (a *openstackActuator) deleteHibernationConfigSecret(cd *hivev1.ClusterDeployment, hiveClient client.Client, logger log.FieldLogger) error {
	ctx := context.Background()
	secretName := fmt.Sprintf("%s-hibernation-config", cd.Name)

	secret := &corev1.Secret{}
	err := hiveClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: cd.Namespace}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("hibernation config secret already deleted")
			return nil
		}
		return fmt.Errorf("failed to get hibernation config secret: %v", err)
	}

	err = hiveClient.Delete(ctx, secret)
	if err != nil {
		return fmt.Errorf("failed to delete hibernation config secret: %v", err)
	}

	logger.Infof("deleted hibernation config secret %s", secretName)
	return nil
}

// Find the network ID for a specific cluster using infraID
func (a *openstackActuator) getNetworkIDForCluster(openstackClient openstackclient.Client, infraID string) (string, error) {
	ctx := context.Background()
	networkName := fmt.Sprintf("%s-openshift", infraID)

	network, err := openstackClient.GetNetworkByName(ctx, networkName)
	if err != nil {
		return "", fmt.Errorf("failed to find network '%s': %w", networkName, err)
	}

	return network.ID, nil
}

func (a *openstackActuator) cleanupRestorationSnapshots(openstackClient openstackclient.Client, instances []OpenStackInstanceConfig, logger log.FieldLogger) error {
	ctx := context.Background()

	logger.Info("attempting best-effort cleanup of restoration snapshots")

	successCount := 0
	for _, instance := range instances {
		if instance.SnapshotID != "" {
			logger.Infof("attempting to delete restoration snapshot %s for instance %s", instance.SnapshotID, instance.Name)
			err := openstackClient.DeleteImage(ctx, instance.SnapshotID)
			if err != nil {
				if strings.Contains(err.Error(), "in use") || strings.Contains(err.Error(), "409") {
					logger.Infof("snapshot %s still in use (will be cleaned up next hibernation cycle)", instance.SnapshotID)
				} else {
					logger.Warnf("failed to delete snapshot %s: %v", instance.SnapshotID, err)
				}
			} else {
				logger.Infof("successfully deleted restoration snapshot %s", instance.SnapshotID)
				successCount++
			}
		}
	}

	logger.Infof("restoration snapshot cleanup: %d/%d snapshots deleted (remaining will be cleaned up next hibernation)", successCount, len(instances))
	return nil // Always succeed
}

func (a *openstackActuator) cleanupOldSnapshotsAfterDeletion(cd *hivev1.ClusterDeployment, hiveClient client.Client, openstackClient openstackclient.Client, infraID string, currentSnapshotIDs []string, logger log.FieldLogger) error {
	// Get all hibernation snapshots for this cluster
	allSnapshots, err := a.findHibernationSnapshotsByExactNames(cd, hiveClient, openstackClient, infraID, logger)
	if err != nil {
		return err
	}

	if len(allSnapshots) == 0 {
		logger.Info("no hibernation snapshots found")
		return nil
	}

	// Create map of current snapshot IDs for exclusion
	currentIDs := sets.NewString(currentSnapshotIDs...)

	// Filter out current snapshots - only delete OLD ones
	var oldSnapshots []images.Image
	for _, snapshot := range allSnapshots {
		if !currentIDs.Has(snapshot.ID) {
			oldSnapshots = append(oldSnapshots, snapshot)
			logger.Infof("found OLD snapshot to delete: %s (ID: %s)", snapshot.Name, snapshot.ID)
		} else {
			logger.Infof("keeping NEW snapshot: %s (ID: %s)", snapshot.Name, snapshot.ID)
		}
	}

	if len(oldSnapshots) == 0 {
		logger.Info("no old snapshots to clean up")
		return nil
	}

	logger.Infof("deleting %d old hibernation snapshots", len(oldSnapshots))

	successCount := 0
	for _, snapshot := range oldSnapshots {
		logger.Infof("deleting old hibernation snapshot: %s", snapshot.Name)
		err := openstackClient.DeleteImage(context.Background(), snapshot.ID)
		if err != nil {
			logger.Warnf("failed to delete old snapshot %s: %v", snapshot.Name, err)
		} else {
			logger.Infof("successfully deleted old snapshot: %s", snapshot.Name)
			successCount++
		}
	}

	logger.Infof("old snapshot cleanup completed: %d/%d snapshots deleted", successCount, len(oldSnapshots))
	return nil
}

func (a *openstackActuator) getExpectedSnapshotNames(cd *hivev1.ClusterDeployment, hiveClient client.Client, logger log.FieldLogger) ([]string, error) {
	instances, err := a.loadHibernationConfigFromSecret(cd, hiveClient, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to load hibernation config: %v", err)
	}

	var snapshotNames []string
	for _, instance := range instances {
		if instance.SnapshotName != "" {
			snapshotNames = append(snapshotNames, instance.SnapshotName)
		}
	}

	return snapshotNames, nil
}

func (a *openstackActuator) findHibernationSnapshotsByExactNames(cd *hivev1.ClusterDeployment, hiveClient client.Client, openstackClient openstackclient.Client, infraID string, logger log.FieldLogger) ([]images.Image, error) {
	ctx := context.Background()

	// Get the exact snapshot names from hibernation config
	expectedSnapshotNames, err := a.getExpectedSnapshotNames(cd, hiveClient, logger)

	if err != nil {
		return nil, fmt.Errorf("failed to get expected snapshot names: %v", err)
	}

	if len(expectedSnapshotNames) == 0 {
		return []images.Image{}, nil
	}

	nameFilter := fmt.Sprintf("in:%s", strings.Join(expectedSnapshotNames, ","))
	listOpts := &images.ListOpts{
		Name: nameFilter,
	}

	hibernationSnapshots, err := openstackClient.ListImages(ctx, listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to query snapshots by exact names: %v", err)
	}

	logger.WithFields(log.Fields{
		"expected_count": len(expectedSnapshotNames),
		"found_count":    len(hibernationSnapshots),
	}).Info("found snapshots by exact names")

	return hibernationSnapshots, nil
}

func (a *openstackActuator) pauseInstances(openstackClient openstackclient.Client, servers []*servers.Server, logger log.FieldLogger) error {
	ctx := context.Background()

	for i, server := range servers {
		serverLogger := logger.WithFields(log.Fields{
			"current":  i + 1,
			"total":    len(servers),
			"instance": server.Name,
		})

		serverLogger.Info("pausing instance for snapshot consistency")

		err := openstackClient.PauseServer(ctx, server.ID)
		if err != nil {
			// If pause fails, try to unpause any previously paused instances
			if i > 0 {
				logger.Warn("pause failed - attempting to unpause previously paused instances")
				a.unpauseInstances(openstackClient, servers[:i], logger)
			}
			return fmt.Errorf("failed to pause instance %s: %w", server.Name, err)
		}

		serverLogger.Info("instance paused successfully")
	}

	logger.WithField("count", len(servers)).Info("all instances paused successfully")
	return nil
}

// Unpause instances
func (a *openstackActuator) unpauseInstances(openstackClient openstackclient.Client, servers []*servers.Server, logger log.FieldLogger) {
	ctx := context.Background()

	for _, server := range servers {
		serverLogger := logger.WithField("instance", server.Name)
		serverLogger.Info("unpausing instance")

		err := openstackClient.UnpauseServer(ctx, server.ID)
		if err != nil {
			serverLogger.WithField("error", err).Error("failed to unpause instance")
		} else {
			serverLogger.Info("instance unpaused successfully")
		}
	}
}
