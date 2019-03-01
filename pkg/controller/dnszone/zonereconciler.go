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
	"time"

	"github.com/aws/aws-sdk-go/service/route53"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	awsclient "github.com/openshift/hive/pkg/awsclient"

	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aws/aws-sdk-go/aws"
)

// ZoneReconciler manages getting the desired state, getting the current state and reconciling the two.
type ZoneReconciler struct {
	// desiredState is the kube object that represents the desired state
	desiredState *hivev1.DNSZone

	logger log.FieldLogger

	// kubeClient is a kubernetes client to access general cluster / project related objects.
	kubeClient client.Client

	// awsClient is a utility for making it easy for controllers to interface with AWS
	awsClient awsclient.Client
}

// NewZoneReconciler creates a new ZoneReconciler object. A new ZoneReconciler is expected to be created for each controller sync.
func NewZoneReconciler(
	desiredState *hivev1.DNSZone,
	kubeClient client.Client,
	logger log.FieldLogger,
	awsClient awsclient.Client,
) (*ZoneReconciler, error) {
	if desiredState == nil {
		return nil, fmt.Errorf("ZoneReconciler requires desiredState to be set")
	}

	zoneReconciler := &ZoneReconciler{
		desiredState: desiredState,
		kubeClient:   kubeClient,
		logger:       logger,
		awsClient:    awsClient,
	}

	return zoneReconciler, nil
}

// Reconcile attempts to make the current state reflect the desired state. It does this idempotently.
func (zr *ZoneReconciler) Reconcile() error {
	currentState, err := zr.getCurrentState()
	if err != nil {
		return err
	}

	// Deletion case
	if zr.desiredState.DeletionTimestamp != nil {
		return zr.deleteRoute53HostedZone(currentState)
	}

	// Creation case
	if currentState == nil {
		return zr.createRoute53HostedZone()
	}

	// Update case
	//    NOTE: Since we're only tracking the dns zone, update case is not necessary right now.
	//              In the future, we may need this if we start syncing things like "comment"

	zr.logger.Debugf("Route53 hostedzone matches desired state, no action taken: %v", zr.desiredState.Spec.Zone)
	zr.addRateLimitingStatusEntries()
	return nil
}

// getCurrentState gets the AWS object for the zone.
// If ZoneReconciler.currentState is not nil, an error will be returned.
func (zr *ZoneReconciler) getCurrentState() (*route53.HostedZone, error) {
	output, err := zr.awsClient.ListHostedZones(&route53.ListHostedZonesInput{})
	if err != nil {
		return nil, err
	}

	for _, hostedzone := range output.HostedZones {
		// Find our specific hostedzone
		cmpStr := zr.desiredState.Spec.Zone

		if string(cmpStr[len(cmpStr)-1]) != "." {
			cmpStr = cmpStr + "."
		}

		if strings.EqualFold(cmpStr, *hostedzone.Name) {
			zr.logger.Infof("Route53 hostedzone found for: %v", zr.desiredState.Spec.Zone)
			return hostedzone, nil
		}
	}

	zr.logger.Debugf("Route53 hostedzone doesn't exist: %v", zr.desiredState.Spec.Zone)

	// It is ok to reach here. This means the current state couldn't be found.
	return nil, nil
}

// createRoute53HostedZone creates an AWS Route53 hosted zone given the desired state
func (zr *ZoneReconciler) createRoute53HostedZone() error {
	zr.logger.Infof("Creating route53 hostedzone: %v", zr.desiredState.Spec.Zone)
	_, err := zr.awsClient.CreateHostedZone(&route53.CreateHostedZoneInput{
		Name:            aws.String(zr.desiredState.Spec.Zone),
		CallerReference: aws.String(time.Now().String()), // A timestamp is what is suggested by Amazon (https://tinyurl.com/yd74xjwx)
	})
	if err != nil {
		return err
	}

	zr.addRateLimitingStatusEntries()

	// Only add a finalizer after a route53 hostedzone was successfully created (nothing to clean up otherwise).
	return zr.addDNSZoneFinalizer(zr.desiredState)
}

func (zr *ZoneReconciler) addDNSZoneFinalizer(dnsZone *hivev1.DNSZone) error {
	dnsZone = dnsZone.DeepCopy()

	finalizers := sets.NewString(dnsZone.GetFinalizers()...)
	finalizers.Insert(hivev1.FinalizerDNSZone)
	dnsZone.SetFinalizers(finalizers.List())

	return zr.kubeClient.Update(context.TODO(), dnsZone)
}

// deleteRoute53HostedZone deletes an AWS Route53 hosted zone, typically because the desired state is in a deleting state.
func (zr *ZoneReconciler) deleteRoute53HostedZone(currentState *route53.HostedZone) error {
	// Check if there is still something in AWS for us to delete
	if currentState != nil {
		zr.logger.Infof("Deleting route53 hostedzone: %v", zr.desiredState.Spec.Zone)
		_, err := zr.awsClient.DeleteHostedZone(&route53.DeleteHostedZoneInput{
			Id: currentState.Id,
		})

		if err != nil {
			return err
		}
	}

	zr.addRateLimitingStatusEntries()

	// Only reomve the finalizer after the route53 hostedzone was successfully deleted (still need to clean up otherwise).
	return zr.removeDNSZoneFinalizer(zr.desiredState)
}

func (zr *ZoneReconciler) removeDNSZoneFinalizer(dnsZone *hivev1.DNSZone) error {
	dnsZone = dnsZone.DeepCopy()

	finalizers := sets.NewString(dnsZone.GetFinalizers()...)
	finalizers.Delete(hivev1.FinalizerDNSZone)
	dnsZone.SetFinalizers(finalizers.List())

	return zr.kubeClient.Update(context.TODO(), dnsZone)
}

// addRateLimitingStatusEntries adds the status entries specific to the AWS rate limiting that we do to abuse the AWS API.
func (zr *ZoneReconciler) addRateLimitingStatusEntries() {
	// We need to keep track of the last object generation and time we sync'd on.
	// This is used to rate limit our calls to AWS.
	zr.desiredState.Status.LastSyncGeneration = zr.desiredState.ObjectMeta.Generation
	tmpTime := metav1.Now()
	zr.desiredState.Status.LastSyncTimestamp = &tmpTime
}
