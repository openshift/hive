package hibernation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/openstackclient"

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

// CanHandle returns true if this actuator can handle the given ClusterDeployment
func (a *openstackActuator) CanHandle(cd *hivev1.ClusterDeployment) bool {
	return cd.Spec.Platform.OpenStack != nil
}

// StopMachines creates snapshots and saves configuration, then stops machines
func (a *openstackActuator) StopMachines(cd *hivev1.ClusterDeployment, hiveClient client.Client, logger log.FieldLogger) error {
	logger = logger.WithField("cloud", "openstack")
	logger.Info("stopping machines and creating snapshots")

	_, err := a.loadHibernationConfigFromSecret(cd, hiveClient, logger)
	if err == nil {
		logger.Info("Hibernation config already exists - checking if hibernation completed")

		openstackClient, err := a.openstackClientFn(cd, hiveClient, logger)
		if err != nil {
			return fmt.Errorf("failed to create OpenStack client: %v", err)
		}

		infraID := cd.Spec.ClusterMetadata.InfraID
		matchingServers, err := a.findInstancesByPrefix(openstackClient, infraID)
		if err != nil {
			return fmt.Errorf("error finding instances: %v", err)
		}

		if len(matchingServers) == 0 {
			logger.Info("Hibernation already completed - config exists and no instances found")
			return nil
		}

		logger.Info("Hibernation config exists but instances still found - proceeding with cleanup")
	}

	logger = logger.WithField("cloud", "openstack")
	logger.Info("stopping machines and creating snapshots")

	openstackClient, err := a.openstackClientFn(cd, hiveClient, logger)
	if err != nil {
		return fmt.Errorf("failed to create OpenStack client: %v", err)
	}

	infraID := cd.Spec.ClusterMetadata.InfraID

	matchingServers, err := a.findInstancesByPrefix(openstackClient, infraID)
	if err != nil {
		return fmt.Errorf("error finding instances: %v", err)
	}

	if len(matchingServers) == 0 {
		logger.Info("no instances found - cluster already hibernated")
		return nil
	}

	logger.Infof("found %d instances to hibernate", len(matchingServers))

	// Validate instance states before snapshotting
	err = a.validateInstanceStates(openstackClient, matchingServers, logger)
	if err != nil {
		return err
	}

	// Create snapshots for each instance
	snapshotIDs, err := a.createSnapshots(openstackClient, matchingServers, logger)
	if err != nil {
		return err
	}

	// Wait for all snapshots to complete
	err = a.waitForSnapshots(openstackClient, snapshotIDs, matchingServers, logger)
	if err != nil {
		return err
	}

	// Save configuration to Secret
	err = a.saveInstanceConfigurationToSecret(cd, hiveClient, openstackClient, matchingServers, snapshotIDs, logger)
	if err != nil {
		return fmt.Errorf("error saving configuration: %v", err)
	}

	// Delete the instances
	err = a.deleteInstances(openstackClient, matchingServers, logger)
	if err != nil {
		return err
	}

	logger.Info("waiting for OpenStack to clean up deleted instances...")
	time.Sleep(30 * time.Second)

	logger.Info("hibernation completed successfully")
	return nil
}

// validateInstanceStates checks if instances are in valid states for hibernation
func (a *openstackActuator) validateInstanceStates(openstackClient openstackclient.Client, servers []ServerInfo, logger log.FieldLogger) error {
	ctx := context.Background()

	for _, server := range servers {
		serverDetails, err := openstackClient.GetServer(ctx, server.ID)
		if err != nil {
			return fmt.Errorf("failed to get server %s details: %v", server.Name, err)
		}

		logger.Infof("instance %s status: %s", server.Name, serverDetails.Status)

		// Check for deleting states that would cause conflicts
		if strings.Contains(strings.ToLower(serverDetails.Status), "delet") {
			return fmt.Errorf("cannot hibernate: instance %s is being deleted by another process", server.Name)
		}

		if serverDetails.Status != "ACTIVE" {
			logger.Warnf("instance %s status is %s (not ACTIVE) - snapshot may fail", server.Name, serverDetails.Status)
		}
	}
	return nil
}

// Create snapshots for all instances
func (a *openstackActuator) createSnapshots(openstackClient openstackclient.Client, servers []ServerInfo, logger log.FieldLogger) ([]string, error) {
	ctx := context.Background()
	snapshotIDs := make([]string, 0, len(servers))

	for i, server := range servers {
		logger.Infof("creating snapshot %d/%d for instance %s", i+1, len(servers), server.Name)

		snapshotID, err := openstackClient.CreateServerSnapshot(ctx, server.ID, server.Name)
		if err != nil {
			// Enhanced error handling for conflicts
			if strings.Contains(err.Error(), "task_state deleting") || strings.Contains(err.Error(), "409") {
				return nil, fmt.Errorf("hibernation conflict: instance %s is being modified by another process", server.Name)
			}
			return nil, fmt.Errorf("failed to create snapshot for %s: %v", server.Name, err)
		}

		snapshotIDs = append(snapshotIDs, snapshotID)
		logger.Infof("snapshot created for %s (ID: %s)", server.Name, snapshotID)
	}
	return snapshotIDs, nil
}

// waitForSnapshots waits for all snapshots to complete
func (a *openstackActuator) waitForSnapshots(openstackClient openstackclient.Client, snapshotIDs []string, servers []ServerInfo, logger log.FieldLogger) error {
	for i, snapshotID := range snapshotIDs {
		serverName := servers[i].Name
		logger.Infof("waiting for snapshot %s to complete for %s", snapshotID, serverName)

		err := a.waitForSnapshotCompletion(openstackClient, snapshotID, serverName, logger)
		if err != nil {
			return fmt.Errorf("failed to wait for snapshot %s: %v", snapshotID, err)
		}
	}
	return nil
}

// deleteInstances deletes all instances
func (a *openstackActuator) deleteInstances(openstackClient openstackclient.Client, servers []ServerInfo, logger log.FieldLogger) error {
	ctx := context.Background()

	for i, server := range servers {
		logger.Infof("deleting instance %d/%d: %s", i+1, len(servers), server.Name)

		err := openstackClient.DeleteServer(ctx, server.ID)
		if err != nil {
			return fmt.Errorf("failed to delete %s: %v", server.Name, err)
		}
	}
	return nil
}

// waitForSnapshotCompletion waits for a snapshot to reach ACTIVE state
func (a *openstackActuator) waitForSnapshotCompletion(openstackClient openstackclient.Client, snapshotID, serverName string, logger log.FieldLogger) error {
	ctx := context.Background()
	maxWaitTime := 30 * time.Minute
	checkInterval := 10 * time.Second
	timeout := time.After(maxWaitTime)
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for snapshot %s to complete after %v", snapshotID, maxWaitTime)
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

// StartMachines recreates instances from snapshots using saved configuration
func (a *openstackActuator) StartMachines(cd *hivev1.ClusterDeployment, hiveClient client.Client, logger log.FieldLogger) error {
	logger = logger.WithField("cloud", "openstack")
	logger.Info("starting machines from snapshots")

	// Only proceed if PowerState is Running
	if cd.Spec.PowerState != hivev1.ClusterPowerStateRunning {
		logger.Infof("PowerState is %s, not Running - refusing to start machines", cd.Spec.PowerState)
		return nil
	}

	openstackClient, err := a.openstackClientFn(cd, hiveClient, logger)
	if err != nil {
		return fmt.Errorf("failed to create OpenStack client: %v", err)
	}

	// Check for existing instances
	infraID := cd.Spec.ClusterMetadata.InfraID
	existingServers, err := a.findInstancesByPrefix(openstackClient, infraID)
	if err != nil {
		logger.Warnf("could not check existing instances: %v", err)
	}

	if len(existingServers) > 0 {
		logger.Info("instances already exist - clearing hibernation config")
		_ = a.deleteHibernationConfigSecret(cd, hiveClient, logger) // Best effort cleanup
		return nil
	}

	// Load hibernation config and restore
	instances, err := a.loadHibernationConfigFromSecret(cd, hiveClient, logger)
	if err != nil {
		logger.Warnf("no hibernation config found: %v", err)
		logger.Warn("cannot recreate instances without hibernation snapshots")
		return nil // Don't fail - let controller handle this gracefully
	}

	logger.Infof("restoring %d instances from hibernation snapshots", len(instances))
	return a.restoreFromHibernationConfig(cd, hiveClient, openstackClient, instances, logger)
}

// restoreFromHibernationConfig recreates instances from hibernation configuration
func (a *openstackActuator) restoreFromHibernationConfig(cd *hivev1.ClusterDeployment, hiveClient client.Client, openstackClient openstackclient.Client, instances []OpenStackInstanceConfig, logger log.FieldLogger) error {
	ctx := context.Background()

	logger.Infof("restoring %d instances from hibernation", len(instances))

	// Recreate each instance using saved configuration
	createdServerIDs := make([]string, 0, len(instances))

	for i, instance := range instances {
		logger.Infof("creating instance %d/%d: %s", i+1, len(instances), instance.Name)

		// Validate snapshot exists
		_, err := openstackClient.GetImage(ctx, instance.SnapshotID)
		if err != nil {
			return fmt.Errorf("snapshot %s not found: %v", instance.SnapshotID, err)
		}

		// Build server creation options
		createOpts := &openstackclient.ServerCreateOpts{
			Name:           instance.Name,
			ImageRef:       instance.SnapshotID,
			FlavorRef:      instance.Flavor,
			NetworkID:      instance.NetworkID,
			PortID:         instance.PortID,
			SecurityGroups: instance.SecurityGroups,
			Metadata: map[string]string{
				"openshiftClusterID": instance.OpenshiftClusterID,
			},
		}

		newServer, err := openstackClient.CreateServerFromOpts(ctx, createOpts)
		if err != nil {
			return fmt.Errorf("failed to create instance %s: %v", instance.Name, err)
		}

		createdServerIDs = append(createdServerIDs, newServer.ID)
		logger.Infof("created instance %s (ID: %s)", instance.Name, newServer.ID)
	}

	// Wait for instances to become ACTIVE
	logger.Info("waiting for instances to become active")
	for i, serverID := range createdServerIDs {
		instanceName := instances[i].Name
		err := a.waitForServerActive(openstackClient, serverID, instanceName, logger)
		if err != nil {
			return fmt.Errorf("failed to wait for instance %s to become active: %v", instanceName, err)
		}
	}

	// Clear hibernation config since restoration is complete
	logger.Info("restoration completed - clearing hibernation configuration")
	err := a.deleteHibernationConfigSecret(cd, hiveClient, logger)
	if err != nil {
		logger.Warnf("could not clear hibernation config: %v", err)
	}

	logger.Info("restoration completed successfully")
	return nil
}

// waitForServerActive waits for a server to reach ACTIVE state
func (a *openstackActuator) waitForServerActive(openstackClient openstackclient.Client, serverID, serverName string, logger log.FieldLogger) error {
	ctx := context.Background()
	maxWaitTime := 30 * time.Minute
	checkInterval := 15 * time.Second
	timeout := time.After(maxWaitTime)
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for server %s to become ACTIVE after %v", serverID, maxWaitTime)
		case <-ticker.C:
			server, err := openstackClient.GetServer(ctx, serverID)
			if err != nil {
				logger.Warnf("error checking server %s status: %v", serverID, err)
				continue
			}

			logger.Infof("server %s (%s) status: %s", serverID, serverName, server.Status)

			switch server.Status {
			case "ACTIVE":
				return nil
			case "BUILD", "REBUILD":
				continue
			case "ERROR", "DELETED":
				return fmt.Errorf("server %s failed with status: %s", serverID, server.Status)
			default:
				continue
			}
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
	matchingServers, err := a.findInstancesByPrefix(openstackClient, infraID)
	if err != nil {
		return false, nil, fmt.Errorf("error finding instances: %v", err)
	}

	logger.Infof("found %d instances with prefix '%s'", len(matchingServers), infraID)

	if len(matchingServers) == 0 {
		logger.Info("no instances found - machines are not running")
		return false, []string{"no instances found"}, nil
	}

	// OpenStack-specific: Check actual instance states
	runningCount, deletingInstances := a.categorizeInstanceStates(openstackClient, matchingServers, logger)

	// If instances are being deleted, hibernation is in progress
	if len(deletingInstances) > 0 && runningCount == 0 {
		logger.Infof("all instances are being deleted (%v) - hibernation in progress", deletingInstances)
		return false, []string{"instances-being-deleted"}, nil
	}

	return runningCount > 0, []string{}, nil
}

// categorizeInstanceStates checks the actual state of instances in OpenStack
func (a *openstackActuator) categorizeInstanceStates(openstackClient openstackclient.Client, servers []ServerInfo, logger log.FieldLogger) (int, []string) {
	ctx := context.Background()
	runningCount := 0
	var deletingInstances []string

	for _, server := range servers {
		serverDetails, err := openstackClient.GetServer(ctx, server.ID)
		if err != nil {
			logger.Warnf("could not get server %s details: %v", server.Name, err)
			runningCount++ // Assume running if we can't check
			continue
		}

		status := strings.ToLower(serverDetails.Status)
		if strings.Contains(status, "delet") || status == "shutoff" || status == "error" {
			logger.Infof("instance %s is being deleted/stopped (status: %s)", server.Name, serverDetails.Status)
			deletingInstances = append(deletingInstances, server.Name)
		} else {
			logger.Infof("instance %s is running (status: %s)", server.Name, serverDetails.Status)
			runningCount++
		}
	}

	return runningCount, deletingInstances
}

// MachinesStopped checks if machines are stopped
func (a *openstackActuator) MachinesStopped(cd *hivev1.ClusterDeployment, hiveClient client.Client, logger log.FieldLogger) (bool, []string, error) {
	logger = logger.WithField("cloud", "openstack")
	logger.Info("checking if machines are stopped")

	openstackClient, err := a.openstackClientFn(cd, hiveClient, logger)
	if err != nil {
		return false, nil, fmt.Errorf("failed to create OpenStack client: %v", err)
	}

	infraID := cd.Spec.ClusterMetadata.InfraID
	matchingServers, err := a.findInstancesByPrefix(openstackClient, infraID)
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

// ServerInfo holds basic server information
type ServerInfo struct {
	ID   string
	Name string
}

type OpenStackInstanceConfig struct {
	Name               string   `json:"name"`
	Flavor             string   `json:"flavor"`
	PortID             string   `json:"portID"`
	SnapshotID         string   `json:"snapshotID"`
	SecurityGroups     []string `json:"securityGroups"`
	ClusterID          string   `json:"clusterID"`
	NetworkID          string   `json:"networkID"`
	OpenshiftClusterID string   `json:"openshiftClusterID"`
}

// findInstancesByPrefix returns servers that match the infraID prefix
func (a *openstackActuator) findInstancesByPrefix(openstackClient openstackclient.Client, prefix string) ([]ServerInfo, error) {
	ctx := context.Background()

	servers, err := openstackClient.ListServers(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("error listing servers: %v", err)
	}

	var matchingServers []ServerInfo
	for _, server := range servers {
		if strings.HasPrefix(server.Name, prefix) {
			matchingServers = append(matchingServers, ServerInfo{
				ID:   server.ID,
				Name: server.Name,
			})
		}
	}

	return matchingServers, nil
}

// Configuration persistence methods
func (a *openstackActuator) saveInstanceConfigurationToSecret(cd *hivev1.ClusterDeployment, hiveClient client.Client, openstackClient openstackclient.Client, matchingServers []ServerInfo, snapshotIDs []string, logger log.FieldLogger) error {
	ctx := context.Background()

	if len(matchingServers) == 0 {
		return nil
	}

	if len(snapshotIDs) != len(matchingServers) {
		return fmt.Errorf("mismatch between servers (%d) and snapshot IDs (%d)", len(matchingServers), len(snapshotIDs))
	}

	// Use infraID directly instead of extracting from instance name
	infraID := cd.Spec.ClusterMetadata.InfraID

	// Get shared configuration
	networkID, err := a.getNetworkIDForCluster(openstackClient, infraID)
	if err != nil {
		return fmt.Errorf("error getting network ID: %v", err)
	}

	// Get openshiftClusterID from first instance
	server, err := openstackClient.GetServer(ctx, matchingServers[0].ID)
	if err != nil {
		return fmt.Errorf("error getting server details: %v", err)
	}

	var openshiftClusterID string
	if server.Metadata != nil {
		if id, exists := server.Metadata["openshiftClusterID"]; exists {
			openshiftClusterID = id
		}
	}

	// Get all ports once for efficiency
	allPorts, err := openstackClient.ListPorts(ctx)
	if err != nil {
		return fmt.Errorf("error listing ports: %v", err)
	}

	// Build configuration for each instance
	var instanceConfigs []OpenStackInstanceConfig
	for i, serverInfo := range matchingServers {
		serverDetails, err := openstackClient.GetServer(ctx, serverInfo.ID)
		if err != nil {
			return fmt.Errorf("error getting server details for %s: %v", serverInfo.Name, err)
		}

		// Get flavor ID
		var flavorID string
		if serverDetails.Flavor != nil {
			if id, ok := serverDetails.Flavor["id"].(string); ok {
				flavorID = id
			} else {
				return fmt.Errorf("could not extract flavor ID for %s", serverInfo.Name)
			}
		} else {
			return fmt.Errorf("no flavor information found for %s", serverInfo.Name)
		}

		// Find matching port
		var portID string
		for _, port := range allPorts {
			if port.Name == serverInfo.Name || port.Name == serverInfo.Name+"-0" {
				portID = port.ID
				break
			}
		}

		if portID == "" {
			return fmt.Errorf("no port found for instance %s", serverInfo.Name)
		}

		// Get security groups
		secGroups, err := openstackClient.GetServerSecurityGroups(ctx, serverInfo.ID)
		if err != nil {
			return fmt.Errorf("error getting security groups for %s: %v", serverInfo.Name, err)
		}

		instanceConfigs = append(instanceConfigs, OpenStackInstanceConfig{
			Name:               serverInfo.Name,
			Flavor:             flavorID,
			PortID:             portID,
			SnapshotID:         snapshotIDs[i],
			SecurityGroups:     secGroups,
			ClusterID:          infraID, // Use infraID directly
			NetworkID:          networkID,
			OpenshiftClusterID: openshiftClusterID,
		})
	}

	return a.saveHibernationConfigToSecret(cd, hiveClient, instanceConfigs, logger)
}

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

// getNetworkIDForCluster finds the network ID for a specific cluster using infraID
func (a *openstackActuator) getNetworkIDForCluster(openstackClient openstackclient.Client, infraID string) (string, error) {
	ctx := context.Background()
	networkName := fmt.Sprintf("%s-openshift", infraID)

	network, err := openstackClient.GetNetworkByName(ctx, networkName)
	if err != nil {
		return "", fmt.Errorf("failed to find network '%s': %w", networkName, err)
	}

	return network.ID, nil
}
