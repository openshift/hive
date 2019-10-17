package dnszone

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/miekg/dns"
	log "github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go/service/route53"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apihelpers "github.com/openshift/hive/pkg/apis/helpers"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	awsclient "github.com/openshift/hive/pkg/awsclient"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	hiveDNSZoneTag                  = "hive.openshift.io/dnszone"
	domainAvailabilityCheckInterval = 30 * time.Second
	defaultNSRecordTTL              = hivev1.TTL(60)
	dnsClientTimeout                = 30 * time.Second
	resolverConfigFile              = "/etc/resolv.conf"
	zoneCheckDNSServersEnvVar       = "ZONE_CHECK_DNS_SERVERS"
)

// ZoneReconciler manages getting the desired state, getting the current state and reconciling the two.
type ZoneReconciler struct {
	// dnsZone is the kube object that represents the desired state
	dnsZone *hivev1.DNSZone

	// logger is the logger used for this controller
	logger log.FieldLogger

	// kubeClient is a kubernetes client to access general cluster / project related objects.
	kubeClient client.Client

	// awsClient is a utility for making it easy for controllers to interface with AWS
	awsClient awsclient.Client

	// scheme is the controller manager's scheme
	scheme *runtime.Scheme

	// soaLookup is a function that looks up a zone's SOA record
	soaLookup func(string, log.FieldLogger) (bool, error)
}

// NewZoneReconciler creates a new ZoneReconciler object. A new ZoneReconciler is expected to be created for each controller sync.
func NewZoneReconciler(
	dnsZone *hivev1.DNSZone,
	kubeClient client.Client,
	logger log.FieldLogger,
	awsClient awsclient.Client,
	scheme *runtime.Scheme,
) (*ZoneReconciler, error) {
	if dnsZone == nil {
		return nil, fmt.Errorf("ZoneReconciler requires dnsZone to be set")
	}
	zoneReconciler := &ZoneReconciler{
		dnsZone:    dnsZone,
		kubeClient: kubeClient,
		logger:     logger,
		awsClient:  awsClient,
		soaLookup:  lookupSOARecord,
		scheme:     scheme,
	}

	return zoneReconciler, nil
}

// Reconcile attempts to make the current state reflect the desired state. It does this idempotently.
func (zr *ZoneReconciler) Reconcile() (reconcile.Result, error) {
	zr.logger.Debug("Retrieving current state")
	hostedZone, tags, err := zr.getCurrentState()
	if err != nil {
		zr.logger.WithError(err).Error("Failed to retrieve hosted zone and corresponding tags")
		return reconcile.Result{}, err
	}
	if zr.dnsZone.DeletionTimestamp != nil {
		if hostedZone != nil {
			zr.logger.Debug("DNSZone resource is deleted, deleting hosted zone")
			err = zr.deleteRoute53HostedZone(hostedZone)
			if err != nil {
				zr.logger.WithError(err).Error("Failed to delete hosted zone")
				return reconcile.Result{}, err
			}
		}
		// Remove the finalizer from the DNSZone. It will be persisted when we persist status
		zr.logger.Info("Removing DNSZone finalizer")
		controllerutils.DeleteFinalizer(zr.dnsZone, hivev1.FinalizerDNSZone)
		err := zr.kubeClient.Update(context.TODO(), zr.dnsZone)
		if err != nil {
			zr.logger.WithError(err).Log(controllerutils.LogLevel(err), "Failed to remove DNSZone finalizer")
		}
		return reconcile.Result{}, err
	}
	if !controllerutils.HasFinalizer(zr.dnsZone, hivev1.FinalizerDNSZone) {
		zr.logger.Info("DNSZone does not have a finalizer. Adding one.")
		controllerutils.AddFinalizer(zr.dnsZone, hivev1.FinalizerDNSZone)
		err := zr.kubeClient.Update(context.TODO(), zr.dnsZone)
		if err != nil {
			zr.logger.WithError(err).Log(controllerutils.LogLevel(err), "Failed to add finalizer to DNSZone")
		}
		return reconcile.Result{}, err
	}
	if hostedZone == nil {
		zr.logger.Info("No corresponding hosted zone found on cloud provider, creating one")
		hostedZone, err = zr.createRoute53HostedZone()
		if err != nil {
			zr.logger.WithError(err).Error("Failed to create hosted zone")
			return reconcile.Result{}, err
		}
	} else {
		zr.logger.Info("Existing hosted zone found. Syncing with DNSZone resource")
		// For now, tags are the only things we can sync with existing zones.
		err = zr.syncTags(hostedZone.Id, tags)
		if err != nil {
			zr.logger.WithError(err).Error("failed to sync tags for hosted zone")
			return reconcile.Result{}, err
		}
	}

	nameServers, err := zr.getHostedZoneNSRecord(*hostedZone.Id)
	if err != nil {
		zr.logger.WithError(err).Error("Failed to get hosted zone name servers")
		return reconcile.Result{}, err
	}

	if zr.dnsZone.Spec.LinkToParentDomain {
		err = zr.syncParentDomainLink(nameServers)
		if err != nil {
			zr.logger.WithError(err).Error("failed syncing parent domain link")
			return reconcile.Result{}, err
		}
	}

	isZoneSOAAvailable, err := zr.soaLookup(zr.dnsZone.Spec.Zone, zr.logger)
	if err != nil {
		zr.logger.WithError(err).Error("error looking up SOA record for zone")
	}

	reconcileResult := reconcile.Result{}
	if !isZoneSOAAvailable {
		zr.logger.Info("SOA record for DNS zone not available")
		reconcileResult.Requeue = true
		reconcileResult.RequeueAfter = domainAvailabilityCheckInterval
	}

	return reconcileResult, zr.updateStatus(hostedZone, nameServers, isZoneSOAAvailable)
}

func (zr *ZoneReconciler) syncParentDomainLink(nameServers []string) error {
	existingLinkRecord := &hivev1.DNSEndpoint{}
	existingLinkRecordName := types.NamespacedName{
		Namespace: zr.dnsZone.Namespace,
		Name:      parentLinkRecordName(zr.dnsZone.Name),
	}
	err := zr.kubeClient.Get(context.TODO(), existingLinkRecordName, existingLinkRecord)
	if err != nil && !errors.IsNotFound(err) {
		zr.logger.WithError(err).Error("failed retrieving existing DNSEndpoint")
		return err
	}
	dnsEndpointNotFound := err != nil

	linkRecord, err := zr.parentLinkRecord(nameServers)
	if err != nil {
		zr.logger.WithError(err).Error("failed to create parent link DNSEndpoint")
		return err
	}

	if dnsEndpointNotFound {
		if err = zr.kubeClient.Create(context.TODO(), linkRecord); err != nil {
			zr.logger.WithError(err).Log(controllerutils.LogLevel(err), "failed creating DNSEndpoint")
			return err
		}
		return nil
	}

	if !reflect.DeepEqual(existingLinkRecord.Spec, linkRecord.Spec) {
		existingLinkRecord.Spec = linkRecord.Spec
		if err = zr.kubeClient.Update(context.TODO(), existingLinkRecord); err != nil {
			zr.logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to update existing DNSEndpoint")
			return err
		}
	}

	return nil
}

func (zr *ZoneReconciler) parentLinkRecord(nameServers []string) (*hivev1.DNSEndpoint, error) {
	targets := hivev1.Targets{}
	for _, nameServer := range nameServers {
		// external-dns will compare the NS record values without the
		// dot at the end. If a dot is present, an update happens every sync.
		targets = append(targets, strings.TrimSuffix(nameServer, "."))
	}
	endpoint := &hivev1.DNSEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      parentLinkRecordName(zr.dnsZone.Name),
			Namespace: zr.dnsZone.Namespace,
		},
		Spec: hivev1.DNSEndpointSpec{
			Endpoints: []*hivev1.Endpoint{
				{
					DNSName:    zr.dnsZone.Spec.Zone,
					Targets:    targets,
					RecordType: "NS",
					RecordTTL:  defaultNSRecordTTL,
				},
			},
		},
	}
	if err := controllerutil.SetControllerReference(zr.dnsZone, endpoint, zr.scheme); err != nil {
		return nil, err
	}
	return endpoint, nil
}

// syncTags determines if there are changes that need to happen to match tags in the spec
func (zr *ZoneReconciler) syncTags(zoneID *string, existingTags []*route53.Tag) error {
	expected := zr.expectedTags()
	toAdd := []*route53.Tag{}
	toDelete := make([]*route53.Tag, len(existingTags))
	// Initially add all existing tags to the toDelete array
	// As they're found in the expected array, remove them from
	// the toDelete array
	copy(toDelete, existingTags)

	logger := zr.logger.WithField("id", aws.StringValue(zoneID))
	logger.WithField("current", tagsString(existingTags)).WithField("expected", tagsString(expected)).Debug("syncing tags")

	for _, tag := range expected {
		found := false
		for i, actualTag := range toDelete {
			if tagEquals(tag, actualTag) {
				found = true
				toDelete = append(toDelete[:i], toDelete[i+1:]...)
				logger.WithField("tag", tagString(tag)).Debug("tag already exists, will not be added")
				break
			}
		}
		if !found {
			logger.WithField("tag", tagString(tag)).Debug("tag will be added")
			toAdd = append(toAdd, tag)
		}
	}

	if len(toDelete) == 0 && len(toAdd) == 0 {
		logger.Debug("tags are in sync, no action required")
		return nil
	}

	keysToDelete := make([]*string, 0, len(toDelete))
	for _, tag := range toDelete {
		logger.WithField("tag", tagString(tag)).Debug("tag will be deleted")
		keysToDelete = append(keysToDelete, tag.Key)
	}

	// Only 10 tags can be added/removed at a time. Iterate until all tags are added/removed
	index := 0
	for len(toAdd) > index || len(keysToDelete) > index {
		toAddSegment := []*route53.Tag{}
		keysToDeleteSegment := []*string{}

		if len(toAdd) > index {
			toAddSegment = toAdd[index:min(index+10, len(toAdd))]
		}

		if len(keysToDelete) > index {
			keysToDeleteSegment = keysToDelete[index:min(index+10, len(keysToDelete))]
		}

		if len(toAddSegment) == 0 {
			toAddSegment = nil
		}
		if len(keysToDeleteSegment) == 0 {
			keysToDeleteSegment = nil
		}

		logger.Debugf("Adding %d tags, deleting %d tags", len(toAddSegment), len(keysToDeleteSegment))
		_, err := zr.awsClient.ChangeTagsForResource(&route53.ChangeTagsForResourceInput{
			AddTags:       toAddSegment,
			RemoveTagKeys: keysToDeleteSegment,
			ResourceId:    zoneID,
			ResourceType:  aws.String("hostedzone"),
		})
		if err != nil {
			logger.WithError(err).Error("Cannot update tags for hosted zone")
			return err
		}
		index += 10
	}

	return nil
}

func min(a, b int) int {
	if a <= b {
		return a
	}
	return b
}

// getCurrentState gets the AWS object for the zone.
// If a zone cannot be found or no longer exists, nil is returned.
func (zr *ZoneReconciler) getCurrentState() (*route53.HostedZone, []*route53.Tag, error) {
	var zoneID string
	var err error
	if zr.dnsZone.Status.AWS != nil && zr.dnsZone.Status.AWS.ZoneID != nil {
		zr.logger.Debug("Zone ID is set in status, will retrieve by ID")
		zoneID = *zr.dnsZone.Status.AWS.ZoneID
	}
	if len(zoneID) == 0 {
		zr.logger.Debug("Zone ID is not set in status, looking up by tag")
		zoneID, err = zr.findZoneIDByTag()
		if err != nil {
			zr.logger.WithError(err).Error("Failed to lookup zone by tag")
			return nil, nil, err
		}
	}
	if len(zoneID) == 0 {
		zr.logger.Debug("No matching existing zone found")
		return nil, nil, nil
	}

	// Fetch the hosted zone
	logger := zr.logger.WithField("id", zoneID)
	logger.Debug("Fetching hosted zone by ID")
	resp, err := zr.awsClient.GetHostedZone(&route53.GetHostedZoneInput{Id: aws.String(zoneID)})
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			if awsErr.Code() == route53.ErrCodeNoSuchHostedZone {
				logger.Debug("Zone no longer exists")
				return nil, nil, nil
			}
		}
		logger.WithError(err).Error("Cannot get hosted zone")
		return nil, nil, err
	}
	logger.Debug("Found hosted zone")

	logger.Debug("Fetching hosted zone tags")
	tags, err := zr.existingTags(resp.HostedZone.Id)
	if err != nil {
		logger.WithError(err).Error("Cannot get hosted zone tags")
		return nil, nil, err
	}

	return resp.HostedZone, tags, nil
}

func (zr *ZoneReconciler) findZoneIDByTag() (string, error) {
	tagFilter := &resourcegroupstaggingapi.TagFilter{
		Key:    aws.String(hiveDNSZoneTag),
		Values: []*string{aws.String(fmt.Sprintf("%s/%s", zr.dnsZone.Namespace, zr.dnsZone.Name))},
	}
	filterString := fmt.Sprintf("%s=%s", aws.StringValue(tagFilter.Key), aws.StringValue(tagFilter.Values[0]))
	zr.logger.WithField("filter", filterString).Debug("Searching for zone by tag")
	id := ""
	err := zr.awsClient.GetResourcesPages(&resourcegroupstaggingapi.GetResourcesInput{
		ResourceTypeFilters: []*string{aws.String("route53:hostedzone")},
		TagFilters:          []*resourcegroupstaggingapi.TagFilter{tagFilter},
	}, func(resp *resourcegroupstaggingapi.GetResourcesOutput, lastPage bool) bool {
		for _, zone := range resp.ResourceTagMappingList {
			logger := zr.logger.WithField("arn", aws.StringValue(zone.ResourceARN))
			logger.Debug("Processing search result")
			zoneARN, err := arn.Parse(aws.StringValue(zone.ResourceARN))
			if err != nil {
				logger.WithError(err).Error("Failed to parse hostedzone ARN")
				continue
			}
			elems := strings.Split(zoneARN.Resource, "/")
			if len(elems) != 2 || elems[0] != "hostedzone" {
				logger.Error("Unexpected hostedzone ARN")
				continue
			}
			id = elems[1]
			logger.WithField("id", id).Debug("Found hosted zone")
			return false
		}
		return true
	})
	return id, err
}

func (zr *ZoneReconciler) expectedTags() []*route53.Tag {
	tags := []*route53.Tag{
		{
			Key:   aws.String(hiveDNSZoneTag),
			Value: aws.String(fmt.Sprintf("%s/%s", zr.dnsZone.Namespace, zr.dnsZone.Name)),
		},
	}
	if zr.dnsZone.Spec.AWS != nil {
		for _, tag := range zr.dnsZone.Spec.AWS.AdditionalTags {
			tags = append(tags, &route53.Tag{
				Key:   aws.String(tag.Key),
				Value: aws.String(tag.Value),
			})
		}
	}
	zr.logger.WithField("tags", tagsString(tags)).Debug("Expected tags")
	return tags
}

func (zr *ZoneReconciler) existingTags(zoneID *string) ([]*route53.Tag, error) {
	logger := zr.logger.WithField("zone", aws.StringValue(zoneID))
	logger.Debug("listing existing tags for zone")
	resp, err := zr.awsClient.ListTagsForResource(&route53.ListTagsForResourceInput{
		ResourceId:   zoneID,
		ResourceType: aws.String("hostedzone"),
	})
	if err != nil {
		logger.WithError(err).Error("cannot list tags for zone")
		return nil, err
	}
	logger.WithField("tags", tagsString(resp.ResourceTagSet.Tags)).Debug("retrieved zone tags")
	return resp.ResourceTagSet.Tags, nil
}

// createRoute53HostedZone creates an AWS Route53 hosted zone given the desired state
func (zr *ZoneReconciler) createRoute53HostedZone() (*route53.HostedZone, error) {
	logger := zr.logger.WithField("zone", zr.dnsZone.Spec.Zone)
	logger.Info("Creating route53 hostedzone")
	var hostedZone *route53.HostedZone
	resp, err := zr.awsClient.CreateHostedZone(&route53.CreateHostedZoneInput{
		Name: aws.String(zr.dnsZone.Spec.Zone),
		// We use the UID of the HostedZone resource as the caller reference so that if
		// we fail to update the status of the HostedZone with the ID of the recently
		// created zone, we don't attempt to recreate it. Same if communication fails on
		// the response from AWS.
		CallerReference: aws.String(string(zr.dnsZone.UID)),
	})
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok && awsErr.Code() == route53.ErrCodeHostedZoneAlreadyExists {
			// If the zone was already created, we need to find its ID
			logger.WithField("callerRef", zr.dnsZone.UID).Debug("Hosted zone already exists, looking up by caller reference")
			hostedZone, err = zr.findZoneByCallerReference(zr.dnsZone.Spec.Zone, string(zr.dnsZone.UID))
			if err != nil {
				logger.Error("Failed to find zone by caller reference")
				return nil, err
			}
		} else {
			logger.WithError(err).Error("Error creating hosted zone")
			return nil, err
		}
	} else {
		logger.Debug("Hosted zone successfully created")
		hostedZone = resp.HostedZone
	}

	logger = logger.WithField("id", aws.StringValue(hostedZone.Id))
	logger.Debug("Fetching zone tags")
	existingTags, err := zr.existingTags(hostedZone.Id)
	if err != nil {
		logger.WithError(err).Error("Failed to fetch zone tags")
		return nil, err
	}

	logger.Debug("Syncing zone tags")
	err = zr.syncTags(hostedZone.Id, existingTags)
	if err != nil {
		// When an error occurs tagging the resource, we return an error. This will result in a retry of the create call.
		// Because we're using the DNSZone's UID as the CallerReference, the create should succeed without creating a duplicate
		// zone. We will then retry adding the tags.
		logger.WithError(err).Error("Failed to apply tags to newly created zone")
		return nil, err
	}

	return hostedZone, err
}

func (zr *ZoneReconciler) findZoneByCallerReference(domain, callerRef string) (*route53.HostedZone, error) {
	logger := zr.logger.WithField("domain", domain).WithField("callerRef", callerRef)
	logger.Debug("Searching for zone by domain and callerRef")
	var nextZoneID *string
	var nextName = aws.String(domain)
	for {
		logger.Debug("listing hosted zones by name")
		resp, err := zr.awsClient.ListHostedZonesByName(&route53.ListHostedZonesByNameInput{
			DNSName:      nextName,
			HostedZoneId: nextZoneID,
			MaxItems:     aws.String("50"),
		})
		if err != nil {
			logger.WithError(err).Error("cannot list zones by name")
			return nil, err
		}
		for _, zone := range resp.HostedZones {
			if aws.StringValue(zone.CallerReference) == callerRef {
				logger.WithField("zone", aws.StringValue(zone.Id)).Debug("found hosted zone matching caller reference")
				return zone, nil
			}
			if aws.StringValue(zone.Name) != domain {
				logger.WithField("name", aws.StringValue(zone.Name)).Debug("reached zone with different domain name, aborting search")
				return nil, fmt.Errorf("Hosted zone not found")
			}
		}
		if !aws.BoolValue(resp.IsTruncated) {
			logger.Debug("reached end of results, did not find hosted zone")
			return nil, fmt.Errorf("Hosted zone not found")
		}
		nextZoneID = resp.NextHostedZoneId
		nextName = resp.NextDNSName
	}
}

// deleteRoute53HostedZone deletes an AWS Route53 hosted zone, typically because the desired state is in a deleting state.
func (zr *ZoneReconciler) deleteRoute53HostedZone(hostedZone *route53.HostedZone) error {
	logger := zr.logger.WithField("zone", zr.dnsZone.Spec.Zone).WithField("id", aws.StringValue(hostedZone.Id))
	logger.Info("Deleting route53 hostedzone")
	_, err := zr.awsClient.DeleteHostedZone(&route53.DeleteHostedZoneInput{
		Id: hostedZone.Id,
	})
	if err != nil {
		log.WithError(err).Error("Cannot delete hosted zone")
	}
	return err
}

func (zr *ZoneReconciler) updateStatus(hostedZone *route53.HostedZone, nameServers []string, isSOAAvailable bool) error {
	orig := zr.dnsZone.DeepCopy()
	zr.logger.Debug("Updating DNSZone status")

	zr.dnsZone.Status.NameServers = nameServers
	zr.dnsZone.Status.AWS = &hivev1.AWSDNSZoneStatus{
		ZoneID: hostedZone.Id,
	}
	var availableStatus corev1.ConditionStatus
	var availableReason, availableMessage string
	if isSOAAvailable {
		// We need to keep track of the last time we synced to rate limit our AWS calls.
		tmpTime := metav1.Now()
		zr.dnsZone.Status.LastSyncTimestamp = &tmpTime

		availableStatus = corev1.ConditionTrue
		availableReason = "ZoneAvailable"
		availableMessage = "DNS SOA record for zone is reachable"
	} else {
		availableStatus = corev1.ConditionFalse
		availableReason = "ZoneUnavailable"
		availableMessage = "DNS SOA record for zone is not reachable"
	}
	zr.dnsZone.Status.LastSyncGeneration = zr.dnsZone.ObjectMeta.Generation
	zr.dnsZone.Status.Conditions = controllerutils.SetDNSZoneCondition(
		zr.dnsZone.Status.Conditions,
		hivev1.ZoneAvailableDNSZoneCondition,
		availableStatus,
		availableReason,
		availableMessage,
		controllerutils.UpdateConditionNever)

	if !reflect.DeepEqual(orig.Status, zr.dnsZone.Status) {
		err := zr.kubeClient.Status().Update(context.TODO(), zr.dnsZone)
		if err != nil {
			zr.logger.WithError(err).Log(controllerutils.LogLevel(err), "Cannot update DNSZone status")
		}
		return err
	}
	return nil
}

func (zr *ZoneReconciler) getHostedZoneNSRecord(zoneID string) ([]string, error) {
	logger := zr.logger.WithField("id", zoneID)
	logger.Debug("Listing hosted zone NS records")
	resp, err := zr.awsClient.ListResourceRecordSets(&route53.ListResourceRecordSetsInput{
		HostedZoneId:    aws.String(zoneID),
		StartRecordType: aws.String("NS"),
		StartRecordName: aws.String(zr.dnsZone.Spec.Zone),
		MaxItems:        aws.String("1"),
	})
	if err != nil {
		logger.WithError(err).Error("Error listing recordsets for zone")
		return nil, err
	}
	if len(resp.ResourceRecordSets) != 1 {
		msg := fmt.Sprintf("unexpected number of recordsets returned: %d", len(resp.ResourceRecordSets))
		logger.Error(msg)
		return nil, fmt.Errorf(msg)
	}
	if aws.StringValue(resp.ResourceRecordSets[0].Type) != "NS" {
		msg := "name server record not found"
		logger.Error(msg)
		return nil, fmt.Errorf(msg)
	}
	if aws.StringValue(resp.ResourceRecordSets[0].Name) != (zr.dnsZone.Spec.Zone + ".") {
		msg := fmt.Sprintf("name server record not found for domain %s", zr.dnsZone.Spec.Zone)
		logger.Error(msg)
		return nil, fmt.Errorf(msg)
	}
	result := make([]string, 0, len(resp.ResourceRecordSets[0].ResourceRecords))
	for _, record := range resp.ResourceRecordSets[0].ResourceRecords {
		result = append(result, aws.StringValue(record.Value))
	}
	logger.WithField("nameservers", strings.Join(result, ",")).Debug("found hosted zone name servers")
	return result, nil
}

func parentLinkRecordName(dnsZoneName string) string {
	return apihelpers.GetResourceName(dnsZoneName, "ns")
}

func lookupSOARecord(zone string, logger log.FieldLogger) (bool, error) {
	// TODO: determine if there's a better way to obtain resolver endpoints
	clientConfig, _ := dns.ClientConfigFromFile(resolverConfigFile)
	client := dns.Client{Timeout: dnsClientTimeout}

	dnsServers := []string{}
	serversFromEnv := os.Getenv(zoneCheckDNSServersEnvVar)
	if len(serversFromEnv) > 0 {
		dnsServers = strings.Split(serversFromEnv, ",")
		// Add port to servers with unspecified port
		for i := range dnsServers {
			if !strings.Contains(dnsServers[i], ":") {
				dnsServers[i] = dnsServers[i] + ":53"
			}
		}
	} else {
		for _, s := range clientConfig.Servers {
			dnsServers = append(dnsServers, fmt.Sprintf("%s:%s", s, clientConfig.Port))
		}
	}
	logger.WithField("servers", dnsServers).Info("looking up domain SOA record")

	m := &dns.Msg{}
	m.SetQuestion(zone+".", dns.TypeSOA)
	for _, s := range dnsServers {
		in, rtt, err := client.Exchange(m, s)
		if err != nil {
			logger.WithError(err).WithField("server", s).Info("query for SOA record failed")
			continue
		}
		log.WithField("server", s).Infof("SOA query duration: %v", rtt)
		if len(in.Answer) > 0 {
			for _, rr := range in.Answer {
				soa, ok := rr.(*dns.SOA)
				if !ok {
					logger.Info("Record returned is not an SOA record: %#v", rr)
					continue
				}
				if soa.Hdr.Name != appendPeriod(zone) {
					logger.WithField("zone", soa.Hdr.Name).Info("SOA record returned but it does not match the lookup zone")
					return false, nil
				}
				logger.WithField("zone", soa.Hdr.Name).Info("SOA record returned, zone is reachable")
				return true, nil
			}
		}
		logger.WithField("server", s).Info("no answer for SOA record returned")
		return false, nil
	}
	return false, nil
}

func appendPeriod(name string) string {
	if !strings.HasSuffix(name, ".") {
		return name + "."
	}
	return name
}

func tagEquals(a, b *route53.Tag) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return aws.StringValue(a.Key) == aws.StringValue(b.Key) &&
		aws.StringValue(a.Value) == aws.StringValue(b.Value)
}

func tagString(tag *route53.Tag) string {
	return fmt.Sprintf("%s=%s", aws.StringValue(tag.Key), aws.StringValue(tag.Value))
}

func tagsString(tags []*route53.Tag) string {
	return strings.Join(func() []string {
		result := []string{}
		for _, tag := range tags {
			result = append(result, tagString(tag))
		}
		return result
	}(), ",")
}
