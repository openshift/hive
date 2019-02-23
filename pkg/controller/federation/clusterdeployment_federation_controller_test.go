package federation

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	fedv1alpha1 "github.com/kubernetes-sigs/federation-v2/pkg/apis/core/v1alpha1"
	federationutil "github.com/kubernetes-sigs/federation-v2/pkg/controller/util"
	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	crv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
)

const (
	testName      = "test-cluster-deployment"
	testNamespace = "test-namespace"
	testAMI       = "ami-totallyfake"

	testFedClusterName      = "test-federated-cluster"
	testFedClusterNamespace = federationutil.DefaultFederationSystemNamespace
)

var (
	testLabels = map[string]string{
		"one":   "1",
		"two":   "2",
		"three": "3",
	}
)

func TestReconcile(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	fedv1alpha1.AddToScheme(scheme.Scheme)
	crv1alpha1.AddToScheme(scheme.Scheme)

	tests := []struct {
		name                      string
		federationNotInstalled    bool
		existing                  []runtime.Object
		expectJoinClusterCalled   bool
		expectError               bool
		validateClusterDeployment func(*testing.T, *hivev1.ClusterDeployment)
		validateFederatedCluster  func(*testing.T, *fedv1alpha1.FederatedCluster)
		joinCluster               func(*hivev1.ClusterDeployment, log.FieldLogger) error
	}{
		{
			name: "federation not installed -> noop",
			federationNotInstalled:    true,
			existing:                  []runtime.Object{testClusterDeployment()},
			validateClusterDeployment: isSameCD(testClusterDeployment()),
		},
		{
			name:                      "cluster not installed -> noop",
			existing:                  []runtime.Object{testClusterDeployment()},
			validateClusterDeployment: isSameCD(testClusterDeployment()),
		},
		{
			name:                      "cluster deployment installed, no finalizer -> finalized should be added",
			existing:                  []runtime.Object{installed(testClusterDeployment())},
			validateClusterDeployment: hasFinalizer,
		},
		{
			name:                      "cluster deployment installed, with finalizer -> fed clusterref should be added",
			existing:                  []runtime.Object{withFinalizer(installed(testClusterDeployment()))},
			validateClusterDeployment: hasFedClusterRef,
		},
		{
			name:                      "cluster deployment installed, with fed cluster ref -> should be federated",
			existing:                  []runtime.Object{withFedClusterRef(withFinalizer(installed(testClusterDeployment())))},
			validateClusterDeployment: isFederated,
			expectJoinClusterCalled:   true,
		},
		{
			name:        "if joinCluster returns an error, expect an error from Reconcile",
			existing:    []runtime.Object{withFedClusterRef(withFinalizer(installed(testClusterDeployment())))},
			expectError: true,
			joinCluster: func(*hivev1.ClusterDeployment, log.FieldLogger) error {
				return fmt.Errorf("error")
			},
		},
		{
			name: "cluster deployment federated -> fed cluster should get annotations and labels set",
			existing: []runtime.Object{
				federated(withFedClusterRef(withFinalizer(installed(testClusterDeployment())))),
				testFederatedCluster(),
			},
			validateFederatedCluster: hasAnnotationsAndLabels,
		},
		{
			name: "cluster deployment federated with no cluster ref -> fed cluster should get legacy cluster ref set",
			existing: []runtime.Object{
				federated(withFinalizer(installed(testClusterDeployment()))),
				testFederatedCluster(),
			},
			validateClusterDeployment: hasLegacyFedClusterRef,
		},
		{
			name: "cluster deployment with finalizer is deleted -> finalizer should be removed",
			existing: []runtime.Object{
				withDeletionTimestamp(withFinalizer(installed(testClusterDeployment()))),
			},
			validateClusterDeployment: hasNoFinalizer,
		},
		{
			name: "federated cluster deployment with finalizer is deleted -> finalizer should be removed",
			existing: []runtime.Object{
				withDeletionTimestamp(federated(withFedClusterRef(withFinalizer(installed(testClusterDeployment()))))),
				testFederatedCluster(),
				testFederationSecret(),
				testClusterRegistryCluster(),
			},
			validateClusterDeployment: hasNoFinalizer,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := fake.NewFakeClient(test.existing...)
			joinClusterCalled := false
			r := &ReconcileClusterDeploymentFederation{
				Client: client,
				scheme: scheme.Scheme,
				isFederationInstalled: func() (bool, error) {
					return !test.federationNotInstalled, nil
				},
				joinCluster: func(*hivev1.ClusterDeployment, log.FieldLogger) error {
					joinClusterCalled = true
					return nil
				},
			}
			if test.joinCluster != nil {
				r.joinCluster = test.joinCluster
			}
			cdName := types.NamespacedName{Name: testName, Namespace: testNamespace}
			_, err := r.Reconcile(reconcile.Request{
				NamespacedName: cdName,
			})
			if err != nil && !test.expectError {
				t.Errorf("unexpected error from Reconcile: %v", err)
				return
			}
			if err == nil && test.expectError {
				t.Errorf("expected error but got none")
				return
			}
			if test.expectJoinClusterCalled && !joinClusterCalled {
				t.Errorf("joinCluster was not called")
			}
			if test.validateClusterDeployment != nil {
				cd := &hivev1.ClusterDeployment{}
				err := client.Get(context.TODO(), cdName, cd)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				test.validateClusterDeployment(t, cd)
			}
			if test.validateFederatedCluster != nil {
				cd := &hivev1.ClusterDeployment{}
				err := client.Get(context.TODO(), cdName, cd)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				if cd.Status.FederatedClusterRef == nil {
					t.Errorf("federated cluster reference is nil")
					return
				}
				fcName := types.NamespacedName{Name: cd.Status.FederatedClusterRef.Name, Namespace: cd.Status.FederatedClusterRef.Namespace}
				fc := &fedv1alpha1.FederatedCluster{}
				err = client.Get(context.TODO(), fcName, fc)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				test.validateFederatedCluster(t, fc)
			}
		})
	}
}

func testClusterDeployment() *hivev1.ClusterDeployment {
	cd := &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       testName,
			Namespace:  testNamespace,
			Finalizers: []string{hivev1.FinalizerDeprovision},
			UID:        types.UID("1234"),
			Labels:     testLabels,
		},
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterName: testName,
			SSHKey: &corev1.LocalObjectReference{
				Name: "key-secret",
			},
			ControlPlane: hivev1.MachinePool{},
			Compute:      []hivev1.MachinePool{},
			PullSecret: corev1.LocalObjectReference{
				Name: "pull-secret",
			},
			Platform: hivev1.Platform{
				AWS: &hivev1.AWSPlatform{
					Region: "us-east-1",
				},
			},
			Networking: hivev1.Networking{
				Type: hivev1.NetworkTypeOpenshiftSDN,
			},
			PlatformSecrets: hivev1.PlatformSecrets{
				AWS: &hivev1.AWSPlatformSecrets{
					Credentials: corev1.LocalObjectReference{
						Name: "aws-credentials",
					},
				},
			},
		},
		Status: hivev1.ClusterDeploymentStatus{
			ClusterID: "cluster-id",
			InfraID:   "infra-id",
		},
	}
	controllerutils.FixupEmptyClusterVersionFields(&cd.Status.ClusterVersionStatus)
	return cd
}

func testFederatedCluster() *fedv1alpha1.FederatedCluster {
	fc := &fedv1alpha1.FederatedCluster{}
	fc.Name = testFedClusterName
	fc.Namespace = testFedClusterNamespace
	return fc
}

func testFederationSecret() *corev1.Secret {
	secret := &corev1.Secret{}
	secret.Name = testFedClusterName
	secret.Namespace = testFedClusterNamespace
	return secret
}

func testClusterRegistryCluster() *crv1alpha1.Cluster {
	cluster := &crv1alpha1.Cluster{}
	cluster.Name = testFedClusterName
	cluster.Namespace = federationutil.MulticlusterPublicNamespace
	return cluster
}

func installed(cd *hivev1.ClusterDeployment) *hivev1.ClusterDeployment {
	cd.Status.Installed = true
	cd.Status.AdminKubeconfigSecret.Name = "admin-kubeconfig-secret"
	return cd
}

func withFinalizer(cd *hivev1.ClusterDeployment) *hivev1.ClusterDeployment {
	controllerutils.AddFinalizer(cd, hivev1.FinalizerFederation)
	return cd
}

func withFedClusterRef(cd *hivev1.ClusterDeployment) *hivev1.ClusterDeployment {
	cd.Status.FederatedClusterRef = &corev1.ObjectReference{
		Name:      testFedClusterName,
		Namespace: testFedClusterNamespace,
	}
	return cd
}

func federated(cd *hivev1.ClusterDeployment) *hivev1.ClusterDeployment {
	cd.Status.Federated = true
	return cd
}

func withDeletionTimestamp(cd *hivev1.ClusterDeployment) *hivev1.ClusterDeployment {
	now := metav1.Now()
	cd.DeletionTimestamp = &now
	return cd
}

func isSameCD(cd *hivev1.ClusterDeployment) func(*testing.T, *hivev1.ClusterDeployment) {
	return func(t *testing.T, cd2 *hivev1.ClusterDeployment) {
		if !reflect.DeepEqual(cd, cd2) {
			t.Errorf("unexpected cluster deployment: %#v", cd2)
		}
	}
}

func hasFedClusterRef(t *testing.T, cd *hivev1.ClusterDeployment) {
	if cd.Status.FederatedClusterRef == nil || cd.Status.FederatedClusterRef.Name == "" {
		t.Errorf("federated cluster ref is not set")
	}
}

func hasLegacyFedClusterRef(t *testing.T, cd *hivev1.ClusterDeployment) {
	hasFedClusterRef(t, cd)
	if cd.Status.FederatedClusterRef.Name != legacyFederatedClusterName(cd) {
		t.Errorf("federated cluster ref is not legacy ref")
	}
}

func hasFinalizer(t *testing.T, cd *hivev1.ClusterDeployment) {
	if !controllerutils.HasFinalizer(cd, hivev1.FinalizerFederation) {
		t.Errorf("missing federation finalizer")
	}
}

func hasNoFinalizer(t *testing.T, cd *hivev1.ClusterDeployment) {
	if controllerutils.HasFinalizer(cd, hivev1.FinalizerFederation) {
		t.Errorf("finalizer should not be present")
	}
}

func isFederated(t *testing.T, cd *hivev1.ClusterDeployment) {
	if !cd.Status.Federated {
		t.Errorf("cluster deployment is not federated")
	}
}

func hasAnnotationsAndLabels(t *testing.T, fc *fedv1alpha1.FederatedCluster) {
	if fc.Annotations == nil {
		t.Errorf("federated cluster has no annotations")
		return
	}
	ref := fc.Annotations[clusterDeploymentReferenceAnnotation]
	namespace, name, err := cache.SplitMetaNamespaceKey(ref)
	if err != nil {
		t.Errorf("cannot parse cluster deployment reference: %v", err)
	}
	if name != testName || namespace != testNamespace {
		t.Errorf("federated cluster name and/or namespace annotations have unexpected values: %s/%s", namespace, name)
	}
	if !reflect.DeepEqual(fc.Labels, testLabels) {
		t.Errorf("federated cluster labels do not have the expected values: %v", fc.Labels)
	}
}
