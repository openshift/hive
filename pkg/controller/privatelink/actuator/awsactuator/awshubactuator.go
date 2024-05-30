package awsactuator

import (
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/route53"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/awsclient"
	"github.com/openshift/hive/pkg/controller/privatelink/actuator"
	"github.com/openshift/hive/pkg/controller/privatelink/conditions"
	"github.com/openshift/hive/pkg/controller/privatelink/dnsrecord"
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
}

// New creates a new AWS Hub Actuator
func NewAWSHubActuator(client *client.Client, config *hivev1.AWSPrivateLinkConfig, logger log.FieldLogger) (*AWSHubActuator, error) {
	actuator := &AWSHubActuator{
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

	actuator.awsClientFn = awsclient.New

	return actuator, nil
}

// Cleanup cleans up the cloud resources.
func (a *AWSHubActuator) Cleanup(cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata, logger log.FieldLogger) error {
	awsClient, err := newAWSClient(*a.client, a.awsClientFn, cd, a.config)
	if err != nil {
		logger.WithError(err).Error("error creating AWS client for the cluster")
		return err
	}

	if err := a.cleanupHostedZone(awsClient.hub, cd, metadata, logger); err != nil {
		logger.WithError(err).Error("error cleaning up Hosted Zone")
		return err
	}

	initPrivateLinkStatus(cd)
	cd.Status.Platform.AWS.PrivateLink.HostedZoneID = ""
	if err := updatePrivateLinkStatus(a.client, cd, logger); err != nil {
		logger.WithError(err).Error("error updating clusterdeployment after cleanup of private link hosted zone")
		return err
	}

	return nil
}

// CleanupRequired returns true if there are resources to be cleaned up.
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

// Reconcile reconciles the required resources.
func (a *AWSHubActuator) Reconcile(
	cd *hivev1.ClusterDeployment,
	metadata *hivev1.ClusterMetadata,
	dnsRecord *dnsrecord.DnsRecord,
	logger log.FieldLogger) (reconcile.Result, error) {

	logger.Debug("reconciling hub resources")
	awsClient, err := newAWSClient(*a.client, a.awsClientFn, cd, a.config)
	if err != nil {
		logger.WithError(err).Error("error creating AWS client for the cluster")
		return reconcile.Result{}, err
	}

	// Figure out the API address for cluster.0
	apiDomain, err := initialURL(*a.client,
		client.ObjectKey{Namespace: cd.Namespace, Name: metadata.AdminKubeconfigSecretRef.Name})
	if err != nil {
		logger.WithError(err).Error("could not get API URL from kubeconfig")

		if err := conditions.SetErrCondition(cd, "CouldNotCalculateAPIDomain", err, *a.client, logger); err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, err
	}

	// Create the Private Hosted Zone for the VPC Endpoint.
	hzModified, hostedZoneID, err := a.reconcileHostedZone(awsClient, cd, metadata, dnsRecord, apiDomain, logger)
	if err != nil {
		logger.WithError(err).Error("could not reconcile the Hosted Zone")

		if err := conditions.SetErrCondition(cd, "PrivateHostedZoneReconcileFailed", err, *a.client, logger); err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, err
	}

	if hzModified {
		if err := conditions.SetReadyCondition(cd, corev1.ConditionFalse,
			"ReconciledPrivateHostedZone",
			"reconciled the Private Hosted Zone for the VPC Endpoint of the cluster",
			*a.client, logger); err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
			return reconcile.Result{}, err
		}
	}

	// Associate the VPCs to the hosted zone.
	associationsModified, err := a.reconcileHostedZoneAssociations(awsClient, cd, metadata, hostedZoneID, logger)
	if err != nil {
		logger.WithError(err).Error("could not reconcile the associations of the Hosted Zone")

		if err := conditions.SetErrCondition(cd, "AssociatingVPCsToHostedZoneFailed", err, *a.client, logger); err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, err
	}

	if associationsModified {
		if err := conditions.SetReadyCondition(cd, corev1.ConditionFalse,
			"ReconciledAssociationsToVPCs",
			"reconciled the associations of all the required VPCs to the Private Hosted Zone for the VPC Endpoint",
			*a.client, logger); err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
			return reconcile.Result{}, err
		}
	}

	if err := conditions.SetReadyCondition(cd, corev1.ConditionTrue,
		"PrivateLinkAccessReady",
		"private link access is ready for use",
		*a.client, logger); err != nil {
		logger.WithError(err).Error("failed to update condition on cluster deployment")
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// ShouldSync returns true if there are changes that need to be made.
func (a *AWSHubActuator) ShouldSync(cd *hivev1.ClusterDeployment) bool {
	// TODO: finish this out based on if hub should sync.
	return false
}

// Validate validates a cluster deployment.
func (a *AWSHubActuator) Validate(cd *hivev1.ClusterDeployment, logger log.FieldLogger) error {
	return nil
}

// reconcileHostedZone ensures that a Private Hosted Zone apiDomain exists for the VPC
// where VPC endpoint was created. It also make sure the DNS zone has an ALIAS record pointing
// to the regional DNS name of the VPC endpoint.
func (a *AWSHubActuator) reconcileHostedZone(
	awsClient *awsClient,
	cd *hivev1.ClusterDeployment,
	metadata *hivev1.ClusterMetadata,
	dnsRecord *dnsrecord.DnsRecord,
	apiDomain string,
	logger log.FieldLogger) (bool, string, error) {

	logger.Debug("reconciling HostedZone")

	modified, hostedZoneID, err := a.ensureHostedZone(awsClient.hub, cd, metadata, apiDomain, logger)
	if err != nil {
		logger.WithError(err).Error("error ensuring Hosted Zone was created")
		return modified, "", err
	}

	hzLog := logger.WithField("hostedZoneID", hostedZoneID)

	rSet, err := a.recordSet(awsClient.hub, apiDomain, dnsRecord)
	if err != nil {
		hzLog.WithError(err).Error("error generating DNS records")
		return modified, "", err
	}

	_, err = awsClient.hub.ChangeResourceRecordSets(&route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(hostedZoneID),
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{{
				Action:            aws.String(route53.ChangeActionUpsert),
				ResourceRecordSet: rSet,
			}},
		},
	})
	if err != nil {
		hzLog.WithError(err).Error("error adding record to Hosted Zone for VPC Endpoint")
		return modified, "", err
	}
	return modified, hostedZoneID, nil
}

func (a *AWSHubActuator) recordSet(awsClient awsclient.Client, apiDomain string, dnsRecord *dnsrecord.DnsRecord) (*route53.ResourceRecordSet, error) {
	rSet := &route53.ResourceRecordSet{
		Name: aws.String(apiDomain),
	}
	switch a.config.DNSRecordType {
	case hivev1.ARecordAWSPrivateLinkDNSRecordType:
		rSet.Type = aws.String("A")
		rSet.TTL = aws.Int64(10)

		sort.Strings(dnsRecord.IpAddresses)
		for _, ip := range dnsRecord.IpAddresses {
			rSet.ResourceRecords = append(rSet.ResourceRecords, &route53.ResourceRecord{
				Value: aws.String(ip),
			})
		}
	default:
		// TODO: AWS should default to AliasTarget
		//rSet.Type = aws.String("A")
		//rSet.AliasTarget = &route53.AliasTarget{
		//	DNSName:              dnsRecord.Route53AliasTarget.DnsName,
		//	HostedZoneId:         dnsRecord.Route53AliasTarget.HostedZoneId,
		//	EvaluateTargetHealth: aws.Bool(false),
		//}
		return rSet, errors.New("unsupported dns configuration")
	}
	return rSet, nil
}

func (a *AWSHubActuator) ensureHostedZone(
	awsClient awsclient.Client,
	cd *hivev1.ClusterDeployment,
	metadata *hivev1.ClusterMetadata,
	apiDomain string,
	logger log.FieldLogger) (bool, string, error) {

	modified := false

	// TODO: See if we can just get/use the list of associatedVPC IDs
	associatedVPCs, err := a.getAssociatedVPCs(awsClient, cd, metadata, logger)
	if err != nil {
		logger.WithError(err).Error("could not get associated VPCs")
		return modified, "", err
	}

	hzID, err := findHostedZone(awsClient, associatedVPCs, apiDomain, logger)
	if err != nil && errors.Is(err, errNoHostedZoneFoundForVPC) {
		modified = true
		hzID, err = createHostedZone(awsClient, &associatedVPCs[0], apiDomain, logger)
		if err != nil {
			return modified, "", err
		}
	}
	if err != nil {
		logger.WithError(err).Error("failed to get Hosted Zone")
		return modified, "", err
	}

	initPrivateLinkStatus(cd)
	cd.Status.Platform.AWS.PrivateLink.HostedZoneID = hzID
	if err := updatePrivateLinkStatus(a.client, cd, logger); err != nil {
		logger.WithError(err).Error("failed to update the hosted zone ID for cluster deployment")
		return modified, "", err
	}

	return modified, hzID, nil
}

// findHostedZone finds a Private Hosted Zone for apiDomain that is associated with the given VPCs.
// If no such hosted zone exists, it return an errNoHostedZoneFoundForVPC error.
func findHostedZone(awsClient awsclient.Client, associatedVPCs []hivev1.AWSAssociatedVPC, apiDomain string, logger log.FieldLogger) (string, error) {
	for _, vpc := range associatedVPCs {

		// TODO: Also see if we can directly find it using hostedzone id from status

		input := &route53.ListHostedZonesByVPCInput{
			VPCId:     aws.String(vpc.VPCID),
			VPCRegion: aws.String(vpc.Region),

			MaxItems: aws.String("100"),
		}

		var nextToken *string
		for {
			input.NextToken = nextToken
			resp, err := awsClient.ListHostedZonesByVPC(input)
			if err != nil {
				return "", err
			}
			for _, summary := range resp.HostedZoneSummaries {
				if strings.EqualFold(apiDomain, strings.TrimSuffix(aws.StringValue(summary.Name), ".")) {
					return *summary.HostedZoneId, nil
				}
			}
			if resp.NextToken == nil {
				break
			}
			nextToken = resp.NextToken
		}
	}
	return "", errNoHostedZoneFoundForVPC
}

func createHostedZone(
	awsClient awsclient.Client,
	associatedVPC *hivev1.AWSAssociatedVPC,
	apiDomain string,
	logger log.FieldLogger) (string, error) {

	hzLog := logger.WithField("vpcID", associatedVPC.VPCID).WithField("apiDomain", apiDomain)
	resp, err := awsClient.CreateHostedZone(&route53.CreateHostedZoneInput{
		CallerReference: aws.String(time.Now().String()),
		Name:            aws.String(apiDomain),
		HostedZoneConfig: &route53.HostedZoneConfig{
			PrivateZone: aws.Bool(true),
		},
		VPC: &route53.VPC{
			VPCId:     aws.String(associatedVPC.VPCID),
			VPCRegion: aws.String(associatedVPC.Region),
		},
	})
	if err != nil {
		hzLog.WithError(err).Error("could not create Private Hosted Zone")
		return "", err
	}

	return *resp.HostedZone.Id, nil
}

// reconcileHostedZoneAssociations ensures that the all the VPCs in the associatedVPCs list from
// the controller config are associated to the PHZ hostedZoneID.
func (a *AWSHubActuator) reconcileHostedZoneAssociations(
	awsClient *awsClient,
	cd *hivev1.ClusterDeployment,
	metadata *hivev1.ClusterMetadata,
	hostedZoneID string,
	logger log.FieldLogger) (bool, error) {

	hzLog := logger.WithField("hostedZoneID", hostedZoneID)
	hzLog.Debug("reconciling HostedZoneAssociations")

	modified := false
	vpcInfo := a.config.DeepCopy().AssociatedVPCs
	vpcIdx := map[string]int{}
	for i, v := range vpcInfo {
		vpcIdx[v.VPCID] = i
	}

	zoneResp, err := awsClient.hub.GetHostedZone(&route53.GetHostedZoneInput{
		Id: aws.String(hostedZoneID),
	})
	if err != nil {
		hzLog.WithError(err).Error("failed to get the Hosted Zone")
		return modified, err
	}

	oldVPCs := sets.NewString()
	for _, vpc := range zoneResp.VPCs {
		id := aws.StringValue(vpc.VPCId)
		oldVPCs.Insert(id)
		if _, ok := vpcIdx[id]; !ok { // make sure we have info for all VPCs for later use
			vpcInfo = append(vpcInfo, hivev1.AWSAssociatedVPC{
				AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{
					VPCID:  id,
					Region: aws.StringValue(vpc.VPCRegion),
				},
			})
			vpcIdx[id] = len(vpcInfo) - 1
		}
	}

	associatedVPCs, err := a.getAssociatedVPCs(awsClient.hub, cd, metadata, logger)
	if err != nil {
		logger.WithError(err).Error("could not get associated VPCs")
		return modified, err
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
		vpcLog := hzLog.WithField("vpc", vpc)
		info := vpcInfo[vpcIdx[vpc]]

		awsAssociationClient := awsClient.hub
		if info.CredentialsSecretRef != nil {
			// since this VPC is in different account we need to authorize before continuing
			_, err := awsClient.hub.CreateVPCAssociationAuthorization(&route53.CreateVPCAssociationAuthorizationInput{
				HostedZoneId: aws.String(hostedZoneID),
				VPC: &route53.VPC{
					VPCId:     aws.String(vpc),
					VPCRegion: aws.String(info.Region),
				},
			})
			if err != nil {
				vpcLog.WithError(err).Error("failed to create authorization for association of the Hosted Zone to the VPC")
				return modified, err
			}

			awsAssociationClient, err = a.awsClientFn(*a.client, awsclient.Options{
				Region: info.Region,
				CredentialsSource: awsclient.CredentialsSource{
					Secret: &awsclient.SecretCredentialsSource{
						Namespace: controllerutils.GetHiveNamespace(),
						Ref:       info.CredentialsSecretRef,
					},
				},
			})
			if err != nil {
				vpcLog.WithError(err).Error("failed to create AWS client for association of the Hosted Zone to the VPC")
				return modified, err
			}
		}

		_, err = awsAssociationClient.AssociateVPCWithHostedZone(&route53.AssociateVPCWithHostedZoneInput{
			HostedZoneId: aws.String(hostedZoneID),
			VPC: &route53.VPC{
				VPCId:     aws.String(vpc),
				VPCRegion: aws.String(info.Region),
			},
		})
		if err != nil {
			hzLog.WithField("vpc", vpc).WithError(err).Error("failed to associate the Hosted Zone to the VPC")
			return modified, err
		}

		if info.CredentialsSecretRef != nil {
			// since we created an authorization and association is complete, we should remove the object
			// as recommended by AWS best practices.
			_, err := awsClient.hub.DeleteVPCAssociationAuthorization(&route53.DeleteVPCAssociationAuthorizationInput{
				HostedZoneId: aws.String(hostedZoneID),
				VPC: &route53.VPC{
					VPCId:     aws.String(vpc),
					VPCRegion: aws.String(info.Region),
				},
			})
			if err != nil {
				vpcLog.WithError(err).Error("failed to delete authorization for association of the Hosted Zone to the VPC")
				return modified, err
			}
		}
	}
	for _, vpc := range removed {
		vpcLog := hzLog.WithField("vpc", vpc)
		info := vpcInfo[vpcIdx[vpc]]
		_, err = awsClient.hub.DisassociateVPCFromHostedZone(&route53.DisassociateVPCFromHostedZoneInput{
			HostedZoneId: aws.String(hostedZoneID),
			VPC: &route53.VPC{
				VPCId:     aws.String(vpc),
				VPCRegion: aws.String(info.Region),
			},
		})
		if err != nil {
			vpcLog.WithError(err).Error("failed to disassociate the Hosted Zone to the VPC")
			return modified, err
		}
	}

	return modified, nil
}

func (a *AWSHubActuator) cleanupHostedZone(
	awsClient awsclient.Client,
	cd *hivev1.ClusterDeployment,
	metadata *hivev1.ClusterMetadata,
	logger log.FieldLogger) error {

	var hzID string
	if cd.Status.Platform != nil &&
		cd.Status.Platform.AWS != nil &&
		cd.Status.Platform.AWS.PrivateLink != nil &&
		cd.Status.Platform.AWS.PrivateLink.HostedZoneID != "" {
		hzID = cd.Status.Platform.AWS.PrivateLink.HostedZoneID
	}

	// TODO: share this code with findHostedZone
	if hzID == "" { // since we don't have the hz ID, we try to discover it to prevent leaks
		apiDomain, err := initialURL(*a.client,
			client.ObjectKey{Namespace: cd.Namespace, Name: metadata.AdminKubeconfigSecretRef.Name})
		if apierrors.IsNotFound(err) {
			logger.Info("no hostedZoneID in status and admin kubeconfig does not exist, skipping hosted zone cleanup")
			return nil
		} else if err != nil {
			logger.WithError(err).Error("could not get API URL from kubeconfig")
			return err
		}

		idLog := logger.WithField("infraID", metadata.InfraID)

		associatedVPCs, err := a.getAssociatedVPCs(awsClient, cd, metadata, logger)
		if err != nil {
			idLog.WithError(err).Error("could not get associated VPCs")
			return err
		}

		hzID, err = findHostedZone(awsClient, associatedVPCs, apiDomain, logger)
		if err != nil && errors.Is(err, errNoHostedZoneFoundForVPC) {
			return nil // no work
		}
		if err != nil {
			idLog.WithError(err).Error("error getting the Hosted Zone")
			return err
		}
	}

	hzLog := logger.WithField("hostedZoneID", hzID)
	recordsResp, err := awsClient.ListResourceRecordSets(&route53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String(hzID),
	})
	if awsErrCodeEquals(err, "NoSuchHostedZone") {
		return nil // no more work
	}
	if err != nil {
		hzLog.WithError(err).Error("failed to list the hosted zone")
		return err
	}
	for _, record := range recordsResp.ResourceRecordSets {
		if *record.Type == "SOA" || *record.Type == "NS" {
			// can't delete SOA and NS types
			continue
		}
		_, err := awsClient.ChangeResourceRecordSets(&route53.ChangeResourceRecordSetsInput{
			HostedZoneId: aws.String(hzID),
			ChangeBatch: &route53.ChangeBatch{
				Changes: []*route53.Change{{
					Action:            aws.String("DELETE"),
					ResourceRecordSet: record,
				}},
			},
		})
		if err != nil {
			hzLog.WithField("record", *record.Name).WithError(err).Error("failed to list the hosted zone")
			return err
		}
	}

	_, err = awsClient.DeleteHostedZone(&route53.DeleteHostedZoneInput{
		Id: aws.String(hzID),
	})
	if err != nil && !awsErrCodeEquals(err, "NoSuchHostedZone") {
		hzLog.WithError(err).Error("error deleting the hosted zone")
		return err
	}

	return nil
}

func (a *AWSHubActuator) getAssociatedVPCs(
	awsClient awsclient.Client,
	cd *hivev1.ClusterDeployment,
	metadata *hivev1.ClusterMetadata,
	logger log.FieldLogger) ([]hivev1.AWSAssociatedVPC, error) {

	associatedVPCs := a.config.DeepCopy().AssociatedVPCs

	// For clusterdeployments that are on AWS, also add the VPCEndpoint VPC
	if cd.Status.Platform != nil &&
		cd.Status.Platform.AWS != nil &&
		cd.Status.Platform.AWS.PrivateLink != nil &&
		cd.Status.Platform.AWS.PrivateLink.VPCEndpointID != "" {

		endpointResp, err := awsClient.DescribeVpcEndpoints(&ec2.DescribeVpcEndpointsInput{
			Filters: []*ec2.Filter{ec2FilterForCluster(metadata)},
		})
		if err != nil {
			logger.WithError(err).Error("error getting the VPC Endpoint")
			return associatedVPCs, err
		}

		if len(endpointResp.VpcEndpoints) == 0 {
			return associatedVPCs, nil // no vpc endpoint
		}

		logger.Debugf("adding VpcEndpoint [%s] VPC [%s]", *endpointResp.VpcEndpoints[0].VpcEndpointId, *endpointResp.VpcEndpoints[0].VpcId)
		endpointVPC := hivev1.AWSAssociatedVPC{
			AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{
				VPCID:  *endpointResp.VpcEndpoints[0].VpcId,
				Region: cd.Spec.Platform.AWS.Region,
			},
		}

		associatedVPCs = append(associatedVPCs, endpointVPC)
	}

	return associatedVPCs, nil
}
