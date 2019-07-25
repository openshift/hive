package clusterversion

import (
	"context"
	"reflect"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	testName              = "foo-lqmsh"
	testClusterName       = "bar"
	testClusterID         = "testFooClusterUUID"
	installJobName        = "foo-lqmsh-install"
	uninstallJobName      = "foo-lqmsh-uninstall"
	testNamespace         = "default"
	metadataName          = "foo-lqmsh-metadata"
	adminPasswordSecret   = "admin-password"
	sshKeySecret          = "ssh-key"
	pullSecretSecret      = "pull-secret"
	testUUID              = "fakeUUID"
	testAMI               = "ami-totallyfake"
	adminKubeconfigSecret = "foo-lqmsh-admin-kubeconfig"
	adminKubeconfig       = `clusters:
- cluster:
    certificate-authority-data: JUNK
    server: https://bar-api.clusters.example.com:6443
  name: bar
`
	testRemoteClusterCurrentVersion = "4.0.0"
	remoteClusterVersionObjectName  = "version"

	remoteClusterRouteObjectName      = "console"
	remoteClusterRouteObjectNamespace = "openshift-console"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestClusterVersionReconcile(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	configv1.Install(scheme.Scheme)

	tests := []struct {
		name        string
		existing    []runtime.Object
		expectError bool
		validate    func(*testing.T, *hivev1.ClusterDeployment)
	}{
		{
			// no cluster deployment, no error expected
			name: "clusterdeployment doesn't exist",
		},
		{
			name: "deleted clusterdeployment",
			existing: []runtime.Object{
				testDeletedClusterDeployment(),
			},
		},
		{
			name: "no admin kubeconfig secret",
			existing: []runtime.Object{
				testClusterDeployment(),
			},
			expectError: true,
		},
		{
			name: "needs update",
			existing: []runtime.Object{
				testClusterDeployment(),
				testKubeconfigSecret(),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				expected := testRemoteClusterVersionStatus()
				controllerutils.FixupEmptyClusterVersionFields(&expected)
				if !reflect.DeepEqual(cd.Status.ClusterVersionStatus, expected) {
					t.Errorf("did not get expected clusterversion status. Expected: \n%#v\nGot: \n%#v", expected, cd.Status.ClusterVersionStatus)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClient(test.existing...)
			rcd := &ReconcileClusterVersion{
				Client:                        fakeClient,
				scheme:                        scheme.Scheme,
				remoteClusterAPIClientBuilder: testRemoteClusterAPIClientBuilder,
			}

			namespacedName := types.NamespacedName{
				Name:      testName,
				Namespace: testNamespace,
			}

			_, err := rcd.Reconcile(reconcile.Request{NamespacedName: namespacedName})

			if test.validate != nil {
				cd := &hivev1.ClusterDeployment{}
				err := fakeClient.Get(context.TODO(), namespacedName, cd)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				test.validate(t, cd)
			}

			if err != nil && !test.expectError {
				t.Errorf("Unexpected error: %v", err)
			}
			if err == nil && test.expectError {
				t.Errorf("Expected error but got none")
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
		},
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterName: testClusterName,
			SSHKey: &corev1.LocalObjectReference{
				Name: sshKeySecret,
			},
			ControlPlane: hivev1.MachinePool{},
			Compute:      []hivev1.MachinePool{},
			PullSecret: &corev1.LocalObjectReference{
				Name: pullSecretSecret,
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
			ClusterID: testClusterID,
			Installed: true,
			AdminKubeconfigSecret: corev1.LocalObjectReference{
				Name: "kubeconfig-secret",
			},
		},
	}
	controllerutils.FixupEmptyClusterVersionFields(&cd.Status.ClusterVersionStatus)
	return cd
}

func testDeletedClusterDeployment() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	now := metav1.Now()
	cd.DeletionTimestamp = &now
	return cd
}

func testKubeconfigSecret() *corev1.Secret {
	return testSecret("kubeconfig-secret", "kubeconfig", "KUBECONFIG-DATA")
}

func testSecret(name, key, value string) *corev1.Secret {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			key: []byte(value),
		},
	}
	return s
}

func testRemoteClusterAPIClientBuilder(secretData string, controllerName string) (client.Client, error) {
	remoteClusterVersion := &configv1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name: remoteClusterVersionObjectName,
		},
	}
	remoteClusterVersion.Status = testRemoteClusterVersionStatus()

	remoteClient := fake.NewFakeClient(remoteClusterVersion)
	return remoteClient, nil
}

func testRemoteClusterVersionStatus() configv1.ClusterVersionStatus {
	zeroTime := metav1.NewTime(time.Unix(0, 0))
	status := configv1.ClusterVersionStatus{
		History: []configv1.UpdateHistory{
			{
				State:          configv1.CompletedUpdate,
				Version:        testRemoteClusterCurrentVersion,
				Image:          "TESTIMAGE",
				CompletionTime: &zeroTime,
			},
		},
		ObservedGeneration: 123456789,
		VersionHash:        "TESTVERSIONHASH",
	}
	return status
}
