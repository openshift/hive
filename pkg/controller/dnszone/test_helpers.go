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
	"testing"
	"time"

	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakekubeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/golang/mock/gomock"
	mockaws "github.com/openshift/hive/pkg/awsclient/mock"
	"github.com/stretchr/testify/assert"
)

var (
	kubeTimeNow = func() *metav1.Time {
		t := metav1.NewTime(time.Now())
		return &t
	}()

	validDNSZone = func() *hivev1.DNSZone {
		return &hivev1.DNSZone{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "dnszoneobject",
				Namespace:  "ns",
				Generation: 6,
				Finalizers: []string{hivev1.FinalizerDNSZone},
				UID:        types.UID("abcdef"),
			},
			Spec: hivev1.DNSZoneSpec{
				Zone: "blah.example.com",
				AWS: &hivev1.AWSDNSZoneSpec{
					AccountSecret: corev1.LocalObjectReference{
						Name: "some secret",
					},
					Region: "us-east-1",
					AdditionalTags: []hivev1.AWSResourceTag{
						{
							Key:   "foo",
							Value: "bar",
						},
					},
				},
			},
			Status: hivev1.DNSZoneStatus{
				AWS: &hivev1.AWSDNSZoneStatus{
					ZoneID: aws.String("1234"),
				},
			},
		}
	}

	validDNSEndpoint = func() *hivev1.DNSEndpoint {
		ep := &hivev1.DNSEndpoint{}
		ep.Namespace = "ns"
		ep.Name = "dnszoneobject-ns"
		return ep
	}

	validDNSZoneWithLinkToParent = func() *hivev1.DNSZone {
		zone := validDNSZone()
		zone.Spec.LinkToParentDomain = true
		return zone
	}

	validDNSZoneWithoutFinalizer = func() *hivev1.DNSZone {
		zone := validDNSZone()
		zone.Finalizers = []string{}
		return zone
	}

	validDNSZoneWithoutID = func() *hivev1.DNSZone {
		zone := validDNSZone()
		zone.Status.AWS = nil
		return zone
	}

	validDNSZoneWithAdditionalTags = func() *hivev1.DNSZone {
		zone := validDNSZone()
		zone.Spec.AWS.AdditionalTags = append(zone.Spec.AWS.AdditionalTags, []hivev1.AWSResourceTag{
			{
				Key:   "foo1",
				Value: "bar1",
			},
			{
				Key:   "foo2",
				Value: "bar2",
			},
		}...)
		return zone
	}

	validDNSZoneBeingDeleted = func() *hivev1.DNSZone {
		// Take a copy of the default validDNSZone object
		zone := validDNSZone()

		// And make the 1 change needed to signal the object is being deleted.
		zone.DeletionTimestamp = kubeTimeNow
		return zone
	}

	validDNSZoneGensDontMatch = func() *hivev1.DNSZone {
		// Take a copy of the default validDNSZone object
		zone := validDNSZone()

		// And make the 1 change needed to signal the object has not been sync'd
		zone.Status.LastSyncGeneration = 5
		tmpTime := metav1.Now() // LastSyncTimestamp needs to be set so that the generation difference is meaningful in the "shouldSync" check.
		zone.Status.LastSyncTimestamp = &tmpTime
		return zone
	}

	validDNSZoneGenTimestampSyncNotNeeded = func() *hivev1.DNSZone {
		// Take a copy of the default validDNSZone object
		zone := validDNSZone()

		// And make the 1 change needed to signal the object has not been sync'd
		zone.Status.LastSyncGeneration = 6
		tmpTime := metav1.NewTime(time.Now().Add(-2 * time.Minute)) // Set the time to 2 minutes ago, which should NOT cause a sync
		zone.Status.LastSyncTimestamp = &tmpTime

		return zone
	}

	validDNSZoneGenTimestampSyncNeeded = func() *hivev1.DNSZone {
		// Take a copy of the default validDNSZone object
		zone := validDNSZone()

		// And make the 1 change needed to signal the object has not been sync'd
		zone.Status.LastSyncGeneration = 6
		tmpTime := metav1.NewTime(time.Now().AddDate(0, 0, -3)) // Set the date to 3 days ago, which should cause a sync
		zone.Status.LastSyncTimestamp = &tmpTime

		return zone
	}

	validRoute53HostedZone = func() *route53.HostedZone {
		return &route53.HostedZone{
			Name: aws.String(validDNSZone().Spec.Zone + "."), // hosted zones always come back with a period on the end.
		}
	}
)

type mocks struct {
	fakeKubeClient client.Client
	mockCtrl       *gomock.Controller
	mockAWSClient  *mockaws.MockClient
}

// assertErrorNilOrMessage allows for comparing an error against a string.
// if string == "" the error must equal nil.
func assertErrorNilOrMessage(t *testing.T, theError error, errString string) {
	if errString == "" {
		assert.Nil(t, theError)
	} else {
		assert.EqualError(t, theError, errString)
	}
}

// setupDefaultMocks is an easy way to setup all of the default mocks
func setupDefaultMocks(t *testing.T) *mocks {
	mocks := &mocks{
		fakeKubeClient: fakekubeclient.NewFakeClient(),
		mockCtrl:       gomock.NewController(t),
	}

	mocks.mockAWSClient = mockaws.NewMockClient(mocks.mockCtrl)

	return mocks
}

// setFakeDNSZoneInKube is an easy way to register a dns zone object with kube.
func setFakeDNSZoneInKube(mocks *mocks, dnsZone *hivev1.DNSZone) error {
	return mocks.fakeKubeClient.Create(context.TODO(), dnsZone)
}

// setFakeDNSEndpointInKube creates a fake DNSEndpoint
func setFakeDNSEndpointInKube(mocks *mocks, endpoint *hivev1.DNSEndpoint) error {
	return mocks.fakeKubeClient.Create(context.TODO(), endpoint)
}

// inTimeSpan says if a given time is withing the start and end times.
func inTimeSpan(timeToCheck *metav1.Time, start, end time.Time) bool {
	if timeToCheck == nil {
		return false
	}

	return timeToCheck.Time.After(start) && timeToCheck.Time.Before(end)
}
