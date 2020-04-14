package utils

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	testName           = "foo-lqmsh"
	differentNamespace = "differentns"
)

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

func validClusterDeployment(namespace string) *hivev1.ClusterDeployment {
	return &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        testName,
			Namespace:   namespace,
			UID:         types.UID("1234"),
			Annotations: map[string]string{},
			Labels:      map[string]string{},
		},
	}
}

func validOtherClusterDeployment(namespace string) *hivev1.ClusterDeployment {
	return &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        testName + "-other",
			Namespace:   namespace,
			UID:         types.UID("abcd"),
			Annotations: map[string]string{},
			Labels:      map[string]string{},
		},
	}
}

func validDNSZoneWithLabelOwner(labelOwner hivev1.MetaRuntimeObject, scheme *runtime.Scheme, namespace string) *hivev1.DNSZone {
	zone := &hivev1.DNSZone{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "dnszoneobject",
			Namespace:  namespace,
			Generation: 6,
			Finalizers: []string{hivev1.FinalizerDNSZone},
			UID:        types.UID("abcdef"),
			Labels: map[string]string{
				constants.ClusterDeploymentNameLabel: labelOwner.GetName(),
				constants.DNSZoneTypeLabel:           constants.DNSZoneTypeChild,
			},
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

	return zone
}

func validDNSZoneWithMissingOwnership(owner hivev1.MetaRuntimeObject, scheme *runtime.Scheme, namespace string) *hivev1.DNSZone {
	zone := validDNSZoneWithLabelOwner(owner, scheme, namespace)
	return zone
}

func validDNSZoneWithControllerOwnership(labelOwner, controllerOwner hivev1.MetaRuntimeObject, scheme *runtime.Scheme, namespace string) *hivev1.DNSZone {
	zone := validDNSZoneWithLabelOwner(labelOwner, scheme, namespace)
	controllerutil.SetControllerReference(controllerOwner, zone, scheme)
	return zone
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
			owner:                                validClusterDeployment(testNamespace),
			listRuntimeObjectsOwnershipUniqueKey: validOwnershipUniqueKey(validClusterDeployment(testNamespace)),
			ownershipUniqueKeys:                  validOwnershipUniqueKeys(validClusterDeployment(testNamespace)),
		},
		{
			name:                                 "ownership set correctly (do nothing)",
			owner:                                validClusterDeployment(testNamespace),
			listRuntimeObjectsOwnershipUniqueKey: validOwnershipUniqueKey(validClusterDeployment(testNamespace)),
			ownershipUniqueKeys:                  validOwnershipUniqueKeys(validClusterDeployment(testNamespace)),
			existingObjects: []runtime.Object{
				validDNSZoneWithControllerOwnership(validClusterDeployment(testNamespace), validClusterDeployment(testNamespace), testscheme, testNamespace),
				validDNSZoneWithControllerOwnership(validClusterDeployment(differentNamespace), validClusterDeployment(differentNamespace), testscheme, differentNamespace),
			},
			expectedObjects: []runtime.Object{
				validDNSZoneWithControllerOwnership(validClusterDeployment(testNamespace), validClusterDeployment(testNamespace), testscheme, testNamespace),
				validDNSZoneWithControllerOwnership(validClusterDeployment(differentNamespace), validClusterDeployment(differentNamespace), testscheme, differentNamespace),
			},
		},
		{
			name:                                 "ownership missing (add ownership back)",
			owner:                                validClusterDeployment(testNamespace),
			listRuntimeObjectsOwnershipUniqueKey: validOwnershipUniqueKey(validClusterDeployment(testNamespace)),
			ownershipUniqueKeys:                  validOwnershipUniqueKeys(validClusterDeployment(testNamespace)),
			existingObjects: []runtime.Object{
				validDNSZoneWithMissingOwnership(validClusterDeployment(testNamespace), testscheme, testNamespace),
				validDNSZoneWithMissingOwnership(validClusterDeployment(differentNamespace), testscheme, differentNamespace),
			},
			expectedObjects: []runtime.Object{
				validDNSZoneWithControllerOwnership(validClusterDeployment(testNamespace), validClusterDeployment(testNamespace), testscheme, testNamespace),
				validDNSZoneWithMissingOwnership(validClusterDeployment(differentNamespace), testscheme, differentNamespace),
			},
		},
		{
			name:                                 "ownership incorrect (fix it)",
			owner:                                validClusterDeployment(testNamespace),
			listRuntimeObjectsOwnershipUniqueKey: validOwnershipUniqueKey(validClusterDeployment(testNamespace)),
			ownershipUniqueKeys:                  validOwnershipUniqueKeys(validClusterDeployment(testNamespace)),
			existingObjects: []runtime.Object{
				validDNSZoneWithControllerOwnership(validClusterDeployment(testNamespace), validOtherClusterDeployment(testNamespace), testscheme, testNamespace),
				validDNSZoneWithControllerOwnership(validClusterDeployment(differentNamespace), validOtherClusterDeployment(differentNamespace), testscheme, differentNamespace),
			},
			expectedObjects: []runtime.Object{
				validDNSZoneWithControllerOwnership(validClusterDeployment(testNamespace), validClusterDeployment(testNamespace), testscheme, testNamespace),
				validDNSZoneWithControllerOwnership(validClusterDeployment(differentNamespace), validOtherClusterDeployment(differentNamespace), testscheme, differentNamespace),
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
