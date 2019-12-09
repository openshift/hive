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

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	awsclient "github.com/openshift/hive/pkg/awsclient"
	gcpclient "github.com/openshift/hive/pkg/gcpclient"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/golang/mock/gomock"
	mockaws "github.com/openshift/hive/pkg/awsclient/mock"
	mockgcp "github.com/openshift/hive/pkg/gcpclient/mock"
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

	validAWSSecret = func() *corev1.Secret {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "somesecret",
				Namespace: "ns",
			},
			Data: map[string][]byte{
				"aws_access_key_id":     []byte("notrealaccesskey"),
				"aws_secret_access_key": []byte("notrealsecretaccesskey"),
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
)

type mocks struct {
	fakeKubeClient client.Client
	mockCtrl       *gomock.Controller
	mockAWSClient  *mockaws.MockClient
	mockGCPClient  *mockgcp.MockClient
}

// setupDefaultMocks is an easy way to setup all of the default mocks
func setupDefaultMocks(t *testing.T) *mocks {
	mocks := &mocks{
		fakeKubeClient: fakekubeclient.NewFakeClient(),
		mockCtrl:       gomock.NewController(t),
	}

	mocks.mockAWSClient = mockaws.NewMockClient(mocks.mockCtrl)
	mocks.mockGCPClient = mockgcp.NewMockClient(mocks.mockCtrl)

	return mocks
}

func fakeAWSClientBuilder(mockAWSClient *mockaws.MockClient) awsClientBuilderType {
	return func(secret *corev1.Secret, region string) (awsclient.Client, error) {
		return mockAWSClient, nil
	}
}

func fakeGCPClientBuilder(mockGCPClient *mockgcp.MockClient) gcpClientBuilderType {
	return func(secret *corev1.Secret) (gcpclient.Client, error) {
		return mockGCPClient, nil
	}
}

// setFakeDNSZoneInKube is an easy way to register a dns zone object with kube.
func setFakeDNSZoneInKube(mocks *mocks, dnsZone *hivev1.DNSZone) error {
	return mocks.fakeKubeClient.Create(context.TODO(), dnsZone)
}
