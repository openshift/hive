package utils

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"

	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	testclusterdeployment "github.com/openshift/hive/pkg/test/clusterdeployment"
	testdnszone "github.com/openshift/hive/pkg/test/dnszone"
	"github.com/openshift/hive/pkg/test/generic"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testName           = "foo-lqmsh"
	differentNamespace = "differentns"
)

// withControllerOwnerReferenceAPIVersion sets the api version on the controller owner reference.
func withControllerOwnerReferenceAPIVersion(apiVersion string) testdnszone.Option {
	return func(dnsZone *hivev1.DNSZone) {
		for ix, ownerRef := range dnsZone.OwnerReferences {
			if ownerRef.Controller != nil && *ownerRef.Controller {
				dnsZone.OwnerReferences[ix].APIVersion = apiVersion
			}
		}
	}
}

func validOwnershipUniqueKey(owner *hivev1.ClusterDeployment) *OwnershipUniqueKey {
	return &OwnershipUniqueKey{
		TypeToList: &hivev1.DNSZoneList{},
		LabelSelector: map[string]string{
			constants.ClusterDeploymentNameLabel: owner.Name,
			constants.DNSZoneTypeLabel:           constants.DNSZoneTypeChild,
		},
	}
}

func validOwnershipUniqueKeys(owner *hivev1.ClusterDeployment) []*OwnershipUniqueKey {
	return []*OwnershipUniqueKey{
		validOwnershipUniqueKey(owner),
	}
}

func buildClusterDeployment(options ...testclusterdeployment.Option) *hivev1.ClusterDeployment {
	options = append(
		[]testclusterdeployment.Option{
			func(clusterDeployment *hivev1.ClusterDeployment) {
				clusterDeployment.ObjectMeta = metav1.ObjectMeta{
					Name:        testName,
					Namespace:   testNamespace,
					UID:         types.UID("1234"),
					Annotations: map[string]string{},
					Labels:      map[string]string{},
				}
			},
		},
		options...,
	)

	return testclusterdeployment.Build(options...)
}

func buildDNSZone(options ...testdnszone.Option) *hivev1.DNSZone {
	options = append(
		[]testdnszone.Option{
			testdnszone.WithTypeMeta(),
			testdnszone.WithResourceVersion("1"),
			func(dnsZone *hivev1.DNSZone) {
				dnsZone.Name = "dnszoneobject"
				dnsZone.Namespace = testNamespace
				dnsZone.Generation = 6
				dnsZone.Finalizers = []string{hivev1.FinalizerDNSZone}
				dnsZone.UID = types.UID("abcdef")
				dnsZone.Spec.Zone = "blah.example.com"
				dnsZone.Spec.AWS = &hivev1.AWSDNSZoneSpec{
					CredentialsSecretRef: corev1.LocalObjectReference{
						Name: "somesecret",
					},
					AdditionalTags: []hivev1.AWSResourceTag{
						{
							Key:   "foo",
							Value: "bar",
						},
					},
				}
				dnsZone.Status.AWS = &hivev1.AWSDNSZoneStatus{
					ZoneID: aws.String("1234"),
				}
			},
		},
		options...,
	)
	return testdnszone.Build(options...)
}

func TestReconcile(t *testing.T) {
	testscheme := scheme.Scheme
	apis.AddToScheme(testscheme)

	tests := []struct {
		name                                 string
		owner                                hivev1.MetaRuntimeObject
		listRuntimeObjectsOwnershipUniqueKey *OwnershipUniqueKey
		ownershipUniqueKeys                  []*OwnershipUniqueKey
		existingObjects                      []runtime.Object
		expectedObjects                      []runtime.Object
	}{
		{
			name:                                 "no objects in kube (do nothing)",
			owner:                                buildClusterDeployment(),
			listRuntimeObjectsOwnershipUniqueKey: validOwnershipUniqueKey(buildClusterDeployment()),
			ownershipUniqueKeys:                  validOwnershipUniqueKeys(buildClusterDeployment()),
		},
		{
			name:                                 "ownership set correctly (do nothing)",
			owner:                                buildClusterDeployment(),
			listRuntimeObjectsOwnershipUniqueKey: validOwnershipUniqueKey(buildClusterDeployment()),
			ownershipUniqueKeys:                  validOwnershipUniqueKeys(buildClusterDeployment()),
			existingObjects: []runtime.Object{
				buildDNSZone(
					testdnszone.WithLabelOwner(buildClusterDeployment()),
					testdnszone.WithControllerOwnerReference(buildClusterDeployment()),
				),
				buildDNSZone(
					testdnszone.Generic(generic.WithNamespace(differentNamespace)),
					testdnszone.WithLabelOwner(buildClusterDeployment(testclusterdeployment.Generic(generic.WithNamespace(differentNamespace)))),
					testdnszone.WithControllerOwnerReference(buildClusterDeployment(testclusterdeployment.Generic(generic.WithNamespace(differentNamespace)))),
				),
			},
			expectedObjects: []runtime.Object{
				buildDNSZone(
					testdnszone.WithLabelOwner(buildClusterDeployment()),
					testdnszone.WithControllerOwnerReference(buildClusterDeployment()),
				),
				buildDNSZone(
					testdnszone.Generic(generic.WithNamespace(differentNamespace)),
					testdnszone.WithLabelOwner(buildClusterDeployment(testclusterdeployment.Generic(generic.WithNamespace(differentNamespace)))),
					testdnszone.WithControllerOwnerReference(buildClusterDeployment(testclusterdeployment.Generic(generic.WithNamespace(differentNamespace)))),
				),
			},
		},
		{
			name:                                 "ownership incorrect, wrong version (fix)",
			owner:                                buildClusterDeployment(),
			listRuntimeObjectsOwnershipUniqueKey: validOwnershipUniqueKey(buildClusterDeployment()),
			ownershipUniqueKeys:                  validOwnershipUniqueKeys(buildClusterDeployment()),
			existingObjects: []runtime.Object{
				buildDNSZone(
					testdnszone.WithLabelOwner(buildClusterDeployment()),
					testdnszone.WithControllerOwnerReference(buildClusterDeployment()),
					withControllerOwnerReferenceAPIVersion("not/a/real/version"),
				),
				buildDNSZone(
					testdnszone.Generic(generic.WithNamespace(differentNamespace)),
					testdnszone.WithLabelOwner(buildClusterDeployment(testclusterdeployment.Generic(generic.WithNamespace(differentNamespace)))),
					testdnszone.WithControllerOwnerReference(buildClusterDeployment(testclusterdeployment.Generic(generic.WithNamespace(differentNamespace)))),
				),
			},
			expectedObjects: []runtime.Object{
				buildDNSZone(
					testdnszone.WithLabelOwner(buildClusterDeployment()),
					testdnszone.WithControllerOwnerReference(buildClusterDeployment()),
					testdnszone.WithIncrementedResourceVersion(),
				),
				buildDNSZone(
					testdnszone.Generic(generic.WithNamespace(differentNamespace)),
					testdnszone.WithLabelOwner(buildClusterDeployment(testclusterdeployment.Generic(generic.WithNamespace(differentNamespace)))),
					testdnszone.WithControllerOwnerReference(buildClusterDeployment(testclusterdeployment.Generic(generic.WithNamespace(differentNamespace)))),
				),
			},
		},
		{
			name:                                 "ownership missing (add ownership back)",
			owner:                                buildClusterDeployment(),
			listRuntimeObjectsOwnershipUniqueKey: validOwnershipUniqueKey(buildClusterDeployment()),
			ownershipUniqueKeys:                  validOwnershipUniqueKeys(buildClusterDeployment()),
			existingObjects: []runtime.Object{
				buildDNSZone(
					testdnszone.WithLabelOwner(buildClusterDeployment()),
				),
				buildDNSZone(
					testdnszone.Generic(generic.WithNamespace(differentNamespace)),
					testdnszone.WithLabelOwner(buildClusterDeployment(testclusterdeployment.Generic(generic.WithNamespace(differentNamespace)))),
					testdnszone.WithControllerOwnerReference(buildClusterDeployment()),
				),
			},
			expectedObjects: []runtime.Object{
				buildDNSZone(
					testdnszone.WithLabelOwner(buildClusterDeployment()),
					testdnszone.WithControllerOwnerReference(buildClusterDeployment()),
					testdnszone.WithIncrementedResourceVersion(),
				),
				buildDNSZone(
					testdnszone.Generic(generic.WithNamespace(differentNamespace)),
					testdnszone.WithLabelOwner(buildClusterDeployment(testclusterdeployment.Generic(generic.WithNamespace(differentNamespace)))),
					testdnszone.WithControllerOwnerReference(buildClusterDeployment()),
				),
			},
		},
		{
			name:                                 "ownership incorrect, wrong object (fix it)",
			owner:                                buildClusterDeployment(),
			listRuntimeObjectsOwnershipUniqueKey: validOwnershipUniqueKey(buildClusterDeployment()),
			ownershipUniqueKeys:                  validOwnershipUniqueKeys(buildClusterDeployment()),
			existingObjects: []runtime.Object{
				buildDNSZone(
					testdnszone.WithLabelOwner(buildClusterDeployment()),
					testdnszone.WithControllerOwnerReference(buildClusterDeployment(
						testclusterdeployment.Generic(generic.WithNamePostfix("-other")),
						testclusterdeployment.Generic(generic.WithUID("abcd")),
					),
					),
				),
				buildDNSZone(
					testdnszone.Generic(generic.WithNamespace(differentNamespace)),
					testdnszone.WithLabelOwner(buildClusterDeployment(testclusterdeployment.Generic(generic.WithNamespace(differentNamespace)))),
					testdnszone.WithControllerOwnerReference(buildClusterDeployment(testclusterdeployment.Generic(generic.WithNamespace(differentNamespace)))),
				),
			},
			expectedObjects: []runtime.Object{
				buildDNSZone(
					testdnszone.WithLabelOwner(buildClusterDeployment()),
					testdnszone.WithControllerOwnerReference(buildClusterDeployment()),
					testdnszone.WithIncrementedResourceVersion(),
				),
				buildDNSZone(
					testdnszone.Generic(generic.WithNamespace(differentNamespace)),
					testdnszone.WithLabelOwner(buildClusterDeployment(testclusterdeployment.Generic(generic.WithNamespace(differentNamespace)))),
					testdnszone.WithControllerOwnerReference(buildClusterDeployment(testclusterdeployment.Generic(generic.WithNamespace(differentNamespace)))),
				),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			fakeKubeClient := fake.NewFakeClientWithScheme(testscheme, test.existingObjects...)
			logger := log.WithField("fake", "fake")

			// Act
			err := ReconcileOwnerReferences(test.owner, test.ownershipUniqueKeys, fakeKubeClient, testscheme, logger)
			actualObjects, listErr := ListRuntimeObjects(fakeKubeClient, []runtime.Object{test.listRuntimeObjectsOwnershipUniqueKey.TypeToList}, client.MatchingLabels(test.listRuntimeObjectsOwnershipUniqueKey.LabelSelector))

			// Assert
			assert.NoError(t, err, "Unexpected error from ReconcileOwnerReferences")
			assert.NoError(t, listErr, "Unexpected error from ListRuntimeObjects")
			assert.ElementsMatch(t, test.expectedObjects, actualObjects, "The returned objects don't match the expected objects")
		})
	}
}
