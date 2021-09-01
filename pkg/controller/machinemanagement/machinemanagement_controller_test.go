package machinemanagement

import (
	"context"
	"reflect"
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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openshiftapiv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"

	"github.com/openshift/hive/apis"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/apis/hive/v1/aws"
	"github.com/openshift/hive/pkg/constants"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	"github.com/openshift/hive/pkg/test/generic"
)

const (
	testName         = "foo-lqmsh"
	testClusterName  = "bar"
	testClusterID    = "testFooClusterUUID"
	testInfraID      = "testFooInfraID"
	testNamespace    = "test-namespace"
	pullSecretSecret = "pull-secret"
	credsSecret      = "aws-credentials"
	targetNamespace  = "foo-lqmsh-targetns-vxx6f"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestMachineManagementReconcile(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	openshiftapiv1.Install(scheme.Scheme)
	routev1.Install(scheme.Scheme)

	cdBuilder := testcd.FullBuilder(testNamespace, testName, scheme.Scheme)

	getCD := func(c client.Client) *hivev1.ClusterDeployment {
		cd := &hivev1.ClusterDeployment{}
		err := c.Get(context.TODO(), client.ObjectKey{Name: testName, Namespace: testNamespace}, cd)
		if err == nil {
			return cd
		}
		return nil
	}

	getTargetNS := func(c client.Client, name string) *corev1.Namespace {
		ns := &corev1.Namespace{}
		err := c.Get(context.TODO(), client.ObjectKey{Name: name}, ns)
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
				cdBuilder.Build(
					testcd.WithPullSecretRef(pullSecretSecret),
					testcd.WithAWSPlatform(&aws.Platform{CredentialsSecretRef: corev1.LocalObjectReference{Name: credsSecret}}),
					testcd.WithCentralMachineManagement(),
				),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(cdBuilder.Build()), corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, credsSecret, corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				ns := getTargetNS(c, cd.Spec.MachineManagement.TargetNamespace)
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
				cdBuilder.GenericOptions(generic.Deleted()).Build(
					testcd.WithPullSecretRef(pullSecretSecret),
					testcd.WithAWSPlatform(&aws.Platform{CredentialsSecretRef: corev1.LocalObjectReference{Name: credsSecret}}),
					testcd.WithCentralMachineManagement(),
					testcd.WithTargetNamespace(targetNamespace),
					testcd.WithFinalizer(hivev1.FinalizerMachineManagementTargetNamespace),
				),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(cdBuilder.Build()), corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, credsSecret, corev1.DockerConfigJsonKey, "{}"),
				testNs(targetNamespace),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				// cd will be deleted when the finalizer is removed
				assert.Nil(t, cd, "expected cluster deployment to be deleted")
				ns := getTargetNS(c, targetNamespace)
				assert.Nil(t, ns, "expected target namespace to be deleted")
			},
		},
		{
			name: "Changes to secret in cd namespace are synced to target namespace",
			existing: []runtime.Object{
				cdBuilder.Build(
					testcd.WithPullSecretRef(pullSecretSecret),
					testcd.WithAWSPlatform(&aws.Platform{CredentialsSecretRef: corev1.LocalObjectReference{Name: credsSecret}}),
					testcd.WithCentralMachineManagement(),
					testcd.WithTargetNamespace(targetNamespace),
					testcd.WithFinalizer(hivev1.FinalizerMachineManagementTargetNamespace),
				),
				testNs(targetNamespace),
				testSecretWithNamespace(corev1.SecretTypeOpaque, credsSecret, targetNamespace, "username", "test"),
				testSecretWithNamespace(corev1.SecretTypeDockerConfigJson, pullSecretSecret, targetNamespace, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{\"update\": \"updated\"}"),
				testSecret(corev1.SecretTypeOpaque, credsSecret, "username", "updated"),
			},
			validate: func(c client.Client, t *testing.T) {
				secretNames := []string{pullSecretSecret, credsSecret}
				for _, secretName := range secretNames {
					cdSecret := getSecret(c, secretName, testNamespace)
					targetSecret := getSecret(c, secretName, targetNamespace)
					assert.True(t, reflect.DeepEqual(cdSecret.Data, targetSecret.Data))
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logger := log.WithField("controller", "machineManagement")
			fakeClient := fake.NewFakeClient(test.existing...)
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			rmm := &ReconcileMachineManagement{
				Client: fakeClient,
				scheme: scheme.Scheme,
				logger: logger,
			}

			if test.reconcilerSetup != nil {
				test.reconcilerSetup(rmm)
			}

			reconcileRequest := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testName,
					Namespace: testNamespace,
				},
			}

			result, err := rmm.Reconcile(context.TODO(), reconcileRequest)

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
