/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package dnszone

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go/service/route53"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	awsclient "github.com/openshift/hive/pkg/awsclient"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"

	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	hiveDNSZoneTag = "hive.openshift.io/dnszone"
)

// ZoneReconciler manages getting the desired state, getting the current state and reconciling the two.
type ZoneReconciler struct {
	// dnsZone is the kube object that represents the desired state
	dnsZone *hivev1.DNSZone

	logger log.FieldLogger

	// kubeClient is a kubernetes client to access general cluster / project related objects.
	kubeClient client.Client

	// awsClient is a utility for making it easy for controllers to interface with AWS
	awsClient awsclient.Client
}

// NewZoneReconciler creates a new ZoneReconciler object. A new ZoneReconciler is expected to be created for each controller sync.
func NewZoneReconciler(
	dnsZone *hivev1.DNSZone,
	kubeClient client.Client,
	logger log.FieldLogger,
	awsClient awsclient.Client,
) (*ZoneReconciler, error) {
	if dnsZone == nil {
		return nil, fmt.Errorf("ZoneReconciler requires dnsZone to be set")
	}
	zoneReconciler := &ZoneReconciler{
		dnsZone:    dnsZone,
		kubeClient: kubeClient,
		logger:     logger,
		awsClient:  awsClient,
	}

	return zoneReconciler, nil
}

// Reconcile attempts to make the current state reflect the desired state. It does this idempotently.
func (zr *ZoneReconciler) Reconcile() error {
	zr.logger.Debug("Retrieving current state")
	hostedZone, tags, err := zr.getCurrentState()
	if err != nil {
		zr.logger.WithError(err).Error("Failed to retrieve hosted zone and corresponding tags")
		return err
	}
	if zr.dnsZone.DeletionTimestamp != nil {
		if hostedZone != nil {
			zr.logger.Debug("DNSZone resource is deleted, deleting hosted zone")
			err = zr.deleteRoute53HostedZone(hostedZone)
			if err != nil {
				zr.logger.WithError(err).Error("Failed to delete hosted zone")
				return err
			}
		}
		// Remove the finalizer from the DNSZone. It will be persisted when we persist status
		zr.logger.Debug("Removing DNSZone finalizer")
		controllerutils.DeleteFinalizer(zr.dnsZone, hivev1.FinalizerDNSZone)
		err := zr.kubeClient.Update(context.TODO(), zr.dnsZone)
		if err != nil {
			zr.logger.WithError(err).Error("Failed to remove DNSZone finalizer")
		}
		return err
	}
	if !controllerutils.HasFinalizer(zr.dnsZone, hivev1.FinalizerDNSZone) {
		zr.logger.Debug("DNSZone does not have a finalizer. Adding one.")
		controllerutils.AddFinalizer(zr.dnsZone, hivev1.FinalizerDNSZone)
		err := zr.kubeClient.Update(context.TODO(), zr.dnsZone)
		if err != nil {
			zr.logger.WithError(err).Error("Failed to add finalizer to DNSZone")
		}
		return err
	}
	if hostedZone == nil {
		zr.logger.Debug("No corresponding hosted zone found on cloud provider, creating one")
		hostedZone, err = zr.createRoute53HostedZone()
		if err != nil {
			zr.logger.WithError(err).Error("Failed to create hosted zone")
			return err
		}
	} else {
		zr.logger.Debug("Existing hosted zone found. Syncing with DNSZone resource")
		// For now, tags are the only things we can sync with existing zones.
		err = zr.syncTags(hostedZone.Id, tags)
		if err != nil {
			zr.logger.WithError(err).Error("failed to sync tags for hosted zone")
			return err
		}
	}
	nameServers, err := zr.getHostedZoneNSRecord(*hostedZone.Id)
	if err != nil {
		zr.logger.WithError(err).Error("Failed to get hosted zone name servers")
		return err
	}
	return zr.updateStatus(hostedZone, nameServers)
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
	var nextName *string = aws.String(domain)
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

func (zr *ZoneReconciler) updateStatus(hostedZone *route53.HostedZone, nameServers []string) error {
	zr.logger.Debug("Updating DNSZone status")

	zr.dnsZone.Status.NameServers = nameServers
	zr.dnsZone.Status.AWS = &hivev1.AWSDNSZoneStatus{
		ZoneID: hostedZone.Id,
	}
	zr.addRateLimitingStatusEntries()
	err := zr.kubeClient.Status().Update(context.TODO(), zr.dnsZone)
	if err != nil {
		zr.logger.WithError(err).Error("Cannot update DNSZone status")
	}
	return err
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

// addRateLimitingStatusEntries adds the status entries specific to the AWS rate limiting that we do to abuse the AWS API.
func (zr *ZoneReconciler) addRateLimitingStatusEntries() {
	zr.logger.Debug("Adding rate limiting status entries to DNSZone")
	// We need to keep track of the last object generation and time we sync'd on.
	// This is used to rate limit our calls to AWS.
	zr.dnsZone.Status.LastSyncGeneration = zr.dnsZone.ObjectMeta.Generation
	tmpTime := metav1.Now()
	zr.dnsZone.Status.LastSyncTimestamp = &tmpTime
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
