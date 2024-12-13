package gcpactuator

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1gcp "github.com/openshift/hive/apis/hive/v1/gcp"
	"github.com/openshift/hive/pkg/controller/privatelink/actuator"
	mockclient "github.com/openshift/hive/pkg/gcpclient/mock"
	testassert "github.com/openshift/hive/pkg/test/assert"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	testfake "github.com/openshift/hive/pkg/test/fake"
	"github.com/openshift/hive/pkg/test/generic"
	testlogger "github.com/openshift/hive/pkg/test/logger"
	"github.com/openshift/hive/pkg/util/scheme"
)

const (
	testNameSpace         = "testnamespace"
	testClusterDeployment = "testclusterdeployment"
	testInfraID           = "testinfraid"
	testProjectName       = "testproject"
	testRegion            = "testregion"
)

var (
	mockNetwork = &compute.Network{
		Name:     "testnetwork",
		SelfLink: "networkselflink",
	}

	mockSubnet = &compute.Subnetwork{
		Name:     "testsubnet",
		Network:  mockNetwork.Name,
		Region:   testRegion,
		SelfLink: "testsubnetselflink",
	}

	mockAddress = &compute.Address{
		Name:       "testaddress",
		Address:    "testipaddress",
		Region:     testRegion,
		Subnetwork: mockSubnet.Name,
		SelfLink:   "testaddressselflink",
	}

	mockCredentialsSecretRef = corev1.LocalObjectReference{
		Name: "testCredentialSecretName",
	}

	mockFirewall = &compute.Firewall{
		SelfLink: "testfirewallselflink",
	}

	mockForwardingRule = &compute.ForwardingRule{
		Name:       "testforwardingrule",
		IPAddress:  mockAddress.Address,
		Region:     testRegion,
		Target:     "testtarget",
		Subnetwork: mockSubnet.Name,
		SelfLink:   "testforwardingruleselflink",
	}

	mockServiceAttachment = &compute.ServiceAttachment{
		Name:     "testserviceattachment",
		Region:   testRegion,
		SelfLink: "testserviceattachmentselflink",
	}
)

func newTestGCPLinkActuator(t *testing.T, config *hivev1.GCPPrivateServiceConnectConfig, cd *hivev1.ClusterDeployment, existing []runtime.Object, configureGCPClient func(*mockclient.MockClient)) (*GCPLinkActuator, error) {
	fakeClient := client.Client(testfake.NewFakeClientBuilder().
		WithRuntimeObjects(cd).
		WithRuntimeObjects(existing...).
		Build())

	mockedGCPClient := mockclient.NewMockClient(gomock.NewController(t))
	if configureGCPClient != nil {
		configureGCPClient(mockedGCPClient)
	}

	return &GCPLinkActuator{
		client:         &fakeClient,
		config:         config,
		gcpClientHub:   mockedGCPClient,
		gcpClientSpoke: mockedGCPClient,
	}, nil
}

func newGoogleApiError(code int, reason string, message string) error {
	err := errors.New(message)
	herr := &googleapi.Error{
		Code:    code,
		Message: message,
		Errors: []googleapi.ErrorItem{{
			Reason:  reason,
			Message: message,
		}},
	}
	errors.As(err, &herr)
	return herr
}

func Test_Cleanup(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		gcpClientConfig func(*mockclient.MockClient)

		expectError  string
		expectStatus *hivev1gcp.PrivateServiceConnectStatus
	}{{ // There should be an error on cleanupEndpoint failure
		name: "cleanupEndpoint failure",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{Endpoint: "testEndpoint"},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().DeleteForwardingRule(gomock.Any(), gomock.Any()).Return(newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to DeleteForwardingRule"))
		},
		expectError:  "error cleaning up endpoint: error deleting the Endpoint: googleapi: Error 401: not authorized to DeleteForwardingRule, AccessDenied",
		expectStatus: &hivev1gcp.PrivateServiceConnectStatus{Endpoint: "testEndpoint"},
	}, { // There should be an error on cleanupEndpointAddress failure
		name: "cleanupEndpointAddress failure",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{EndpointAddress: "testEndpointAddress"},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().DeleteForwardingRule(gomock.Any(), gomock.Any()).Return(nil)
			m.EXPECT().DeleteAddress(gomock.Any(), gomock.Any()).Return(newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to DeleteAddress"))
		},
		expectError:  "error cleaning up endpoint address: error deleting the Endpoint Address: googleapi: Error 401: not authorized to DeleteAddress, AccessDenied",
		expectStatus: &hivev1gcp.PrivateServiceConnectStatus{EndpointAddress: "testEndpointAddress"},
	}, { // There should be an error on cleanupServiceAttachment failure
		name: "cleanupServiceAttachment failure",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{ServiceAttachment: "testServiceAttachment"},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().DeleteForwardingRule(gomock.Any(), gomock.Any()).Return(nil)
			m.EXPECT().DeleteAddress(gomock.Any(), gomock.Any()).Return(nil)
			m.EXPECT().DeleteServiceAttachment(gomock.Any(), gomock.Any()).Return(newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to DeleteServiceAttachment"))
		},
		expectError:  "error cleaning up service attachment: error deleting the Service Attachment: googleapi: Error 401: not authorized to DeleteServiceAttachment, AccessDenied",
		expectStatus: &hivev1gcp.PrivateServiceConnectStatus{ServiceAttachment: "testServiceAttachment"},
	}, { // Should not attempt to remove firewall or subnet when using an existing subnet
		name: "existing subnet",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{
					ServiceAttachment: &hivev1gcp.ServiceAttachment{
						Subnet: &hivev1gcp.ServiceAttachmentSubnet{
							Existing: &hivev1gcp.ServiceAttachmentSubnetExisting{
								Name: "existingsubnet",
							},
						},
					},
				},
			}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{ServiceAttachmentFirewall: "testServiceAttachmentFirewall"},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().DeleteForwardingRule(gomock.Any(), gomock.Any()).Return(nil)
			m.EXPECT().DeleteAddress(gomock.Any(), gomock.Any()).Return(nil)
			m.EXPECT().DeleteServiceAttachment(gomock.Any(), gomock.Any()).Return(nil)
		},
		expectStatus: &hivev1gcp.PrivateServiceConnectStatus{ServiceAttachmentFirewall: "testServiceAttachmentFirewall"},
	}, { // There should be an error on cleanupServiceAttachmentFirewall failure
		name: "cleanupServiceAttachmentFirewall failure",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{ServiceAttachmentFirewall: "testServiceAttachmentFirewall"},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().DeleteForwardingRule(gomock.Any(), gomock.Any()).Return(nil)
			m.EXPECT().DeleteAddress(gomock.Any(), gomock.Any()).Return(nil)
			m.EXPECT().DeleteServiceAttachment(gomock.Any(), gomock.Any()).Return(nil)
			m.EXPECT().DeleteFirewall(gomock.Any()).Return(newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to DeleteFirewall"))
		},
		expectError:  "error cleaning up service attachment firewall: error deleting the Service Attachment Firewall: googleapi: Error 401: not authorized to DeleteFirewall, AccessDenied",
		expectStatus: &hivev1gcp.PrivateServiceConnectStatus{ServiceAttachmentFirewall: "testServiceAttachmentFirewall"},
	}, { // There should be an error on cleanupServiceAttachmentSubnet failure
		name: "cleanupServiceAttachmentSubnet failure",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{ServiceAttachmentSubnet: "testServiceAttachmentSubnet"},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().DeleteForwardingRule(gomock.Any(), gomock.Any()).Return(nil)
			m.EXPECT().DeleteAddress(gomock.Any(), gomock.Any()).Return(nil)
			m.EXPECT().DeleteServiceAttachment(gomock.Any(), gomock.Any()).Return(nil)
			m.EXPECT().DeleteFirewall(gomock.Any()).Return(nil)
			m.EXPECT().DeleteSubnet(gomock.Any(), gomock.Any()).Return(newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to DeleteSubnet"))
		},
		expectError:  "error cleaning up service attachment subnet: error deleting the Service Attachment Subnet: googleapi: Error 401: not authorized to DeleteSubnet, AccessDenied",
		expectStatus: &hivev1gcp.PrivateServiceConnectStatus{ServiceAttachmentSubnet: "testServiceAttachmentSubnet"},
	}, { // All of privateServiceConnect status should be cleared on Success
		name: "success",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{
					Endpoint:                  "testEndpoint",
					EndpointAddress:           "testEndpointAddress",
					ServiceAttachment:         "testServiceAttachment",
					ServiceAttachmentFirewall: "testServiceAttachmentFirewall",
					ServiceAttachmentSubnet:   "testServiceAttachmentSubnet",
				},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().DeleteForwardingRule(gomock.Any(), gomock.Any()).Return(nil)
			m.EXPECT().DeleteAddress(gomock.Any(), gomock.Any()).Return(nil)
			m.EXPECT().DeleteServiceAttachment(gomock.Any(), gomock.Any()).Return(nil)
			m.EXPECT().DeleteFirewall(gomock.Any()).Return(nil)
			m.EXPECT().DeleteSubnet(gomock.Any(), gomock.Any()).Return(nil)
		},
		expectStatus: &hivev1gcp.PrivateServiceConnectStatus{},
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			logger, _ := testlogger.NewLoggerWithHook()

			gcpLinkActuator, err := newTestGCPLinkActuator(t,
				&hivev1.GCPPrivateServiceConnectConfig{},
				test.cd,
				[]runtime.Object{},
				test.gcpClientConfig,
			)
			require.NoError(t, err)

			err = gcpLinkActuator.Cleanup(test.cd, test.cd.Spec.ClusterMetadata, logger)

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			assert.Equal(t, test.expectStatus, test.cd.Status.Platform.GCP.PrivateServiceConnect)
		})
	}
}

func Test_CleanupRequired(t *testing.T) {
	gcpLinkActuator := &GCPLinkActuator{}
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		gcpClientConfig func(*mockclient.MockClient)

		expect bool
	}{{ // Cleanup is not required when status.platform is nil
		name: "status.platform is nil",
		cd:   cdBuilder.Build(),
	}, { // Cleanup is not required when status.platform.gcp is nil
		name: "status.platform.gcp is nil",
		cd:   cdBuilder.Options(testcd.WithEmptyPlatformStatus()).Build(),
	}, { // Cleanup is not required when when status.platform.gcp.privateServiceConnect is nil
		name: "status.platform.gcp.privateServiceConnect is nil",
		cd:   cdBuilder.Options(testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{})).Build(),
	}, { // Cleanup is not required when deleting a cluster with preserve-on-delete
		name: "preserve on delete",
		cd: cdBuilder.GenericOptions(
			generic.Deleted(),
		).Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{Enabled: true},
			}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{
					Endpoint: "testEndpoint",
				},
			}),
			testcd.PreserveOnDelete(),
		),
	}, { // Cleanup when EndpointAddress is not empty
		name: "EndpointAddress is empty",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{Enabled: true},
			}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{
					Endpoint: "testEndpoint",
				},
			}),
		),
		expect: true,
	}, { // Cleanup when ServiceAttachment is not empty
		name: "ServiceAttachment is empty",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{Enabled: true},
			}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{
					ServiceAttachment: "testServiceAttachment",
				},
			}),
		),
		expect: true,
	}, { // Cleanup not required when using an existing subnet
		name: "existing subnet",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{
					Enabled: true,
					ServiceAttachment: &hivev1gcp.ServiceAttachment{
						Subnet: &hivev1gcp.ServiceAttachmentSubnet{
							Existing: &hivev1gcp.ServiceAttachmentSubnetExisting{
								Name: "existingsubnet",
							},
						},
					},
				},
			}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{
					ServiceAttachmentFirewall: "testServiceAttachmentFirewall",
				},
			}),
		),
	}, { // Cleanup when ServiceAttachmentFirewall is not empty
		name: "ServiceAttachmentFirewall is empty",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{Enabled: true},
			}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{
					ServiceAttachmentFirewall: "testServiceAttachmentFirewall",
				},
			}),
		),
		expect: true,
	}, { // Cleanup when ServiceAttachmentSubnet is not empty
		name: "ServiceAttachmentSubnet is empty",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{Enabled: true},
			}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{
					ServiceAttachmentSubnet: "testServiceAttachmentSubnet",
				},
			}),
		),
		expect: true,
	}, { // Cleanup is not required when all fields are empty
		name: "all fields empty",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{Enabled: true},
			}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{},
			}),
		),
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result := gcpLinkActuator.CleanupRequired(test.cd)
			assert.Equal(t, test.expect, result)
		})
	}
}

func Test_Reconcile(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name   string
		cd     *hivev1.ClusterDeployment
		config *hivev1.GCPPrivateServiceConnectConfig

		gcpClientConfig func(*mockclient.MockClient)

		expect           reconcile.Result
		expectConditions []hivev1.ClusterDeploymentCondition
		expectError      string
		expectRecord     *actuator.DnsRecord
		expectStatus     *hivev1gcp.PrivateServiceConnectStatus
	}{{ // There should be an error on chooseSubnetForEndpoint failure
		name: "chooseSubnetForEndpoint failure",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{Region: testRegion}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{}}),
		),
		config: &hivev1.GCPPrivateServiceConnectConfig{
			EndpointVPCInventory: []hivev1.GCPPrivateServiceConnectInventory{{
				Network: mockNetwork.Name,
				Subnets: []hivev1.GCPPrivateServiceConnectSubnet{{
					Subnet: mockSubnet.SelfLink,
					Region: testRegion,
				}},
			}},
		},
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to GetSubnet"))
		},
		expectError: "error choosing a Subnet for the Endpoint: googleapi: Error 401: not authorized to GetSubnet, AccessDenied",
	}, { // Should return requeue later when forwarding rule is not found
		name: "forwarding rule not found",
		cd: cdBuilder.Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
			testcd.WithGCPPlatform(&hivev1gcp.Platform{Region: testRegion}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{}}),
		),
		config: &hivev1.GCPPrivateServiceConnectConfig{
			EndpointVPCInventory: []hivev1.GCPPrivateServiceConnectInventory{{
				Network: mockNetwork.Name,
				Subnets: []hivev1.GCPPrivateServiceConnectSubnet{{
					Subnet: mockSubnet.SelfLink,
					Region: testRegion,
				}},
			}},
		},
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
			m.EXPECT().ListAddresses(gomock.Any(), gomock.Any()).Return(&compute.AddressList{
				Items: []*compute.Address{mockAddress},
			}, nil)
			m.EXPECT().GetForwardingRule(gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusNotFound, "NotFound", "ForwardingRule not found"))
		},
		expect: requeueLater,
	}, { // There should be an error on GetForwardingRule failure
		name: "GetForwardingRule failure",
		cd: cdBuilder.Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
			testcd.WithGCPPlatform(&hivev1gcp.Platform{Region: testRegion}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{}}),
		),
		config: &hivev1.GCPPrivateServiceConnectConfig{
			EndpointVPCInventory: []hivev1.GCPPrivateServiceConnectInventory{{
				Network: mockNetwork.Name,
				Subnets: []hivev1.GCPPrivateServiceConnectSubnet{{
					Subnet: mockSubnet.SelfLink,
					Region: testRegion,
				}},
			}},
		},
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
			m.EXPECT().ListAddresses(gomock.Any(), gomock.Any()).Return(&compute.AddressList{
				Items: []*compute.Address{mockAddress},
			}, nil)
			m.EXPECT().GetForwardingRule(gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to GetForwardingRule"))
		},
		expectError: "failed to find the cluster api Forwarding Rule: googleapi: Error 401: not authorized to GetForwardingRule, AccessDenied",
	}, { // There should be an error on GetSubnet failure when existing subnet is specified
		name: "Existing Subnet, GetSubnet failure",
		cd: cdBuilder.Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
			testcd.WithGCPPlatform(&hivev1gcp.Platform{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{
					ServiceAttachment: &hivev1gcp.ServiceAttachment{
						Subnet: &hivev1gcp.ServiceAttachmentSubnet{
							Existing: &hivev1gcp.ServiceAttachmentSubnetExisting{
								Name: mockSubnet.Name,
							},
						},
					},
				},
				Region: testRegion,
			}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{}}),
		),
		config: &hivev1.GCPPrivateServiceConnectConfig{
			EndpointVPCInventory: []hivev1.GCPPrivateServiceConnectInventory{{
				Network: mockNetwork.Name,
				Subnets: []hivev1.GCPPrivateServiceConnectSubnet{{
					Subnet: mockSubnet.SelfLink,
					Region: testRegion,
				}},
			}},
		},
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
			m.EXPECT().ListAddresses(gomock.Any(), gomock.Any()).Return(&compute.AddressList{
				Items: []*compute.Address{mockAddress},
			}, nil)
			m.EXPECT().GetForwardingRule(gomock.Any(), gomock.Any()).Return(mockForwardingRule, nil)
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusNotFound, "NotFound", "Subnet not found"))
		},
		expectError: "failed to find the specified service attachment subnet: googleapi: Error 404: Subnet not found, NotFound",
	}, { // An endpoint IP address should be returned on success when existing subnet is specified
		name: "Existing Subnet, success",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{
					ServiceAttachment: &hivev1gcp.ServiceAttachment{
						Subnet: &hivev1gcp.ServiceAttachmentSubnet{
							Existing: &hivev1gcp.ServiceAttachmentSubnetExisting{
								Name: mockSubnet.Name,
							},
						},
					},
				},
				Region: testRegion,
			}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{
				ServiceAttachment: mockServiceAttachment.SelfLink,
				EndpointAddress:   mockAddress.SelfLink,
				Endpoint:          mockForwardingRule.SelfLink,
			}}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		config: &hivev1.GCPPrivateServiceConnectConfig{
			EndpointVPCInventory: []hivev1.GCPPrivateServiceConnectInventory{{
				Network: mockNetwork.Name,
				Subnets: []hivev1.GCPPrivateServiceConnectSubnet{{
					Subnet: mockSubnet.SelfLink,
					Region: testRegion,
				}},
			}},
		},
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
			m.EXPECT().ListAddresses(gomock.Any(), gomock.Any()).Return(&compute.AddressList{
				Items: []*compute.Address{mockAddress},
			}, nil)
			m.EXPECT().GetForwardingRule(gomock.Any(), gomock.Any()).Return(mockForwardingRule, nil)
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
			m.EXPECT().GetServiceAttachment(gomock.Any(), gomock.Any()).Return(mockServiceAttachment, nil)
			m.EXPECT().GetAddress(gomock.Any(), gomock.Any()).Return(mockAddress, nil)
			m.EXPECT().GetForwardingRule(gomock.Any(), gomock.Any()).Return(mockForwardingRule, nil)
		},
		expectStatus: &hivev1gcp.PrivateServiceConnectStatus{
			ServiceAttachment: mockServiceAttachment.SelfLink,
			EndpointAddress:   mockAddress.SelfLink,
			Endpoint:          mockForwardingRule.SelfLink,
		},
		expectRecord: &actuator.DnsRecord{IpAddress: []string{mockAddress.Address}},
	}, { // There should be an error on GetNetwork failure
		name: "GetNetwork failure",
		cd: cdBuilder.Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
			testcd.WithGCPPlatform(&hivev1gcp.Platform{Region: testRegion}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{}}),
		),
		config: &hivev1.GCPPrivateServiceConnectConfig{
			EndpointVPCInventory: []hivev1.GCPPrivateServiceConnectInventory{{
				Network: mockNetwork.Name,
				Subnets: []hivev1.GCPPrivateServiceConnectSubnet{{
					Subnet: mockSubnet.SelfLink,
					Region: testRegion,
				}},
			}},
		},
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
			m.EXPECT().ListAddresses(gomock.Any(), gomock.Any()).Return(&compute.AddressList{
				Items: []*compute.Address{mockAddress},
			}, nil)
			m.EXPECT().GetForwardingRule(gomock.Any(), gomock.Any()).Return(mockForwardingRule, nil)
			m.EXPECT().GetNetwork(gomock.Any()).Return(nil, newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to GetNetwork"))
		},
		expectError: "failed to find the cluster Network: googleapi: Error 401: not authorized to GetNetwork, AccessDenied",
	}, { // There should be an error on ensureServiceAttachmentSubnet failure
		name: "ensureServiceAttachmentSubnet failure",
		cd: cdBuilder.Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
			testcd.WithGCPPlatform(&hivev1gcp.Platform{Region: testRegion}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{}}),
		),
		config: &hivev1.GCPPrivateServiceConnectConfig{
			EndpointVPCInventory: []hivev1.GCPPrivateServiceConnectInventory{{
				Network: mockNetwork.Name,
				Subnets: []hivev1.GCPPrivateServiceConnectSubnet{{
					Subnet: mockSubnet.SelfLink,
					Region: testRegion,
				}},
			}},
		},
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
			m.EXPECT().ListAddresses(gomock.Any(), gomock.Any()).Return(&compute.AddressList{
				Items: []*compute.Address{mockAddress},
			}, nil)
			m.EXPECT().GetForwardingRule(gomock.Any(), gomock.Any()).Return(mockForwardingRule, nil)
			m.EXPECT().GetNetwork(gomock.Any()).Return(mockNetwork, nil)
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to GetSubnet"))
		},
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionTrue,
			Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
			Reason:  "ServiceAttachmentSubnetReconcileFailed",
			Message: "googleapi: Error 401: not authorized to GetSubnet, AccessDenied",
		}},
		expectError: "failed to reconcile the Service Attachment Subnet: googleapi: Error 401: not authorized to GetSubnet, AccessDenied",
	}, { // Ready condition should be updated when subnetModified
		name: "subnetModified",
		cd: cdBuilder.Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
			testcd.WithGCPPlatform(&hivev1gcp.Platform{Region: testRegion}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{}}),
		),
		config: &hivev1.GCPPrivateServiceConnectConfig{
			EndpointVPCInventory: []hivev1.GCPPrivateServiceConnectInventory{{
				Network: mockNetwork.Name,
				Subnets: []hivev1.GCPPrivateServiceConnectSubnet{{
					Subnet: mockSubnet.SelfLink,
					Region: testRegion,
				}},
			}},
		},
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
			m.EXPECT().ListAddresses(gomock.Any(), gomock.Any()).Return(&compute.AddressList{
				Items: []*compute.Address{mockAddress},
			}, nil)
			m.EXPECT().GetForwardingRule(gomock.Any(), gomock.Any()).Return(mockForwardingRule, nil)
			m.EXPECT().GetNetwork(gomock.Any()).Return(mockNetwork, nil)
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusNotFound, "NotFound", "Subnet not found"))
			m.EXPECT().CreateSubnet(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
		},
		expect: reconcile.Result{Requeue: true},
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionFalse,
			Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
			Reason:  "ReconciledServiceAttachmentSubnet",
			Message: "reconciled the Service Attachment Subnet",
		}},
		expectStatus: &hivev1gcp.PrivateServiceConnectStatus{
			ServiceAttachmentSubnet: mockSubnet.SelfLink,
		},
	}, { // There should be an error on ensureServiceAttachmentFirewall failure
		name: "ensureServiceAttachmentFirewall failure",
		cd: cdBuilder.Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
			testcd.WithGCPPlatform(&hivev1gcp.Platform{Region: testRegion}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{
				ServiceAttachmentSubnet: mockSubnet.SelfLink,
			}}),
		),
		config: &hivev1.GCPPrivateServiceConnectConfig{
			EndpointVPCInventory: []hivev1.GCPPrivateServiceConnectInventory{{
				Network: mockNetwork.Name,
				Subnets: []hivev1.GCPPrivateServiceConnectSubnet{{
					Subnet: mockSubnet.SelfLink,
					Region: testRegion,
				}},
			}},
		},
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
			m.EXPECT().ListAddresses(gomock.Any(), gomock.Any()).Return(&compute.AddressList{
				Items: []*compute.Address{mockAddress},
			}, nil)
			m.EXPECT().GetForwardingRule(gomock.Any(), gomock.Any()).Return(mockForwardingRule, nil)
			m.EXPECT().GetNetwork(gomock.Any()).Return(mockNetwork, nil)
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
			m.EXPECT().GetFirewall(gomock.Any()).Return(nil, newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to GetFirewall"))
		},
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionTrue,
			Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
			Reason:  "ServiceAttachmentFirewallReconcileFailed",
			Message: "googleapi: Error 401: not authorized to GetFirewall, AccessDenied",
		}},
		expectError: "failed to reconcile the Service Attachment Firwall: googleapi: Error 401: not authorized to GetFirewall, AccessDenied",
	}, { // Ready condition should be updated when firewallModified
		name: "firewallModified",
		cd: cdBuilder.Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
			testcd.WithGCPPlatform(&hivev1gcp.Platform{Region: testRegion}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{
				ServiceAttachmentSubnet: mockSubnet.SelfLink,
			}}),
		),
		config: &hivev1.GCPPrivateServiceConnectConfig{
			EndpointVPCInventory: []hivev1.GCPPrivateServiceConnectInventory{{
				Network: mockNetwork.Name,
				Subnets: []hivev1.GCPPrivateServiceConnectSubnet{{
					Subnet: mockSubnet.SelfLink,
					Region: testRegion,
				}},
			}},
		},
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
			m.EXPECT().ListAddresses(gomock.Any(), gomock.Any()).Return(&compute.AddressList{
				Items: []*compute.Address{mockAddress},
			}, nil)
			m.EXPECT().GetForwardingRule(gomock.Any(), gomock.Any()).Return(mockForwardingRule, nil)
			m.EXPECT().GetNetwork(gomock.Any()).Return(mockNetwork, nil)
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
			m.EXPECT().GetFirewall(gomock.Any()).Return(nil, newGoogleApiError(http.StatusNotFound, "NotFound", "Firewall not found"))
			m.EXPECT().CreateFirewall(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(mockFirewall, nil)
		},
		expect: reconcile.Result{Requeue: true},
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionFalse,
			Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
			Reason:  "ReconciledServiceAttachmentFirewall",
			Message: "reconciled the Service Attachment Firewall",
		}},
		expectStatus: &hivev1gcp.PrivateServiceConnectStatus{
			ServiceAttachmentSubnet:   mockSubnet.SelfLink,
			ServiceAttachmentFirewall: mockFirewall.SelfLink,
		},
	}, { // There should be an error on ensureServiceAttachment failure
		name: "ensureServiceAttachment failure",
		cd: cdBuilder.Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
			testcd.WithGCPPlatform(&hivev1gcp.Platform{Region: testRegion}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{
				ServiceAttachmentSubnet:   mockSubnet.SelfLink,
				ServiceAttachmentFirewall: mockFirewall.SelfLink,
			}}),
		),
		config: &hivev1.GCPPrivateServiceConnectConfig{
			EndpointVPCInventory: []hivev1.GCPPrivateServiceConnectInventory{{
				Network: mockNetwork.Name,
				Subnets: []hivev1.GCPPrivateServiceConnectSubnet{{
					Subnet: mockSubnet.SelfLink,
					Region: testRegion,
				}},
			}},
		},
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
			m.EXPECT().ListAddresses(gomock.Any(), gomock.Any()).Return(&compute.AddressList{
				Items: []*compute.Address{mockAddress},
			}, nil)
			m.EXPECT().GetForwardingRule(gomock.Any(), gomock.Any()).Return(mockForwardingRule, nil)
			m.EXPECT().GetNetwork(gomock.Any()).Return(mockNetwork, nil)
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
			m.EXPECT().GetFirewall(gomock.Any()).Return(mockFirewall, nil)
			m.EXPECT().GetServiceAttachment(gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to GetServiceAttachment"))
		},
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionTrue,
			Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
			Reason:  "ServiceAttachmentReconcileFailed",
			Message: "googleapi: Error 401: not authorized to GetServiceAttachment, AccessDenied",
		}},
		expectError: "failed to reconcile the Service Attachment: googleapi: Error 401: not authorized to GetServiceAttachment, AccessDenied",
	}, { // Ready condition should be updated when serviceAttachmentModified
		name: "serviceAttachmentModified",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{Region: testRegion}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{
				ServiceAttachmentSubnet:   mockSubnet.SelfLink,
				ServiceAttachmentFirewall: mockFirewall.SelfLink,
			}}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		config: &hivev1.GCPPrivateServiceConnectConfig{
			EndpointVPCInventory: []hivev1.GCPPrivateServiceConnectInventory{{
				Network: mockNetwork.Name,
				Subnets: []hivev1.GCPPrivateServiceConnectSubnet{{
					Subnet: mockSubnet.SelfLink,
					Region: testRegion,
				}},
			}},
		},
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
			m.EXPECT().ListAddresses(gomock.Any(), gomock.Any()).Return(&compute.AddressList{
				Items: []*compute.Address{mockAddress},
			}, nil)
			m.EXPECT().GetForwardingRule(gomock.Any(), gomock.Any()).Return(mockForwardingRule, nil)
			m.EXPECT().GetNetwork(gomock.Any()).Return(mockNetwork, nil)
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
			m.EXPECT().GetFirewall(gomock.Any()).Return(mockFirewall, nil)
			m.EXPECT().GetProjectName().Return(testProjectName)
			m.EXPECT().GetServiceAttachment(gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusNotFound, "NotFound", "ServiceAttachment not found"))
			m.EXPECT().CreateServiceAttachment(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(mockServiceAttachment, nil)
		},
		expect: reconcile.Result{Requeue: true},
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionFalse,
			Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
			Reason:  "ReconciledServiceAttachment",
			Message: "reconciled the Service Attachment",
		}},
		expectStatus: &hivev1gcp.PrivateServiceConnectStatus{
			ServiceAttachmentSubnet:   mockSubnet.SelfLink,
			ServiceAttachmentFirewall: mockFirewall.SelfLink,
			ServiceAttachment:         mockServiceAttachment.SelfLink,
		},
	}, { // There should be an error on ensureEndpointAddress failure
		name: "ensureEndpointAddress failure",
		cd: cdBuilder.Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
			testcd.WithGCPPlatform(&hivev1gcp.Platform{Region: testRegion}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{
				ServiceAttachmentSubnet:   mockSubnet.SelfLink,
				ServiceAttachmentFirewall: mockFirewall.SelfLink,
				ServiceAttachment:         mockServiceAttachment.SelfLink,
			}}),
		),
		config: &hivev1.GCPPrivateServiceConnectConfig{
			EndpointVPCInventory: []hivev1.GCPPrivateServiceConnectInventory{{
				Network: mockNetwork.Name,
				Subnets: []hivev1.GCPPrivateServiceConnectSubnet{{
					Subnet: mockSubnet.SelfLink,
					Region: testRegion,
				}},
			}},
		},
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
			m.EXPECT().ListAddresses(gomock.Any(), gomock.Any()).Return(&compute.AddressList{
				Items: []*compute.Address{mockAddress},
			}, nil)
			m.EXPECT().GetForwardingRule(gomock.Any(), gomock.Any()).Return(mockForwardingRule, nil)
			m.EXPECT().GetNetwork(gomock.Any()).Return(mockNetwork, nil)
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
			m.EXPECT().GetFirewall(gomock.Any()).Return(mockFirewall, nil)
			m.EXPECT().GetServiceAttachment(gomock.Any(), gomock.Any()).Return(mockServiceAttachment, nil)
			m.EXPECT().GetAddress(gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to GetAddress"))
		},
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionTrue,
			Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
			Reason:  "EndpointAddressReconcileFailed",
			Message: "googleapi: Error 401: not authorized to GetAddress, AccessDenied",
		}},
		expectError: "failed to reconcile the Endpoint Address: googleapi: Error 401: not authorized to GetAddress, AccessDenied",
	}, { // Ready condition should be updated when addressModified
		name: "addressModified",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{Region: testRegion}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{
				ServiceAttachmentSubnet:   mockSubnet.SelfLink,
				ServiceAttachmentFirewall: mockFirewall.SelfLink,
				ServiceAttachment:         mockServiceAttachment.SelfLink,
			}}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		config: &hivev1.GCPPrivateServiceConnectConfig{
			EndpointVPCInventory: []hivev1.GCPPrivateServiceConnectInventory{{
				Network: mockNetwork.Name,
				Subnets: []hivev1.GCPPrivateServiceConnectSubnet{{
					Subnet: mockSubnet.SelfLink,
					Region: testRegion,
				}},
			}},
		},
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
			m.EXPECT().ListAddresses(gomock.Any(), gomock.Any()).Return(&compute.AddressList{
				Items: []*compute.Address{mockAddress},
			}, nil)
			m.EXPECT().GetForwardingRule(gomock.Any(), gomock.Any()).Return(mockForwardingRule, nil)
			m.EXPECT().GetNetwork(gomock.Any()).Return(mockNetwork, nil)
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
			m.EXPECT().GetFirewall(gomock.Any()).Return(mockFirewall, nil)
			m.EXPECT().GetServiceAttachment(gomock.Any(), gomock.Any()).Return(mockServiceAttachment, nil)
			m.EXPECT().GetAddress(gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusNotFound, "NotFound", "Address not found"))
			m.EXPECT().CreateAddress(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockAddress, nil)
		},
		expect: reconcile.Result{Requeue: true},
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionFalse,
			Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
			Reason:  "ReconciledEndpointAddress",
			Message: "reconciled the Endpoint Address",
		}},
		expectStatus: &hivev1gcp.PrivateServiceConnectStatus{
			ServiceAttachmentSubnet:   mockSubnet.SelfLink,
			ServiceAttachmentFirewall: mockFirewall.SelfLink,
			ServiceAttachment:         mockServiceAttachment.SelfLink,
			EndpointAddress:           mockAddress.SelfLink,
		},
	}, { // There should be an error on ensureEndpoint failure
		name: "ensureEndpoint failure",
		cd: cdBuilder.Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
			testcd.WithGCPPlatform(&hivev1gcp.Platform{Region: testRegion}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{
				ServiceAttachmentSubnet:   mockSubnet.SelfLink,
				ServiceAttachmentFirewall: mockFirewall.SelfLink,
				ServiceAttachment:         mockServiceAttachment.SelfLink,
				EndpointAddress:           mockAddress.SelfLink,
			}}),
		),
		config: &hivev1.GCPPrivateServiceConnectConfig{
			EndpointVPCInventory: []hivev1.GCPPrivateServiceConnectInventory{{
				Network: mockNetwork.Name,
				Subnets: []hivev1.GCPPrivateServiceConnectSubnet{{
					Subnet: mockSubnet.SelfLink,
					Region: testRegion,
				}},
			}},
		},
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
			m.EXPECT().ListAddresses(gomock.Any(), gomock.Any()).Return(&compute.AddressList{
				Items: []*compute.Address{mockAddress},
			}, nil)
			m.EXPECT().GetForwardingRule(gomock.Any(), gomock.Any()).Return(mockForwardingRule, nil)
			m.EXPECT().GetNetwork(gomock.Any()).Return(mockNetwork, nil)
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
			m.EXPECT().GetFirewall(gomock.Any()).Return(mockFirewall, nil)
			m.EXPECT().GetServiceAttachment(gomock.Any(), gomock.Any()).Return(mockServiceAttachment, nil)
			m.EXPECT().GetAddress(gomock.Any(), gomock.Any()).Return(mockAddress, nil)
			m.EXPECT().GetForwardingRule(gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to GetForwardingRule"))
		},
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionTrue,
			Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
			Reason:  "EndpointReconcileFailed",
			Message: "googleapi: Error 401: not authorized to GetForwardingRule, AccessDenied",
		}},
		expectError: "failed to reconcile the Endpoint: googleapi: Error 401: not authorized to GetForwardingRule, AccessDenied",
	}, { // Ready condition should be updated when endpointModified
		name: "endpointModified",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{Region: testRegion}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{
				ServiceAttachmentSubnet:   mockSubnet.SelfLink,
				ServiceAttachmentFirewall: mockFirewall.SelfLink,
				ServiceAttachment:         mockServiceAttachment.SelfLink,
				EndpointAddress:           mockAddress.SelfLink,
			}}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		config: &hivev1.GCPPrivateServiceConnectConfig{
			EndpointVPCInventory: []hivev1.GCPPrivateServiceConnectInventory{{
				Network: mockNetwork.Name,
				Subnets: []hivev1.GCPPrivateServiceConnectSubnet{{
					Subnet: mockSubnet.SelfLink,
					Region: testRegion,
				}},
			}},
		},
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
			m.EXPECT().ListAddresses(gomock.Any(), gomock.Any()).Return(&compute.AddressList{
				Items: []*compute.Address{mockAddress},
			}, nil)
			m.EXPECT().GetForwardingRule(gomock.Any(), gomock.Any()).Return(mockForwardingRule, nil)
			m.EXPECT().GetNetwork(gomock.Any()).Return(mockNetwork, nil)
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
			m.EXPECT().GetFirewall(gomock.Any()).Return(mockFirewall, nil)
			m.EXPECT().GetServiceAttachment(gomock.Any(), gomock.Any()).Return(mockServiceAttachment, nil)
			m.EXPECT().GetAddress(gomock.Any(), gomock.Any()).Return(mockAddress, nil)
			m.EXPECT().GetForwardingRule(gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusNotFound, "NotFound", "ForwardingRule not found"))
			m.EXPECT().CreateForwardingRule(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(mockForwardingRule, nil)
		},
		expect: reconcile.Result{Requeue: true},
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionFalse,
			Type:    hivev1.PrivateLinkReadyClusterDeploymentCondition,
			Reason:  "ReconciledEndpoint",
			Message: "reconciled the Endpoint",
		}},
		expectStatus: &hivev1gcp.PrivateServiceConnectStatus{
			ServiceAttachmentSubnet:   mockSubnet.SelfLink,
			ServiceAttachmentFirewall: mockFirewall.SelfLink,
			ServiceAttachment:         mockServiceAttachment.SelfLink,
			EndpointAddress:           mockAddress.SelfLink,
			Endpoint:                  mockForwardingRule.SelfLink,
		},
	}, { // An endpoint IP address should be returned on success
		name: "success",
		cd: cdBuilder.Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
			testcd.WithGCPPlatform(&hivev1gcp.Platform{Region: testRegion}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{
				ServiceAttachmentSubnet:   mockSubnet.SelfLink,
				ServiceAttachmentFirewall: mockFirewall.SelfLink,
				ServiceAttachment:         mockServiceAttachment.SelfLink,
				EndpointAddress:           mockAddress.SelfLink,
				Endpoint:                  mockForwardingRule.SelfLink,
			}}),
		),
		config: &hivev1.GCPPrivateServiceConnectConfig{
			EndpointVPCInventory: []hivev1.GCPPrivateServiceConnectInventory{{
				Network: mockNetwork.Name,
				Subnets: []hivev1.GCPPrivateServiceConnectSubnet{{
					Subnet: mockSubnet.SelfLink,
					Region: testRegion,
				}},
			}},
		},
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
			m.EXPECT().ListAddresses(gomock.Any(), gomock.Any()).Return(&compute.AddressList{
				Items: []*compute.Address{mockAddress},
			}, nil)
			m.EXPECT().GetForwardingRule(gomock.Any(), gomock.Any()).Return(mockForwardingRule, nil)
			m.EXPECT().GetNetwork(gomock.Any()).Return(mockNetwork, nil)
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
			m.EXPECT().GetFirewall(gomock.Any()).Return(mockFirewall, nil)
			m.EXPECT().GetServiceAttachment(gomock.Any(), gomock.Any()).Return(mockServiceAttachment, nil)
			m.EXPECT().GetAddress(gomock.Any(), gomock.Any()).Return(mockAddress, nil)
			m.EXPECT().GetForwardingRule(gomock.Any(), gomock.Any()).Return(mockForwardingRule, nil)
		},
		expectRecord: &actuator.DnsRecord{IpAddress: []string{mockAddress.Address}},
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			logger, _ := testlogger.NewLoggerWithHook()
			gcpLinkActuator, err := newTestGCPLinkActuator(t,
				test.config,
				test.cd,
				[]runtime.Object{},
				test.gcpClientConfig,
			)
			require.NoError(t, err)

			dnsRecord := &actuator.DnsRecord{}
			result, err := gcpLinkActuator.Reconcile(test.cd, test.cd.Spec.ClusterMetadata, dnsRecord, logger)

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			assert.Equal(t, test.expect, result)

			if test.expectRecord == nil {
				test.expectRecord = &actuator.DnsRecord{}
			}
			assert.Equal(t, test.expectRecord, dnsRecord)

			if test.expectStatus != nil {
				assert.Equal(t, test.expectStatus, test.cd.Status.Platform.GCP.PrivateServiceConnect)
			}

			curr := &hivev1.ClusterDeployment{}
			fakeClient := *gcpLinkActuator.client
			errGet := fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: test.cd.Namespace, Name: test.cd.Name}, curr)
			assert.NoError(t, errGet)
			testassert.AssertConditions(t, curr, test.expectConditions)
		})
	}
}

func Test_ShouldSync(t *testing.T) {
	gcpLinkActuator := &GCPLinkActuator{}
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		expect bool
	}{{ // Sync is not required when spec.platform.gcp is nil
		name: "spec.platform.gcp is nil",
		cd:   cdBuilder.Build(),
	}, { // Sync is not required when spec.platform.gcp.privateServiceConnect is nil
		name: "spec.platform.gcp.privateServiceConnect",
		cd:   cdBuilder.Build(testcd.WithGCPPlatform(&hivev1gcp.Platform{})),
	}, { // Sync is not required when spec.platform.gcp.privateServiceConnect.enabled is false
		name: "privateServiceConnect is not enabled",
		cd: cdBuilder.Build(testcd.WithGCPPlatform(&hivev1gcp.Platform{
			PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{Enabled: false},
		})),
	}, { // Sync is required when status.platform is nil
		name: "status.platform is nil",
		cd: cdBuilder.Build(testcd.WithGCPPlatform(&hivev1gcp.Platform{
			PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{Enabled: true},
		})),
		expect: true,
	}, { // Sync is required when status.platform.gcp is nil
		name: "status.platform.gcp is nil",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{Enabled: true},
			}),
			testcd.WithEmptyPlatformStatus(),
		),
		expect: true,
	}, { // Sync is required when status.platform.gcp.privateServiceConnect is nil
		name: "status.platform.aws.privateServiceConnect is nil",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{Enabled: true},
			}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{}),
		),
		expect: true,
	}, { // Sync is required when endpoint is empty
		name: "Endpoint is empty",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{Enabled: true},
			}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{
					EndpointAddress:           mockAddress.SelfLink,
					ServiceAttachment:         mockServiceAttachment.SelfLink,
					ServiceAttachmentFirewall: mockFirewall.SelfLink,
					ServiceAttachmentSubnet:   mockSubnet.SelfLink,
				},
			}),
		),
		expect: true,
	}, { // Sync is required when EndpointAddress is empty
		name: "EndpointAddress is empty",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{Enabled: true},
			}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{
					Endpoint:                  mockForwardingRule.SelfLink,
					ServiceAttachment:         mockServiceAttachment.SelfLink,
					ServiceAttachmentFirewall: mockFirewall.SelfLink,
					ServiceAttachmentSubnet:   mockSubnet.SelfLink,
				},
			}),
		),
		expect: true,
	}, { // Sync is required when ServiceAttachment is empty
		name: "ServiceAttachment is empty",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{Enabled: true},
			}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{
					Endpoint:                  mockForwardingRule.SelfLink,
					EndpointAddress:           mockAddress.SelfLink,
					ServiceAttachmentFirewall: mockFirewall.SelfLink,
					ServiceAttachmentSubnet:   mockSubnet.SelfLink,
				},
			}),
		),
		expect: true,
	}, { // Sync is required when ServiceAttachmentFirewall is empty
		name: "ServiceAttachmentFirewall is empty",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{Enabled: true},
			}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{
					Endpoint:                mockForwardingRule.SelfLink,
					EndpointAddress:         mockAddress.SelfLink,
					ServiceAttachment:       mockServiceAttachment.SelfLink,
					ServiceAttachmentSubnet: mockSubnet.SelfLink,
				},
			}),
		),
		expect: true,
	}, { // Sync is required when ServiceAttachmentSubnet is empty
		name: "ServiceAttachmentSubnet is empty",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{Enabled: true},
			}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{
					Endpoint:                  mockForwardingRule.SelfLink,
					EndpointAddress:           mockAddress.SelfLink,
					ServiceAttachment:         mockServiceAttachment.SelfLink,
					ServiceAttachmentFirewall: mockFirewall.SelfLink,
				},
			}),
		),
		expect: true,
	}, { // Sync is not required when hostedZoneID is set
		name: "no changes required",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{Enabled: true},
			}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{
					Endpoint:                  mockForwardingRule.SelfLink,
					EndpointAddress:           mockAddress.SelfLink,
					ServiceAttachment:         mockServiceAttachment.SelfLink,
					ServiceAttachmentFirewall: mockFirewall.SelfLink,
					ServiceAttachmentSubnet:   mockSubnet.SelfLink,
				},
			}),
		),
	}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result := gcpLinkActuator.ShouldSync(test.cd)
			assert.Equal(t, test.expect, result)
		})
	}
}

func Test_ensureServiceAttachmentSubnet(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		gcpClientConfig func(*mockclient.MockClient)

		expect         *compute.Subnetwork
		expectError    string
		expectModified bool
		expectStatus   string
	}{{ // There should be an error on failure to createServiceAttachmentSubnet
		name: "createServiceAttachmentSubnet failed",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusNotFound, "NotFound", "Subnet not found"))
			m.EXPECT().CreateSubnet(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to CreateSubnet"))
		},
		expectError: "error creating the Service Attachment Subnet: googleapi: Error 401: not authorized to CreateSubnet, AccessDenied",
	}, { // Should return modified on createServiceAttachmentSubnet success
		name: "createServiceAttachmentSubnet success",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusNotFound, "NotFound", "Subnet not found"))
			m.EXPECT().CreateSubnet(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
		},
		expect:         mockSubnet,
		expectModified: true,
		expectStatus:   mockSubnet.SelfLink,
	}, { // There should be an error on failure to GetSubnet
		name: "GetSubnet failed",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to GetSubnet"))
		},
		expectError: "googleapi: Error 401: not authorized to GetSubnet, AccessDenied",
	}, { // Should return modified when subnet status changes
		name: "status change",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
		},
		expect:         mockSubnet,
		expectModified: true,
		expectStatus:   mockSubnet.SelfLink,
	}, { // Should return unmodified when there are no changes
		name: "unmodified",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{ServiceAttachmentSubnet: mockSubnet.SelfLink},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
		},
		expect:       mockSubnet,
		expectStatus: mockSubnet.SelfLink,
	}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			gcpLinkActuator, err := newTestGCPLinkActuator(t,
				&hivev1.GCPPrivateServiceConnectConfig{CredentialsSecretRef: mockCredentialsSecretRef},
				test.cd,
				[]runtime.Object{},
				test.gcpClientConfig,
			)
			require.NoError(t, err)

			modified, result, err := gcpLinkActuator.ensureServiceAttachmentSubnet(test.cd, test.cd.Spec.ClusterMetadata, mockNetwork)

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			assert.Equal(t, test.expectModified, modified)

			assert.Equal(t, test.expect, result)

			assert.Equal(t, test.expectStatus, test.cd.Status.Platform.GCP.PrivateServiceConnect.ServiceAttachmentSubnet)
		})
	}
}

func Test_getServiceAttachmentSubnetCIDR(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		expect string
	}{{ // Default cidr when cd is nil
		name:   "cd is nil",
		cd:     nil,
		expect: defaultServiceAttachmentSubnetCidr,
	}, { // Default cidr when platform.gcp is nil
		name:   "status.platform.gcp is nil",
		cd:     cdBuilder.Build(),
		expect: defaultServiceAttachmentSubnetCidr,
	}, { // Default cidr when PrivateServiceConnect is nil
		name:   "PrivateServiceConnect is nil",
		cd:     cdBuilder.Build(testcd.WithGCPPlatform(&hivev1gcp.Platform{})),
		expect: defaultServiceAttachmentSubnetCidr,
	}, { // Default cidr when ServiceAttachment is nil
		name: "ServiceAttachment is nil",
		cd: cdBuilder.Build(testcd.WithGCPPlatform(&hivev1gcp.Platform{
			PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{},
		})),
		expect: defaultServiceAttachmentSubnetCidr,
	}, { // Default cidr when Subnet is nil
		name: "Subnet is nil",
		cd: cdBuilder.Build(testcd.WithGCPPlatform(&hivev1gcp.Platform{
			PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{
				ServiceAttachment: &hivev1gcp.ServiceAttachment{},
			},
		})),
		expect: defaultServiceAttachmentSubnetCidr,
	}, { // The value of cidr
		name: "cidr",
		cd: cdBuilder.Build(testcd.WithGCPPlatform(&hivev1gcp.Platform{
			PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{
				ServiceAttachment: &hivev1gcp.ServiceAttachment{
					Subnet: &hivev1gcp.ServiceAttachmentSubnet{
						Cidr: "testCidr",
					},
				},
			},
		})),
		expect: "testCidr",
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result := getServiceAttachmentSubnetCIDR(test.cd)
			assert.Equal(t, test.expect, result)
		})
	}

}

func Test_getServiceAttachmentSubnetExistingName(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		expect string
	}{{ // Empty string when cd is nil
		name: "cd is nil",
		cd:   nil,
	}, { // Empty string when platform.gcp is nil
		name: "status.platform.gcp is nil",
		cd:   cdBuilder.Build(),
	}, { // Empty string when PrivateServiceConnect is nil
		name: "PrivateServiceConnect is nil",
		cd:   cdBuilder.Build(testcd.WithGCPPlatform(&hivev1gcp.Platform{})),
	}, { // Empty string when ServiceAttachment is nil
		name: "ServiceAttachment is nil",
		cd: cdBuilder.Build(testcd.WithGCPPlatform(&hivev1gcp.Platform{
			PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{},
		})),
	}, { // Empty string when Subnet is nil
		name: "Subnet is nil",
		cd: cdBuilder.Build(testcd.WithGCPPlatform(&hivev1gcp.Platform{
			PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{
				ServiceAttachment: &hivev1gcp.ServiceAttachment{},
			},
		})),
	}, { // Empty string when Existing is nil
		name: "Existing is nil",
		cd: cdBuilder.Build(testcd.WithGCPPlatform(&hivev1gcp.Platform{
			PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{
				ServiceAttachment: &hivev1gcp.ServiceAttachment{
					Subnet: &hivev1gcp.ServiceAttachmentSubnet{},
				},
			},
		})),
	}, { // The value of the existing subnet name
		name: "exsting name",
		cd: cdBuilder.Build(testcd.WithGCPPlatform(&hivev1gcp.Platform{
			PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{
				ServiceAttachment: &hivev1gcp.ServiceAttachment{
					Subnet: &hivev1gcp.ServiceAttachmentSubnet{
						Existing: &hivev1gcp.ServiceAttachmentSubnetExisting{
							Name: "testSubnet",
						},
					},
				},
			},
		})),
		expect: "testSubnet",
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result := getServiceAttachmentSubnetExistingName(test.cd)
			assert.Equal(t, test.expect, result)
		})
	}
}

func Test_getServiceAttachmentSubnetExistingProject(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		expect string
	}{{ // Empty string when cd is nil
		name: "cd is nil",
		cd:   nil,
	}, { // Empty string when platform.gcp is nil
		name: "status.platform.gcp is nil",
		cd:   cdBuilder.Build(),
	}, { // Empty string when PrivateServiceConnect is nil
		name: "PrivateServiceConnect is nil",
		cd:   cdBuilder.Build(testcd.WithGCPPlatform(&hivev1gcp.Platform{})),
	}, { // Empty string when ServiceAttachment is nil
		name: "ServiceAttachment is nil",
		cd: cdBuilder.Build(testcd.WithGCPPlatform(&hivev1gcp.Platform{
			PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{},
		})),
	}, { // Empty string when Subnet is nil
		name: "Subnet is nil",
		cd: cdBuilder.Build(testcd.WithGCPPlatform(&hivev1gcp.Platform{
			PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{
				ServiceAttachment: &hivev1gcp.ServiceAttachment{},
			},
		})),
	}, { // Empty string when Existing is nil
		name: "Existing is nil",
		cd: cdBuilder.Build(testcd.WithGCPPlatform(&hivev1gcp.Platform{
			PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{
				ServiceAttachment: &hivev1gcp.ServiceAttachment{
					Subnet: &hivev1gcp.ServiceAttachmentSubnet{},
				},
			},
		})),
	}, { // The value of Project
		name: "project",
		cd: cdBuilder.Build(testcd.WithGCPPlatform(&hivev1gcp.Platform{
			PrivateServiceConnect: &hivev1gcp.PrivateServiceConnect{
				ServiceAttachment: &hivev1gcp.ServiceAttachment{
					Subnet: &hivev1gcp.ServiceAttachmentSubnet{
						Existing: &hivev1gcp.ServiceAttachmentSubnetExisting{
							Project: testProjectName,
						},
					},
				},
			},
		})),
		expect: testProjectName,
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result := getServiceAttachmentSubnetExistingProject(test.cd)
			assert.Equal(t, test.expect, result)
		})
	}
}

func Test_createServiceAttachmentSubnet(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		gcpClientConfig func(*mockclient.MockClient)

		expect      *compute.Subnetwork
		expectError string
	}{{ // There should be a error on failure to CreateSubnet
		name: "CreateSubnet failed",
		cd:   cdBuilder.Build(),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().CreateSubnet(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to CreateSubnet"))
		},
		expectError: "error creating the Service Attachment Subnet: googleapi: Error 401: not authorized to CreateSubnet, AccessDenied",
	}, { // The ServiceAttachmentSubnet should be returned on success
		name: "success",
		cd:   cdBuilder.Build(),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().CreateSubnet(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
		},
		expect: mockSubnet,
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			gcpLinkActuator, err := newTestGCPLinkActuator(t,
				&hivev1.GCPPrivateServiceConnectConfig{CredentialsSecretRef: mockCredentialsSecretRef},
				test.cd,
				[]runtime.Object{},
				test.gcpClientConfig,
			)
			require.NoError(t, err)

			result, err := gcpLinkActuator.createServiceAttachmentSubnet(mockSubnet.Name, "", mockSubnet.Network, mockSubnet.Region)

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			assert.Equal(t, test.expect, result)
		})
	}
}

func Test_cleanupServiceAttachmentSubnet(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		gcpClientConfig func(*mockclient.MockClient)

		expectError  string
		expectStatus string
	}{{ // There should be a error on failure to DeleteSubnet
		name: "DeleteSubnet failed",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{ServiceAttachmentSubnet: "testServiceAttachmentSubnet"},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().DeleteSubnet(gomock.Any(), gomock.Any()).Return(newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to DeleteSubnet"))
		},
		expectStatus: "testServiceAttachmentSubnet",
		expectError:  "error deleting the Service Attachment Subnet: googleapi: Error 401: not authorized to DeleteSubnet, AccessDenied",
	}, { // The ServiceAttachmentFirewall should be cleared from the status on success
		name: "success",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{ServiceAttachmentSubnet: "ServiceAttachmentSubnet"},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().DeleteSubnet(gomock.Any(), gomock.Any()).Return(nil)
		},
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			logger, _ := testlogger.NewLoggerWithHook()
			gcpLinkActuator, err := newTestGCPLinkActuator(t,
				&hivev1.GCPPrivateServiceConnectConfig{CredentialsSecretRef: mockCredentialsSecretRef},
				test.cd,
				[]runtime.Object{},
				test.gcpClientConfig,
			)
			require.NoError(t, err)

			err = gcpLinkActuator.cleanupServiceAttachmentSubnet(test.cd, test.cd.Spec.ClusterMetadata, logger)

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			assert.Equal(t, test.expectStatus, test.cd.Status.Platform.GCP.PrivateServiceConnect.ServiceAttachmentSubnet)
		})
	}
}

func Test_ensureServiceAttachmentFirewall(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		gcpClientConfig func(*mockclient.MockClient)

		expect         *compute.Firewall
		expectError    string
		expectModified bool
		expectStatus   string
	}{{ // There should be an error on failure to createServiceAttachmentFirewall
		name: "createServiceAttachmentFirewall failed",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetFirewall(gomock.Any()).Return(nil, newGoogleApiError(http.StatusNotFound, "NotFound", "Firewall not found"))
			m.EXPECT().CreateFirewall(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to CreateFirewall"))
		},
		expectError: "error creating the Service Attachment Firewall: googleapi: Error 401: not authorized to CreateFirewall, AccessDenied",
	}, { // Should return modified on createServiceAttachment success
		name: "createServiceAttachment success",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetFirewall(gomock.Any()).Return(nil, newGoogleApiError(http.StatusNotFound, "NotFound", "Firewall not found"))
			m.EXPECT().CreateFirewall(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(mockFirewall, nil)
		},
		expect:         mockFirewall,
		expectModified: true,
		expectStatus:   mockFirewall.SelfLink,
	}, { // There should be an error on failure to GetFirewall
		name: "GetFirewall failed",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetFirewall(gomock.Any()).Return(nil, newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to GetFirewall"))
		},
		expectError: "googleapi: Error 401: not authorized to GetFirewall, AccessDenied",
	}, { // Should return modified when firewall status changes
		name: "status change",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetFirewall(gomock.Any()).Return(mockFirewall, nil)
		},
		expect:         mockFirewall,
		expectModified: true,
		expectStatus:   mockFirewall.SelfLink,
	}, { // Should return unmodified when there are no changes
		name: "unmodified",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{ServiceAttachmentFirewall: mockFirewall.SelfLink},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetFirewall(gomock.Any()).Return(mockFirewall, nil)
		},
		expect:       mockFirewall,
		expectStatus: mockFirewall.SelfLink,
	}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			gcpLinkActuator, err := newTestGCPLinkActuator(t,
				&hivev1.GCPPrivateServiceConnectConfig{CredentialsSecretRef: mockCredentialsSecretRef},
				test.cd,
				[]runtime.Object{},
				test.gcpClientConfig,
			)
			require.NoError(t, err)

			modified, result, err := gcpLinkActuator.ensureServiceAttachmentFirewall(test.cd, test.cd.Spec.ClusterMetadata, mockNetwork)

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			assert.Equal(t, test.expectModified, modified)

			assert.Equal(t, test.expect, result)

			assert.Equal(t, test.expectStatus, test.cd.Status.Platform.GCP.PrivateServiceConnect.ServiceAttachmentFirewall)
		})
	}
}

func Test_createServiceAttachmentFirewall(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		gcpClientConfig func(*mockclient.MockClient)

		expect      *compute.Firewall
		expectError string
	}{{ // There should be a error on failure to CreateFirewall
		name: "CreateFirewall failed",
		cd:   cdBuilder.Build(),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().CreateFirewall(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to CreateFirewall"))
		},
		expectError: "error creating the Service Attachment Firewall: googleapi: Error 401: not authorized to CreateFirewall, AccessDenied",
	}, { // The ServiceAttachmentFirewall should be returned on success
		name: "success",
		cd:   cdBuilder.Build(),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().CreateFirewall(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(mockFirewall, nil)
		},
		expect: mockFirewall,
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			gcpLinkActuator, err := newTestGCPLinkActuator(t,
				&hivev1.GCPPrivateServiceConnectConfig{CredentialsSecretRef: mockCredentialsSecretRef},
				test.cd,
				[]runtime.Object{},
				test.gcpClientConfig,
			)
			require.NoError(t, err)

			result, err := gcpLinkActuator.createServiceAttachmentFirewall(mockFirewall.Name, "testcidr", mockNetwork, []string{})

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			assert.Equal(t, test.expect, result)
		})
	}
}

func Test_cleanupServiceAttachmentFirewall(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		gcpClientConfig func(*mockclient.MockClient)

		expectError  string
		expectStatus string
	}{{ // There should be a error on failure to DeleteFirewall
		name: "DeleteFirewall failed",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{ServiceAttachmentFirewall: "testServiceAttachmentFirewall"},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().DeleteFirewall(gomock.Any()).Return(newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to DeleteFirewall"))
		},
		expectStatus: "testServiceAttachmentFirewall",
		expectError:  "error deleting the Service Attachment Firewall: googleapi: Error 401: not authorized to DeleteFirewall, AccessDenied",
	}, { // The ServiceAttachmentFirewall should be cleared from the status on success
		name: "success",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{ServiceAttachmentFirewall: "testServiceAttachmentFirewall"},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().DeleteFirewall(gomock.Any()).Return(nil)
		},
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			logger, _ := testlogger.NewLoggerWithHook()
			gcpLinkActuator, err := newTestGCPLinkActuator(t,
				&hivev1.GCPPrivateServiceConnectConfig{CredentialsSecretRef: mockCredentialsSecretRef},
				test.cd,
				[]runtime.Object{},
				test.gcpClientConfig,
			)
			require.NoError(t, err)

			err = gcpLinkActuator.cleanupServiceAttachmentFirewall(test.cd, test.cd.Spec.ClusterMetadata, logger)

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			assert.Equal(t, test.expectStatus, test.cd.Status.Platform.GCP.PrivateServiceConnect.ServiceAttachmentFirewall)
		})
	}
}

func Test_ensureServiceAttachment(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		gcpClientConfig func(*mockclient.MockClient)

		expect         *compute.ServiceAttachment
		expectError    string
		expectModified bool
		expectStatus   string
	}{{ // There should be an error on failure to createServiceAttachment
		name: "createServiceAttachment failed",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetProjectName().Return(testProjectName)
			m.EXPECT().GetServiceAttachment(gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusNotFound, "NotFound", "ServiceAttachment not found"))
			m.EXPECT().CreateServiceAttachment(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to CreateServiceAttachment"))
		},
		expectError: "error creating the Service Attachment: googleapi: Error 401: not authorized to CreateServiceAttachment, AccessDenied",
	}, { // Should return modified on createServiceAttachment success
		name: "createServiceAttachment success",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetProjectName().Return(testProjectName)
			m.EXPECT().GetServiceAttachment(gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusNotFound, "NotFound", "ServiceAttachment not found"))
			m.EXPECT().CreateServiceAttachment(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(mockServiceAttachment, nil)
		},
		expect:         mockServiceAttachment,
		expectModified: true,
		expectStatus:   mockServiceAttachment.SelfLink,
	}, { // There should be an error on failure to GetServiceAttachment
		name: "GetServiceAttachment failed",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetServiceAttachment(gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to GetServiceAttachment"))
		},
		expectError: "googleapi: Error 401: not authorized to GetServiceAttachment, AccessDenied",
	}, { // Should return modified when endpointAddress status changes
		name: "status change",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetServiceAttachment(gomock.Any(), gomock.Any()).Return(mockServiceAttachment, nil)
		},
		expect:         mockServiceAttachment,
		expectModified: true,
		expectStatus:   mockServiceAttachment.SelfLink,
	}, { // Should return unmodified when there are no changes
		name: "unmodified",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{ServiceAttachment: mockServiceAttachment.SelfLink},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetServiceAttachment(gomock.Any(), gomock.Any()).Return(mockServiceAttachment, nil)
		},
		expect:       mockServiceAttachment,
		expectStatus: mockServiceAttachment.SelfLink,
	}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			gcpLinkActuator, err := newTestGCPLinkActuator(t,
				&hivev1.GCPPrivateServiceConnectConfig{CredentialsSecretRef: mockCredentialsSecretRef},
				test.cd,
				[]runtime.Object{},
				test.gcpClientConfig,
			)
			require.NoError(t, err)

			modified, result, err := gcpLinkActuator.ensureServiceAttachment(test.cd, test.cd.Spec.ClusterMetadata, mockForwardingRule, mockSubnet)

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			assert.Equal(t, test.expectModified, modified)

			assert.Equal(t, test.expect, result)

			assert.Equal(t, test.expectStatus, test.cd.Status.Platform.GCP.PrivateServiceConnect.ServiceAttachment)
		})
	}
}

func Test_createServiceAttachment(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		gcpClientConfig func(*mockclient.MockClient)

		expect      *compute.ServiceAttachment
		expectError string
	}{{ // There should be a error on failure to CreateServiceAttachment
		name: "CreateServiceAttachment failed",
		cd:   cdBuilder.Build(),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetProjectName().Return(testProjectName)
			m.EXPECT().CreateServiceAttachment(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to CreateServiceAttachment"))
		},
		expectError: "error creating the Service Attachment: googleapi: Error 401: not authorized to CreateServiceAttachment, AccessDenied",
	}, { // The serviceAttachment should be returned on success
		name: "success",
		cd:   cdBuilder.Build(),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetProjectName().Return(testProjectName)
			m.EXPECT().CreateServiceAttachment(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(mockServiceAttachment, nil)
		},
		expect: mockServiceAttachment,
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			gcpLinkActuator, err := newTestGCPLinkActuator(t,
				&hivev1.GCPPrivateServiceConnectConfig{CredentialsSecretRef: mockCredentialsSecretRef},
				test.cd,
				[]runtime.Object{},
				test.gcpClientConfig,
			)
			require.NoError(t, err)

			result, err := gcpLinkActuator.createServiceAttachment(mockServiceAttachment.Name, mockServiceAttachment.Region, mockForwardingRule, mockSubnet)

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			assert.Equal(t, test.expect, result)
		})
	}
}

func Test_cleanupServiceAttachment(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		gcpClientConfig func(*mockclient.MockClient)

		expectError  string
		expectStatus string
	}{{ // There should be a error on failure to DeleteServiceAttachment
		name: "DeleteServiceAttachment failed",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{ServiceAttachment: "testServiceAttachment"},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().DeleteServiceAttachment(gomock.Any(), gomock.Any()).Return(newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to DeleteServiceAttachment"))
		},
		expectStatus: "testServiceAttachment",
		expectError:  "error deleting the Service Attachment: googleapi: Error 401: not authorized to DeleteServiceAttachment, AccessDenied",
	}, { // The ServiceAttachment should be cleared from the status on success
		name: "success",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{ServiceAttachment: "testServiceAttachment"},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().DeleteServiceAttachment(gomock.Any(), gomock.Any()).Return(nil)
		},
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			logger, _ := testlogger.NewLoggerWithHook()
			gcpLinkActuator, err := newTestGCPLinkActuator(t,
				&hivev1.GCPPrivateServiceConnectConfig{CredentialsSecretRef: mockCredentialsSecretRef},
				test.cd,
				[]runtime.Object{},
				test.gcpClientConfig,
			)
			require.NoError(t, err)

			err = gcpLinkActuator.cleanupServiceAttachment(test.cd, test.cd.Spec.ClusterMetadata, logger)

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			assert.Equal(t, test.expectStatus, test.cd.Status.Platform.GCP.PrivateServiceConnect.ServiceAttachment)
		})
	}
}

func Test_ensureEndpointAddress(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		gcpClientConfig func(*mockclient.MockClient)

		expect         *compute.Address
		expectError    string
		expectModified bool
		expectStatus   string
	}{{ // There should be an error on failure to createEndpointAddress
		name: "createEndpointAddress failed",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetAddress(gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusNotFound, "NotFound", "Address not found"))
			m.EXPECT().CreateAddress(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to CreateAddress"))
		},
		expectError: "error creating the Endpoint Address: googleapi: Error 401: not authorized to CreateAddress, AccessDenied",
	}, { // Should return modified on createEndpointAddress success
		name: "createEndpointAddress success",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetAddress(gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusNotFound, "NotFound", "Address not found"))
			m.EXPECT().CreateAddress(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockAddress, nil)
		},
		expect:         mockAddress,
		expectModified: true,
		expectStatus:   mockAddress.SelfLink,
	}, { // There should be an error on failure to GetAddress
		name: "GetAddress failed",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetAddress(gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to GetAddress"))
		},
		expectError: "googleapi: Error 401: not authorized to GetAddress, AccessDenied",
	}, { // Should return modified when endpointAddress status changes
		name: "status change",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetAddress(gomock.Any(), gomock.Any()).Return(mockAddress, nil)
		},
		expect:         mockAddress,
		expectModified: true,
		expectStatus:   mockAddress.SelfLink,
	}, { // Should return unmodified when there are no changes
		name: "unmodified",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{EndpointAddress: mockAddress.SelfLink},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetAddress(gomock.Any(), gomock.Any()).Return(mockAddress, nil)
		},
		expect:       mockAddress,
		expectStatus: mockAddress.SelfLink,
	}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			gcpLinkActuator, err := newTestGCPLinkActuator(t,
				&hivev1.GCPPrivateServiceConnectConfig{CredentialsSecretRef: mockCredentialsSecretRef},
				test.cd,
				[]runtime.Object{},
				test.gcpClientConfig,
			)
			require.NoError(t, err)

			modified, result, err := gcpLinkActuator.ensureEndpointAddress(test.cd, test.cd.Spec.ClusterMetadata, "")

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			assert.Equal(t, test.expectModified, modified)

			assert.Equal(t, test.expect, result)

			assert.Equal(t, test.expectStatus, test.cd.Status.Platform.GCP.PrivateServiceConnect.EndpointAddress)
		})
	}
}

func Test_createEndpointAddress(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		gcpClientConfig func(*mockclient.MockClient)

		expect      *compute.Address
		expectError string
	}{{ // There should be a error on failure to CreateAddress
		name: "CreateAddress failed",
		cd:   cdBuilder.Build(),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().CreateAddress(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to CreateAddress"))
		},
		expectError: "error creating the Endpoint Address: googleapi: Error 401: not authorized to CreateAddress, AccessDenied",
	}, { // The endpointAddress should be returned on success
		name: "success",
		cd:   cdBuilder.Build(),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().CreateAddress(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockAddress, nil)
		},
		expect: mockAddress,
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			gcpLinkActuator, err := newTestGCPLinkActuator(t,
				&hivev1.GCPPrivateServiceConnectConfig{CredentialsSecretRef: mockCredentialsSecretRef},
				test.cd,
				[]runtime.Object{},
				test.gcpClientConfig,
			)
			require.NoError(t, err)

			result, err := gcpLinkActuator.createEndpointAddress(mockAddress.Name, mockAddress.Region, mockAddress.Subnetwork)

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			assert.Equal(t, test.expect, result)
		})
	}
}

func Test_cleanupEndpointAddress(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		gcpClientConfig func(*mockclient.MockClient)

		expectError  string
		expectStatus string
	}{{ // There should be a error on failure to DeleteAddress
		name: "DeleteAddress failed",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{EndpointAddress: "testEndpointAddress"},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().DeleteAddress(gomock.Any(), gomock.Any()).Return(newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to DeleteAddress"))
		},
		expectStatus: "testEndpointAddress",
		expectError:  "error deleting the Endpoint Address: googleapi: Error 401: not authorized to DeleteAddress, AccessDenied",
	}, { // The endpointAddress should be cleared from the status on success
		name: "success",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{EndpointAddress: "testEndpointAddress"},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().DeleteAddress(gomock.Any(), gomock.Any()).Return(nil)
		},
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			logger, _ := testlogger.NewLoggerWithHook()
			gcpLinkActuator, err := newTestGCPLinkActuator(t,
				&hivev1.GCPPrivateServiceConnectConfig{CredentialsSecretRef: mockCredentialsSecretRef},
				test.cd,
				[]runtime.Object{},
				test.gcpClientConfig,
			)
			require.NoError(t, err)

			err = gcpLinkActuator.cleanupEndpointAddress(test.cd, test.cd.Spec.ClusterMetadata, logger)

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			assert.Equal(t, test.expectStatus, test.cd.Status.Platform.GCP.PrivateServiceConnect.EndpointAddress)
		})
	}
}

func Test_ensureEndpoint(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		gcpClientConfig func(*mockclient.MockClient)

		expect         *compute.ForwardingRule
		expectError    string
		expectModified bool
		expectStatus   string
	}{{ // There should be an error on failure to createEndpoint
		name: "createEndpoint failed",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetForwardingRule(gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusNotFound, "NotFound", "ForwardingRule not found"))
			m.EXPECT().CreateForwardingRule(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to CreateForwardingRule"))
		},
		expectError: "error creating the Endpoint: googleapi: Error 401: not authorized to CreateForwardingRule, AccessDenied",
	}, { // Should return modified on createEndpoint success
		name: "createEndpoint success",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetForwardingRule(gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusNotFound, "NotFound", "ForwardingRule not found"))
			m.EXPECT().CreateForwardingRule(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(mockForwardingRule, nil)
		},
		expect:         mockForwardingRule,
		expectModified: true,
		expectStatus:   mockForwardingRule.SelfLink,
	}, { // There should be an error on failure to GetForwardingRule
		name: "GetForwardingRule failed",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetForwardingRule(gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to GetForwardingRule"))
		},
		expectError: "googleapi: Error 401: not authorized to GetForwardingRule, AccessDenied",
	}, { // Should return modified when endpoint status changes
		name: "status change",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetForwardingRule(gomock.Any(), gomock.Any()).Return(mockForwardingRule, nil)
		},
		expect:         mockForwardingRule,
		expectModified: true,
		expectStatus:   mockForwardingRule.SelfLink,
	}, { // Should return unmodified when there are no changes
		name: "unmodified",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{Endpoint: mockForwardingRule.SelfLink},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetForwardingRule(gomock.Any(), gomock.Any()).Return(mockForwardingRule, nil)
		},
		expect:       mockForwardingRule,
		expectStatus: mockForwardingRule.SelfLink,
	}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			gcpLinkActuator, err := newTestGCPLinkActuator(t,
				&hivev1.GCPPrivateServiceConnectConfig{CredentialsSecretRef: mockCredentialsSecretRef},
				test.cd,
				[]runtime.Object{},
				test.gcpClientConfig,
			)
			require.NoError(t, err)

			modified, result, err := gcpLinkActuator.ensureEndpoint(test.cd, test.cd.Spec.ClusterMetadata, "", "testsubnet", "testserviceattachment")

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			assert.Equal(t, test.expectModified, modified)

			assert.Equal(t, test.expect, result)

			assert.Equal(t, test.expectStatus, test.cd.Status.Platform.GCP.PrivateServiceConnect.Endpoint)
		})
	}
}

func Test_createEndpoint(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		gcpClientConfig func(*mockclient.MockClient)

		expect      *compute.ForwardingRule
		expectError string
	}{{ // There should be a error on failure to CreateForwardingRule
		name: "CreateForwardingRule failed",
		cd:   cdBuilder.Build(),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().CreateForwardingRule(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to CreateForwardingRule"))
		},
		expectError: "error creating the Endpoint: googleapi: Error 401: not authorized to CreateForwardingRule, AccessDenied",
	}, { // The endpoint should be returned on success
		name: "success",
		cd:   cdBuilder.Build(),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().CreateForwardingRule(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(mockForwardingRule, nil)
		},
		expect: mockForwardingRule,
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			gcpLinkActuator, err := newTestGCPLinkActuator(t,
				&hivev1.GCPPrivateServiceConnectConfig{CredentialsSecretRef: mockCredentialsSecretRef},
				test.cd,
				[]runtime.Object{},
				test.gcpClientConfig,
			)
			require.NoError(t, err)

			result, err := gcpLinkActuator.createEndpoint(mockForwardingRule.Name, mockForwardingRule.IPAddress, mockForwardingRule.Region, mockForwardingRule.Subnetwork, mockForwardingRule.Target)

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			assert.Equal(t, test.expect, result)
		})
	}
}

func Test_cleanupEndpoint(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		gcpClientConfig func(*mockclient.MockClient)

		expectError  string
		expectStatus string
	}{{ // There should be a error on failure to DeleteForwardingRule
		name: "DeleteForwardingRule failed",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{Endpoint: "testEndpoint"},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().DeleteForwardingRule(gomock.Any(), gomock.Any()).Return(newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to DeleteForwardingRule"))
		},
		expectStatus: "testEndpoint",
		expectError:  "error deleting the Endpoint: googleapi: Error 401: not authorized to DeleteForwardingRule, AccessDenied",
	}, { // The endpoint should be cleared from the status on success
		name: "success",
		cd: cdBuilder.Build(
			testcd.WithGCPPlatform(&hivev1gcp.Platform{}),
			testcd.WithGCPPlatformStatus(&hivev1gcp.PlatformStatus{
				PrivateServiceConnect: &hivev1gcp.PrivateServiceConnectStatus{Endpoint: "testEndpoint"},
			}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().DeleteForwardingRule(gomock.Any(), gomock.Any()).Return(nil)
		},
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			logger, _ := testlogger.NewLoggerWithHook()
			gcpLinkActuator, err := newTestGCPLinkActuator(t,
				&hivev1.GCPPrivateServiceConnectConfig{CredentialsSecretRef: mockCredentialsSecretRef},
				test.cd,
				[]runtime.Object{},
				test.gcpClientConfig,
			)
			require.NoError(t, err)

			err = gcpLinkActuator.cleanupEndpoint(test.cd, test.cd.Spec.ClusterMetadata, logger)

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			assert.Equal(t, test.expectStatus, test.cd.Status.Platform.GCP.PrivateServiceConnect.Endpoint)
		})
	}
}

func Test_chooseSubnetForEndpoint(t *testing.T) {
	cases := []struct {
		name   string
		config hivev1.GCPPrivateServiceConnectConfig

		gcpClientConfig func(*mockclient.MockClient)

		expectError string
		expect      string
	}{{ // There should be an error on failure to filterSubnetInventory
		name: "filterSubnetInventory failure",
		config: hivev1.GCPPrivateServiceConnectConfig{
			EndpointVPCInventory: []hivev1.GCPPrivateServiceConnectInventory{{
				Network: mockNetwork.Name,
				Subnets: []hivev1.GCPPrivateServiceConnectSubnet{{
					Subnet: mockSubnet.SelfLink,
					Region: testRegion,
				}},
			}},
		},
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to GetSubnet"))
		},
		expectError: "googleapi: Error 401: not authorized to GetSubnet, AccessDenied",
	}, { // There should be an error when there are no candidates
		name:        "no candidate subnets",
		config:      hivev1.GCPPrivateServiceConnectConfig{},
		expectError: "no supported subnet in inventory for region testregion",
	}, { // There should be an error on ListAddresses failure
		name: "ListAddressess failure",
		config: hivev1.GCPPrivateServiceConnectConfig{
			EndpointVPCInventory: []hivev1.GCPPrivateServiceConnectInventory{{
				Network: mockNetwork.Name,
				Subnets: []hivev1.GCPPrivateServiceConnectSubnet{{
					Subnet: mockSubnet.SelfLink,
					Region: testRegion,
				}},
			}},
		},
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
			m.EXPECT().ListAddresses(gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to ListAddresses"))

		},
		expectError: "googleapi: Error 401: not authorized to ListAddresses, AccessDenied",
	}, { // The candidate with the least number of addresses should be returned
		name: "success",
		config: hivev1.GCPPrivateServiceConnectConfig{
			EndpointVPCInventory: []hivev1.GCPPrivateServiceConnectInventory{{
				Network: mockNetwork.Name,
				Subnets: []hivev1.GCPPrivateServiceConnectSubnet{{
					Subnet: mockSubnet.SelfLink,
					Region: testRegion,
				}, {
					Subnet: "subnetwithlessaddresses",
					Region: testRegion,
				}},
			}},
		},
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
			m.EXPECT().ListAddresses(gomock.Any(), gomock.Any()).Return(&compute.AddressList{
				Items: []*compute.Address{mockAddress},
			}, nil)
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(&compute.Subnetwork{
				Name:     "subnetwithlessaddresses",
				Network:  "testnetwork",
				Region:   testRegion,
				SelfLink: "subnetwithlessaddressesselflink",
			}, nil)
			m.EXPECT().ListAddresses(gomock.Any(), gomock.Any()).Return(&compute.AddressList{
				Items: []*compute.Address{},
			}, nil)
		},
		expect: "subnetwithlessaddressesselflink",
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			mockedGCPClient := mockclient.NewMockClient(gomock.NewController(t))
			if test.gcpClientConfig != nil {
				test.gcpClientConfig(mockedGCPClient)
			}

			result, err := chooseSubnetForEndpoint(mockedGCPClient, test.config, testRegion)

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			assert.Equal(t, test.expect, result)
		})
	}
}

func Test_filterSubnetInventory(t *testing.T) {
	cases := []struct {
		name      string
		inventory []hivev1.GCPPrivateServiceConnectInventory

		gcpClientConfig func(*mockclient.MockClient)

		expectError string
		expect      []hivev1.GCPPrivateServiceConnectSubnet
	}{{ // The result should be empty when there are no networks configured
		name:      "no networks configured",
		inventory: []hivev1.GCPPrivateServiceConnectInventory{},
	}, { // The result should be empty when there are no subnets configured
		name: "no subnets configured",
		inventory: []hivev1.GCPPrivateServiceConnectInventory{{
			Network: mockNetwork.SelfLink,
		}},
	}, { // The result should be empty when there are no subnets in the same region
		name: "no subnets in same region",
		inventory: []hivev1.GCPPrivateServiceConnectInventory{{
			Network: mockNetwork.SelfLink,
			Subnets: []hivev1.GCPPrivateServiceConnectSubnet{{
				Subnet: mockSubnet.SelfLink,
				Region: "someotherregion",
			}},
		}},
	}, { // Networks that are not found should be ignored
		name: "network not found",
		inventory: []hivev1.GCPPrivateServiceConnectInventory{{
			Network: mockNetwork.SelfLink,
			Subnets: []hivev1.GCPPrivateServiceConnectSubnet{{
				Subnet: mockSubnet.SelfLink,
				Region: testRegion,
			}},
		}, {
			Network: mockNetwork.Name,
			Subnets: []hivev1.GCPPrivateServiceConnectSubnet{{
				Subnet: "doesnotexist",
				Region: testRegion,
			}},
		}},
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusNotFound, "NotFound", "Subnet not found"))
		},
		expect: []hivev1.GCPPrivateServiceConnectSubnet{{
			Subnet: mockSubnet.SelfLink,
			Region: mockSubnet.Region,
		}},
	}, { // There should be an error when GetSubnet fails
		name: "GetSubnet failure",
		inventory: []hivev1.GCPPrivateServiceConnectInventory{{
			Network: mockNetwork.Name,
			Subnets: []hivev1.GCPPrivateServiceConnectSubnet{{
				Subnet: mockSubnet.SelfLink,
				Region: testRegion,
			}},
		}},
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, newGoogleApiError(http.StatusUnauthorized, "AccessDenied", "not authorized to GetSubnet"))
		},
		expectError: "googleapi: Error 401: not authorized to GetSubnet, AccessDenied",
	}, { // The result should contain matching subnets with correct selflinks.
		name: "success with selflink",
		inventory: []hivev1.GCPPrivateServiceConnectInventory{{
			Network: mockNetwork.Name,
			Subnets: []hivev1.GCPPrivateServiceConnectSubnet{{
				Subnet: "subnetbyname",
				Region: testRegion,
			}},
		}},
		gcpClientConfig: func(m *mockclient.MockClient) {
			m.EXPECT().GetSubnet(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockSubnet, nil)
		},
		expect: []hivev1.GCPPrivateServiceConnectSubnet{{
			Subnet: mockSubnet.SelfLink,
			Region: mockSubnet.Region,
		}},
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			mockedGCPClient := mockclient.NewMockClient(gomock.NewController(t))
			if test.gcpClientConfig != nil {
				test.gcpClientConfig(mockedGCPClient)
			}

			result, err := filterSubnetInventory(mockedGCPClient, test.inventory, testRegion)

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			assert.Equal(t, test.expect, result)
		})
	}
}
