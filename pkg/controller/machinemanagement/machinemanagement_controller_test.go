package machinemanagement

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openshiftapiv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"

	"github.com/openshift/hive/apis"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/remoteclient"
	remoteclientmock "github.com/openshift/hive/pkg/remoteclient/mock"
)

const (
	testName              = "foo-lqmsh"
	testClusterName       = "bar"
	testClusterID         = "testFooClusterUUID"
	testInfraID           = "testFooInfraID"
	testNamespace         = "default"
	pullSecretSecret      = "pull-secret"
	adminKubeconfigSecret = "foo-lqmsh-admin-kubeconfig"
	adminKubeconfig       = `clusters:
- cluster:
    certificate-authority-data: JUNK
    server: https://bar-api.clusters.example.com:6443
  name: bar
contexts:
- context:
    cluster: bar
  name: admin
current-context: admin
`
	adminPasswordSecret = "foo-lqmsh-admin-password"
	adminPassword       = "foo"

	remoteClusterRouteObjectName      = "console"
	remoteClusterRouteObjectNamespace = "openshift-console"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestMachineManagementReconcile(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	openshiftapiv1.Install(scheme.Scheme)
	routev1.Install(scheme.Scheme)

	getCD := func(c client.Client) *hivev1.ClusterDeployment {
		cd := &hivev1.ClusterDeployment{}
		err := c.Get(context.TODO(), client.ObjectKey{Name: testName, Namespace: testNamespace}, cd)
		if err == nil {
			return cd
		}
		return nil
	}

	getTargetNS := func(c client.Client, cd *hivev1.ClusterDeployment) *corev1.Namespace {
		ns := &corev1.Namespace{}
		err := c.Get(context.TODO(), client.ObjectKey{Name: cd.Spec.MachineManagement.TargetNamespace}, ns)
		if err == nil {
			return ns
		}
		return nil
	}

	getSecret := func(c client.Client, name string, namespace string) *corev1.Secret {
		secret := &corev1.Secret{}
		err := c.Get(context.TODO(), client.ObjectKey{Name: name, Namespace: namespace}, secret)
		if err == nil {
			return secret
		}
		return nil
	}

	tests := []struct {
		name                 string
		existing             []runtime.Object
		expectErr            bool
		expectedRequeueAfter time.Duration
		validate             func(client.Client, *testing.T)
		reconcilerSetup      func(*ReconcileMachineManagement)
	}{
		{
			name: "Create target namespace and add finalizer to cd",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
					cd.Spec.MachineManagement = &hivev1.MachineManagement{
						Central: &hivev1.CentralMachineManagement{},
					}
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, "aws-credentials", corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				ns := getTargetNS(c, cd)
				assert.Equal(t, ns.Annotations[constants.MachineManagementAnnotation], testName)
				assert.Equal(t, ns.Name, cd.Spec.MachineManagement.TargetNamespace)
				assert.True(t, controllerutil.ContainsFinalizer(cd, hivev1.FinalizerMachineManagementTargetNamespace))

				credsSecret := getSecret(c, cd.Spec.Platform.AWS.CredentialsSecretRef.Name, ns.Name)
				assert.NotNil(t, credsSecret)
				pullSecret := getSecret(c, cd.Spec.PullSecretRef.Name, ns.Name)
				assert.NotNil(t, pullSecret)
			},
		},
		{
			name: "Cleanup target namespace and remove finalizer from cd",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testDeletedClusterDeployment()
					cd.Spec.MachineManagement = &hivev1.MachineManagement{
						Central:         &hivev1.CentralMachineManagement{},
						TargetNamespace: "foo-lqmsh-targetns-vxx6f",
					}
					controllerutil.AddFinalizer(cd, hivev1.FinalizerMachineManagementTargetNamespace)
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, "aws-credentials", corev1.DockerConfigJsonKey, "{}"),
				testNs("foo-lqmsh-targetns-vxx6f"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				assert.True(t, !controllerutil.ContainsFinalizer(cd, hivev1.FinalizerMachineManagementTargetNamespace))
				ns := getTargetNS(c, cd)
				assert.Nil(t, ns)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logger := log.WithField("controller", "machineManagement")
			fakeClient := fake.NewFakeClient(test.existing...)
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockRemoteClientBuilder := remoteclientmock.NewMockBuilder(mockCtrl)

			rcd := &ReconcileMachineManagement{
				Client:                        fakeClient,
				scheme:                        scheme.Scheme,
				logger:                        logger,
				remoteClusterAPIClientBuilder: func(*hivev1.ClusterDeployment) remoteclient.Builder { return mockRemoteClientBuilder },
			}

			if test.reconcilerSetup != nil {
				test.reconcilerSetup(rcd)
			}

			reconcileRequest := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testName,
					Namespace: testNamespace,
				},
			}

			result, err := rcd.Reconcile(reconcileRequest)

			if test.validate != nil {
				test.validate(fakeClient, t)
			}

			if err != nil && !test.expectErr {
				t.Errorf("Unexpected error: %v", err)
			}
			if err == nil && test.expectErr {
				t.Errorf("Expected error but got none")
			}

			if test.expectedRequeueAfter == 0 {
				assert.Zero(t, result.RequeueAfter, "expected empty requeue after")
			} else {
				assert.InDelta(t, test.expectedRequeueAfter, result.RequeueAfter, float64(10*time.Second), "unexpected requeue after")
			}
		})
	}
}

func testEmptyClusterDeployment() *hivev1.ClusterDeployment {
	cd := &hivev1.ClusterDeployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: hivev1.SchemeGroupVersion.String(),
			Kind:       "ClusterDeployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       testName,
			Namespace:  testNamespace,
			Finalizers: []string{hivev1.FinalizerDeprovision},
			UID:        types.UID("1234"),
		},
	}
	return cd
}

func testClusterDeployment() *hivev1.ClusterDeployment {
	cd := testEmptyClusterDeployment()

	cd.Spec = hivev1.ClusterDeploymentSpec{
		ClusterName: testClusterName,
		PullSecretRef: &corev1.LocalObjectReference{
			Name: pullSecretSecret,
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
			InstallConfigSecretRef: &corev1.LocalObjectReference{Name: "install-config-secret"},
		},
		ClusterMetadata: &hivev1.ClusterMetadata{
			ClusterID:                testClusterID,
			InfraID:                  testInfraID,
			AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: adminKubeconfigSecret},
			AdminPasswordSecretRef:   corev1.LocalObjectReference{Name: adminPasswordSecret},
		},
	}

	if cd.Labels == nil {
		cd.Labels = make(map[string]string, 2)
	}
	cd.Labels[hivev1.HiveClusterPlatformLabel] = "aws"
	cd.Labels[hivev1.HiveClusterRegionLabel] = "us-east-1"

	cd.Status = hivev1.ClusterDeploymentStatus{
		InstallerImage: pointer.StringPtr("installer-image:latest"),
		CLIImage:       pointer.StringPtr("cli:latest"),
	}

	return cd
}

func testDeletedClusterDeployment() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	now := metav1.Now()
	cd.DeletionTimestamp = &now
	return cd
}

func testRemoteClusterAPIClient() client.Client {
	remoteClusterRouteObject := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      remoteClusterRouteObjectName,
			Namespace: remoteClusterRouteObjectNamespace,
		},
	}
	remoteClusterRouteObject.Spec.Host = "bar-api.clusters.example.com:6443/console"

	return fake.NewFakeClient(remoteClusterRouteObject)
}

func testSecret(secretType corev1.SecretType, name, key, value string) *corev1.Secret {
	return testSecretWithNamespace(secretType, name, testNamespace, key, value)
}

func testSecretWithNamespace(secretType corev1.SecretType, name, namespace, key, value string) *corev1.Secret {
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

func testNs(name string) *corev1.Namespace {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	return ns
}
