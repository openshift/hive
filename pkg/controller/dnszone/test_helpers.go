package dnszone

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	awsclient "github.com/openshift/hive/pkg/awsclient"
	azureclient "github.com/openshift/hive/pkg/azureclient"
	gcpclient "github.com/openshift/hive/pkg/gcpclient"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	mockaws "github.com/openshift/hive/pkg/awsclient/mock"
	mockazure "github.com/openshift/hive/pkg/azureclient/mock"
	mockgcp "github.com/openshift/hive/pkg/gcpclient/mock"
	testfake "github.com/openshift/hive/pkg/test/fake"
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
					CredentialsSecretRef: corev1.LocalObjectReference{
						Name: "somesecret",
					},
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

	validAzureDNSZone = func() *hivev1.DNSZone {
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
				Azure: &hivev1.AzureDNSZoneSpec{
					ResourceGroupName: "default",
				},
			},
			Status: hivev1.DNSZoneStatus{
				Azure: &hivev1.AzureDNSZoneStatus{},
			},
		}
	}

	validGCPSecret = func() *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "somesecret",
				Namespace: "ns",
			},
			Data: map[string][]byte{
				"osServiceAccount.json": []byte("notrealsecrettoken"),
			},
		}
	}

	validAzureSecret = func() *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "somesecret",
				Namespace: "ns",
			},
			Data: map[string][]byte{
				"osServicePrincipal.json": []byte("notrealsecrettoken"),
			},
		}
	}

	validDNSZoneWithLinkToParent = func() *hivev1.DNSZone {
		zone := validDNSZone()
		zone.Spec.LinkToParentDomain = true
		return zone
	}

	validAzureDNSZoneWithLinkToParent = func() *hivev1.DNSZone {
		zone := validAzureDNSZone()
		zone.Spec.LinkToParentDomain = true
		return zone
	}

	validDNSZoneWithoutFinalizer = func() *hivev1.DNSZone {
		zone := validDNSZone()
		zone.Finalizers = []string{}
		return zone
	}

	validAzureDNSZoneWithoutFinalizer = func() *hivev1.DNSZone {
		zone := validAzureDNSZone()
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

	validDNSZoneBeingDeletedWithPreserve = func() *hivev1.DNSZone {
		zone := validDNSZoneBeingDeleted()
		zone.Spec.PreserveOnDelete = true
		return zone
	}

	validAzureDNSZoneBeingDeleted = func() *hivev1.DNSZone {
		// Take a copy of the default validAzureDNSZone object
		zone := validAzureDNSZone()

		// And make the 1 change needed to signal the object is being deleted.
		zone.DeletionTimestamp = kubeTimeNow
		return zone
	}
)

type mocks struct {
	fakeKubeClient  client.Client
	mockCtrl        *gomock.Controller
	mockAWSClient   *mockaws.MockClient
	mockGCPClient   *mockgcp.MockClient
	mockAzureClient *mockazure.MockClient
}

// setupDefaultMocks is an easy way to setup all of the default mocks
func setupDefaultMocks(t *testing.T, existing ...runtime.Object) *mocks {
	mocks := &mocks{
		fakeKubeClient: testfake.NewFakeClientBuilder().WithRuntimeObjects(existing...).Build(),
		mockCtrl:       gomock.NewController(t),
	}

	mocks.mockAWSClient = mockaws.NewMockClient(mocks.mockCtrl)
	mocks.mockGCPClient = mockgcp.NewMockClient(mocks.mockCtrl)
	mocks.mockAzureClient = mockazure.NewMockClient(mocks.mockCtrl)

	return mocks
}

func fakeAWSClientBuilder(mockAWSClient *mockaws.MockClient) awsClientBuilderType {
	return func(_ client.Client, _ awsclient.Options) (awsclient.Client, error) {
		return mockAWSClient, nil
	}
}

func fakeGCPClientBuilder(mockGCPClient *mockgcp.MockClient) gcpClientBuilderType {
	return func(secret *corev1.Secret) (gcpclient.Client, error) {
		return mockGCPClient, nil
	}
}

func fakeAzureClientBuilder(mockAzureClient *mockazure.MockClient) azureClientBuilderType {
	return func(secret *corev1.Secret, cloudName string) (azureclient.Client, error) {
		return mockAzureClient, nil
	}
}
