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
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	log "github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/golang/mock/gomock"
	awsclient "github.com/openshift/hive/pkg/awsclient"

	"github.com/openshift/hive/pkg/controller/dnszone"
)

type testContext struct {
	controller *dnszone.ReconcileDNSZone
	mocks      *mocks
}

// Sets up all of the mocks, informers, cache, etc for tests.
func setupTestContext(t *testing.T) *testContext {
	mocks := setupDefaultMocks(t)

	ctx := &testContext{
		mocks: mocks,
		controller: dnszone.NewReconcileDNSZone(
			mocks.fakeKubeClient,
			scheme.Scheme,
			log.WithField("controller", "dnszone"),
			func(client.Client, string, string, string) (awsclient.Client, error) {
				return mocks.mockAWSClient, nil
			},
		),
	}

	return ctx
}

func namespacedNameFromKey(key string) types.NamespacedName {
	splitString := strings.Split(key, "/")
	return types.NamespacedName{
		Name:      splitString[1],
		Namespace: splitString[0],
	}
}

func TestReconcileOldStyle(t *testing.T) {
	cases := []struct {
		name                     string
		key                      string
		expectedErrString        string
		dnsZone                  *hivev1.DNSZone
		expectHostedZoneCreation bool
		listHostedZonesOutput    *route53.ListHostedZonesOutput
		awsClientBuilderError    error
		listHostedZonesErr       error
	}{
		{
			name: "DNSZone not found, no errors",
			key:  "namespace/somezone",
		},
		{
			name: "DNSZone found, Create hostedzone as no corresponding route53 hostedzone exists",
			key:  validDNSZone.Namespace + "/" + validDNSZone.Name,
			expectHostedZoneCreation: true,
			dnsZone:                  validDNSZone.DeepCopy(),
			listHostedZonesOutput: &route53.ListHostedZonesOutput{
				HostedZones: []*route53.HostedZone{
					{
						Name: aws.String("not.the.right.zone."),
					},
				},
			},
		},
		{
			name: "DNSZone found, Status Generation doesn't match object generation (sync needed)",
			key:  validDNSZone.Namespace + "/" + validDNSZone.Name,
			expectHostedZoneCreation: true,
			dnsZone:                  validDNSZoneGensDontMatch.DeepCopy(),
			listHostedZonesOutput:    &route53.ListHostedZonesOutput{},
		},
		{
			name:                  "DNSZone found, In deleting state (sync needed)",
			key:                   validDNSZone.Namespace + "/" + validDNSZone.Name,
			dnsZone:               validDNSZoneBeingDeleted.DeepCopy(),
			listHostedZonesOutput: &route53.ListHostedZonesOutput{},
		},
		{
			name:    "DNSZone found, Status Generation Timestamp within sync period (no sync needed)",
			dnsZone: validDNSZoneGenTimestampSyncNotNeeded.DeepCopy(),
			key:     validDNSZone.Namespace + "/" + validDNSZone.Name,
		},
		{
			name:    "DNSZone found, Status Generation Timestamp outside sync period (sync needed)",
			dnsZone: validDNSZoneGenTimestampSyncNeeded.DeepCopy(),
			key:     validDNSZone.Namespace + "/" + validDNSZone.Name,
			expectHostedZoneCreation: true,
			listHostedZonesOutput:    &route53.ListHostedZonesOutput{},
		},
		{
			name:                  "Error creating Cluster Operator AWS Client",
			key:                   validDNSZone.Namespace + "/" + validDNSZone.Name,
			dnsZone:               validDNSZone.DeepCopy(),
			awsClientBuilderError: apierrors.NewBadRequest("shame on you"),
			expectedErrString:     "shame on you",
		},
		{
			name:                  "Error reconciling desired state",
			key:                   validDNSZone.Namespace + "/" + validDNSZone.Name,
			dnsZone:               validDNSZone.DeepCopy(),
			listHostedZonesOutput: &route53.ListHostedZonesOutput{},
			listHostedZonesErr:    apierrors.NewBadRequest("shame on you"),
			expectedErrString:     "shame on you",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			ctx := setupTestContext(t)

			// This is necessary for the mocks to report failures like methods not being called an expected number of times.
			defer ctx.mocks.mockCtrl.Finish()

			if tc.dnsZone != nil {
				setFakeDNSZoneInKube(ctx.mocks, tc.dnsZone)
				ctx.controller.SetAWSClientBuilder(func(client.Client, string, string, string) (awsclient.Client, error) {
					return ctx.mocks.mockAWSClient, tc.awsClientBuilderError
				})

				if tc.listHostedZonesOutput != nil {
					listHostedZonesMock := ctx.mocks.mockAWSClient.EXPECT().
						ListHostedZones(gomock.Any()).
						Return(tc.listHostedZonesOutput, tc.listHostedZonesErr).
						Times(1)

					if tc.expectHostedZoneCreation {
						ctx.mocks.mockAWSClient.EXPECT().
							CreateHostedZone(gomock.Any()).
							Return(nil, nil).
							Times(1).
							After(listHostedZonesMock)
					}
				}
			}

			// Act
			//err := ctx.controller.syncHandler(tc.key)
			_, err := ctx.controller.Reconcile(reconcile.Request{
				NamespacedName: namespacedNameFromKey(tc.key),
			})

			// Assert
			assertErrorNilOrMessage(t, err, tc.expectedErrString)
		})
	}

}
