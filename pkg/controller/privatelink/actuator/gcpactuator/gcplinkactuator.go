package gcpactuator

import (
	"fmt"
	"sort"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"google.golang.org/api/compute/v1"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/controller/privatelink/actuator"
	"github.com/openshift/hive/pkg/controller/privatelink/conditions"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/gcpclient"
)

const (
	// Default cidr to use for the service attachment subnet.
	defaultServiceAttachmentSubnetCidr = "192.168.0.0/29"
)

// Ensure GCPLinkActuator implements the Actuator interface. This will fail at compile time when false.
var _ actuator.Actuator = &GCPLinkActuator{}

type GCPLinkActuator struct {
	client *client.Client

	config *hivev1.GCPPrivateServiceConnectConfig

	gcpClientHub   gcpclient.Client
	gcpClientSpoke gcpclient.Client
}

func NewGCPLinkActuator(
	client *client.Client,
	config *hivev1.GCPPrivateServiceConnectConfig,
	cd *hivev1.ClusterDeployment,
	gcpClientFn gcpClientFn,
	logger log.FieldLogger) (*GCPLinkActuator, error) {

	actuator := &GCPLinkActuator{
		client: client,
		config: config,
	}

	if config == nil {
		return nil, errors.New("unable to create GCP actuator: config is empty")
	}

	if cd == nil || cd.Spec.Platform.GCP == nil {
		return nil, errors.New("unable to create GCP actuator: cluster deployment spec does not contain GCP platform")
	}

	hubClient, err := newGCPClient(*client, gcpClientFn, actuator.config.CredentialsSecretRef.Name, controllerutils.GetHiveNamespace())
	if err != nil {
		return nil, err
	}
	actuator.gcpClientHub = hubClient

	spokeClient, err := newGCPClient(*client, gcpClientFn, cd.Spec.Platform.GCP.CredentialsSecretRef.Name, cd.Namespace)
	if err != nil {
		return nil, err
	}
	actuator.gcpClientSpoke = spokeClient

	return actuator, nil
}

// Cleanup is the actuator interface for cleaning up the cloud resources.
func (a *GCPLinkActuator) Cleanup(cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata, logger log.FieldLogger) error {
	if err := a.cleanupEndpoint(cd, metadata, logger); err != nil {
		return errors.Wrap(err, "error cleaning up endpoint")
	}

	if err := a.cleanupEndpointAddress(cd, metadata, logger); err != nil {
		return errors.Wrap(err, "error cleaning up endpoint address")
	}

	if err := a.cleanupServiceAttachment(cd, metadata, logger); err != nil {
		return errors.Wrap(err, "error cleaning up service attachment")
	}

	// Skip the remaining subnet components when using a preexisting subnet.
	if subnetName := getServiceAttachmentSubnetExistingName(cd); subnetName != "" {
		return nil
	}

	if err := a.cleanupServiceAttachmentFirewall(cd, metadata, logger); err != nil {
		return errors.Wrap(err, "error cleaning up service attachment firewall")
	}

	if err := a.cleanupServiceAttachmentSubnet(cd, metadata, logger); err != nil {
		return errors.Wrap(err, "error cleaning up service attachment subnet")
	}

	return nil
}

// CleanupRequired is the actuator interface for determining if cleanup is required.
func (a *GCPLinkActuator) CleanupRequired(cd *hivev1.ClusterDeployment) bool {
	// There is nothing to do when PrivateServiceConnect is undefined.
	// This either means it was never enabled, or it was already cleaned up.
	if cd.Status.Platform == nil ||
		cd.Status.Platform.GCP == nil ||
		cd.Status.Platform.GCP.PrivateServiceConnect == nil {
		return false
	}

	// There is nothing to do when deleting a ClusterDeployment with PreserveOnDelete and PrivateServiceConnect enabled.
	// If a ClusterDeployment is deleted after a failed install with PreserveOnDelete set, the PrivateServiceConnect
	// resources are not cleaned up. This is by design as the rest of the cloud resources are also not cleaned up.
	if cd.DeletionTimestamp != nil &&
		cd.Spec.PreserveOnDelete &&
		cd.Spec.Platform.GCP.PrivateServiceConnect.Enabled {
		return false
	}

	// The subnet resources do not need to be cleaned up if a preexisting subnet was used.
	shouldCleanupSubnet := getServiceAttachmentSubnetExistingName(cd) == ""

	return cd.Status.Platform.GCP.PrivateServiceConnect.Endpoint != "" ||
		cd.Status.Platform.GCP.PrivateServiceConnect.EndpointAddress != "" ||
		cd.Status.Platform.GCP.PrivateServiceConnect.ServiceAttachment != "" ||
		(shouldCleanupSubnet && cd.Status.Platform.GCP.PrivateServiceConnect.ServiceAttachmentFirewall != "") ||
		(shouldCleanupSubnet && cd.Status.Platform.GCP.PrivateServiceConnect.ServiceAttachmentSubnet != "")
}

// Reconcile is the actuator interface for reconciling the cloud resources.
func (a *GCPLinkActuator) Reconcile(cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata, dnsRecord *actuator.DnsRecord, logger log.FieldLogger) (reconcile.Result, error) {
	logger.Debug("reconciling link resources")

	endpointSubnet, err := chooseSubnetForEndpoint(a.gcpClientHub, *a.config, cd.Spec.Platform.GCP.Region)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "error choosing a Subnet for the Endpoint")
	}

	forwardingRule, err := a.gcpClientSpoke.GetForwardingRule(metadata.InfraID+"-api-internal", cd.Spec.Platform.GCP.Region)
	if isNotFound(err) {
		logger.Debug("waiting for cluster api forwarding rule to be provisioned, will retry soon.")
		return requeueLater, nil
	}
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to find the cluster api Forwarding Rule")
	}

	var subnet *compute.Subnetwork
	// Use a preexisting subnet if configured.
	if subnetName := getServiceAttachmentSubnetExistingName(cd); subnetName != "" {
		subnet, err = a.gcpClientSpoke.GetSubnet(subnetName, cd.Spec.Platform.GCP.Region, getServiceAttachmentSubnetExistingProject(cd))
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to find the specified service attachment subnet")
		}
		logger.WithField("subnetName", subnet.Name).Debug("reconciling Service Attachment Subnet: using existing subnet")
	} else { // Otherwise create a new subnet
		network, err := a.gcpClientSpoke.GetNetwork(metadata.InfraID + "-network")
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to find the cluster Network")
		}

		logger.Debug("reconciling Service Attachment Subnet")
		subnetModified, newSubnet, err := a.ensureServiceAttachmentSubnet(cd, metadata, network)
		if err != nil {
			if err := conditions.SetErrConditionWithRetry(*a.client, cd, "ServiceAttachmentSubnetReconcileFailed", errors.New(controllerutils.ErrorScrub(err)), logger); err != nil {
				logger.WithError(err).Error("failed to update condition on cluster deployment")
			}
			return reconcile.Result{}, errors.Wrap(err, "failed to reconcile the Service Attachment Subnet")
		}
		if subnetModified {
			err := conditions.SetReadyConditionWithRetry(*a.client, cd, corev1.ConditionFalse,
				"ReconciledServiceAttachmentSubnet",
				"reconciled the Service Attachment Subnet",
				logger)
			if err != nil {
				return reconcile.Result{}, errors.Wrap(err, "failed to update condition on cluster deployment")
			}
		}
		subnet = newSubnet

		logger.Debug("reconciling Service Attachment Firewall")
		firewallModified, _, err := a.ensureServiceAttachmentFirewall(cd, metadata, network)
		if err != nil {
			if err := conditions.SetErrConditionWithRetry(*a.client, cd, "ServiceAttachmentFirewallReconcileFailed", errors.New(controllerutils.ErrorScrub(err)), logger); err != nil {
				logger.WithError(err).Error("failed to update condition on cluster deployment")
			}
			return reconcile.Result{}, errors.Wrap(err, "failed to reconcile the Service Attachment Firwall")
		}
		if firewallModified {
			err := conditions.SetReadyConditionWithRetry(*a.client, cd, corev1.ConditionFalse,
				"ReconciledServiceAttachmentFirewall",
				"reconciled the Service Attachment Firewall",
				logger)
			if err != nil {
				return reconcile.Result{}, errors.Wrap(err, "failed to update condition on cluster deployment")
			}
		}
	}

	logger.Debug("reconciling Service Attachment")
	serviceAttachmentModified, serviceAttachment, err := a.ensureServiceAttachment(cd, metadata, forwardingRule, subnet)
	if err != nil {
		if err := conditions.SetErrConditionWithRetry(*a.client, cd, "ServiceAttachmentReconcileFailed", errors.New(controllerutils.ErrorScrub(err)), logger); err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
		}
		return reconcile.Result{}, errors.Wrap(err, "failed to reconcile the Service Attachment")
	}
	if serviceAttachmentModified {
		err := conditions.SetReadyConditionWithRetry(*a.client, cd, corev1.ConditionFalse,
			"ReconciledServiceAttachment",
			"reconciled the Service Attachment",
			logger)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to update condition on cluster deployment")
		}
	}

	logger.Debug("reconciling Endpoint Address")
	addressModified, ipAddress, err := a.ensureEndpointAddress(cd, metadata, endpointSubnet)
	if err != nil {
		if err := conditions.SetErrConditionWithRetry(*a.client, cd, "EndpointAddressReconcileFailed", errors.New(controllerutils.ErrorScrub(err)), logger); err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
		}
		return reconcile.Result{}, errors.Wrap(err, "failed to reconcile the Endpoint Address")
	}
	if addressModified {
		err := conditions.SetReadyConditionWithRetry(*a.client, cd, corev1.ConditionFalse,
			"ReconciledEndpointAddress",
			"reconciled the Endpoint Address",
			logger)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to update condition on cluster deployment")
		}
	}

	logger.Debug("reconciling Endpoint")
	endpointModified, endpoint, err := a.ensureEndpoint(cd, metadata, ipAddress.SelfLink, ipAddress.Subnetwork, serviceAttachment.SelfLink)
	if err != nil {
		if err := conditions.SetErrConditionWithRetry(*a.client, cd, "EndpointReconcileFailed", errors.New(controllerutils.ErrorScrub(err)), logger); err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
		}
		return reconcile.Result{}, errors.Wrap(err, "failed to reconcile the Endpoint")
	}
	if endpointModified {
		err := conditions.SetReadyConditionWithRetry(*a.client, cd, corev1.ConditionFalse,
			"ReconciledEndpoint",
			"reconciled the Endpoint",
			logger)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to update condition on cluster deployment")
		}
	}

	// Set the DNS IP Addresses for the hub actuator.
	dnsRecord.IpAddress = []string{endpoint.IPAddress}

	return reconcile.Result{}, nil
}

// ShouldSync is the actuator interface to determine if there are changes that need to be made.
func (a *GCPLinkActuator) ShouldSync(cd *hivev1.ClusterDeployment) bool {
	shouldCreateSubnet := getServiceAttachmentSubnetExistingName(cd) == ""

	return cd.Status.Platform == nil ||
		cd.Status.Platform.GCP == nil ||
		cd.Status.Platform.GCP.PrivateServiceConnect == nil ||
		cd.Status.Platform.GCP.PrivateServiceConnect.Endpoint == "" ||
		cd.Status.Platform.GCP.PrivateServiceConnect.EndpointAddress == "" ||
		cd.Status.Platform.GCP.PrivateServiceConnect.ServiceAttachment == "" ||
		(shouldCreateSubnet && cd.Status.Platform.GCP.PrivateServiceConnect.ServiceAttachmentFirewall == "") ||
		(shouldCreateSubnet && cd.Status.Platform.GCP.PrivateServiceConnect.ServiceAttachmentSubnet == "")
}

// ensureServiceAttachmentSubnet creates the service attachment subnet if it does not already exist.
func (a *GCPLinkActuator) ensureServiceAttachmentSubnet(cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata, network *compute.Network) (bool, *compute.Subnetwork, error) {
	modified := false

	subnetName := metadata.InfraID + "-psc"
	subnet, err := a.gcpClientSpoke.GetSubnet(subnetName, cd.Spec.Platform.GCP.Region, "")
	if isNotFound(err) {
		cidr := getServiceAttachmentSubnetCIDR(cd)
		newSubnet, err := a.createServiceAttachmentSubnet(subnetName, cidr, network.SelfLink, cd.Spec.Platform.GCP.Region)
		if err != nil {
			return false, nil, err
		}
		modified = true
		subnet = newSubnet
	} else if err != nil {
		return false, nil, err
	}

	initPrivateServiceConnectStatus(cd)
	if cd.Status.Platform.GCP.PrivateServiceConnect.ServiceAttachmentSubnet != subnet.SelfLink {
		cd.Status.Platform.GCP.PrivateServiceConnect.ServiceAttachmentSubnet = subnet.SelfLink
		if err := updatePrivateServiceConnectStatus(a.client, cd); err != nil {
			return false, nil, errors.Wrap(err, "error updating clusterdeployment status with ServiceAttachmentSubnet")
		}
		modified = true
	}

	return modified, subnet, nil
}

func getServiceAttachmentSubnetCIDR(cd *hivev1.ClusterDeployment) string {
	if cd != nil &&
		cd.Spec.Platform.GCP != nil &&
		cd.Spec.Platform.GCP.PrivateServiceConnect != nil &&
		cd.Spec.Platform.GCP.PrivateServiceConnect.ServiceAttachment != nil &&
		cd.Spec.Platform.GCP.PrivateServiceConnect.ServiceAttachment.Subnet != nil &&
		cd.Spec.Platform.GCP.PrivateServiceConnect.ServiceAttachment.Subnet.Cidr != "" {

		return cd.Spec.Platform.GCP.PrivateServiceConnect.ServiceAttachment.Subnet.Cidr
	}
	return defaultServiceAttachmentSubnetCidr
}

func getServiceAttachmentSubnetExistingName(cd *hivev1.ClusterDeployment) string {
	if cd != nil &&
		cd.Spec.Platform.GCP != nil &&
		cd.Spec.Platform.GCP.PrivateServiceConnect != nil &&
		cd.Spec.Platform.GCP.PrivateServiceConnect.ServiceAttachment != nil &&
		cd.Spec.Platform.GCP.PrivateServiceConnect.ServiceAttachment.Subnet != nil &&
		cd.Spec.Platform.GCP.PrivateServiceConnect.ServiceAttachment.Subnet.Existing != nil {

		return cd.Spec.Platform.GCP.PrivateServiceConnect.ServiceAttachment.Subnet.Existing.Name
	}
	return ""
}

func getServiceAttachmentSubnetExistingProject(cd *hivev1.ClusterDeployment) string {
	if cd != nil &&
		cd.Spec.Platform.GCP != nil &&
		cd.Spec.Platform.GCP.PrivateServiceConnect != nil &&
		cd.Spec.Platform.GCP.PrivateServiceConnect.ServiceAttachment != nil &&
		cd.Spec.Platform.GCP.PrivateServiceConnect.ServiceAttachment.Subnet != nil &&
		cd.Spec.Platform.GCP.PrivateServiceConnect.ServiceAttachment.Subnet.Existing != nil {

		return cd.Spec.Platform.GCP.PrivateServiceConnect.ServiceAttachment.Subnet.Existing.Project
	}
	return ""
}

// createServiceAttachmentSubnet creates the service attachment subnet.
func (a *GCPLinkActuator) createServiceAttachmentSubnet(subnetName string, cidr string, network string, region string) (*compute.Subnetwork, error) {
	subnet, err := a.gcpClientSpoke.CreateSubnet(subnetName, cidr, network, region, "PRIVATE_SERVICE_CONNECT")
	if err != nil || subnet == nil {
		return nil, errors.Wrap(err, "error creating the Service Attachment Subnet")
	}

	return subnet, nil
}

// cleanupServiceAttachmentSubnet deletes the service attachment subnet.
func (a *GCPLinkActuator) cleanupServiceAttachmentSubnet(cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata, logger log.FieldLogger) error {
	logger.Debug("cleaning up Service Attachment Subnet")

	subnetName := metadata.InfraID + "-psc"
	err := a.gcpClientSpoke.DeleteSubnet(subnetName, cd.Spec.Platform.GCP.Region)
	if err != nil && !isNotFound(err) {
		return errors.Wrap(err, "error deleting the Service Attachment Subnet")
	}

	initPrivateServiceConnectStatus(cd)
	if cd.Status.Platform.GCP.PrivateServiceConnect.ServiceAttachmentSubnet != "" {
		cd.Status.Platform.GCP.PrivateServiceConnect.ServiceAttachmentSubnet = ""
		if err := updatePrivateServiceConnectStatus(a.client, cd); err != nil {
			return errors.Wrap(err, "error updating clusterdeployment after cleanup of ServiceAttachmentSubnet")
		}
	}

	return nil
}

// ensureServiceAttachmentFirewall creates the service attachment firewall if it does not already exist.
func (a *GCPLinkActuator) ensureServiceAttachmentFirewall(cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata, network *compute.Network) (bool, *compute.Firewall, error) {
	modified := false

	firewallName := metadata.InfraID + "-psc"
	firewall, err := a.gcpClientSpoke.GetFirewall(firewallName)
	if isNotFound(err) {
		cidr := getServiceAttachmentSubnetCIDR(cd)
		targets := []string{
			metadata.InfraID + "-master",        // 4.15 and older
			metadata.InfraID + "-control-plane", // 4.16 and newer
		}
		newFirewall, err := a.createServiceAttachmentFirewall(firewallName, cidr, network, targets)
		if err != nil {
			return false, nil, err
		}
		modified = true
		firewall = newFirewall
	} else if err != nil {
		return false, nil, err
	}

	initPrivateServiceConnectStatus(cd)
	if cd.Status.Platform.GCP.PrivateServiceConnect.ServiceAttachmentFirewall != firewall.SelfLink {
		cd.Status.Platform.GCP.PrivateServiceConnect.ServiceAttachmentFirewall = firewall.SelfLink
		if err := updatePrivateServiceConnectStatus(a.client, cd); err != nil {
			return false, nil, errors.Wrap(err, "error updating clusterdeployment status with ServiceAttachmentFirewall")
		}
		modified = true
	}

	return modified, firewall, nil
}

// createServiceAttachmentFirewall creates the service attachment firewall.
func (a *GCPLinkActuator) createServiceAttachmentFirewall(firewallName string, cidr string, network *compute.Network, targets []string) (*compute.Firewall, error) {
	allowed := []*compute.FirewallAllowed{{IPProtocol: "TCP", Ports: []string{"6443"}}}

	firewall, err := a.gcpClientSpoke.CreateFirewall(firewallName, allowed, "INBOUND", network.SelfLink, []string{cidr}, targets)
	if err != nil || firewall == nil {
		return nil, errors.Wrap(err, "error creating the Service Attachment Firewall")
	}

	return firewall, nil
}

// cleanupServiceAttachmentFirewall deletes the service attachment firewall.
func (a *GCPLinkActuator) cleanupServiceAttachmentFirewall(cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata, logger log.FieldLogger) error {
	logger.Debug("cleaning up Service Attachment Firewall")

	firewallName := metadata.InfraID + "-psc"
	err := a.gcpClientSpoke.DeleteFirewall(firewallName)
	if err != nil && !isNotFound(err) {
		return errors.Wrap(err, "error deleting the Service Attachment Firewall")
	}

	initPrivateServiceConnectStatus(cd)
	if cd.Status.Platform.GCP.PrivateServiceConnect.ServiceAttachmentFirewall != "" {
		cd.Status.Platform.GCP.PrivateServiceConnect.ServiceAttachmentFirewall = ""
		if err := updatePrivateServiceConnectStatus(a.client, cd); err != nil {
			return errors.Wrap(err, "error updating clusterdeployment after cleanup of ServiceAttachmentFirewall")
		}
	}

	return nil
}

// ensureServiceAttachment creates the service attachment if it does not already exist.
func (a *GCPLinkActuator) ensureServiceAttachment(cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata, forwardingRule *compute.ForwardingRule, subnet *compute.Subnetwork) (bool, *compute.ServiceAttachment, error) {
	modified := false

	serviceAttachmentName := metadata.InfraID + "psc"
	serviceAttachment, err := a.gcpClientSpoke.GetServiceAttachment(serviceAttachmentName, cd.Spec.Platform.GCP.Region)
	if isNotFound(err) {
		newServiceAttachment, err := a.createServiceAttachment(serviceAttachmentName, cd.Spec.Platform.GCP.Region, forwardingRule, subnet)
		if err != nil {
			return false, nil, err
		}
		modified = true
		serviceAttachment = newServiceAttachment
	} else if err != nil {
		return false, nil, err
	}

	initPrivateServiceConnectStatus(cd)
	if cd.Status.Platform.GCP.PrivateServiceConnect.ServiceAttachment != serviceAttachment.SelfLink {
		cd.Status.Platform.GCP.PrivateServiceConnect.ServiceAttachment = serviceAttachment.SelfLink
		if err := updatePrivateServiceConnectStatus(a.client, cd); err != nil {
			return false, nil, errors.Wrap(err, "error updating clusterdeployment status with ServiceAttachment")
		}
		modified = true
	}

	return modified, serviceAttachment, nil
}

// createServiceAttachment creates the service attachment.
func (a *GCPLinkActuator) createServiceAttachment(serviceAttachmentName string, region string, forwardingRule *compute.ForwardingRule, subnet *compute.Subnetwork) (*compute.ServiceAttachment, error) {
	subnets := []string{subnet.SelfLink}
	allowedProjects := []string{a.gcpClientHub.GetProjectName()}
	serviceAttachment, err := a.gcpClientSpoke.CreateServiceAttachment(serviceAttachmentName, region, forwardingRule.SelfLink, subnets, allowedProjects)
	if err != nil || serviceAttachment == nil {
		return nil, errors.Wrap(err, "error creating the Service Attachment")
	}

	return serviceAttachment, nil
}

// cleanupServiceAttachment deletes the service attachment.
func (a *GCPLinkActuator) cleanupServiceAttachment(cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata, logger log.FieldLogger) error {
	logger.Debug("cleaning up Service Attachment")

	serviceAttachmentName := metadata.InfraID + "psc"
	err := a.gcpClientSpoke.DeleteServiceAttachment(serviceAttachmentName, cd.Spec.Platform.GCP.Region)
	if err != nil && !isNotFound(err) {
		return errors.Wrap(err, "error deleting the Service Attachment")
	}

	initPrivateServiceConnectStatus(cd)
	if cd.Status.Platform.GCP.PrivateServiceConnect.ServiceAttachment != "" {
		cd.Status.Platform.GCP.PrivateServiceConnect.ServiceAttachment = ""
		if err := updatePrivateServiceConnectStatus(a.client, cd); err != nil {
			return errors.Wrap(err, "error updating clusterdeployment after cleanup of ServiceAttachment")
		}
	}

	return nil
}

// ensureEndpointAddress creates the endpoint address if it does not already exist.
func (a *GCPLinkActuator) ensureEndpointAddress(cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata, subnetwork string) (bool, *compute.Address, error) {
	modified := false

	endpointAddressName := metadata.InfraID + "-psc"
	endpointAddress, err := a.gcpClientHub.GetAddress(endpointAddressName, cd.Spec.Platform.GCP.Region)
	if isNotFound(err) {
		newAddress, err := a.createEndpointAddress(endpointAddressName, cd.Spec.Platform.GCP.Region, subnetwork)
		if err != nil {
			return false, nil, err
		}
		modified = true
		endpointAddress = newAddress
	} else if err != nil {
		return false, nil, err
	}

	initPrivateServiceConnectStatus(cd)
	if cd.Status.Platform.GCP.PrivateServiceConnect.EndpointAddress != endpointAddress.SelfLink {
		cd.Status.Platform.GCP.PrivateServiceConnect.EndpointAddress = endpointAddress.SelfLink
		if err := updatePrivateServiceConnectStatus(a.client, cd); err != nil {
			return false, nil, errors.Wrap(err, "error updating clusterdeployment status with EndpointAddress")
		}
		modified = true
	}

	return modified, endpointAddress, nil
}

// createEndpointAddress creates the endpoint address.
func (a *GCPLinkActuator) createEndpointAddress(addressName string, region string, subnetwork string) (*compute.Address, error) {
	address, err := a.gcpClientHub.CreateAddress(addressName, region, subnetwork)
	if err != nil || address == nil {
		return nil, errors.Wrap(err, "error creating the Endpoint Address")
	}

	return address, nil
}

// cleanupEndpointAddress deletes the endpoint address.
func (a *GCPLinkActuator) cleanupEndpointAddress(cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata, logger log.FieldLogger) error {
	logger.Debug("cleaning up Endpoint Address")

	addressName := metadata.InfraID + "-psc"
	err := a.gcpClientHub.DeleteAddress(addressName, cd.Spec.Platform.GCP.Region)
	if err != nil && !isNotFound(err) {
		return errors.Wrap(err, "error deleting the Endpoint Address")
	}

	initPrivateServiceConnectStatus(cd)
	if cd.Status.Platform.GCP.PrivateServiceConnect.EndpointAddress != "" {
		cd.Status.Platform.GCP.PrivateServiceConnect.EndpointAddress = ""
		if err := updatePrivateServiceConnectStatus(a.client, cd); err != nil {
			return errors.Wrap(err, "error updating clusterdeployment after cleanup of EndpointAddress")
		}
	}

	return nil
}

// ensureEndpoint creates the endpoint if it does not already exist.
func (a *GCPLinkActuator) ensureEndpoint(cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata, ipAddress string, subnet string, serviceAttachment string) (bool, *compute.ForwardingRule, error) {
	modified := false

	endpointName := metadata.InfraID + "-psc"
	endpoint, err := a.gcpClientHub.GetForwardingRule(endpointName, cd.Spec.Platform.GCP.Region)
	if isNotFound(err) {
		newEndpoint, err := a.createEndpoint(endpointName, ipAddress, cd.Spec.Platform.GCP.Region, subnet, serviceAttachment)
		if err != nil || newEndpoint == nil {
			return false, nil, err
		}
		modified = true
		endpoint = newEndpoint
	} else if err != nil {
		return false, nil, err
	}

	initPrivateServiceConnectStatus(cd)
	if cd.Status.Platform.GCP.PrivateServiceConnect.Endpoint != endpoint.SelfLink {
		cd.Status.Platform.GCP.PrivateServiceConnect.Endpoint = endpoint.SelfLink
		if err := updatePrivateServiceConnectStatus(a.client, cd); err != nil {
			return false, nil, errors.Wrap(err, "error updating clusterdeployment status with Endpoint")
		}
		modified = true
	}

	return modified, endpoint, nil
}

// createEndpoint creates the endpoint
func (a *GCPLinkActuator) createEndpoint(endpoint string, ipAddress string, region string, subnet string, target string) (*compute.ForwardingRule, error) {
	forwardingRule, err := a.gcpClientHub.CreateForwardingRule(endpoint, ipAddress, region, subnet, target)
	if err != nil || forwardingRule == nil {
		return nil, errors.Wrap(err, "error creating the Endpoint")
	}

	return forwardingRule, nil
}

// cleanupEndpoint deletes the endpoint
func (a *GCPLinkActuator) cleanupEndpoint(cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata, logger log.FieldLogger) error {
	logger.Debug("cleaning up Endpoint")

	endpointName := metadata.InfraID + "-psc"
	err := a.gcpClientHub.DeleteForwardingRule(endpointName, cd.Spec.Platform.GCP.Region)
	if err != nil && !isNotFound(err) {
		return errors.Wrap(err, "error deleting the Endpoint")
	}

	initPrivateServiceConnectStatus(cd)
	if cd.Status.Platform.GCP.PrivateServiceConnect.Endpoint != "" {
		cd.Status.Platform.GCP.PrivateServiceConnect.Endpoint = ""
		if err := updatePrivateServiceConnectStatus(a.client, cd); err != nil {
			return errors.Wrap(err, "error updating clusterdeployment after cleanup of Endpoint")
		}
	}

	return nil
}

func chooseSubnetForEndpoint(gcpClient gcpclient.Client, config hivev1.GCPPrivateServiceConnectConfig, region string) (string, error) {
	// Filter out the subnets not in cluster region.
	candidates, err := filterSubnetInventory(gcpClient, config.DeepCopy().EndpointVPCInventory, region)
	if err != nil {
		return "", err
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("no supported subnet in inventory for region %s", region)
	}

	// Determine how many addresses are in each subnet
	addressesPerSubnet := map[string]int{}
	for _, candidate := range candidates {
		opts := gcpclient.ListAddressesOptions{
			Filter: fmt.Sprintf("subnetwork eq '.*%s.*'", candidate.Subnet),
		}
		addresses, err := gcpClient.ListAddresses(region, opts)
		if err != nil {
			return "", err
		}
		addressesPerSubnet[candidate.Subnet] = len(addresses.Items)
	}

	// "Spread" strategy: sort the candidates by the number of addresses already used, ascending,
	// and return the first (emptiest) one.
	sort.Slice(candidates, func(i, j int) bool {
		return addressesPerSubnet[candidates[i].Subnet] < addressesPerSubnet[candidates[j].Subnet]
	})

	// Return the first caididate, which should be one with the least number of addresses
	return candidates[0].Subnet, nil
}

func filterSubnetInventory(gcpClient gcpclient.Client, input []hivev1.GCPPrivateServiceConnectInventory, region string) ([]hivev1.GCPPrivateServiceConnectSubnet, error) {
	var filtered []hivev1.GCPPrivateServiceConnectSubnet

	for _, network := range input {
		for _, cand := range network.Subnets {
			if cand.Region == region {
				subnet, err := gcpClient.GetSubnet(cand.Subnet, cand.Region, "")
				if isNotFound(err) {
					// Ignoring subnets that are not found
					continue
				} else if err != nil {
					return nil, err
				}

				// Make sure we are using the selfLink
				cand.Subnet = subnet.SelfLink

				filtered = append(filtered, cand)
			}
		}
	}
	return filtered, nil
}
