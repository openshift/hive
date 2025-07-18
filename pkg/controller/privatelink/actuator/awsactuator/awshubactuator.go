package awsactuator

import (
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/smithy-go"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/awsclient"
	"github.com/openshift/hive/pkg/controller/privatelink/actuator"
	"github.com/openshift/hive/pkg/controller/privatelink/conditions"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

var (
	errNoHostedZoneFoundForVPC = errors.New("no hosted zone found")
)

// Ensure AWSHubActuator implements the Actuator interface. This will fail at compile time when false.
var _ actuator.Actuator = &AWSHubActuator{}

type AWSHubActuator struct {
	client *client.Client

	config *hivev1.AWSPrivateLinkConfig

	// testing purpose
	awsClientFn awsClientFn

	awsClientHub awsclient.Client
}

// New creates a new AWS Hub Actuator
func NewAWSHubActuator(
	client *client.Client,
	config *hivev1.AWSPrivateLinkConfig,
	awsClientFn awsClientFn,
	logger log.FieldLogger) (*AWSHubActuator, error) {

	actuator := &AWSHubActuator{
		client:      client,
		config:      config,
		awsClientFn: awsClientFn,
	}

	// Fall back to older AWSPrivateLinkController config for backwards compatibility
	// If the new config is defined, it will be used. Otherwise we will
	// pull from the old config here overriding everything.
	if config == nil {
		oldConfig, err := ReadAWSPrivateLinkControllerConfigFile()
		if err != nil {
			return nil, err
		}
		if oldConfig != nil {
			logger.Debug("falling back to AWSPrivateLinkController config")
			actuator.config = oldConfig
		} else {
			return nil, errors.New("unable to create AWS actuator: config is empty")
		}
	}

	hubClient, err := newAWSClient(*client, awsClientFn, "", controllerutils.GetHiveNamespace(), &actuator.config.CredentialsSecretRef, nil)
	if err != nil {
		return nil, err
	}
	actuator.awsClientHub = hubClient

	return actuator, nil
}

// Cleanup is the actuator interface for cleaning up the cloud resources.
func (a *AWSHubActuator) Cleanup(cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata, logger log.FieldLogger) error {
	if err := a.cleanupHostedZone(cd, metadata, logger); err != nil {
		return errors.Wrap(err, "error cleaning up Hosted Zone")
	}

	return nil
}

// CleanupRequired is the actuator interface for determining if cleanup is required.
func (a *AWSHubActuator) CleanupRequired(cd *hivev1.ClusterDeployment) bool {
	// There is nothing to do when PrivateLink is undefined. This either means it was never enabled, or it was already cleaned up.
	if cd.Status.Platform == nil ||
		cd.Status.Platform.AWS == nil ||
		cd.Status.Platform.AWS.PrivateLink == nil {
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

	return cd.Status.Platform.AWS.PrivateLink.HostedZoneID != ""
}

// Reconcile is the actuator interface for reconciling the cloud resources.
func (a *AWSHubActuator) Reconcile(cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata, dnsRecord *actuator.DnsRecord, logger log.FieldLogger) (reconcile.Result, error) {
	logger.Debug("reconciling hub resources")

	// Figure out the API address for cluster.0
	apiDomain, err := initialURL(*a.client,
		client.ObjectKey{Namespace: cd.Namespace, Name: metadata.AdminKubeconfigSecretRef.Name})
	if err != nil {
		if err := conditions.SetErrConditionWithRetry(*a.client, cd, "CouldNotCalculateAPIDomain", err, logger); err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
		}
		return reconcile.Result{}, errors.Wrap(err, "could not get API URL from kubeconfig")
	}

	logger.Debug("reconciling Hosted Zone")
	hzModified, hostedZoneID, err := a.ensureHostedZone(cd, metadata, apiDomain, logger)
	if err != nil {
		if err := conditions.SetErrConditionWithRetry(*a.client, cd, "PrivateHostedZoneReconcileFailed", err, logger); err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
		}
		return reconcile.Result{}, errors.Wrap(err, "failed to reconcile the Hosted Zone")
	}
	if hzModified {
		err := conditions.SetReadyConditionWithRetry(*a.client, cd, corev1.ConditionFalse,
			"ReconciledPrivateHostedZone",
			"reconciled the Private Hosted Zone for the VPC Endpoint of the cluster",
			logger)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to update condition on cluster deployment")
		}
	}

	logger.Debug("reconciling Hosted Zone Records")
	recordsModified, err := a.ReconcileHostedZoneRecords(cd, hostedZoneID, dnsRecord, apiDomain, logger)
	if err != nil {
		if err := conditions.SetErrConditionWithRetry(*a.client, cd, "PrivateHostedZoneRecordsReconcileFailed", err, logger); err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
		}
		return reconcile.Result{}, errors.Wrap(err, "failed to reconcile the Hosted Zone Records")
	}
	if recordsModified {
		err := conditions.SetReadyConditionWithRetry(*a.client, cd, corev1.ConditionFalse,
			"ReconciledPrivateHostedZoneRecords",
			"reconciled the Private Hosted Zone Records for the VPC Endpoint of the cluster",
			logger)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to update condition on cluster deployment")
		}
	}

	logger.Debug("reconciling Hosted Zone Associations")
	associationsModified, err := a.reconcileHostedZoneAssociations(cd, metadata, hostedZoneID, logger)
	if err != nil {
		if err := conditions.SetErrConditionWithRetry(*a.client, cd, "AssociatingVPCsToHostedZoneFailed", err, logger); err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
		}
		return reconcile.Result{}, errors.Wrap(err, "failed to reconcile the Hosted Zone Associations")
	}

	if associationsModified {
		err := conditions.SetReadyConditionWithRetry(*a.client, cd, corev1.ConditionFalse,
			"ReconciledAssociationsToVPCs",
			"reconciled the associations of all the required VPCs to the Private Hosted Zone for the VPC Endpoint",
			logger)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to update condition on cluster deployment")
		}
	}

	return reconcile.Result{}, nil
}

// ShouldSync is the actuator interface to determine if there are changes that need to be made.
func (a *AWSHubActuator) ShouldSync(cd *hivev1.ClusterDeployment) bool {
	return cd.Status.Platform == nil ||
		cd.Status.Platform.AWS == nil ||
		cd.Status.Platform.AWS.PrivateLink == nil ||
		cd.Status.Platform.AWS.PrivateLink.HostedZoneID == ""
}

func (a *AWSHubActuator) ensureHostedZone(cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata, apiDomain string, logger log.FieldLogger) (bool, string, error) {
	modified := false

	associatedVPCs, err := a.getAssociatedVPCs(cd, metadata, logger)
	if err != nil {
		return false, "", errors.Wrap(err, "could not get associated VPCs")
	}

	if len(associatedVPCs) == 0 {
		return false, "", errors.New("at least one associated VPC must be configured")
	}

	hzID, err := a.findHostedZone(associatedVPCs, apiDomain)
	if err != nil && errors.Is(err, errNoHostedZoneFoundForVPC) {
		selectedVPC, err := a.selectHostedZoneVPC(cd, metadata, logger)
		if err != nil {
			return false, "", err
		}

		newHzID, err := a.createHostedZone(&selectedVPC, apiDomain)
		if err != nil {
			return false, "", err
		}
		modified = true
		hzID = newHzID
	} else if err != nil {
		return false, "", errors.Wrap(err, "failed to get Hosted Zone")
	}

	initPrivateLinkStatus(cd)
	if cd.Status.Platform.AWS.PrivateLink.HostedZoneID != hzID {
		cd.Status.Platform.AWS.PrivateLink.HostedZoneID = hzID
		if err := updatePrivateLinkStatus(a.client, cd); err != nil {
			return false, "", errors.Wrap(err, "failed to update the hosted zone ID for cluster deployment")
		}
		modified = true
	}

	return modified, hzID, nil
}

func (a *AWSHubActuator) createHostedZone(associatedVPC *hivev1.AWSAssociatedVPC, apiDomain string) (string, error) {
	caller := aws.String("hive")
	vpc := &route53types.VPC{
		VPCId:     aws.String(associatedVPC.VPCID),
		VPCRegion: route53types.VPCRegion(associatedVPC.Region),
	}

	resp, err := a.awsClientHub.CreateHostedZone(&route53.CreateHostedZoneInput{
		CallerReference: caller,
		Name:            aws.String(apiDomain),
		VPC:             vpc,
		HostedZoneConfig: &route53types.HostedZoneConfig{
			Comment:     aws.String("Created by Hive"),
			PrivateZone: true,
		},
	})
	if err != nil {
		return "", err
	}

	return aws.ToString(resp.HostedZone.Id), nil
}

// findHostedZone finds a Private Hosted Zone for apiDomain that is associated with the given VPCs.
// If no such hosted zone exists, it return an errNoHostedZoneFoundForVPC error.
func (a *AWSHubActuator) findHostedZone(associatedVPCs []hivev1.AWSAssociatedVPC, apiDomain string) (string, error) {
	for _, associatedVPC := range associatedVPCs {
		hostedZones, err := a.awsClientHub.ListHostedZonesByVPC(&route53.ListHostedZonesByVPCInput{
			VPCId:     aws.String(associatedVPC.VPCID),
			VPCRegion: route53types.VPCRegion(associatedVPC.Region),
		})
		if err != nil {
			return "", err
		}

		for _, hostedZone := range hostedZones.HostedZoneSummaries {
			// Match the api domain to the hosted zone name.
			if aws.ToString(hostedZone.Name) == strings.TrimSuffix(apiDomain, ".")+"." ||
				aws.ToString(hostedZone.Name) == apiDomain+"." {
				return aws.ToString(hostedZone.HostedZoneId), nil
			}
		}
	}

	return "", errNoHostedZoneFoundForVPC
}

func (a *AWSHubActuator) cleanupHostedZone(cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata, logger log.FieldLogger) error {
	logger.Debug("cleaning up Hosted Zone")

	var hzID string
	if cd.Status.Platform != nil &&
		cd.Status.Platform.AWS != nil &&
		cd.Status.Platform.AWS.PrivateLink != nil &&
		cd.Status.Platform.AWS.PrivateLink.HostedZoneID != "" {
		hzID = cd.Status.Platform.AWS.PrivateLink.HostedZoneID
	}

	if hzID == "" { // since we don't have the hz ID, we try to discover it to prevent leaks
		apiDomain, err := initialURL(*a.client,
			client.ObjectKey{Namespace: cd.Namespace, Name: metadata.AdminKubeconfigSecretRef.Name})
		if apierrors.IsNotFound(err) {
			logger.Info("no hostedZoneID in status and admin kubeconfig does not exist, skipping hosted zone cleanup")
			return nil
		} else if err != nil {
			return errors.Wrap(err, "could not get API URL from kubeconfig")
		}

		associatedVPCs, err := a.getAssociatedVPCs(cd, metadata, logger)
		if err != nil {
			return errors.Wrap(err, "could not get associated VPCs")
		}

		hzID, err = a.findHostedZone(associatedVPCs, apiDomain)
		if err != nil && errors.Is(err, errNoHostedZoneFoundForVPC) {
			return nil // no work
		}
		if err != nil {
			return errors.Wrap(err, "error getting the Hosted Zone")
		}
	}

	recordsResp, err := a.awsClientHub.ListResourceRecordSets(&route53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String(hzID),
	})
	if err != nil {
		// Handle case where hosted zone no longer exists (cleanup is idempotent)
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "NoSuchHostedZone" {
			logger.Debug("hosted zone no longer exists, cleanup complete")
			return nil
		}
		return errors.Wrapf(err, "failed to list the hosted zone %s", hzID)
	}
	for _, record := range recordsResp.ResourceRecordSets {
		if record.Type == route53types.RRTypeSoa || record.Type == route53types.RRTypeNs {
			// can't delete SOA and NS types
			continue
		}
		_, err := a.awsClientHub.ChangeResourceRecordSets(&route53.ChangeResourceRecordSetsInput{
			HostedZoneId: aws.String(hzID),
			ChangeBatch: &route53types.ChangeBatch{
				Changes: []route53types.Change{{
					Action:            route53types.ChangeActionDelete,
					ResourceRecordSet: &record,
				}},
			},
		})
		if err != nil {
			return errors.Wrapf(err, "failed to delete the record from the hosted zone %s", *record.Name)
		}
	}

	_, err = a.awsClientHub.DeleteHostedZone(&route53.DeleteHostedZoneInput{
		Id: aws.String(hzID),
	})
	if err != nil {
		return errors.Wrapf(err, "error deleting the hosted zone %s", hzID)
	}

	initPrivateLinkStatus(cd)
	if cd.Status.Platform.AWS.PrivateLink.HostedZoneID != "" {
		cd.Status.Platform.AWS.PrivateLink.HostedZoneID = ""
		if err := updatePrivateLinkStatus(a.client, cd); err != nil {
			return errors.Wrap(err, "error updating clusterdeployment after cleanup of private link hosted zone")
		}
	}

	return nil
}

func (a *AWSHubActuator) ReconcileHostedZoneRecords(cd *hivev1.ClusterDeployment, hostedZoneID string, dnsRecord *actuator.DnsRecord, apiDomain string, logger log.FieldLogger) (bool, error) {
	rSet, err := a.recordSet(cd, apiDomain, dnsRecord)
	if err != nil {
		return false, errors.Wrap(err, "error generating DNS records")
	}

	_, err = a.awsClientHub.ChangeResourceRecordSets(&route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(hostedZoneID),
		ChangeBatch: &route53types.ChangeBatch{
			Changes: []route53types.Change{{
				Action:            route53types.ChangeActionUpsert,
				ResourceRecordSet: rSet,
			}},
		},
	})
	if err != nil {
		return false, errors.Wrapf(err, "error adding record to Hosted Zone %s for VPC Endpoint", hostedZoneID)
	}
	return true, nil
}

func (a *AWSHubActuator) recordSet(cd *hivev1.ClusterDeployment, apiDomain string, dnsRecord *actuator.DnsRecord) (*route53types.ResourceRecordSet, error) {
	if dnsRecord == nil {
		return nil, errors.New("dns record is nil")
	}

	// Create resource records from all IP addresses
	var resourceRecords []route53types.ResourceRecord
	for _, ip := range dnsRecord.IpAddress {
		resourceRecords = append(resourceRecords, route53types.ResourceRecord{
			Value: aws.String(ip),
		})
	}

	recordSet := &route53types.ResourceRecordSet{
		Name:            aws.String(apiDomain),
		Type:            route53types.RRTypeA,
		ResourceRecords: resourceRecords,
		TTL:             aws.Int64(60),
	}

	return recordSet, nil
}

// reconcileHostedZoneAssociations ensures that the all the VPCs in the associatedVPCs list from
// the controller config are associated to the PHZ hostedZoneID.
func (a *AWSHubActuator) reconcileHostedZoneAssociations(cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata, hostedZoneID string, logger log.FieldLogger) (bool, error) {
	hzLog := logger.WithField("hostedZoneID", hostedZoneID)
	modified := false

	vpcInfo := a.config.DeepCopy().AssociatedVPCs
	vpcIdx := map[string]int{}
	for i, v := range vpcInfo {
		vpcIdx[v.VPCID] = i
	}

	zoneResp, err := a.awsClientHub.GetHostedZone(&route53.GetHostedZoneInput{
		Id: aws.String(hostedZoneID),
	})
	if err != nil {
		return false, errors.Wrap(err, "failed to get the Hosted Zone")
	}

	oldVPCs := sets.NewString()
	for _, vpc := range zoneResp.VPCs {
		id := aws.ToString(vpc.VPCId)
		oldVPCs.Insert(id)
		if _, ok := vpcIdx[id]; !ok { // make sure we have info for all VPCs for later use
			vpcInfo = append(vpcInfo, hivev1.AWSAssociatedVPC{
							AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{
				VPCID:  id,
				Region: string(vpc.VPCRegion),
			},
			})
			vpcIdx[id] = len(vpcInfo) - 1
		}
	}

	associatedVPCs, err := a.getAssociatedVPCs(cd, metadata, logger)
	if err != nil {
		return false, errors.Wrap(err, "could not get associated VPCs")
	}

	desiredVPCs := sets.NewString()
	for _, vpc := range associatedVPCs {
		desiredVPCs.Insert(vpc.VPCID)
	}

	added := desiredVPCs.Difference(oldVPCs).List()
	removed := oldVPCs.Difference(desiredVPCs).List()
	if len(added) > 0 || len(removed) > 0 {
		modified = true
		hzLog.WithFields(log.Fields{
			"associate":    added,
			"disassociate": removed,
		}).Debug("updating the VPCs attached to the Hosted Zone")
	}

	for _, vpc := range added {
		info := vpcInfo[vpcIdx[vpc]]

		awsAssociationClient := a.awsClientHub
		if info.CredentialsSecretRef != nil {
			// since this VPC is in different account we need to authorize before continuing
			_, err := a.awsClientHub.CreateVPCAssociationAuthorization(&route53.CreateVPCAssociationAuthorizationInput{
				HostedZoneId: aws.String(hostedZoneID),
				VPC: &route53types.VPC{
					VPCId:     aws.String(vpc),
					VPCRegion: route53types.VPCRegion(info.Region),
				},
			})
			if err != nil {
				return false, errors.Wrapf(err, "failed to create authorization for association of the Hosted Zone to the VPC %s", vpc)
			}

			awsAssociationClient, err = newAWSClient(*a.client, a.awsClientFn, info.Region, controllerutils.GetHiveNamespace(), info.CredentialsSecretRef, nil)
			if err != nil {
				return false, errors.Wrapf(err, "failed to create AWS client for association of the Hosted Zone to the VPC %s", vpc)
			}
		}

		_, err = awsAssociationClient.AssociateVPCWithHostedZone(&route53.AssociateVPCWithHostedZoneInput{
			HostedZoneId: aws.String(hostedZoneID),
			VPC: &route53types.VPC{
				VPCId:     aws.String(vpc),
				VPCRegion: route53types.VPCRegion(info.Region),
			},
		})
		if err != nil {
			return false, errors.Wrapf(err, "failed to associate the Hosted Zone to the VPC %s", vpc)
		}

		if info.CredentialsSecretRef != nil {
			// since we created an authorization and association is complete, we should remove the object
			// as recommended by AWS best practices.
			_, err := a.awsClientHub.DeleteVPCAssociationAuthorization(&route53.DeleteVPCAssociationAuthorizationInput{
				HostedZoneId: aws.String(hostedZoneID),
				VPC: &route53types.VPC{
					VPCId:     aws.String(vpc),
					VPCRegion: route53types.VPCRegion(info.Region),
				},
			})
			if err != nil {
				return false, errors.Wrapf(err, "failed to delete authorization for association of the Hosted Zone to the VPC %s", vpc)
			}
		}
	}
	for _, vpc := range removed {
		info := vpcInfo[vpcIdx[vpc]]
		_, err = a.awsClientHub.DisassociateVPCFromHostedZone(&route53.DisassociateVPCFromHostedZoneInput{
			HostedZoneId: aws.String(hostedZoneID),
			VPC: &route53types.VPC{
				VPCId:     aws.String(vpc),
				VPCRegion: route53types.VPCRegion(info.Region),
			},
		})
		if err != nil {
			return false, errors.Wrapf(err, "failed to disassociate the Hosted Zone to the VPC %s", vpc)
		}
	}

	return modified, nil
}

func (a *AWSHubActuator) getAssociatedVPCs(
	cd *hivev1.ClusterDeployment,
	metadata *hivev1.ClusterMetadata,
	logger log.FieldLogger) ([]hivev1.AWSAssociatedVPC, error) {

	associatedVPCs := a.config.DeepCopy().AssociatedVPCs

	// For clusterdeployments that are on AWS, also add the VPCEndpoint VPC
	if cd.Status.Platform != nil &&
		cd.Status.Platform.AWS != nil &&
		cd.Status.Platform.AWS.PrivateLink != nil &&
		cd.Status.Platform.AWS.PrivateLink.VPCEndpointID != "" {

		endpointVPC, err := a.getEndpointVPC(cd, metadata)
		if err != nil {
			return associatedVPCs, err
		}

		if endpointVPC.VPCID != "" {
			logger.Debugf("adding VpcEndpoint VPC %s", endpointVPC.VPCID)
			associatedVPCs = append(associatedVPCs, endpointVPC)
		}
	}

	return associatedVPCs, nil
}

func (a *AWSHubActuator) getEndpointVPC(cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata) (hivev1.AWSAssociatedVPC, error) {
	if metadata == nil {
		return hivev1.AWSAssociatedVPC{}, errors.New("cluster metadata is nil")
	}
	if metadata.InfraID == "" {
		return hivev1.AWSAssociatedVPC{}, errors.New("cluster metadata InfraID is empty")
	}
	
	awsClient, err := newAWSClient(*a.client, a.awsClientFn, cd.Spec.Platform.AWS.Region, cd.Namespace, &cd.Spec.Platform.AWS.CredentialsSecretRef, nil)
	if err != nil {
		return hivev1.AWSAssociatedVPC{}, err
	}

	filter := ec2FilterForCluster(metadata)
	resp, err := awsClient.DescribeVpcEndpoints(&ec2.DescribeVpcEndpointsInput{
		Filters: []ec2types.Filter{filter},
	})
	if err != nil {
		return hivev1.AWSAssociatedVPC{}, err
	}

	if len(resp.VpcEndpoints) == 0 {
		return hivev1.AWSAssociatedVPC{}, errors.New("endpoint not found")
	}

	endpoint := resp.VpcEndpoints[0]
	return hivev1.AWSAssociatedVPC{
		AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{
			VPCID:  aws.ToString(endpoint.VpcId),
			Region: cd.Spec.Platform.AWS.Region,
		},
	}, nil
}

func (a *AWSHubActuator) selectHostedZoneVPC(cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata, logger log.FieldLogger) (hivev1.AWSAssociatedVPC, error) {
	endpointVPC, err := a.getEndpointVPC(cd, metadata)
	if err != nil {
		logger.WithError(err).Warn("could not get endpoint VPC")
		return hivev1.AWSAssociatedVPC{}, err
	}

	logger.WithField("vpc", endpointVPC.VPCID).WithField("region", endpointVPC.Region).Debug("endpoint VPC")

	return endpointVPC, nil
}
