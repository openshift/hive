package argocdregister

import (
	"context"
	"os"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/pointer"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	"github.com/openshift/hive/pkg/constants"
	testfake "github.com/openshift/hive/pkg/test/fake"
	"github.com/openshift/hive/pkg/util/scheme"
)

const (
	testName              = "foo-lqmsh"
	testClusterName       = "bar"
	testClusterID         = "testFooClusterUUID"
	testInfraID           = "testFooInfraID"
	testNamespace         = "default"
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

	pullSecretSecret = "pull-secret"
	credsSecret      = "aws-credentials"
	targetNamespace  = "foo-lqmsh-targetns-vxx6f"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestArgoCDRegisterReconcile(t *testing.T) {

	getCD := func(c client.Client) *hivev1.ClusterDeployment {
		cd := &hivev1.ClusterDeployment{}
		err := c.Get(context.TODO(), client.ObjectKey{Name: testName, Namespace: testNamespace}, cd)
		if err == nil {
			return cd
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
		reconcilerSetup      func(*ArgoCDRegisterController)
		argoCDEnabled        bool
	}{
		{
			name: "Create ArgoCD cluster secret",
			existing: []runtime.Object{
				testClusterDeployment(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, credsSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, "foo-lqmsh-admin-kubeconfig", "kubeconfig", "{}"),
				testServiceAccount("argocd-server", argoCDDefaultNamespace,
					corev1.ObjectReference{Kind: "Secret",
						Name:      "argocd-token",
						Namespace: argoCDDefaultNamespace}),
				testSecretWithNamespace(corev1.SecretTypeDockerConfigJson, "argocd-token", argoCDDefaultNamespace, "token", "{}"),
			},
			argoCDEnabled: true,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				secretName, _ := getPredictableSecretName(cd.Status.APIURL)
				secret := getSecret(c, secretName, argoCDDefaultNamespace)
				assert.NotNil(t, secret, "ArgoCD cluster secret not found")
				assert.Contains(t, cd.Finalizers, hivev1.FinalizerArgoCDCluster)
			},
		},
		{
			name: "Update ArgoCD cluster secret",
			existing: []runtime.Object{
				testClusterDeployment(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, credsSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, "foo-lqmsh-admin-kubeconfig", "kubeconfig", "{}"),
				testServiceAccount("argocd-server", argoCDDefaultNamespace,
					corev1.ObjectReference{Kind: "Secret",
						Name:      "argocd-token",
						Namespace: argoCDDefaultNamespace}),
				testSecretWithNamespace(corev1.SecretTypeDockerConfigJson, "argocd-token", argoCDDefaultNamespace, "token", "{}"),
				// Existing ArgoCD cluster secret
				testSecretWithNamespace(corev1.SecretTypeDockerConfigJson, "cluster-test-api.test.com-2774145043", argoCDDefaultNamespace, "test", "{}"),
			},
			argoCDEnabled: true,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				secretName, _ := getPredictableSecretName(cd.Status.APIURL)
				secret := getSecret(c, secretName, argoCDDefaultNamespace)
				assert.NotNil(t, secret, "ArgoCD cluster secret not found")
				// Existing secret is empty, expect secret.Data and secret.Labels updated
				assert.Equal(t, secret.Labels[hivev1.HiveClusterPlatformLabel], "aws")
				assert.Equal(t, secret.Data["server"], []byte(cd.Status.APIURL))
			},
		},
		{
			name: "Delete ArgoCD cluster secret",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testDeletedClusterDeployment()
					cd.Finalizers = append(cd.Finalizers, hivev1.FinalizerArgoCDCluster)
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, credsSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, "foo-lqmsh-admin-kubeconfig", "kubeconfig", "{}"),
				testServiceAccount("argocd-server", argoCDDefaultNamespace,
					corev1.ObjectReference{Kind: "Secret",
						Name:      "argocd-token",
						Namespace: argoCDDefaultNamespace}),
				testSecretWithNamespace(corev1.SecretTypeDockerConfigJson, "argocd-token", argoCDDefaultNamespace, "token", "{}"),
				// Existing ArgoCD cluster secret
				testSecretWithNamespace(corev1.SecretTypeDockerConfigJson, "cluster-test-api.test.com-2774145043", argoCDDefaultNamespace, corev1.DockerConfigJsonKey, "{}"),
			},
			argoCDEnabled: true,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				secretName, _ := getPredictableSecretName(cd.Status.APIURL)
				secret := getSecret(c, secretName, argoCDDefaultNamespace)
				assert.Nil(t, secret, "found unexpected ArgoCD cluster secret")
				assert.NotContains(t, cd.Finalizers, hivev1.FinalizerArgoCDCluster, "found unexpected ArgoCD cluster deployment finalizer")
			},
		},
		{
			name: "ArgoCD not enabled in hive config",
			existing: []runtime.Object{
				testClusterDeployment(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, credsSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, "foo-lqmsh-admin-kubeconfig", "kubeconfig", "{}"),
				testServiceAccount("argocd-server", argoCDDefaultNamespace,
					corev1.ObjectReference{Kind: "Secret",
						Name:      "argocd-token",
						Namespace: argoCDDefaultNamespace}),
				testSecretWithNamespace(corev1.SecretTypeDockerConfigJson, "argocd-token", argoCDDefaultNamespace, "token", "{}"),
			},
			argoCDEnabled: false,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				secretName, _ := getPredictableSecretName(cd.Status.APIURL)
				secret := getSecret(c, secretName, argoCDDefaultNamespace)
				assert.Nil(t, secret, "found unexepcted ArgoCD cluster secret")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logger := log.WithField("controller", "argocdregister")
			scheme := scheme.GetScheme()
			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(test.existing...).Build()

			if test.argoCDEnabled {
				os.Setenv(constants.ArgoCDEnvVar, "true")
				t.Log(os.Getenv(constants.ArgoCDEnvVar))
			} else {
				os.Unsetenv(constants.ArgoCDEnvVar)
			}

			rcd := &ArgoCDRegisterController{
				Client:     fakeClient,
				scheme:     scheme,
				logger:     logger,
				restConfig: &rest.Config{},
				tlsClientConfigBuilder: func(kubeConfig clientcmd.ClientConfig, _ log.FieldLogger) (TLSClientConfig, error) {
					return TLSClientConfig{}, nil
				},
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

			result, err := rcd.Reconcile(context.TODO(), reconcileRequest)

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
		Installed: true,
		Provisioning: &hivev1.Provisioning{
			InstallConfigSecretRef: &corev1.LocalObjectReference{Name: "install-config-secret"},
		},
		ClusterMetadata: &hivev1.ClusterMetadata{
			ClusterID:                testClusterID,
			InfraID:                  testInfraID,
			AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: adminKubeconfigSecret},
			AdminPasswordSecretRef:   &corev1.LocalObjectReference{Name: adminPasswordSecret},
		},
	}

	if cd.Labels == nil {
		cd.Labels = make(map[string]string, 2)
	}
	cd.Labels[hivev1.HiveClusterPlatformLabel] = "aws"
	cd.Labels[hivev1.HiveClusterRegionLabel] = "us-east-1"

	cd.Status = hivev1.ClusterDeploymentStatus{
		APIURL:         "http://test-api.test.com",
		InstallerImage: pointer.String("installer-image:latest"),
		CLIImage:       pointer.String("cli:latest"),
	}

	return cd
}

func testDeletedClusterDeployment() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	now := metav1.Now()
	cd.DeletionTimestamp = &now
	return cd
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

func testServiceAccount(name, namespace string, secrets ...corev1.ObjectReference) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: argoCDDefaultNamespace,
		},
		Secrets: secrets,
	}
}
