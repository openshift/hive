package dnszone

import (
	"errors"
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	tagtypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/smithy-go"

	corev1 "k8s.io/api/core/v1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/awsclient"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	hiveDNSZoneAWSTag = "hive.openshift.io/dnszone"
)

// Ensure AWSActuator implements the Actuator interface. This will fail at compile time when false.
var _ Actuator = &AWSActuator{}

// AWSActuator manages getting the desired state, getting the current state and reconciling the two.
type AWSActuator struct {
	// logger is the logger used for this controller
	logger log.FieldLogger

	// awsClient is a utility for making it easy for controllers to interface with AWS
	awsClient awsclient.Client

	// hostedZone is the AWS object representing the hosted zone in route53
	hostedZone *route53types.HostedZone

	// currentTags are the list of tags associated with the currentHostedZone
	currentHostedZoneTags []route53types.Tag

	// The DNSZone that represents the desired state.
	dnsZone *hivev1.DNSZone
}

type awsClientBuilderType func(client.Client, awsclient.Options) (awsclient.Client, error)

// NewAWSActuator creates a new AWSActuator object. A new AWSActuator is expected to be created for each controller sync.
func NewAWSActuator(
	logger log.FieldLogger,
	kubeClient client.Client,
	credentials awsclient.CredentialsSource,
	dnsZone *hivev1.DNSZone,
	awsClientBuilder awsClientBuilderType,
) (*AWSActuator, error) {
	region := dnsZone.Spec.AWS.Region
	if region == "" {
		region = constants.AWSRoute53Region
	}
	awsClient, err := awsClientBuilder(kubeClient, awsclient.Options{
		Region:            region,
		CredentialsSource: credentials,
	})
	if err != nil {
		logger.WithError(err).Error("Error creating AWSClient")
		return nil, err
	}

	awsActuator := &AWSActuator{
		logger:    logger,
		awsClient: awsClient,
		dnsZone:   dnsZone,
	}

	return awsActuator, nil
}

// UpdateMetadata ensures that the Route53 hosted zone metadata is current with the DNSZone
func (a *AWSActuator) UpdateMetadata() error {
	if a.hostedZone == nil {
		return errors.New("hostedZone is unpopulated")
	}

	// For now, tags are the only things we can sync with existing zones.
	return a.syncTags()
}

// syncTags determines if there are changes that need to happen to match tags in the spec
func (a *AWSActuator) syncTags() error {
	existingTags := a.currentHostedZoneTags
	expected := a.expectedTags()
	toAdd := []route53types.Tag{}
	toDelete := make([]route53types.Tag, len(existingTags))
	// Initially add all existing tags to the toDelete array
	// As they're found in the expected array, remove them from
	// the toDelete array
	copy(toDelete, existingTags)

	zid := strings.TrimPrefix(*a.hostedZone.Id, "/hostedzone/")
	logger := a.logger.WithField("id", zid)
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

	keysToDelete := make([]string, 0, len(toDelete))
	for _, tag := range toDelete {
		logger.WithField("tag", tagString(tag)).Debug("tag will be deleted")
		keysToDelete = append(keysToDelete, *tag.Key)
	}

	// Only 10 tags can be added/removed at a time. Iterate until all tags are added/removed
	index := 0
	for len(toAdd) > index || len(keysToDelete) > index {
		toAddSegment := []route53types.Tag{}
		keysToDeleteSegment := []string{}

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
		_, err := a.awsClient.ChangeTagsForResource(&route53.ChangeTagsForResourceInput{
			AddTags:       toAddSegment,
			RemoveTagKeys: keysToDeleteSegment,
			ResourceId:    aws.String(zid),
			ResourceType:  route53types.TagResourceTypeHostedzone,
		})
		if err != nil {
			logger.WithError(err).Error("Cannot update tags for hosted zone")
			return err
		}
		index += 10
	}

	return nil
}

// modifyStatus updates the DnsZone's status with AWS specific information.
func (a *AWSActuator) modifyStatus() error {
	if a.hostedZone == nil {
		return errors.New("zoneID is unpopulated")
	}

	a.dnsZone.Status.AWS = &hivev1.AWSDNSZoneStatus{
		ZoneID: a.hostedZone.Id,
	}

	return nil
}

func min(a, b int) int {
	if a <= b {
		return a
	}
	return b
}

// Refresh gets the AWS object for the zone.
// If a zone cannot be found or no longer exists, actuator.zoneID remains unset.
func (a *AWSActuator) Refresh() error {
	var zoneIDs []string
	var err error
	if a.dnsZone.Status.AWS != nil && a.dnsZone.Status.AWS.ZoneID != nil {
		a.logger.Debug("Zone ID is set in status, will retrieve by ID")
		zoneIDs = []string{*a.dnsZone.Status.AWS.ZoneID}
	}
	if len(zoneIDs) == 0 {
		a.logger.Debug("Zone ID is not set in status, looking up by tag")
		zoneIDs, err = a.findZoneIDsByTag()
		if err != nil {
			a.logger.WithError(err).Error("Failed to lookup zone by tag")
			return err
		}
	}
	if len(zoneIDs) == 0 {
		a.logger.Debug("No matching existing zone found")
		return nil
	}

	// Fetch the hosted zone
	a.hostedZone = nil
	for _, zoneID := range zoneIDs {
		logger := a.logger.WithField("id", zoneID)
		logger.Debug("Fetching hosted zone by ID")
		resp, err := a.awsClient.GetHostedZone(&route53.GetHostedZoneInput{Id: aws.String(zoneID)})
		if err != nil {
			if awsclient.ErrCodeEquals(err, "NoSuchHostedZone") {
				logger.Debug("Zone no longer exists")
				continue
			}
			logger.WithError(err).Error("Cannot get hosted zone")
			return err
		}
		if name := *resp.HostedZone.Name; name != controllerutils.Dotted(a.dnsZone.Spec.Zone) {
			logger.WithField("zoneName", name).Debug("Zone name does not match expected name")
			continue
		}
		logger.Debug("Found hosted zone")
		a.hostedZone = resp.HostedZone

		// Update dnsZone status now that we have the zoneID
		if err := a.modifyStatus(); err != nil {
			a.logger.WithError(err).Error("failed to update status after refresh")
			return err
		}
	}

	if a.hostedZone == nil {
		a.logger.Debug("No existing zone found")
		return nil
	}

	logger := a.logger.WithField("id", a.hostedZone.Id)
	logger.Debug("Fetching hosted zone tags")
	tags, err := a.existingTags(a.hostedZone.Id)
	if err != nil {
		logger.WithError(err).Error("Cannot get hosted zone tags")
		return err
	}
	a.currentHostedZoneTags = tags

	return nil
}

func (a *AWSActuator) findZoneIDsByTag() ([]string, error) {
	var ids []string
	tagFilter := tagtypes.TagFilter{
		Key:    aws.String(hiveDNSZoneAWSTag),
		Values: []string{fmt.Sprintf("%s/%s", a.dnsZone.Namespace, a.dnsZone.Name)},
	}
	filterString := fmt.Sprintf("%s=%s", *tagFilter.Key, tagFilter.Values[0])
	a.logger.WithField("filter", filterString).Debug("Searching for zone by tag")
	id := ""
	err := a.awsClient.GetResourcesPages(&resourcegroupstaggingapi.GetResourcesInput{
		ResourceTypeFilters: []string{"route53:hostedzone"},
		TagFilters:          []tagtypes.TagFilter{tagFilter},
	}, func(resp *resourcegroupstaggingapi.GetResourcesOutput, lastPage bool) bool {
		for _, zone := range resp.ResourceTagMappingList {
			logger := a.logger.WithField("arn", aws.ToString(zone.ResourceARN))
			logger.Debug("Processing search result")
			zoneARN, err := arn.Parse(aws.ToString(zone.ResourceARN))
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
			ids = append(ids, id)
		}
		return true
	})
	return ids, err
}

func (a *AWSActuator) expectedTags() []route53types.Tag {
	tags := []route53types.Tag{
		{
			Key:   aws.String(hiveDNSZoneAWSTag),
			Value: aws.String(fmt.Sprintf("%s/%s", a.dnsZone.Namespace, a.dnsZone.Name)),
		},
	}
	if a.dnsZone.Spec.AWS != nil {
		for _, tag := range a.dnsZone.Spec.AWS.AdditionalTags {
			tags = append(tags, route53types.Tag{
				Key:   aws.String(tag.Key),
				Value: aws.String(tag.Value),
			})
		}
	}
	a.logger.WithField("tags", tagsString(tags)).Debug("Expected tags")
	return tags
}

func (a *AWSActuator) existingTags(zoneID *string) ([]route53types.Tag, error) {
	zid := strings.TrimPrefix(*zoneID, "/hostedzone/")
	logger := a.logger.WithField("id", zid)
	logger.Debug("listing existing tags for zone")
	resp, err := a.awsClient.ListTagsForResource(&route53.ListTagsForResourceInput{
		ResourceId:   aws.String(zid),
		ResourceType: route53types.TagResourceTypeHostedzone,
	})
	if err != nil {
		logger.WithError(err).Error("cannot list tags for zone")
		return nil, err
	}
	logger.WithField("tags", tagsString(resp.ResourceTagSet.Tags)).Debug("retrieved zone tags")
	return resp.ResourceTagSet.Tags, nil
}

// Create makes an AWS Route53 hosted zone given the DNSZone object.
func (a *AWSActuator) Create() error {
	logger := a.logger.WithField("zone", a.dnsZone.Spec.Zone)
	logger.Info("Creating route53 hostedzone")
	var hostedZone *route53types.HostedZone
	resp, err := a.awsClient.CreateHostedZone(&route53.CreateHostedZoneInput{
		Name: aws.String(a.dnsZone.Spec.Zone),
		// We use the UID of the HostedZone resource as the caller reference so that if
		// we fail to update the status of the HostedZone with the ID of the recently
		// created zone, we don't attempt to recreate it. Same if communication fails on
		// the response from AWS.
		CallerReference: aws.String(string(a.dnsZone.UID)),
	})
	if err != nil {
		if awsclient.ErrCodeEquals(err, "HostedZoneAlreadyExists") {
			// If the zone was already created, we need to find its ID
			logger.WithField("callerRef", a.dnsZone.UID).Debug("Hosted zone already exists, looking up by caller reference")
			hostedZone, err = a.findZoneByCallerReference(a.dnsZone.Spec.Zone, string(a.dnsZone.UID))
			if err != nil {
				logger.Error("Failed to find zone by caller reference")
				return err
			}
		} else {
			logger.WithError(err).Error("Error creating hosted zone")
			return err
		}
	} else {
		logger.Debug("Hosted zone successfully created")
		hostedZone = resp.HostedZone
	}

	logger = logger.WithField("id", aws.ToString(hostedZone.Id))
	logger.Debug("Fetching zone tags")
	existingTags, err := a.existingTags(hostedZone.Id)
	if err != nil {
		logger.WithError(err).Error("Failed to fetch zone tags")
		return err
	}

	a.hostedZone = hostedZone
	if err := a.modifyStatus(); err != nil {
		logger.WithError(err).Error("failed to populate DNSZone status")
		return err
	}
	a.currentHostedZoneTags = existingTags

	logger.Debug("Syncing zone tags")
	err = a.syncTags()
	if err != nil {
		// When an error occurs tagging the resource, we return an error. This will result in a retry of the create call.
		// Because we're using the DNSZone's UID as the CallerReference, the create should succeed without creating a duplicate
		// zone. We will then retry adding the tags.
		logger.WithError(err).Error("Failed to apply tags to newly created zone")
		return err
	}

	return err
}

func (a *AWSActuator) findZoneByCallerReference(domain, callerRef string) (*route53types.HostedZone, error) {
	logger := a.logger.WithField("domain", domain).WithField("callerRef", callerRef)
	logger.Debug("Searching for zone by domain and callerRef")
	var nextZoneID *string
	var nextName = aws.String(domain)
	for {
		logger.Debug("listing hosted zones by name")
		resp, err := a.awsClient.ListHostedZonesByName(&route53.ListHostedZonesByNameInput{
			DNSName:      nextName,
			HostedZoneId: nextZoneID,
			MaxItems:     aws.Int32(50),
		})
		if err != nil {
			logger.WithError(err).Error("cannot list zones by name")
			return nil, err
		}
		for _, zone := range resp.HostedZones {
			if aws.ToString(zone.CallerReference) == callerRef {
				logger.WithField("id", aws.ToString(zone.Id)).Debug("found hosted zone matching caller reference")
				return &zone, nil
			}
			if aws.ToString(zone.Name) != domain {
				logger.WithField("zone", aws.ToString(zone.Name)).Debug("reached zone with different domain name, aborting search")
				return nil, fmt.Errorf("hosted zone not found")
			}
		}
		if !resp.IsTruncated {
			logger.Debug("reached end of results, did not find hosted zone")
			return nil, fmt.Errorf("hosted zone not found")
		}
		nextZoneID = resp.NextHostedZoneId
		nextName = resp.NextDNSName
	}
}

// Delete removes an AWS Route53 hosted zone, typically because the DNSZone object is in a deleting state.
func (a *AWSActuator) Delete() error {
	if a.hostedZone == nil {
		return errors.New("hostedZone is unpopulated")
	}

	logger := a.logger.WithField("zone", a.dnsZone.Spec.Zone).WithField("id", aws.ToString(a.hostedZone.Id))

	logger.Info("Deleting route53 recordsets in hostedzone")
	if err := DeleteAWSRecordSets(a.awsClient, a.dnsZone, logger); err != nil {
		return err
	}

	logger.Info("Deleting route53 hostedzone")
	_, err := a.awsClient.DeleteHostedZone(&route53.DeleteHostedZoneInput{
		Id: a.hostedZone.Id,
	})
	if err != nil {
		logLevel := log.ErrorLevel
		if awsclient.ErrCodeEquals(err, "HostedZoneNotEmpty") {
			logLevel = log.InfoLevel
		}
		log.WithError(err).Log(logLevel, "Cannot delete hosted zone")
	}
	return err
}

// DeleteAWSRecordSets will clean up a DNS zone down to the minimum required record entries
func DeleteAWSRecordSets(awsClient awsclient.Client, dnsZone *hivev1.DNSZone, logger log.FieldLogger) error {

	listInput := &route53.ListResourceRecordSetsInput{
		HostedZoneId: dnsZone.Status.AWS.ZoneID,
		MaxItems:     aws.Int32(100),
	}
	for {
		listOutput, err := awsClient.ListResourceRecordSets(listInput)
		if err != nil {
			return err
		}
		var changes []route53types.Change
		for _, recordSet := range listOutput.ResourceRecordSets {
			// Ignore the 2 recordsets that are created with the hosted zone and that cannot be deleted
			if n, t := aws.ToString(recordSet.Name), recordSet.Type; n == controllerutils.Dotted(dnsZone.Spec.Zone) && (t == route53types.RRTypeNs || t == route53types.RRTypeSoa) {
				continue
			}

			logger.WithField("name", aws.ToString(recordSet.Name)).WithField("type", recordSet.Type).Info("recordset set for deletion")
			changes = append(changes, route53types.Change{
				Action:            route53types.ChangeActionDelete,
				ResourceRecordSet: &recordSet,
			})
		}
		if len(changes) > 0 {
			logger.WithField("count", len(changes)).Info("deleting recordsets")
			if _, err := awsClient.ChangeResourceRecordSets(&route53.ChangeResourceRecordSetsInput{
				ChangeBatch:  &route53types.ChangeBatch{Changes: changes},
				HostedZoneId: dnsZone.Status.AWS.ZoneID,
			}); err != nil {
				return err
			}
		}
		if !listOutput.IsTruncated {
			break
		}
		listInput.StartRecordIdentifier = listOutput.NextRecordIdentifier
		listInput.StartRecordName = listOutput.NextRecordName
		listInput.StartRecordType = listOutput.NextRecordType
	}
	return nil

}

// GetNameServers returns the nameservers listed in the route53 hosted zone NS record.
func (a *AWSActuator) GetNameServers() ([]string, error) {
	if a.hostedZone == nil {
		return nil, errors.New("hostedZone is unpopulated")
	}

	logger := a.logger.WithField("id", a.hostedZone.Id)
	logger.Info("Listing hosted zone NS records")
	resp, err := a.awsClient.ListResourceRecordSets(&route53.ListResourceRecordSetsInput{
		HostedZoneId:    aws.String(*a.hostedZone.Id),
		StartRecordType: route53types.RRTypeNs,
		StartRecordName: aws.String(a.dnsZone.Spec.Zone),
		MaxItems:        aws.Int32(1),
	})
	if err != nil {
		logger.WithError(err).Error("Error listing recordsets for zone")
		return nil, err
	}
	if len(resp.ResourceRecordSets) != 1 {
		msg := fmt.Sprintf("unexpected number of recordsets returned: %d", len(resp.ResourceRecordSets))
		logger.Error(msg)
		return nil, errors.New(msg)
	}
	if resp.ResourceRecordSets[0].Type != route53types.RRTypeNs {
		msg := "name server record not found"
		logger.Error(msg)
		return nil, errors.New(msg)
	}
	if aws.ToString(resp.ResourceRecordSets[0].Name) != (a.dnsZone.Spec.Zone + ".") {
		msg := fmt.Sprintf("name server record not found for domain %s", a.dnsZone.Spec.Zone)
		logger.Error(msg)
		return nil, errors.New(msg)
	}
	result := make([]string, len(resp.ResourceRecordSets[0].ResourceRecords))
	for i, record := range resp.ResourceRecordSets[0].ResourceRecords {
		result[i] = aws.ToString(record.Value)
	}
	logger.WithField("nameservers", result).Info("found hosted zone name servers")
	return result, nil
}

// Exists determines if the route53 hosted zone corresponding to the DNSZone exists
func (a *AWSActuator) Exists() (bool, error) {
	return a.hostedZone != nil, nil
}

func (a *AWSActuator) setInsufficientCredentialsConditionToFalse() bool {
	accessDeniedConds, accessDeniedCondsChanged := controllerutils.SetDNSZoneConditionWithChangeCheck(
		a.dnsZone.Status.Conditions,
		hivev1.InsufficientCredentialsCondition,
		corev1.ConditionFalse,
		accessGrantedReason,
		"credentials are valid",
		controllerutils.UpdateConditionNever,
	)
	if accessDeniedCondsChanged {
		a.dnsZone.Status.Conditions = accessDeniedConds
	}

	return accessDeniedCondsChanged
}

func (a *AWSActuator) setInsufficientCredentialsConditionToTrue(err error) bool {
	accessDeniedConds, accessDeniedCondsChanged := controllerutils.SetDNSZoneConditionWithChangeCheck(
		a.dnsZone.Status.Conditions,
		hivev1.InsufficientCredentialsCondition,
		corev1.ConditionTrue,
		accessDeniedReason,
		controllerutils.ErrorScrub(err),
		controllerutils.UpdateConditionIfReasonOrMessageChange,
	)

	if accessDeniedCondsChanged {
		// Conditions have changed. Update them in the object.
		a.dnsZone.Status.Conditions = accessDeniedConds
	}
	return accessDeniedCondsChanged
}

func (a *AWSActuator) setAPIOptInRequiredConditionToFalse() bool {
	apiOptInConds, apiOptInCondsChanged := controllerutils.SetDNSZoneConditionWithChangeCheck(
		a.dnsZone.Status.Conditions,
		hivev1.APIOptInRequiredCondition,
		corev1.ConditionFalse,
		apiOptInNotRequiredReason,
		"route53 apis are enabled",
		controllerutils.UpdateConditionNever,
	)
	if apiOptInCondsChanged {
		a.dnsZone.Status.Conditions = apiOptInConds
	}

	return apiOptInCondsChanged
}

func (a *AWSActuator) setAPIOptInRequiredConditionToTrue(err error) bool {
	apiOptInConds, apiOptInCondsChanged := controllerutils.SetDNSZoneConditionWithChangeCheck(
		a.dnsZone.Status.Conditions,
		hivev1.APIOptInRequiredCondition,
		corev1.ConditionTrue,
		apiOptInRequiredReason,
		controllerutils.ErrorScrub(err),
		controllerutils.UpdateConditionIfReasonOrMessageChange,
	)

	if apiOptInCondsChanged {
		// Conditions have changed. Update them in the object.
		a.dnsZone.Status.Conditions = apiOptInConds
	}
	return apiOptInCondsChanged
}

func (a *AWSActuator) setAuthenticationFailureConditionToFalse() bool {
	authenticationFailureConds, authenticationFailureCondsChanged := controllerutils.SetDNSZoneConditionWithChangeCheck(
		a.dnsZone.Status.Conditions,
		hivev1.AuthenticationFailureCondition,
		corev1.ConditionFalse,
		authenticationSucceededReason,
		"credentials authenticated",
		controllerutils.UpdateConditionNever,
	)
	if authenticationFailureCondsChanged {
		a.dnsZone.Status.Conditions = authenticationFailureConds
	}

	return authenticationFailureCondsChanged
}

func (a *AWSActuator) setAuthenticationFailureConditionToTrue(err error) bool {
	var authenticationFailureConds []hivev1.DNSZoneCondition
	authenticationFailureConds, authenticationFailureCondsChanged := controllerutils.SetDNSZoneConditionWithChangeCheck(
		a.dnsZone.Status.Conditions,
		hivev1.AuthenticationFailureCondition,
		corev1.ConditionTrue,
		authenticationFailedReason,
		controllerutils.ErrorScrub(err),
		controllerutils.UpdateConditionIfReasonOrMessageChange,
	)

	if authenticationFailureCondsChanged {
		// Conditions have changed. Update them in the object.
		a.dnsZone.Status.Conditions = authenticationFailureConds
	}

	return authenticationFailureCondsChanged
}

func (a *AWSActuator) setCloudErrorsConditionToFalse() bool {
	cloudErrorsConds, cloudErrorsCondsChanged := controllerutils.SetDNSZoneConditionWithChangeCheck(
		a.dnsZone.Status.Conditions,
		hivev1.GenericDNSErrorsCondition,
		corev1.ConditionFalse,
		dnsNoErrorReason,
		"No errors occurred",
		controllerutils.UpdateConditionNever,
	)
	if cloudErrorsCondsChanged {
		a.dnsZone.Status.Conditions = cloudErrorsConds
	}

	return cloudErrorsCondsChanged
}

func (a *AWSActuator) setCloudErrorsConditionToTrue(err error) bool {
	var cloudErrorsConds []hivev1.DNSZoneCondition
	cloudErrorsConds, cloudErrorsCondsChanged := controllerutils.SetDNSZoneConditionWithChangeCheck(
		a.dnsZone.Status.Conditions,
		hivev1.GenericDNSErrorsCondition,
		corev1.ConditionTrue,
		dnsCloudErrorReason,
		controllerutils.ErrorScrub(err),
		controllerutils.UpdateConditionIfReasonOrMessageChange,
	)

	if cloudErrorsCondsChanged {
		// Conditions have changed. Update them in the object.
		a.dnsZone.Status.Conditions = cloudErrorsConds
	}

	return cloudErrorsCondsChanged
}

// SetConditionsForError sets conditions on the dnszone given a specific error. Returns true if conditions changed.
func (a *AWSActuator) SetConditionsForError(err error) bool {
	accessDeniedCondsChanged := false
	authenticationFailureCondsChanged := false
	apiOptInCondsChanged := false
	cloudErrorsCondsChanged := false

	if err == nil {
		cloudErrorsCondsChanged = a.setCloudErrorsConditionToFalse()
		accessDeniedCondsChanged = a.setInsufficientCredentialsConditionToFalse()
		authenticationFailureCondsChanged = a.setAuthenticationFailureConditionToFalse()
		apiOptInCondsChanged = a.setAPIOptInRequiredConditionToFalse()
		return accessDeniedCondsChanged || authenticationFailureCondsChanged || cloudErrorsCondsChanged || apiOptInCondsChanged
	}

	// handle AWS vs non-AWS specific errors
	var awsErr smithy.APIError
	ok := errors.As(err, &awsErr)

	// non-AWS err
	if !ok {
		accessDeniedCondsChanged = a.setInsufficientCredentialsConditionToFalse()
		authenticationFailureCondsChanged = a.setAuthenticationFailureConditionToFalse()
		cloudErrorsCondsChanged = a.setCloudErrorsConditionToTrue(err)
		return accessDeniedCondsChanged || authenticationFailureCondsChanged || cloudErrorsCondsChanged
	}

	// AWS err condition handling
	aec := awsErr.ErrorCode()
	if aec == "AccessDeniedException" || aec == "AccessDenied" {
		accessDeniedCondsChanged = a.setInsufficientCredentialsConditionToTrue(awsErr)
	} else {
		accessDeniedCondsChanged = a.setInsufficientCredentialsConditionToFalse()
	}

	if aec == "InvalidSignatureException" ||
		aec == "UnrecognizedClientException" {
		authenticationFailureCondsChanged = a.setAuthenticationFailureConditionToTrue(awsErr)
	} else {
		authenticationFailureCondsChanged = a.setAuthenticationFailureConditionToFalse()
	}

	if aec == "OptInRequired" {
		apiOptInCondsChanged = a.setAPIOptInRequiredConditionToTrue(awsErr)
	} else {
		apiOptInCondsChanged = a.setAPIOptInRequiredConditionToFalse()
	}

	if !(accessDeniedCondsChanged || authenticationFailureCondsChanged || apiOptInCondsChanged) {
		cloudErrorsCondsChanged = a.setCloudErrorsConditionToTrue(err)
	}

	return accessDeniedCondsChanged || authenticationFailureCondsChanged || apiOptInCondsChanged || cloudErrorsCondsChanged
}

func tagEquals(a, b route53types.Tag) bool {
	return aws.ToString(a.Key) == aws.ToString(b.Key) &&
		aws.ToString(a.Value) == aws.ToString(b.Value)
}

func tagString(tag route53types.Tag) string {
	return fmt.Sprintf("%s=%s", aws.ToString(tag.Key), aws.ToString(tag.Value))
}

func tagsString(tags []route53types.Tag) string {
	return strings.Join(func() []string {
		result := []string{}
		for _, tag := range tags {
			result = append(result, tagString(tag))
		}
		return result
	}(), ",")
}
