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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestNewZoneReconciler tests that a new ZoneReconciler object can be created.
func TestNewZoneReconciler(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	cases := []struct {
		name              string
		expectedErrString string
		dnsZone           *hivev1.DNSZone
	}{
		{
			name:    "Successfully create new zone",
			dnsZone: validDNSZone,
		},
		{
			name:              "Fail to create new zone because desiredState not set",
			expectedErrString: "ZoneReconciler requires desiredState to be set",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			mocks := setupDefaultMocks(t)
			expectedZoneReconciler := &ZoneReconciler{
				desiredState: tc.dnsZone,
				kubeClient:   mocks.fakeKubeClient,
				logger:       log.WithField("controller", controllerName),
				awsClient:    mocks.mockAWSClient,
			}

			// Act
			zr, err := NewZoneReconciler(
				expectedZoneReconciler.desiredState,
				expectedZoneReconciler.kubeClient,
				expectedZoneReconciler.logger,
				expectedZoneReconciler.awsClient,
			)

			// Assert
			assertErrorNilOrMessage(t, err, tc.expectedErrString)
			if tc.expectedErrString == "" {
				// Only check this if we're expected to succeed.
				assert.Equal(t, expectedZoneReconciler, zr)
			}
		})
	}
}

// TestReconcile tests that ZoneReconciler.Reconcile reacts properly under different reconciliation states.
func TestReconcile(t *testing.T) {
	cases := []struct {
		name                       string
		expectedErrString          string
		dnsZone                    *hivev1.DNSZone
		expectHostedZoneCreation   bool
		expectHostedZoneDeletion   bool
		listHostedZonesOutput      *route53.ListHostedZonesOutput
		listHostedZonesError       error
		createHostedZonesError     error
		deleteHostedZonesError     error
		expectedLastSyncGeneration int64
		LastSyncTimestampCheck     func(timeToCheck *metav1.Time, start, end time.Time) bool
	}{
		{
			name: "DNSZone found, No corresponding route53 hostedzone",
			expectHostedZoneCreation: true,
			dnsZone:                  validDNSZone.DeepCopy(),
			listHostedZonesOutput: &route53.ListHostedZonesOutput{
				HostedZones: []*route53.HostedZone{
					{
						Name: aws.String("not.the.right.zone."),
					},
				},
			},
			expectedLastSyncGeneration: 6,
			LastSyncTimestampCheck:     inTimeSpan,
		},
		{
			name:    "DNSZone found, corresponding route53 hostedzone exists",
			dnsZone: validDNSZone.DeepCopy(),
			listHostedZonesOutput: &route53.ListHostedZonesOutput{
				HostedZones: []*route53.HostedZone{
					validRoute53HostedZone,
				},
			},
			expectedLastSyncGeneration: 6,
			LastSyncTimestampCheck:     inTimeSpan,
		},
		{
			name:                     "DNSZone found, in deleting state",
			dnsZone:                  validDNSZoneBeingDeleted.DeepCopy(),
			expectHostedZoneDeletion: true,
			listHostedZonesOutput: &route53.ListHostedZonesOutput{
				HostedZones: []*route53.HostedZone{
					validRoute53HostedZone,
				},
			},
			expectedLastSyncGeneration: 6,
			LastSyncTimestampCheck:     inTimeSpan,
		},
		{
			name:    "DNSZone found, aws.ListHostedZone errors",
			dnsZone: validDNSZone.DeepCopy(),
			listHostedZonesOutput: &route53.ListHostedZonesOutput{
				HostedZones: []*route53.HostedZone{
					validRoute53HostedZone,
				},
			},
			listHostedZonesError:       errors.NewBadRequest("shame on you"),
			expectedErrString:          "shame on you",
			expectedLastSyncGeneration: 0, // LastSyncGeneration not updated since the sync failed.
			LastSyncTimestampCheck:     func(timeToCheck *metav1.Time, start, end time.Time) bool { return timeToCheck == nil },
		},
		{
			name:    "DNSZone found, aws.CreateHostedZone errors",
			dnsZone: validDNSZone.DeepCopy(),
			listHostedZonesOutput: &route53.ListHostedZonesOutput{
				HostedZones: []*route53.HostedZone{},
			},
			expectHostedZoneCreation:   true,
			createHostedZonesError:     errors.NewBadRequest("shame on you"),
			expectedErrString:          "shame on you",
			expectedLastSyncGeneration: 0, // LastSyncGeneration not updated since the sync failed.
			LastSyncTimestampCheck:     func(timeToCheck *metav1.Time, start, end time.Time) bool { return timeToCheck == nil },
		},
		{
			name:    "DNSZone found, aws.DeleteHostedZone errors",
			dnsZone: validDNSZoneBeingDeleted.DeepCopy(),
			listHostedZonesOutput: &route53.ListHostedZonesOutput{
				HostedZones: []*route53.HostedZone{
					validRoute53HostedZone,
				},
			},
			expectHostedZoneDeletion:   true,
			deleteHostedZonesError:     errors.NewBadRequest("shame on you"),
			expectedErrString:          "shame on you",
			expectedLastSyncGeneration: 0, // LastSyncGeneration not updated since the sync failed.
			LastSyncTimestampCheck:     func(timeToCheck *metav1.Time, start, end time.Time) bool { return timeToCheck == nil },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			mocks := setupDefaultMocks(t)

			zr, _ := NewZoneReconciler(
				tc.dnsZone,
				mocks.fakeKubeClient,
				log.WithField("controller", controllerName),
				mocks.mockAWSClient,
			)

			// This is necessary for the mocks to report failures like methods not being called an expected number of times.
			defer mocks.mockCtrl.Finish()

			setFakeDNSZoneInKube(mocks, tc.dnsZone)

			listHostedZonesMock := mocks.mockAWSClient.EXPECT().
				ListHostedZones(gomock.Any()).
				Return(tc.listHostedZonesOutput, tc.listHostedZonesError).
				Times(1)

			if tc.expectHostedZoneCreation {
				mocks.mockAWSClient.EXPECT().
					CreateHostedZone(gomock.Any()).
					Return(nil, tc.createHostedZonesError).
					Times(1).
					After(listHostedZonesMock)
			}

			if tc.expectHostedZoneDeletion {
				mocks.mockAWSClient.EXPECT().
					DeleteHostedZone(gomock.Any()).
					Return(nil, tc.deleteHostedZonesError).
					Times(1).
					After(listHostedZonesMock)
			}

			// Act
			err := zr.Reconcile()

			// Assert
			assertErrorNilOrMessage(t, err, tc.expectedErrString)

			// Ensure the sync generation and timeStamp are being set properly given each test case circumstance.
			assert.Equal(t, tc.expectedLastSyncGeneration, zr.desiredState.Status.LastSyncGeneration)
			assert.True(t, tc.LastSyncTimestampCheck(zr.desiredState.Status.LastSyncTimestamp, time.Now().Add(-5*time.Minute), time.Now().Add(5*time.Minute)))
		})
	}
}
