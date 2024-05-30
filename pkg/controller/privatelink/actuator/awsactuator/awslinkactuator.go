package awsactuator

import (
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/sts"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	"github.com/openshift/hive/pkg/awsclient"
	"github.com/openshift/hive/pkg/controller/privatelink/actuator"
	"github.com/openshift/hive/pkg/controller/privatelink/conditions"
	"github.com/openshift/hive/pkg/controller/privatelink/dnsrecord"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

// Ensure AWSLinkActuator implements the Actuator interface. This will fail at compile time when false.
var _ actuator.Actuator = &AWSLinkActuator{}

type AWSLinkActuator struct {
	client *client.Client

	config *hivev1.AWSPrivateLinkConfig

	// testing purpose
	awsClientFn awsClientFn
}

// New creates a new AWS Link Actuator
func NewAWSLinkActuator(client *client.Client, config *hivev1.AWSPrivateLinkConfig, logger log.FieldLogger) (*AWSLinkActuator, error) {
	actuator := &AWSLinkActuator{
		client: client,
		config: config,
	}

	// Fall back to older AWSPrivateLinkController config for backwards compatability
	// If the new config is defined, it will be used. Otherwise we will
	// pull from the old config here overriding everything.
	if config == nil {
		oldConfig, err := ReadAWSPrivateLinkControllerConfigFile()
		if err != nil {
			return actuator, err
		}
		if oldConfig != nil {
			logger.Debug("falling back to AWSPrivateLinkController config")
			actuator.config = oldConfig
		}
	}

	// Enable tests to use function that generates a mock client
	actuator.awsClientFn = awsclient.New

	return actuator, nil
}

// Cleanup cleans up the cloud resources.
func (a *AWSLinkActuator) Cleanup(cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata, logger log.FieldLogger) error {
	awsClient, err := newAWSClient(*a.client, a.awsClientFn, cd, a.config)
	if err != nil {
		logger.WithError(err).Error("error creating AWS client for the cluster")
		return err
	}
	initPrivateLinkStatus(cd)
	if err := a.cleanupVPCEndpoint(awsClient.hub, cd, metadata, logger); err != nil {
		logger.WithError(err).Error("error cleaning up VPCEndpoint")
		return err
	}
	if err := a.cleanupVPCEndpointService(awsClient.user, cd, metadata, logger); err != nil {
		logger.WithError(err).Error("error cleaning up VPCEndpoint Service")
		return err
	}

	initPrivateLinkStatus(cd)
	cd.Status.Platform.AWS.PrivateLink.VPCEndpointID = ""
	cd.Status.Platform.AWS.PrivateLink.VPCEndpointService = hivev1aws.VPCEndpointService{}
	if err := updatePrivateLinkStatus(a.client, cd, logger); err != nil {
		logger.WithError(err).Error("error updating clusterdeployment after cleanup of private link")
		return err
	}

	return nil
}

// CleanupRequired returns true if there are resources to be cleaned up.
func (a *AWSLinkActuator) CleanupRequired(cd *hivev1.ClusterDeployment) bool {
	// There is nothing to do when PrivateLink is undefined. This either means it was never enabled, or it was already cleaned up.
	if cd.Status.Platform == nil || cd.Status.Platform.AWS == nil || cd.Status.Platform.AWS.PrivateLink == nil {
		return false
	}
	// There is nothing to do when deleting a ClusterDeployment with PreserveOnDelete and PrivateLink enabled.
	// NOTE: If a ClusterDeployment is deleted after a failed install with PreserveOnDelete set, the PrivateLink
	// resources are not cleaned up. This is by design as the rest of the cloud resources are also not cleaned up.
	if cd.DeletionTimestamp != nil &&
		cd.Spec.PreserveOnDelete &&
		cd.Spec.Platform.AWS.PrivateLink.Enabled {
		return false
	}
	return cd.Status.Platform.AWS.PrivateLink.VPCEndpointID != "" ||
		cd.Status.Platform.AWS.PrivateLink.VPCEndpointService.ID != "" ||
		cd.Status.Platform.AWS.PrivateLink.VPCEndpointService.Name != ""
}

// Reconcile reconciles the required resources.
func (a *AWSLinkActuator) Reconcile(
	cd *hivev1.ClusterDeployment,
	metadata *hivev1.ClusterMetadata,
	dnsRecord *dnsrecord.DnsRecord,
	logger log.FieldLogger) (reconcile.Result, error) {

	logger.Debug("reconciling link resources")
	awsClient, err := newAWSClient(*a.client, a.awsClientFn, cd, a.config)
	if err != nil {
		logger.WithError(err).Error("error creating AWS client for the cluster")
		return reconcile.Result{}, err
	}

	// discover the NLB for the cluster.
	nlbARN, err := discoverNLBForCluster(awsClient.user, metadata.InfraID, logger)
	if err != nil {
		if awsErrCodeEquals(err, "LoadBalancerNotFound") {
			logger.WithField("infraID", metadata.InfraID).Debug("NLB is not yet created for the cluster, will retry later")

			err := conditions.SetReadyCondition(cd, corev1.ConditionFalse,
				"DiscoveringNLBNotYetFound",
				"discovering NLB for the cluster, but it does not exist yet",
				*a.client, logger)
			if err != nil {
				logger.WithError(err).Error("failed to update condition on cluster deployment")
				return reconcile.Result{}, err
			}
			return reconcile.Result{RequeueAfter: defaultRequeueLater}, nil
		}

		logger.WithField("infraID", metadata.InfraID).WithError(err).Error("error discovering NLB for the cluster")

		if err := conditions.SetErrCondition(cd, "DiscoveringNLBFailed", errors.New(controllerutils.ErrorScrub(err)), *a.client, logger); err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
			return reconcile.Result{}, err
		}

		return reconcile.Result{}, err
	}

	// reconcile the VPC Endpoint Service
	serviceModified, vpcEndpointService, err := a.reconcileVPCEndpointService(awsClient, cd, metadata, nlbARN, logger)
	if err != nil {
		logger.WithError(err).Error("failed to reconcile the VPC Endpoint Service")

		err := conditions.SetErrCondition(cd, "VPCEndpointServiceReconcileFailed", errors.New(controllerutils.ErrorScrub(err)), *a.client, logger)
		if err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, errors.Wrap(err, "failed to reconcile the VPC Endpoint Service")
	}
	if serviceModified {
		err := conditions.SetReadyCondition(cd, corev1.ConditionFalse,
			"ReconciledVPCEndpointService",
			"reconciled the VPC Endpoint Service for the cluster",
			*a.client, logger)
		if err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
			return reconcile.Result{}, err
		}
	}

	// Create the VPC endpoint with the chosen VPC.
	endpointModified, vpcEndpoint, err := a.reconcileVPCEndpoint(awsClient, cd, metadata, vpcEndpointService, logger)
	if err != nil {
		logger.WithError(err).Error("failed to reconcile the VPC Endpoint")
		reason := "VPCEndpointReconcileFailed"
		if errors.Is(err, errNoSupportedAZsInInventory) {
			reason = "NoSupportedAZsInInventory"
		}
		if err := conditions.SetErrCondition(cd, reason, err, *a.client, logger); err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, errors.Wrap(err, "failed to reconcile the VPC Endpoint")
	}

	if endpointModified {
		if err := conditions.SetReadyCondition(cd, corev1.ConditionFalse,
			"ReconciledVPCEndpoint",
			"reconciled the VPC Endpoint for the cluster",
			*a.client, logger); err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
			return reconcile.Result{}, err
		}
	}

	endpointIps, err := a.getEndpointIPs(awsClient, vpcEndpoint)
	if err != nil {
		logger.WithError(err).Error("failed to get VPC Endpoint IP addresses")
		reason := "VPCEndpointReconcileFailed"
		if err := conditions.SetErrCondition(cd, reason, err, *a.client, logger); err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, errors.Wrap(err, "failed to get VPC Endpoint IP addresses")
	}

	dnsRecord.IpAddresses = endpointIps

	return reconcile.Result{}, nil
}

// ShouldSync returns true if there are changes that need to be made.
func (a *AWSLinkActuator) ShouldSync(cd *hivev1.ClusterDeployment) bool {
	if cd.Spec.Platform.AWS.PrivateLink == nil {
		return false
	}
	statusAdditionalAllowedPrincipals := sets.NewString()
	if cd.Status.Platform != nil &&
		cd.Status.Platform.AWS != nil &&
		cd.Status.Platform.AWS.PrivateLink != nil &&
		cd.Status.Platform.AWS.PrivateLink.VPCEndpointService.AdditionalAllowedPrincipals != nil {
		statusAdditionalAllowedPrincipals = sets.NewString(*cd.Status.Platform.AWS.PrivateLink.VPCEndpointService.AdditionalAllowedPrincipals...)
	}
	specAdditionalAllowedPrincipals := sets.NewString()
	if cd.Spec.Platform.AWS.PrivateLink.AdditionalAllowedPrincipals != nil {
		specAdditionalAllowedPrincipals = sets.NewString(*cd.Spec.Platform.AWS.PrivateLink.AdditionalAllowedPrincipals...)
	}
	return !specAdditionalAllowedPrincipals.Equal(statusAdditionalAllowedPrincipals)
}

// Validate validates a cluster deployment.
func (a *AWSLinkActuator) Validate(cd *hivev1.ClusterDeployment, logger log.FieldLogger) error {
	supportedRegion := false
	for _, item := range a.config.EndpointVPCInventory {
		if strings.EqualFold(item.Region, cd.Spec.Platform.AWS.Region) {
			supportedRegion = true
			break
		}
	}
	if !supportedRegion {
		err := errors.Errorf("cluster deployment region %q is not supported as there is no inventory to create necessary resources",
			cd.Spec.Platform.AWS.Region)
		logger.WithError(err).Error("cluster deployment region is not supported, so skipping")

		if err := conditions.SetErrCondition(cd, "UnsupportedRegion", err, *a.client, logger); err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
			return err
		}
		return nil
	}
	return nil
}

// reconcileVPCEndpointService ensure that a VPC endpoint service is created for cluster using nlbARN.
// It continously makes sure that only the HUB user/role is allowed to create endpoints to the service, and
// also makes sure that acceptance is not required when a VPC endpoint is created for the service.
// The function also continously makes sure that the NLB used by the service is always the one computed
// by the controller for the cluster.
func (a *AWSLinkActuator) reconcileVPCEndpointService(
	awsClient *awsClient,
	cd *hivev1.ClusterDeployment,
	metadata *hivev1.ClusterMetadata,
	nlbARN string,
	logger log.FieldLogger) (bool, *ec2.ServiceConfiguration, error) {

	logger.Debug("reconciling VPCEndpointService")
	modified := false

	serviceModified, serviceConfig, err := a.ensureVPCEndpointService(awsClient.user, cd, metadata, nlbARN, logger)
	if err != nil {
		logger.WithError(err).Error("error making sure VPC Endpoint Service exists for the cluster")
		return modified, nil, err
	}
	modified = serviceModified

	serviceLog := logger.WithField("serviceID", *serviceConfig.ServiceId)

	oldNLBs := sets.NewString(aws.StringValueSlice(serviceConfig.NetworkLoadBalancerArns)...)
	desiredNLBs := sets.NewString(nlbARN)
	if aws.BoolValue(serviceConfig.AcceptanceRequired) ||
		!desiredNLBs.Equal(oldNLBs) {
		modified = true
		modification := &ec2.ModifyVpcEndpointServiceConfigurationInput{
			AcceptanceRequired: aws.Bool(false),
			ServiceId:          serviceConfig.ServiceId,
		}

		if added := desiredNLBs.Difference(oldNLBs).List(); len(added) > 0 {
			modification.AddNetworkLoadBalancerArns = aws.StringSlice(added)
		}
		if removed := oldNLBs.Difference(desiredNLBs).List(); len(removed) > 0 {
			modification.RemoveNetworkLoadBalancerArns = aws.StringSlice(removed)
		}

		_, err := awsClient.user.ModifyVpcEndpointServiceConfiguration(modification)
		if err != nil {
			serviceLog.WithError(err).Error("error updating VPC Endpoint Service configuration to match the desired state")
			return modified, nil, err
		}
	}

	stsResp, err := awsClient.hub.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		serviceLog.WithError(err).Error("error getting the identity of the user that will create the VPC Endpoint")
		return modified, nil, err
	}

	permResp, err := awsClient.user.DescribeVpcEndpointServicePermissions(&ec2.DescribeVpcEndpointServicePermissionsInput{
		ServiceId: serviceConfig.ServiceId,
	})
	if err != nil {
		serviceLog.WithError(err).Error("error getting VPC Endpoint Service permissions")
		return modified, nil, err
	}

	oldPerms := sets.NewString()
	for _, allowed := range permResp.AllowedPrincipals {
		oldPerms.Insert(aws.StringValue(allowed.Principal))
	}
	// desiredPerms is the set of Allowed Principals that will be configured for the cluster's VPC Endpoint Service.
	// desiredPerms only contains the IAM entity used by Hive (defaultARN) by default but may contain additional Allowed Principal
	// ARNs as configured within cd.Spec.Platform.AWS.PrivateLink.AdditionalAllowedPrincipals.
	defaultARN := aws.StringValue(stsResp.Arn)
	desiredPerms := sets.NewString(defaultARN)
	if allowedPrincipals := cd.Spec.Platform.AWS.PrivateLink.AdditionalAllowedPrincipals; allowedPrincipals != nil {
		desiredPerms.Insert(*allowedPrincipals...)
	}

	if !desiredPerms.Equal(oldPerms) {
		modified = true
		input := &ec2.ModifyVpcEndpointServicePermissionsInput{
			ServiceId: serviceConfig.ServiceId,
		}
		if added := desiredPerms.Difference(oldPerms); len(added) > 0 {
			input.AddAllowedPrincipals = aws.StringSlice(added.List())
		}
		if removed := oldPerms.Difference(desiredPerms); len(removed) > 0 {
			input.RemoveAllowedPrincipals = aws.StringSlice(removed.List())
		}
		serviceLog.WithField("addAllowed", aws.StringValueSlice(input.AddAllowedPrincipals)).
			WithField("removeAllowed", aws.StringValueSlice(input.RemoveAllowedPrincipals)).
			Infof("updating VPC Endpoint Service permission to match the desired state")
		_, err := awsClient.user.ModifyVpcEndpointServicePermissions(input)
		if err != nil {
			serviceLog.WithField("addAllowed", aws.StringValueSlice(input.AddAllowedPrincipals)).
				WithField("removeAllowed", aws.StringValueSlice(input.RemoveAllowedPrincipals)).
				WithError(err).Error("error updating VPC Endpoint Service permission to match the desired state")
			return modified, nil, err
		}
	}

	// Update status with modified additionalAllowedPrincipals
	// Permissions were modified on the vpc endpoint service or were not modified (because they were
	// correct) and status is empty so status must be updated.
	// Checking for empty status avoids a rare hotloop that could occur when the vpc endpoint is deleted externally
	// and recreated by hive which wipes out the status in ensureVPCEndpointService(). If the allowed principals on
	// the vpc endpoint service were correct, no modification would occur but status would remain empty resulting in
	// shouldSync() always returning true.
	specHasAdditionalAllowedPrincipals := cd.Spec.Platform.AWS.PrivateLink.AdditionalAllowedPrincipals != nil
	additionalAllowedPrincipalsStatusEmpty := cd.Status.Platform.AWS.PrivateLink.VPCEndpointService.AdditionalAllowedPrincipals == nil
	defaultAllowedPrincipalStatusEmpty := cd.Status.Platform.AWS.PrivateLink.VPCEndpointService.DefaultAllowedPrincipal == nil
	if modified || (specHasAdditionalAllowedPrincipals && additionalAllowedPrincipalsStatusEmpty) || defaultAllowedPrincipalStatusEmpty {
		initPrivateLinkStatus(cd)
		// Remove the defaultARN from the list of AdditionalAllowedPrincipals to be recorded in
		// cd.Status.Platform.AWS.PrivateLink.VPCEndpointService.AdditionalAllowedPrincipals.
		// The defaultARN will be stored in a separate status field
		// cd.Status.Platform.AWS.PrivateLink.VPCEndpointService.DefaultAllowedPrincipal
		desiredPerms.Delete(defaultARN)
		desiredPermsSlice := desiredPerms.List() // sorted by sets.List()
		if len(desiredPermsSlice) == 0 {
			cd.Status.Platform.AWS.PrivateLink.VPCEndpointService.AdditionalAllowedPrincipals = nil
		} else {
			cd.Status.Platform.AWS.PrivateLink.VPCEndpointService.AdditionalAllowedPrincipals = &desiredPermsSlice
		}
		cd.Status.Platform.AWS.PrivateLink.VPCEndpointService.DefaultAllowedPrincipal = &defaultARN
		if err := updatePrivateLinkStatus(a.client, cd, logger); err != nil {
			logger.WithError(err).Error("error updating clusterdeployment status with vpcEndpointService additionalAllowedPrincipals")
			return modified, nil, err
		}
	}

	return modified, serviceConfig, nil
}

func (a *AWSLinkActuator) ensureVPCEndpointService(
	awsClient awsclient.Client,
	cd *hivev1.ClusterDeployment,
	metadata *hivev1.ClusterMetadata,
	clusterNLB string,
	logger log.FieldLogger) (bool, *ec2.ServiceConfiguration, error) {

	modified := false
	tag := ec2FilterForCluster(metadata)
	serviceLog := logger.WithField("tag:key", aws.StringValue(tag.Name)).WithField("tag:value", aws.StringValueSlice(tag.Values))

	var serviceConfig *ec2.ServiceConfiguration
	resp, err := awsClient.DescribeVpcEndpointServiceConfigurations(&ec2.DescribeVpcEndpointServiceConfigurationsInput{
		Filters: []*ec2.Filter{tag},
	})
	if err != nil {
		serviceLog.WithError(err).Error("failed to get VPC Endpoint Service for cluster")
		return modified, nil, err
	}
	if len(resp.ServiceConfigurations) == 0 {
		modified = true
		serviceConfig, err = createVPCEndpointService(awsClient, cd, metadata, clusterNLB, logger)
		if err != nil {
			logger.WithError(err).Error("failed to create VPC Endpoint Service for cluster")
			return modified, nil, errors.Wrap(err, "failed to create VPC Endpoint Service for cluster")
		}
	} else {
		serviceConfig = resp.ServiceConfigurations[0]
	}

	initPrivateLinkStatus(cd)
	cd.Status.Platform.AWS.PrivateLink.VPCEndpointService = hivev1aws.VPCEndpointService{
		ID:   *serviceConfig.ServiceId,
		Name: *serviceConfig.ServiceName,
	}
	if err := updatePrivateLinkStatus(a.client, cd, logger); err != nil {
		logger.WithError(err).Error("error updating clusterdeployment status with vpcEndpointService")
		return modified, nil, err
	}

	return modified, serviceConfig, nil
}

func createVPCEndpointService(
	awsClient awsclient.Client,
	cd *hivev1.ClusterDeployment,
	metadata *hivev1.ClusterMetadata,
	clusterNLB string,
	logger log.FieldLogger) (*ec2.ServiceConfiguration, error) {

	resp, err := awsClient.CreateVpcEndpointServiceConfiguration(&ec2.CreateVpcEndpointServiceConfigurationInput{
		AcceptanceRequired:      aws.Bool(false),
		NetworkLoadBalancerArns: aws.StringSlice([]string{clusterNLB}),
		TagSpecifications:       []*ec2.TagSpecification{ec2TagSpecification(metadata, "vpc-endpoint-service")},
	})
	if err != nil {
		logger.WithError(err).Error("failed to create endpoint service for cluster")
		return nil, err
	}

	serviceLog := logger.WithField("serviceID", *resp.ServiceConfiguration.ServiceId)

	if err := waitForState(ec2.ServiceStateAvailable, 1*time.Minute, func() (string, error) {
		resp, err := awsClient.DescribeVpcEndpointServiceConfigurations(&ec2.DescribeVpcEndpointServiceConfigurationsInput{
			ServiceIds: aws.StringSlice([]string{*resp.ServiceConfiguration.ServiceId}),
		})
		if err != nil {
			return "", errors.Wrap(err, "failed to find the VPC endpoint service")
		}
		return *resp.ServiceConfigurations[0].ServiceState, nil
	}, serviceLog); err != nil {
		serviceLog.WithError(err).Error("VPC Endpoint Service did not become Available in time.")
		return nil, err
	}

	return resp.ServiceConfiguration, nil
}

func (a *AWSLinkActuator) cleanupVPCEndpointService(
	awsClient awsclient.Client,
	cd *hivev1.ClusterDeployment,
	metadata *hivev1.ClusterMetadata,
	logger log.FieldLogger) error {

	idLog := logger.WithField("infraID", metadata.InfraID)
	resp, err := awsClient.DescribeVpcEndpointServiceConfigurations(&ec2.DescribeVpcEndpointServiceConfigurationsInput{
		Filters: []*ec2.Filter{ec2FilterForCluster(metadata)},
	})
	if err != nil {
		idLog.WithError(err).Error("error getting the VPC Endpoint Service")
		return err
	}
	if len(resp.ServiceConfigurations) == 0 {
		return nil // no work
	}

	service := resp.ServiceConfigurations[0]
	serviceLog := logger.WithField("vpcEndpointServiceID", *service.ServiceId)

	_, err = awsClient.DeleteVpcEndpointServiceConfigurations(&ec2.DeleteVpcEndpointServiceConfigurationsInput{
		ServiceIds: aws.StringSlice([]string{*service.ServiceId}),
	})
	if err != nil && !awsErrCodeEquals(err, "InvalidVpcEndpointService.NotFound") {
		serviceLog.WithError(err).Error("error deleting the VPC Endpoint Service")
		return err
	}

	return nil
}

// discoverNLBForCluster uses the AWS client to find the NLB for cluster's internal APIserver.
// The NLB created by the installer is named as {infraID}-int
func discoverNLBForCluster(client awsclient.Client, infraID string, logger log.FieldLogger) (string, error) {
	logger.Debug("discovering NLB for cluster")

	nlbName := infraID + "-int"
	nlbs, err := client.DescribeLoadBalancers(&elbv2.DescribeLoadBalancersInput{
		Names: aws.StringSlice([]string{nlbName}),
	})
	if err != nil {
		return "", errors.Wrap(err, "failed to describe load balancer for the cluster")
	}
	nlbARN := aws.StringValue(nlbs.LoadBalancers[0].LoadBalancerArn)
	nlbLog := logger.WithField("nlbARN", nlbARN)

	if err := waitForState(elbv2.LoadBalancerStateEnumActive, 1*time.Minute, func() (string, error) {
		resp, err := client.DescribeLoadBalancers(&elbv2.DescribeLoadBalancersInput{
			LoadBalancerArns: aws.StringSlice([]string{nlbARN}),
		})
		if err != nil {
			return "", errors.Wrap(err, "failed to find the NLB")
		}
		return *resp.LoadBalancers[0].State.Code, nil
	}, nlbLog); err != nil {
		nlbLog.WithError(err).Error("NLB did not become Available in time.")
		return "", err
	}
	return nlbARN, nil
}

// reconcileVPCEndpoint ensures that a VPC endpoint is created for the VPC endpoint service in the
// HUB account.
// It chooses a VPC from the list of VPCs given to the controller using criteria like
//   - VPC that is in the same region as the VPC endpoint service
//   - VPC that has at least one subnet in the AZs supported by the VPC endpoint service
//   - VPC that has the fewest existing VPC endpoints ("spread" strategy)
//
// It currently doesn't manage any properties of the VPC endpoint once it is created.
func (a *AWSLinkActuator) reconcileVPCEndpoint(
	awsClient *awsClient,
	cd *hivev1.ClusterDeployment,
	metadata *hivev1.ClusterMetadata,
	vpcEndpointService *ec2.ServiceConfiguration,
	logger log.FieldLogger) (bool, *ec2.VpcEndpoint, error) {

	logger.Debug("reconciling VPCEndpoint")
	modified := false
	tag := ec2FilterForCluster(metadata)
	endpointLog := logger.WithField("tag:key", aws.StringValue(tag.Name)).WithField("tag:value", aws.StringValueSlice(tag.Values))

	var vpcEndpoint *ec2.VpcEndpoint
	resp, err := awsClient.hub.DescribeVpcEndpoints(&ec2.DescribeVpcEndpointsInput{
		Filters: []*ec2.Filter{tag},
	})
	if err != nil {
		endpointLog.WithError(err).Error("error getting VPC Endpoint")
		return modified, nil, err
	}
	if len(resp.VpcEndpoints) == 0 {
		chosenVPC, err := chooseVPCForVPCEndpoint(awsClient.hub, cd, *a.config, *vpcEndpointService.ServiceName, logger)
		if err != nil {
			logger.WithError(err).Error("failed to choose VPC for the VPC Endpoint from the inventory")
			return modified, nil, err
		}
		modified = true
		vpcEndpoint, err = createVPCEndpoint(awsClient.hub, metadata, chosenVPC, vpcEndpointService, logger)
		if err != nil {
			logger.WithError(err).Error("error creating VPC Endpoint for service")
			return modified, nil, err
		}
	} else {
		vpcEndpoint = resp.VpcEndpoints[0]
	}

	initPrivateLinkStatus(cd)
	cd.Status.Platform.AWS.PrivateLink.VPCEndpointID = *vpcEndpoint.VpcEndpointId
	if err := updatePrivateLinkStatus(a.client, cd, logger); err != nil {
		logger.WithError(err).Error("error updating clusterdeployment status with vpcEndpointID")
		return modified, nil, err
	}

	return modified, vpcEndpoint, nil
}

func createVPCEndpoint(
	awsClient awsclient.Client,
	metadata *hivev1.ClusterMetadata,
	chosenVPC *hivev1.AWSPrivateLinkInventory,
	vpcEndpointService *ec2.ServiceConfiguration,
	logger log.FieldLogger) (*ec2.VpcEndpoint, error) {

	subnetIDs := make([]string, 0, len(chosenVPC.Subnets))
	for _, subnet := range chosenVPC.Subnets {
		subnetIDs = append(subnetIDs, subnet.SubnetID)
	}
	resp, err := awsClient.CreateVpcEndpoint(&ec2.CreateVpcEndpointInput{
		PrivateDnsEnabled: aws.Bool(false),
		ServiceName:       vpcEndpointService.ServiceName,
		SubnetIds:         aws.StringSlice(subnetIDs),
		TagSpecifications: []*ec2.TagSpecification{ec2TagSpecification(metadata, "vpc-endpoint")},
		VpcEndpointType:   aws.String(ec2.VpcEndpointTypeInterface),
		VpcId:             aws.String(chosenVPC.VPCID),
	})
	if err != nil {
		logger.WithError(err).Error("error creating VPC Endpoint")
		return nil, err
	}
	endpointLog := logger.WithField("endpointID", *resp.VpcEndpoint.VpcEndpointId)

	if err := waitForState("available", 1*time.Minute, func() (string, error) {
		resp, err := awsClient.DescribeVpcEndpoints(&ec2.DescribeVpcEndpointsInput{
			VpcEndpointIds: aws.StringSlice([]string{*resp.VpcEndpoint.VpcEndpointId}),
		})
		if err != nil {
			return "", errors.Wrap(err, "failed to get VPC endpoint")
		}
		return *resp.VpcEndpoints[0].State, nil
	}, endpointLog); err != nil {
		endpointLog.WithError(err).Error("VPC Endpoint did not become Available in time")
		return nil, err
	}

	return resp.VpcEndpoint, nil
}

func (a *AWSLinkActuator) getEndpointIPs(awsClient *awsClient, vpcEndpoint *ec2.VpcEndpoint) ([]string, error) {
	// get the ips from the elastic networking interfaces attached to the VPC endpoint
	enis := vpcEndpoint.NetworkInterfaceIds
	if len(enis) == 0 {
		return nil, errors.New("No network interfaces attached to the vpc endpoint")
	}
	res, err := awsClient.hub.DescribeNetworkInterfaces(&ec2.DescribeNetworkInterfacesInput{NetworkInterfaceIds: vpcEndpoint.NetworkInterfaceIds})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list network interfaces attached to the vpc endpoint")
	}
	if len(res.NetworkInterfaces) == 0 {
		return nil, errors.New("No network interfaces attached to the vpc endpoint")
	}

	var ips []string
	for _, eni := range res.NetworkInterfaces {
		ips = append(ips, aws.StringValue(eni.PrivateIpAddress))
	}
	sort.Strings(ips)

	return ips, nil
}

func (a *AWSLinkActuator) cleanupVPCEndpoint(awsClient awsclient.Client,
	cd *hivev1.ClusterDeployment,
	metadata *hivev1.ClusterMetadata,
	logger log.FieldLogger) error {

	idLog := logger.WithField("infraID", metadata.InfraID)
	resp, err := awsClient.DescribeVpcEndpoints(&ec2.DescribeVpcEndpointsInput{
		Filters: []*ec2.Filter{ec2FilterForCluster(metadata)},
	})
	if err != nil {
		idLog.WithError(err).Error("error getting the VPC Endpoint")
		return err
	}
	if len(resp.VpcEndpoints) == 0 {
		return nil // no work
	}

	vpcEndpoint := resp.VpcEndpoints[0]
	endpointLog := logger.WithField("vpcEndpointID", *vpcEndpoint.VpcEndpointId)

	_, err = awsClient.DeleteVpcEndpoints(&ec2.DeleteVpcEndpointsInput{
		VpcEndpointIds: aws.StringSlice([]string{*vpcEndpoint.VpcEndpointId}),
	})
	if err != nil && !awsErrCodeEquals(err, "InvalidVpcEndpointId.NotFound") {
		endpointLog.WithError(err).Error("error deleting the VPC Endpoint")
		return err
	}

	return nil
}
