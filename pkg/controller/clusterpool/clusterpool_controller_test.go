package clusterpool

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/apis/hive/v1/aws"
	hivev1aws "github.com/openshift/hive/pkg/apis/hive/v1/aws"
)

const (
	testNamespace     = "test-namespace"
	hiveNamespace     = "hive"
	testLeasePoolName = "aws-us-east-1"
	credsSecretName   = "aws-creds"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestReconcileClusterPool(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name      string
		existing  []runtime.Object
		expectErr bool
		//validate                func(client.Client, *testing.T)
		expectedClusters int
	}{
		{
			name: "create all clusters",
			existing: []runtime.Object{
				buildPool(5),
				buildSecret(corev1.SecretTypeOpaque, testNamespace, credsSecretName, "dummykey", "dummyval"),
			},
			expectedClusters: 5,
		},
		{
			name: "scale up",
			existing: []runtime.Object{
				buildPool(5),
				buildSecret(corev1.SecretTypeOpaque, testNamespace, credsSecretName, "dummykey", "dummyval"),
				testClusterDeployment("c1"),
				testClusterDeployment("c2"),
				testClusterDeployment("c3"),
			},
			expectedClusters: 5,
		},
		{
			name: "scale down",
			existing: []runtime.Object{
				buildPool(3),
				buildSecret(corev1.SecretTypeOpaque, testNamespace, credsSecretName, "dummykey", "dummyval"),
				testClusterDeployment("c1"),
				testClusterDeployment("c2"),
				testClusterDeployment("c3"),
				testClusterDeployment("c4"),
				testClusterDeployment("c5"),
				testClusterDeployment("c6"),
			},
			expectedClusters: 3,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			//logger := log.WithField("controller", "clusterProvision")
			fakeClient := fake.NewFakeClient(test.existing...)
			rcp := &ReconcileClusterPool{
				Client: fakeClient,
				scheme: scheme.Scheme,
			}

			reconcileRequest := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testLeasePoolName,
					Namespace: testNamespace,
				},
			}

			_, err := rcp.Reconcile(reconcileRequest)

			if test.expectErr {
				assert.Error(t, err, "expected error from reconcile")
			} else {
				assert.NoError(t, err, "expected no error from reconcile")
			}

			cds := &hivev1.ClusterDeploymentList{}
			err = fakeClient.List(context.Background(), cds)
			require.NoError(t, err)

			var deletingCDs int
			for _, cd := range cds.Items {
				if cd.DeletionTimestamp != nil {
					deletingCDs++
				}
			}

			assert.Equal(t, test.expectedClusters, len(cds.Items))

			for _, cd := range cds.Items {
				assert.Equal(t, testNamespace, cd.Spec.ClusterPoolRef.Namespace)
				assert.Equal(t, testLeasePoolName, cd.Spec.ClusterPoolRef.Name)
				assert.Equal(t, hivev1.ClusterPoolStateUnclaimed, cd.Spec.ClusterPoolRef.State)
			}
		})
	}
}

func buildPool(size int) *hivev1.ClusterPool {
	return &hivev1.ClusterPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testLeasePoolName,
			Namespace: testNamespace,
		},
		Spec: hivev1.ClusterPoolSpec{
			Platform: hivev1.Platform{
				AWS: &aws.Platform{
					CredentialsSecretRef: v1.LocalObjectReference{Name: credsSecretName},
					Region:               "us-east-1",
				},
			},
			Size:       size,
			BaseDomain: "devclusters.example.com",
		},
	}
}

func buildSecret(secretType corev1.SecretType, namespace, name, key, value string) *corev1.Secret {
	s := &corev1.Secret{
		Type: secretType,
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			key: []byte(value),
		},
	}
	return s
}

func testClusterDeployment(clusterName string) *hivev1.ClusterDeployment {
	cd := &hivev1.ClusterDeployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: hivev1.SchemeGroupVersion.String(),
			Kind:       "ClusterDeployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       clusterName,
			Namespace:  testNamespace,
			Finalizers: []string{hivev1.FinalizerDeprovision},
			UID:        types.UID("1234"),
		},
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterName: clusterName,
			PullSecretRef: &corev1.LocalObjectReference{
				Name: "fakesecretref",
			},
			Platform: hivev1.Platform{
				AWS: &hivev1aws.Platform{
					CredentialsSecretRef: corev1.LocalObjectReference{
						Name: "aws-credentials",
					},
					Region: "us-east-1",
				},
			},
			Provisioning: &hivev1.Provisioning{
				InstallConfigSecretRef: corev1.LocalObjectReference{Name: "install-config-secret"},
			},
			ClusterMetadata: &hivev1.ClusterMetadata{
				ClusterID:                "fakeUUID",
				InfraID:                  "fakeInfraID",
				AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: "fakesecret1"},
				AdminPasswordSecretRef:   corev1.LocalObjectReference{Name: "fakesecret2"},
			},
			ClusterPoolRef: &hivev1.ClusterPoolReference{
				Namespace: testNamespace,
				Name:      testLeasePoolName,
				State:     hivev1.ClusterPoolStateUnclaimed,
			},
		},
	}

	return cd
}
