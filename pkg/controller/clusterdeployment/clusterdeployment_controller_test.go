package clusterdeployment

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	configv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	conditionsv1 "github.com/openshift/custom-resource-status/conditions/v1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	"github.com/openshift/hive/apis/hive/v1/azure"
	"github.com/openshift/hive/apis/hive/v1/baremetal"
	"github.com/openshift/hive/apis/hive/v1/gcp"
	"github.com/openshift/hive/apis/hive/v1/metricsconfig"
	hiveintv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/remoteclient"
	remoteclientmock "github.com/openshift/hive/pkg/remoteclient/mock"
	testassert "github.com/openshift/hive/pkg/test/assert"
	testclusterdeployment "github.com/openshift/hive/pkg/test/clusterdeployment"
	testclusterdeprovision "github.com/openshift/hive/pkg/test/clusterdeprovision"
	tcp "github.com/openshift/hive/pkg/test/clusterprovision"
	testdnszone "github.com/openshift/hive/pkg/test/dnszone"
	testfake "github.com/openshift/hive/pkg/test/fake"
	"github.com/openshift/hive/pkg/util/scheme"
	"github.com/openshift/library-go/pkg/verify"
	"github.com/openshift/library-go/pkg/verify/store"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/openpgp"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	testName                = "foo-lqmsh"
	testClusterName         = "bar"
	testClusterID           = "testFooClusterUUID"
	testInfraID             = "testFooInfraID"
	testFinalizer           = "test-finalizer"
	installConfigSecretName = "install-config-secret"
	provisionName           = "foo-lqmsh-random"
	imageSetJobName         = "foo-lqmsh-imageset"
	testNamespace           = "default"
	testSyncsetInstanceName = "testSSI"
	metadataName            = "foo-lqmsh-metadata"
	pullSecretSecret        = "pull-secret"
	installLogSecret        = "install-log-secret"
	globalPullSecret        = "global-pull-secret"
	adminKubeconfigSecret   = "foo-lqmsh-admin-kubeconfig"
	adminKubeconfig         = `clusters:
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
	credsSecret         = "foo-aws-creds"
	sshKeySecret        = "foo-ssh-key"

	remoteClusterRouteObjectName      = "console"
	remoteClusterRouteObjectNamespace = "openshift-console"
	testClusterImageSetName           = "test-image-set"
)

func init() {
	log.SetLevel(log.DebugLevel)
	// While the metrics need not be registered for this test suite, they still need to be defined to avoid panics
	// during the tests
	registerMetrics(&metricsconfig.MetricsConfig{}, log.WithField("controller", "clusterDeployment"))
}

func fakeReadFile(content string) func(string) ([]byte, error) {
	return func(string) ([]byte, error) {
		return []byte(content), nil
	}
}

func TestClusterDeploymentReconcile(t *testing.T) {
	// Fake out readProvisionFailedConfig
	os.Setenv(constants.FailedProvisionConfigFileEnvVar, "fake")

	// Utility function to get the test CD from the fake client
	getCD := func(c client.Client) *hivev1.ClusterDeployment {
		cd := &hivev1.ClusterDeployment{}
		err := c.Get(context.TODO(), client.ObjectKey{Name: testName, Namespace: testNamespace}, cd)
		if err == nil {
			return cd
		}
		return nil
	}

	getCDC := func(c client.Client) *hivev1.ClusterDeploymentCustomization {
		cdc := &hivev1.ClusterDeploymentCustomization{}
		err := c.Get(context.TODO(), client.ObjectKey{Name: testName, Namespace: testNamespace}, cdc)
		if err == nil {
			return cdc
		}
		return nil
	}

	getDNSZone := func(c client.Client) *hivev1.DNSZone {
		zone := &hivev1.DNSZone{}
		err := c.Get(context.TODO(), client.ObjectKey{Name: testName + "-zone", Namespace: testNamespace}, zone)
		if err == nil {
			return zone
		}
		return nil
	}

	getDeprovision := func(c client.Client) *hivev1.ClusterDeprovision {
		req := &hivev1.ClusterDeprovision{}
		err := c.Get(context.TODO(), client.ObjectKey{Name: testName, Namespace: testNamespace}, req)
		if err == nil {
			return req
		}
		return nil
	}

	getImageSetJob := func(c client.Client) *batchv1.Job {
		return getJob(c, imageSetJobName)
	}

	imageVerifier := testReleaseVerifier{known: sets.NewString("sha256:digest1", "sha256:digest2", "sha256:digest3")}

	tests := []struct {
		name                          string
		existing                      []runtime.Object
		riVerifier                    verify.Interface
		pendingCreation               bool
		expectErr                     bool
		expectExplicitRequeue         bool
		expectedRequeueAfter          time.Duration
		expectPendingCreation         bool
		expectConsoleRouteFetch       bool
		validate                      func(client.Client, *testing.T)
		reconcilerSetup               func(*ReconcileClusterDeployment)
		platformCredentialsValidation func(client.Client, *hivev1.ClusterDeployment, log.FieldLogger) (bool, error)
		retryReasons                  *[]string
	}{
		{
			name: "Initialize conditions",
			existing: []runtime.Object{
				testClusterDeployment(),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				compareCD := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
				testassert.AssertConditions(t, cd, compareCD.Status.Conditions)
			},
		},
		{
			name: "Add finalizer",
			existing: []runtime.Object{
				testClusterDeploymentWithInitializedConditions(testClusterDeploymentWithoutFinalizer()),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if cd == nil || !controllerutils.HasFinalizer(cd, hivev1.FinalizerDeprovision) {
					t.Errorf("did not get expected clusterdeployment finalizer")
				}
			},
		},
		{
			name: "Create provision",
			existing: []runtime.Object{
				testInstallConfigSecretAWS(),
				testClusterDeploymentWithDefaultConditions(testClusterDeploymentWithInitializedConditions(testClusterDeployment())),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			expectPendingCreation: true,
			validate: func(c client.Client, t *testing.T) {
				provisions := getProvisions(c)
				assert.Len(t, provisions, 1, "expected provision to exist")
				testassert.AssertConditions(t, getCD(c), []hivev1.ClusterDeploymentCondition{{
					Type:    hivev1.ProvisionedCondition,
					Status:  corev1.ConditionFalse,
					Reason:  hivev1.ProvisionedReasonProvisioning,
					Message: "Cluster provision created",
				}})
			},
		},
		{
			name: "Provision not created when pending create",
			existing: []runtime.Object{
				testClusterDeployment(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			pendingCreation:       true,
			expectPendingCreation: true,
			validate: func(c client.Client, t *testing.T) {
				provisions := getProvisions(c)
				assert.Empty(t, provisions, "expected provision to not exist")
				testassert.AssertConditions(t, getCD(c), []hivev1.ClusterDeploymentCondition{{
					Type:    hivev1.ProvisionedCondition,
					Status:  corev1.ConditionUnknown,
					Reason:  hivev1.InitializedConditionReason,
					Message: "Condition Initialized",
				}})
			},
		},
		{
			name: "Adopt provision",
			existing: []runtime.Object{
				testInstallConfigSecretAWS(),
				testClusterDeploymentWithInitializedConditions(testClusterDeployment()),
				testProvision(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if assert.NotNil(t, cd, "no clusterdeployment found") {
					if assert.NotNil(t, cd.Status.ProvisionRef, "missing provision ref") {
						assert.Equal(t, provisionName, cd.Status.ProvisionRef.Name, "unexpected provision ref name")
					}
					// When adopting, we don't know the provisioning state until the *next* reconcile when we dig into the ClusterProvision.
					testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{{
						Type:    hivev1.ProvisionedCondition,
						Status:  corev1.ConditionUnknown,
						Reason:  hivev1.InitializedConditionReason,
						Message: "Condition Initialized",
					}})
				}
			},
		},
		{
			name: "Initializing provision",
			existing: []runtime.Object{
				testInstallConfigSecretAWS(),
				testClusterDeploymentWithDefaultConditions(testClusterDeploymentWithInitializedConditions(testClusterDeploymentWithProvision())),
				testProvision(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if assert.NotNil(t, cd, "no clusterdeployment found") {
					e := testClusterDeploymentWithDefaultConditions(testClusterDeploymentWithInitializedConditions(
						testClusterDeploymentWithProvision()))
					e.Status.Conditions = addOrUpdateClusterDeploymentCondition(
						*e,
						hivev1.ProvisionedCondition,
						corev1.ConditionFalse,
						hivev1.ProvisionedReasonProvisioning,
						"Cluster provision initializing",
					)
					sanitizeConditions(e, cd)
					testassert.AssertEqualWhereItCounts(t, e, cd, "unexpected change in clusterdeployment")
				}
				provisions := getProvisions(c)
				if assert.Len(t, provisions, 1, "expected provision to exist") {
					e := testProvision()
					testassert.AssertEqualWhereItCounts(t, e, provisions[0], "unexpected change in provision")
				}
			},
		},
		{
			name: "Parse server URL from admin kubeconfig",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.Installed = true
					cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{
						InfraID:                  "fakeinfra",
						AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: adminKubeconfigSecret},
					}
					cd.Status.Conditions = addOrUpdateClusterDeploymentCondition(*cd, hivev1.UnreachableCondition,
						corev1.ConditionFalse, "test-reason", "test-message")
					return cd
				}(),
				testSecret(corev1.SecretTypeOpaque, adminKubeconfigSecret, "kubeconfig", adminKubeconfig),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testMetadataConfigMap(),
			},
			expectConsoleRouteFetch: true,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				assert.Equal(t, "https://bar-api.clusters.example.com:6443", cd.Status.APIURL)
				assert.Equal(t, "https://bar-api.clusters.example.com:6443/console", cd.Status.WebConsoleURL)
			},
		},
		{
			name: "Add additional CAs to admin kubeconfig",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.Installed = true
					cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{
						InfraID:                  "fakeinfra",
						AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: adminKubeconfigSecret},
					}
					cd.Status.WebConsoleURL = "https://example.com"
					cd.Status.APIURL = "https://example.com"
					return cd
				}(),
				testSecret(corev1.SecretTypeOpaque, adminKubeconfigSecret, "kubeconfig", adminKubeconfig),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testMetadataConfigMap(),
			},
			expectConsoleRouteFetch: false,
			validate: func(c client.Client, t *testing.T) {
				// Ensure the admin kubeconfig secret got a copy of the raw data, indicating that we would have
				// added additional CAs if any were configured.
				akcSecret := &corev1.Secret{}
				err := c.Get(context.TODO(), client.ObjectKey{Name: adminKubeconfigSecret, Namespace: testNamespace},
					akcSecret)
				require.NoError(t, err)
				require.NotNil(t, akcSecret)
				assert.Contains(t, akcSecret.Data, constants.RawKubeconfigSecretKey)
			},
		},
		{
			name: "Completed provision",
			existing: []runtime.Object{
				testInstallConfigSecretAWS(),
				testClusterDeploymentWithInitializedConditions(testClusterDeploymentWithProvision()),
				testSuccessfulProvision(tcp.WithMetadata(`{"aws": {"hostedZoneRole": "account-b-role"}}`)),
				testMetadataConfigMap(),
				testSecret(corev1.SecretTypeOpaque, adminKubeconfigSecret, "kubeconfig", adminKubeconfig),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if assert.NotNil(t, cd, "missing clusterdeployment") {
					assert.True(t, cd.Spec.Installed, "expected cluster to be installed")
					assert.NotContains(t, cd.Annotations, constants.ProtectedDeleteAnnotation, "unexpected protected delete annotation")
					testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{{
						Type:    hivev1.ProvisionedCondition,
						Status:  corev1.ConditionTrue,
						Reason:  hivev1.ProvisionedReasonProvisioned,
						Message: "Cluster is provisioned",
					}})
				}
			},
		},
		{
			name: "Completed provision with protected delete",
			existing: []runtime.Object{
				testInstallConfigSecretAWS(),
				testClusterDeploymentWithInitializedConditions(testClusterDeploymentWithProvision()),
				testSuccessfulProvision(tcp.WithMetadata(`{"aws": {"hostedZoneRole": "account-b-role"}}`)),
				testMetadataConfigMap(),
				testSecret(corev1.SecretTypeOpaque, adminKubeconfigSecret, "kubeconfig", adminKubeconfig),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			reconcilerSetup: func(r *ReconcileClusterDeployment) {
				r.protectedDelete = true
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if assert.NotNil(t, cd, "missing clusterdeployment") {
					assert.True(t, cd.Spec.Installed, "expected cluster to be installed")
					assert.Equal(t, "true", cd.Annotations[constants.ProtectedDeleteAnnotation], "unexpected protected delete annotation")
					testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{{
						Type:    hivev1.ProvisionedCondition,
						Status:  corev1.ConditionTrue,
						Reason:  hivev1.ProvisionedReasonProvisioned,
						Message: "Cluster is provisioned",
					}})
				}
			},
		},
		{
			name: "clusterdeployment must specify pull secret when there is no global pull secret ",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.PullSecretRef = nil
					return cd
				}(),
			},
			expectErr: true,
		},
		{
			name: "Legacy dockercfg pull secret causes no errors once installed",
			existing: []runtime.Object{
				testInstalledClusterDeployment(time.Date(2019, 9, 6, 11, 58, 32, 45, time.UTC)),
				testMetadataConfigMap(),
				testSecret(corev1.SecretTypeOpaque, adminKubeconfigSecret, "kubeconfig", adminKubeconfig),
				testSecret(corev1.SecretTypeOpaque, adminPasswordSecret, "password", adminPassword),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
		},
		{
			name: "No-op deleted cluster was garbage collected",
			existing: []runtime.Object{
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				deprovision := getDeprovision(c)
				if deprovision != nil {
					t.Errorf("got unexpected deprovision request")
				}
			},
		},
		{
			name: "Block deprovision when protected delete on",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
					if cd.Annotations == nil {
						cd.Annotations = make(map[string]string, 1)
					}
					cd.Annotations[constants.ProtectedDeleteAnnotation] = "true"
					now := metav1.Now()
					cd.DeletionTimestamp = &now
					cd.Status.Conditions = addOrUpdateClusterDeploymentCondition(
						*cd,
						hivev1.ProvisionedCondition,
						corev1.ConditionTrue,
						hivev1.ProvisionedReasonProvisioned,
						"Cluster is provisioned",
					)
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				deprovision := getDeprovision(c)
				assert.Nil(t, deprovision, "expected no deprovision request")
				cd := getCD(c)
				assert.Contains(t, cd.Finalizers, hivev1.FinalizerDeprovision, "expected finalizer")
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{{
					Type:    hivev1.ProvisionedCondition,
					Status:  corev1.ConditionTrue,
					Reason:  hivev1.ProvisionedReasonProvisioned,
					Message: "Cluster is provisioned",
				}})
			},
		},
		{
			name: "Skip deprovision for deleted BareMetal cluster",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.Platform.AWS = nil
					cd.Spec.Platform.BareMetal = &baremetal.Platform{}
					cd.Labels[hivev1.HiveClusterPlatformLabel] = "baremetal"
					now := metav1.Now()
					cd.DeletionTimestamp = &now
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				deprovision := getDeprovision(c)
				assert.Nil(t, deprovision, "expected no deprovision request")
				cd := getCD(c)
				assert.Nil(t, cd, "expected ClusterDeployment to be deleted")
			},
		},
		{
			name: "Delete expired cluster deployment",
			existing: []runtime.Object{
				testClusterDeploymentWithInitializedConditions(testExpiredClusterDeployment()),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				assert.NotNil(t, cd, "clusterdeployment deleted unexpectedly")
				assert.Contains(t, cd.Finalizers, hivev1.FinalizerDeprovision, "expected hive finalizer")
			},
		},
		{
			name: "Test PreserveOnDelete when cluster deployment is installed",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testDeletedClusterDeployment())
					cd.Spec.Installed = true
					cd.Spec.PreserveOnDelete = true
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				assert.Nil(t, cd, "expected clusterdeployment to be deleted")
				deprovision := getDeprovision(c)
				assert.Nil(t, deprovision, "expected no deprovision request")
			},
		},
		{
			name: "Test PreserveOnDelete when cluster deployment is not installed",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testDeletedClusterDeployment())
					cd.Spec.PreserveOnDelete = true
					cd.Spec.Installed = false
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				assert.Nil(t, cd, "expected clusterdeployment to be deleted")
				deprovision := getDeprovision(c)
				assert.Nil(t, deprovision, "expected no deprovision request")
			},
		},
		{
			name: "Create job to resolve installer image",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Status.InstallerImage = nil
					cd.Spec.Provisioning.ImageSetRef = &hivev1.ClusterImageSetReference{Name: testClusterImageSetName}
					return cd
				}(),
				testClusterImageSet(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				job := getImageSetJob(c)
				if job == nil {
					t.Errorf("did not find expected imageset job")
				}
				// Ensure that the release image from the imageset is used in the job
				//lint:ignore SA5011 never nil due to test setup
				envVars := job.Spec.Template.Spec.Containers[0].Env
				for _, e := range envVars {
					if e.Name == "RELEASE_IMAGE" {
						if e.Value != testClusterImageSet().Spec.ReleaseImage {
							t.Errorf("unexpected release image used in job: %s", e.Value)
						}
						break
					}
				}

				// Ensure job type labels are set correctly
				require.NotNil(t, job, "expected job")
				//lint:ignore SA5011 never nil due to test setup
				assert.Equal(t, testClusterDeployment().Name, job.Labels[constants.ClusterDeploymentNameLabel], "incorrect cluster deployment name label")
				//lint:ignore SA5011 never nil due to test setup
				assert.Equal(t, constants.JobTypeImageSet, job.Labels[constants.JobTypeLabel], "incorrect job type label")
			},
		},
		{
			name: "Minimal install mode requires installerImageOverride",
			existing: []runtime.Object{
				testInstallConfigSecretAWS(),
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Status.InstallerImage = nil
					if cd.Annotations == nil {
						cd.Annotations = map[string]string{}
					}
					cd.Annotations[constants.MinimalInstallModeAnnotation] = "True"
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				testassert.AssertConditions(t, getCD(c), []hivev1.ClusterDeploymentCondition{
					{
						Type:   hivev1.InstallImagesNotResolvedCondition,
						Status: corev1.ConditionTrue,
						Reason: "MinimalInstallModeWithoutImageOverride",
					},
				})
			},
		},
		{
			name: "Minimal install mode 'resolves' images without an imageset job",
			existing: []runtime.Object{
				testInstallConfigSecretAWS(),
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					// Trigger "images not yet resolved"
					cd.Status.InstallerImage = nil
					if cd.Annotations == nil {
						cd.Annotations = map[string]string{}
					}
					cd.Annotations[constants.MinimalInstallModeAnnotation] = "True"
					cd.Spec.Provisioning.InstallerImageOverride = "test-installer-image:latest"
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				assert.Nil(t, getImageSetJob(c), "expected no imageset job")
				cd := getCD(c)
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{
					{
						Type:   hivev1.InstallImagesNotResolvedCondition,
						Status: corev1.ConditionFalse,
						Reason: "ImagesResolved",
					},
				})
				assert.Equal(t, "test-installer-image:latest", *cd.Status.InstallerImage)
				assert.Equal(t, "<NONE>", *cd.Status.CLIImage)
			},
		},
		{
			name: "Minimal install mode updates InstallImagesNotResolved condition if images already resolved",
			existing: []runtime.Object{
				testInstallConfigSecretAWS(),
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					if cd.Annotations == nil {
						cd.Annotations = map[string]string{}
					}
					cd.Annotations[constants.MinimalInstallModeAnnotation] = "True"
					cd.Spec.Provisioning.InstallerImageOverride = "test-installer-image:latest"
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				assert.Nil(t, getImageSetJob(c), "expected no imageset job")
				cd := getCD(c)
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{
					{
						Type:   hivev1.InstallImagesNotResolvedCondition,
						Status: corev1.ConditionFalse,
						Reason: "ImagesResolved",
					},
				})
				// These are from testClusterDeployment(), not the overrides -- we don't reset them if already set
				assert.Equal(t, "installer-image:latest", *cd.Status.InstallerImage)
				assert.Equal(t, "cli:latest", *cd.Status.CLIImage)
			},
		},
		{
			name: "Minimal install mode skips cli initContainer in provision pod",
			existing: []runtime.Object{
				testInstallConfigSecretAWS(),
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithDefaultConditions(testClusterDeploymentWithInitializedConditions(testClusterDeployment()))
					if cd.Annotations == nil {
						cd.Annotations = map[string]string{}
					}
					cd.Annotations[constants.MinimalInstallModeAnnotation] = "True"
					cd.Spec.Provisioning.InstallerImageOverride = "test-installer-image:latest"
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			expectPendingCreation: true,
			validate: func(c client.Client, t *testing.T) {
				provisions := getProvisions(c)
				if assert.Len(t, provisions, 1, "expected exactly one ClusterProvision") {
					podSpec := provisions[0].Spec.PodSpec
					if assert.Len(t, podSpec.InitContainers, 1, "expected exactly one initContainer") {
						assert.Equal(t, "installer", podSpec.InitContainers[0].Name, "expected the initContainer to be 'installer'")
					}
				}
			},
		},
		{
			name: "failed verification of release image using tags should set InstallImagesNotResolvedCondition",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Status.InstallerImage = nil
					cd.Spec.Provisioning.ImageSetRef = &hivev1.ClusterImageSetReference{Name: testClusterImageSetName}
					return cd
				}(),
				testClusterImageSet(),
				testCompletedFailedImageSetJob(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			riVerifier: verify.Reject,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{
					{
						Type:   hivev1.InstallImagesNotResolvedCondition,
						Status: corev1.ConditionTrue,
						Reason: "ReleaseImageVerificationFailed",
					},
				})
			},
		},
		{
			name: "failed verification of release image using unknown digest should set InstallImagesNotResolvedCondition",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Status.InstallerImage = nil
					cd.Spec.Provisioning.ReleaseImage = "test-image@sha256:unknowndigest1"
					return cd
				}(),
				testClusterImageSet(),
				testCompletedFailedImageSetJob(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			riVerifier: imageVerifier,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{
					{
						Type:   hivev1.InstallImagesNotResolvedCondition,
						Status: corev1.ConditionTrue,
						Reason: "ReleaseImageVerificationFailed",
					},
				})
			},
		},
		{
			name: "Create job to resolve installer image using verified image",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Status.InstallerImage = nil
					cd.Spec.Provisioning.ReleaseImage = "test-image@sha256:digest1"
					return cd
				}(),
				testClusterImageSet(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			riVerifier: imageVerifier,
			validate: func(c client.Client, t *testing.T) {
				job := getImageSetJob(c)
				if job == nil {
					t.Errorf("did not find expected imageset job")
				}
				// Ensure that the release image from the imageset is used in the job
				//lint:ignore SA5011 never nil due to test setup
				envVars := job.Spec.Template.Spec.Containers[0].Env
				for _, e := range envVars {
					if e.Name == "RELEASE_IMAGE" {
						if e.Value != "test-image@sha256:digest1" {
							t.Errorf("unexpected release image used in job: %s", e.Value)
						}
						break
					}
				}

				// Ensure job type labels are set correctly
				require.NotNil(t, job, "expected job")
				//lint:ignore SA5011 never nil due to test setup
				assert.Equal(t, testClusterDeployment().Name, job.Labels[constants.ClusterDeploymentNameLabel], "incorrect cluster deployment name label")
				//lint:ignore SA5011 never nil due to test setup
				assert.Equal(t, constants.JobTypeImageSet, job.Labels[constants.JobTypeLabel], "incorrect job type label")
			},
		},
		{
			name: "failed image should set InstallImagesNotResolved condition on clusterdeployment",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Status.InstallerImage = nil
					cd.Spec.Provisioning.ImageSetRef = &hivev1.ClusterImageSetReference{Name: testClusterImageSetName}
					return cd
				}(),
				testClusterImageSet(),
				testCompletedFailedImageSetJob(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{
					{
						Type:   hivev1.InstallImagesNotResolvedCondition,
						Status: corev1.ConditionTrue,
						Reason: "JobToResolveImagesFailed",
					},
				})
			},
		},
		{
			name: "clear InstallImagesNotResolved condition on success",
			existing: []runtime.Object{
				testInstallConfigSecretAWS(),
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithDefaultConditions(testClusterDeploymentWithInitializedConditions(testClusterDeployment()))
					cd.Status.InstallerImage = pointer.String("test-installer-image")
					cd.Status.CLIImage = pointer.String("test-cli-image")
					cd.Spec.Provisioning.ImageSetRef = &hivev1.ClusterImageSetReference{Name: testClusterImageSetName}
					cd.Status.Conditions = addOrUpdateClusterDeploymentCondition(*cd, hivev1.InstallImagesNotResolvedCondition,
						corev1.ConditionTrue, "test-reason", "test-message")
					return cd
				}(),
				testClusterImageSet(),
				testCompletedImageSetJob(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			expectPendingCreation: true,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				testassert.AssertConditionStatus(t, cd, hivev1.InstallImagesNotResolvedCondition, corev1.ConditionFalse)
			},
		},
		{
			name: "Delete imageset job when complete",
			existing: []runtime.Object{
				testInstallConfigSecretAWS(),
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithDefaultConditions(testClusterDeploymentWithInitializedConditions(testClusterDeployment()))
					cd.Status.InstallerImage = pointer.String("test-installer-image")
					cd.Status.CLIImage = pointer.String("test-cli-image")
					cd.Spec.Provisioning.ImageSetRef = &hivev1.ClusterImageSetReference{Name: testClusterImageSetName}
					return cd
				}(),
				testClusterImageSet(),
				testCompletedImageSetJob(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			expectPendingCreation: true,
			validate: func(c client.Client, t *testing.T) {
				job := getImageSetJob(c)
				assert.Nil(t, job, "expected imageset job to be deleted")
			},
		},
		{
			name: "Ensure release image from clusterdeployment (when present) is used to generate imageset job",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Status.InstallerImage = nil
					cd.Spec.Provisioning.ReleaseImage = "embedded-release-image:latest"
					cd.Spec.Provisioning.ImageSetRef = &hivev1.ClusterImageSetReference{Name: testClusterImageSetName}
					return cd
				}(),
				testClusterImageSet(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				job := getImageSetJob(c)
				if job == nil {
					t.Errorf("did not find expected imageset job")
				}
				//lint:ignore SA5011 never nil due to test setup
				envVars := job.Spec.Template.Spec.Containers[0].Env
				for _, e := range envVars {
					if e.Name == "RELEASE_IMAGE" {
						if e.Value != "embedded-release-image:latest" {
							t.Errorf("unexpected release image used in job: %s", e.Value)
						}
						break
					}
				}
			},
		},
		{
			name: "Ensure release image from clusterimageset is used as override image in install job",
			existing: []runtime.Object{
				testInstallConfigSecretAWS(),
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithDefaultConditions(testClusterDeploymentWithInitializedConditions(testClusterDeployment()))
					cd.Status.InstallerImage = pointer.String("test-installer-image:latest")
					cd.Spec.Provisioning.ImageSetRef = &hivev1.ClusterImageSetReference{Name: testClusterImageSetName}
					return cd
				}(),
				func() *hivev1.ClusterImageSet {
					cis := testClusterImageSet()
					cis.Spec.ReleaseImage = "test-release-image:latest"
					return cis
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			expectPendingCreation: true,
			validate: func(c client.Client, t *testing.T) {
				provisions := getProvisions(c)
				if assert.Len(t, provisions, 1, "expected provision to exist") {
					env := provisions[0].Spec.PodSpec.Containers[0].Env
					variable := corev1.EnvVar{}
					found := false
					for _, e := range env {
						if e.Name == "OPENSHIFT_INSTALL_RELEASE_IMAGE_OVERRIDE" {
							variable = e
							found = true
							break
						}
					}
					if !found {
						t.Errorf("did not find expected override environment variable in job")
						return
					}
					if variable.Value != "test-release-image:latest" {
						t.Errorf("environment variable did not have the expected value. actual: %s", variable.Value)
					}
				}
			},
		},
		{
			name: "Create DNSZone when manageDNS is true",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.ManageDNS = true
					cd.Spec.PreserveOnDelete = true
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				zone := getDNSZone(c)
				require.NotNil(t, zone, "dns zone should exist")
				assert.Equal(t, testClusterDeployment().Name, zone.Labels[constants.ClusterDeploymentNameLabel], "incorrect cluster deployment name label")
				assert.Equal(t, constants.DNSZoneTypeChild, zone.Labels[constants.DNSZoneTypeLabel], "incorrect dnszone type label")
				assert.True(t, zone.Spec.PreserveOnDelete, "PreserveOnDelete did not transfer to DNSZone")
			},
		},
		{
			name: "Get AWS HostedZoneRole from provision metadata",
			existing: []runtime.Object{
				testInstallConfigSecret(`
platform:
  aws:
    region: us-east-1
`),
				testClusterDeploymentWithDefaultConditions(testClusterDeploymentWithInitializedConditions(testClusterDeploymentWithProvision())),
				testSuccessfulProvision(tcp.WithMetadata(`{"aws": {"hostedZoneRole": "some-role"}}`)),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				if cd := getCD(c); assert.NotNil(t, cd, "missing clusterdeployment") {
					if hzr := controllerutils.AWSHostedZoneRole(cd); assert.NotNil(t, hzr, "expected HostedZoneRole not to be nil") {
						assert.Equal(t, "some-role", *hzr, "incorrect value for AWS HostedZoneRole")
					}
				}
			},
		},
		{
			name: "Get GCP NetworkProjectID from provision metadata",
			existing: []runtime.Object{
				testInstallConfigSecret(`
platform:
  gcp:
    region: us-central1
`),
				func() *hivev1.ClusterDeployment {
					baseCD := testClusterDeploymentWithProvision()
					baseCD.Labels[hivev1.HiveClusterPlatformLabel] = "gcp"
					baseCD.Labels[hivev1.HiveClusterRegionLabel] = "us-central1"
					baseCD.Spec.Platform.AWS = nil
					baseCD.Spec.Platform.GCP = &gcp.Platform{
						Region: "us-central1",
					}
					return testClusterDeploymentWithDefaultConditions(testClusterDeploymentWithInitializedConditions(baseCD))
				}(),
				testSuccessfulProvision(tcp.WithMetadata(`{"gcp": {"networkProjectID": "some@proj.id"}}`)),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				if cd := getCD(c); assert.NotNil(t, cd, "missing clusterdeployment") {
					if npid := controllerutils.GCPNetworkProjectID(cd); assert.NotNil(t, npid, "expected NetworkProjectID not to be nil") {
						assert.Equal(t, "some@proj.id", *npid, "incorrect value for GCP NetworkProjectID")
					}
				}
			},
		},
		{
			name: "Get Azure ResourceGroupName from provision metadata",
			existing: []runtime.Object{
				testInstallConfigSecret(`
platform:
  azure:
    region: az-region
`),
				func() *hivev1.ClusterDeployment {
					baseCD := testClusterDeploymentWithProvision()
					baseCD.Labels[hivev1.HiveClusterPlatformLabel] = "azure"
					baseCD.Labels[hivev1.HiveClusterRegionLabel] = "az-region"
					baseCD.Spec.Platform.AWS = nil
					baseCD.Spec.Platform.Azure = &azure.Platform{
						Region: "az-region",
					}
					return testClusterDeploymentWithDefaultConditions(testClusterDeploymentWithInitializedConditions(baseCD))
				}(),
				testSuccessfulProvision(tcp.WithMetadata(`{"azure": {"resourceGroupName": "infra-id-rg"}}`)),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				if cd := getCD(c); assert.NotNil(t, cd, "missing clusterdeployment") {
					rg, err := controllerutils.AzureResourceGroup(cd)
					if assert.Nil(t, err, "expected to find Azure resource group in CD") {
						assert.Equal(t, "infra-id-rg", rg, "mismatched resource group name")
					}
				}
			},
		},
		{
			name: "Default Azure ResourceGroupName from provision metadata",
			existing: []runtime.Object{
				testInstallConfigSecret(`
platform:
  azure:
    region: az-region
`),
				func() *hivev1.ClusterDeployment {
					baseCD := testClusterDeploymentWithProvision()
					baseCD.Labels[hivev1.HiveClusterPlatformLabel] = "azure"
					baseCD.Labels[hivev1.HiveClusterRegionLabel] = "az-region"
					baseCD.Spec.Platform.AWS = nil
					baseCD.Spec.Platform.Azure = &azure.Platform{
						Region: "az-region",
					}
					return testClusterDeploymentWithDefaultConditions(testClusterDeploymentWithInitializedConditions(baseCD))
				}(),
				testSuccessfulProvision(tcp.WithMetadata(`{"infraID": "the-infra-id", "azure": {}}`)),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				if cd := getCD(c); assert.NotNil(t, cd, "missing clusterdeployment") {
					rg, err := controllerutils.AzureResourceGroup(cd)
					if assert.Nil(t, err, "expected to find Azure resource group in CD") {
						assert.Equal(t, "the-infra-id-rg", rg, "mismatched resource group name")
					}
				}
			},
		},
		{
			name: "Create DNSZone with Azure CloudName",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					baseCD := testClusterDeployment()
					baseCD.Labels[hivev1.HiveClusterPlatformLabel] = "azure"
					baseCD.Labels[hivev1.HiveClusterRegionLabel] = "usgovvirginia"
					baseCD.Spec.Platform.AWS = nil
					baseCD.Spec.Platform.Azure = &azure.Platform{
						CredentialsSecretRef: corev1.LocalObjectReference{
							Name: "azure-credentials",
						},
						Region:                      "usgovvirginia",
						CloudName:                   azure.USGovernmentCloud,
						BaseDomainResourceGroupName: "os4-common",
					}
					baseCD.Spec.ManageDNS = true
					baseCD.Spec.PreserveOnDelete = true
					// Pre-set the Azure resource group so the Reconcile doesn't short out setting it
					baseCD.Spec.ClusterMetadata = &hivev1.ClusterMetadata{
						Platform: &hivev1.ClusterPlatformMetadata{
							Azure: &azure.Metadata{
								ResourceGroupName: pointer.String("infra-id-rg"),
							},
						},
					}
					return testClusterDeploymentWithInitializedConditions(baseCD)
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				zone := getDNSZone(c)
				require.NotNil(t, zone, "dns zone should exist")
				assert.Equal(t, azure.USGovernmentCloud, zone.Spec.Azure.CloudName, "CloudName did not transfer to DNSZone")
			},
		},
		{
			name: "Create DNSZone without Azure CloudName",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					baseCD := testClusterDeployment()
					baseCD.Labels[hivev1.HiveClusterPlatformLabel] = "azure"
					baseCD.Labels[hivev1.HiveClusterRegionLabel] = "eastus"
					baseCD.Spec.Platform.AWS = nil
					baseCD.Spec.Platform.Azure = &azure.Platform{
						CredentialsSecretRef: corev1.LocalObjectReference{
							Name: "azure-credentials",
						},
						Region:                      "eastus",
						BaseDomainResourceGroupName: "os4-common",
					}
					baseCD.Spec.ManageDNS = true
					baseCD.Spec.PreserveOnDelete = true
					// Pre-set the Azure resource group so the Reconcile doesn't short out setting it
					baseCD.Spec.ClusterMetadata = &hivev1.ClusterMetadata{
						Platform: &hivev1.ClusterPlatformMetadata{
							Azure: &azure.Metadata{
								ResourceGroupName: pointer.String("infra-id-rg"),
							},
						},
					}
					return testClusterDeploymentWithInitializedConditions(baseCD)
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				zone := getDNSZone(c)
				require.NotNil(t, zone, "dns zone should exist")
				assert.Equal(t, azure.CloudEnvironment(""), zone.Spec.Azure.CloudName, "CloudName incorrectly set for DNSZone")
			},
		},
		{
			name: "Update DNSZone when PreserveOnDelete changes",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.ManageDNS = true
					cd.Spec.PreserveOnDelete = true
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testDNSZone(),
			},
			validate: func(c client.Client, t *testing.T) {
				zone := getDNSZone(c)
				require.NotNil(t, zone, "dns zone should exist")
				assert.True(t, zone.Spec.PreserveOnDelete, "PreserveOnDelete was not updated")
			},
		},
		{
			name: "Update DNSZone when PreserveOnDelete changes on deleted cluster",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.ManageDNS = true
					cd.Spec.PreserveOnDelete = true
					now := metav1.Now()
					cd.DeletionTimestamp = &now
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testDNSZoneWithFinalizer(),
			},
			validate: func(c client.Client, t *testing.T) {
				zone := getDNSZone(c)
				require.NotNil(t, zone, "dns zone should exist")
				assert.True(t, zone.Spec.PreserveOnDelete, "PreserveOnDelete was not updated")
			},
		},
		{
			name: "Update DNSZone when PreserveOnDelete changes for installed cluster",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testInstalledClusterDeployment(time.Now()))
					cd.Spec.ManageDNS = true
					cd.Spec.PreserveOnDelete = true
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testAvailableDNSZone(),
			},
			validate: func(c client.Client, t *testing.T) {
				zone := getDNSZone(c)
				require.NotNil(t, zone, "dns zone should exist")
				assert.True(t, zone.Spec.PreserveOnDelete, "PreserveOnDelete was not updated")
			},
		},
		{
			name: "Update DNSZone when PreserveOnDelete changes for installed deleted cluster",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testInstalledClusterDeployment(time.Now()))
					cd.Spec.ManageDNS = true
					cd.Spec.PreserveOnDelete = true
					now := metav1.Now()
					cd.DeletionTimestamp = &now
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testAvailableDNSZone(),
			},
			validate: func(c client.Client, t *testing.T) {
				zone := getDNSZone(c)
				require.NotNil(t, zone, "dns zone should exist")
				assert.True(t, zone.Spec.PreserveOnDelete, "PreserveOnDelete was not updated")
			},
		},
		{
			name: "DNSZone is not available yet",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.ManageDNS = true
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testDNSZone(),
			},
			expectExplicitRequeue: true,
			expectedRequeueAfter:  defaultDNSNotReadyTimeout,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{
					// DNS not ready
					{
						Type:   hivev1.DNSNotReadyCondition,
						Status: corev1.ConditionTrue,
						Reason: dnsNotReadyReason,
					},
					// Still provisioning
					{
						Type:   hivev1.ProvisionedCondition,
						Status: corev1.ConditionUnknown,
						Reason: "Initialized",
					},
					// Didn't stop provisioning
					{
						Type:   hivev1.ProvisionStoppedCondition,
						Status: corev1.ConditionUnknown,
						Reason: "Initialized",
					},
				})
				provisions := getProvisions(c)
				assert.Empty(t, provisions, "provision should not exist")
			},
		},
		{
			name: "DNSZone is not available: requeue time",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.ManageDNS = true
					// Pretend DNSNotReady happened 4m ago
					cond := controllerutils.FindCondition(cd.Status.Conditions, hivev1.DNSNotReadyCondition)
					cond.Status = corev1.ConditionTrue
					cond.Reason = dnsNotReadyReason
					cond.Message = "DNS Zone not yet available"
					cond.LastTransitionTime.Time = time.Now().Add(-4 * time.Minute)
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testDNSZone(),
			},
			expectExplicitRequeue: true,
			expectedRequeueAfter:  defaultDNSNotReadyTimeout - (4 * time.Minute),
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{
					// DNS not ready
					{
						Type:   hivev1.DNSNotReadyCondition,
						Status: corev1.ConditionTrue,
						Reason: dnsNotReadyReason,
					},
					// Still provisioning
					{
						Type:   hivev1.ProvisionedCondition,
						Status: corev1.ConditionUnknown,
						Reason: "Initialized",
					},
					// Didn't stop provisioning
					{
						Type:   hivev1.ProvisionStoppedCondition,
						Status: corev1.ConditionUnknown,
						Reason: "Initialized",
					},
				})
				provisions := getProvisions(c)
				assert.Empty(t, provisions, "provision should not exist")
			},
		},
		{
			name: "DNSZone timeout",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.ManageDNS = true
					// Pretend DNSNotReady happened >10m ago
					cond := controllerutils.FindCondition(cd.Status.Conditions, hivev1.DNSNotReadyCondition)
					cond.Status = corev1.ConditionTrue
					cond.Reason = dnsNotReadyReason
					cond.Message = "DNS Zone not yet available"
					cond.LastTransitionTime.Time = time.Now().Add(-11 * time.Minute)
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testDNSZone(),
			},
			expectExplicitRequeue: true,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{
					// DNS timed out
					{
						Type:   hivev1.DNSNotReadyCondition,
						Status: corev1.ConditionTrue,
						Reason: dnsNotReadyTimedoutReason,
					},
					// Not provisioned
					{
						Type:   hivev1.ProvisionedCondition,
						Status: corev1.ConditionFalse,
						Reason: hivev1.ProvisionedReasonProvisionStopped,
					},
					// Provision stopped
					{
						Type:   hivev1.ProvisionStoppedCondition,
						Status: corev1.ConditionTrue,
						Reason: dnsNotReadyTimedoutReason,
					},
				})
				provisions := getProvisions(c)
				assert.Empty(t, provisions, "provision should not exist")
			},
		},
		{
			name: "Set condition and stop when DNSZone cannot be created due to credentials missing permissions",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.ManageDNS = true
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testDNSZoneWithInvalidCredentialsCondition(),
			},
			expectExplicitRequeue: true,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{
					// Insufficient credentials
					{
						Type:   hivev1.DNSNotReadyCondition,
						Status: corev1.ConditionTrue,
						Reason: "InsufficientCredentials",
					},
					// Provision stopped
					{
						Type:   hivev1.ProvisionedCondition,
						Status: corev1.ConditionFalse,
						Reason: hivev1.ProvisionedReasonProvisionStopped,
					},
					// Not provisioned
					{
						Type:   hivev1.ProvisionStoppedCondition,
						Status: corev1.ConditionTrue,
						Reason: "DNSZoneFailedToReconcile",
					},
				})
				provisions := getProvisions(c)
				assert.Empty(t, provisions, "provision should not exist")
			},
		},
		{
			name: "No-op when DNSZone cannot be created due to credentials missing permissions",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.ManageDNS = true
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testDNSZoneWithInvalidCredentialsCondition(),
			},
			expectExplicitRequeue: true,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{
					// Insufficient credentials
					{
						Type:   hivev1.DNSNotReadyCondition,
						Status: corev1.ConditionTrue,
						Reason: "InsufficientCredentials",
					},
					// Provision stopped
					{
						Type:   hivev1.ProvisionedCondition,
						Status: corev1.ConditionFalse,
						Reason: hivev1.ProvisionedReasonProvisionStopped,
					},
					// Not provisioned
					{
						Type:   hivev1.ProvisionStoppedCondition,
						Status: corev1.ConditionTrue,
						Reason: "DNSZoneFailedToReconcile",
					},
				})
				provisions := getProvisions(c)
				assert.Empty(t, provisions, "provision should not exist")
			},
		},
		{
			name: "Set condition and stop when DNSZone cannot be created due to api opt-in required for DNS apis",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.ManageDNS = true
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testDNSZoneWithAPIOptInRequiredCondition(),
			},
			expectExplicitRequeue: true,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{
					// API opt-in required
					{
						Type:   hivev1.DNSNotReadyCondition,
						Status: corev1.ConditionTrue,
						Reason: "APIOptInRequiredForDNS",
					},
					// Provision stopped
					{
						Type:   hivev1.ProvisionedCondition,
						Status: corev1.ConditionFalse,
						Reason: hivev1.ProvisionedReasonProvisionStopped,
					},
					// Not provisioned
					{
						Type:   hivev1.ProvisionStoppedCondition,
						Status: corev1.ConditionTrue,
						Reason: "DNSZoneFailedToReconcile",
					},
				})
			},
		},
		{
			name: "Set condition and stop when DNSZone cannot be created due to authentication failure",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.ManageDNS = true
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testDNSZoneWithAuthenticationFailureCondition(),
			},
			expectExplicitRequeue: true,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{
					// Auth failure
					{
						Type:   hivev1.DNSNotReadyCondition,
						Status: corev1.ConditionTrue,
						Reason: "AuthenticationFailure",
					},
					// Provision stopped
					{
						Type:   hivev1.ProvisionedCondition,
						Status: corev1.ConditionFalse,
						Reason: hivev1.ProvisionedReasonProvisionStopped,
					},
					// Not provisioned
					{
						Type:   hivev1.ProvisionStoppedCondition,
						Status: corev1.ConditionTrue,
						Reason: "DNSZoneFailedToReconcile",
					},
				})
			},
		},
		{
			name: "Set condition when DNSZone cannot be created due to a cloud error",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.ManageDNS = true
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testDNSZoneWithDNSErrorCondition(),
			},
			expectExplicitRequeue: true,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{
					// DNS not ready
					{
						Type:    hivev1.DNSNotReadyCondition,
						Status:  corev1.ConditionTrue,
						Reason:  "CloudError",
						Message: "Some cloud error occurred",
					},
					// Still provisioning
					{
						Type:   hivev1.ProvisionedCondition,
						Status: corev1.ConditionUnknown,
						Reason: "Initialized",
					},
					// Didn't stop provisioning
					{
						Type:   hivev1.ProvisionStoppedCondition,
						Status: corev1.ConditionUnknown,
						Reason: "Initialized",
					},
				})
			},
		},
		{
			name: "Clear condition when DNSZone is available",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.ManageDNS = true
					cd.Status.Conditions = controllerutils.SetClusterDeploymentCondition(
						cd.Status.Conditions,
						hivev1.DNSNotReadyCondition,
						corev1.ConditionTrue,
						"no reason",
						"no message",
						controllerutils.UpdateConditionIfReasonOrMessageChange)
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testAvailableDNSZone(),
			},
			expectExplicitRequeue: true,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				testassert.AssertConditionStatus(t, cd, hivev1.DNSNotReadyCondition, corev1.ConditionFalse)
			},
		},
		{
			name: "Do not use unowned DNSZone",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.ManageDNS = true
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				func() *hivev1.DNSZone {
					zone := testDNSZone()
					zone.OwnerReferences = []metav1.OwnerReference{}
					return zone
				}(),
			},
			expectErr: true,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if assert.NotNil(t, cd, "missing clusterdeployment") {
					cond := controllerutils.FindCondition(cd.Status.Conditions, hivev1.DNSNotReadyCondition)
					if assert.NotNil(t, cond, "expected to find condition") {
						assert.Equal(t, corev1.ConditionTrue, cond.Status, "unexpected condition status")
						assert.Equal(t, "Existing DNS zone not owned by cluster deployment", cond.Message, "unexpected condition message")
					}
				}
				zone := getDNSZone(c)
				assert.NotNil(t, zone, "expected DNSZone to exist")
			},
		},
		{
			name: "Do not use DNSZone owned by other clusterdeployment",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.ManageDNS = true
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				func() *hivev1.DNSZone {
					zone := testDNSZone()
					zone.OwnerReferences[0].UID = "other-uid"
					return zone
				}(),
			},
			expectErr: true,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if assert.NotNil(t, cd, "missing clusterdeployment") {
					cond := controllerutils.FindCondition(cd.Status.Conditions, hivev1.DNSNotReadyCondition)
					if assert.NotNil(t, cond, "expected to find condition") {
						assert.Equal(t, corev1.ConditionTrue, cond.Status, "unexpected condition status")
						assert.Equal(t, "Existing DNS zone not owned by cluster deployment", cond.Message, "unexpected condition message")
					}
				}
				zone := getDNSZone(c)
				assert.NotNil(t, zone, "expected DNSZone to exist")
			},
		},
		{
			name: "Create provision when DNSZone is ready",
			existing: []runtime.Object{
				testInstallConfigSecretAWS(),
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithDefaultConditions(
						testClusterDeploymentWithInitializedConditions(testClusterDeployment()))
					cd.Spec.ManageDNS = true
					cd.Annotations = map[string]string{dnsReadyAnnotation: "NOW"}
					controllerutils.SetClusterDeploymentCondition(
						cd.Status.Conditions,
						hivev1.DNSNotReadyCondition,
						corev1.ConditionFalse,
						dnsReadyReason,
						"DNS Zone available",
						controllerutils.UpdateConditionAlways,
					)
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testAvailableDNSZone(),
			},
			expectPendingCreation: true,
			validate: func(c client.Client, t *testing.T) {
				provisions := getProvisions(c)
				assert.Len(t, provisions, 1, "expected provision to exist")
				testassert.AssertConditions(t, getCD(c), []hivev1.ClusterDeploymentCondition{{
					Type:    hivev1.ProvisionedCondition,
					Status:  corev1.ConditionFalse,
					Reason:  hivev1.ProvisionedReasonProvisioning,
					Message: "Cluster provision created",
				}})
			},
		},
		{
			name: "Set DNS delay metric",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.ManageDNS = true
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testAvailableDNSZone(),
			},
			expectExplicitRequeue: true,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				assert.NotNil(t, cd.Annotations, "annotations should be set on clusterdeployment")
				assert.Contains(t, cd.Annotations, dnsReadyAnnotation)
			},
		},
		{
			name: "Ensure managed DNSZone is deleted with cluster deployment",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testDeletedClusterDeployment())
					cd.Spec.ManageDNS = true
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testDNSZone(),
			},
			validate: func(c client.Client, t *testing.T) {
				dnsZone := getDNSZone(c)
				assert.Nil(t, dnsZone, "dnsZone should not exist")
			},
		},
		{
			name: "Delete cluster deployment with missing clusterimageset",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testDeletedClusterDeployment())
					cd.Spec.Provisioning.ImageSetRef = &hivev1.ClusterImageSetReference{Name: testClusterImageSetName}
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				deprovision := getDeprovision(c)
				assert.NotNil(t, deprovision, "expected deprovision request to be created")
			},
		},
		{
			name: "Delete old provisions",
			existing: []runtime.Object{
				testInstallConfigSecretAWS(),
				func() runtime.Object {
					cd := testClusterDeploymentWithDefaultConditions(testClusterDeploymentWithInitializedConditions(testClusterDeployment()))
					cd.Status.InstallRestarts = 4
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testProvision(tcp.Failed(), tcp.Attempt(0)),
				testProvision(tcp.Failed(), tcp.Attempt(1)),
				testProvision(tcp.Failed(), tcp.Attempt(2)),
				testProvision(tcp.Failed(), tcp.Attempt(3)),
			},
			expectPendingCreation: true,
			validate: func(c client.Client, t *testing.T) {
				actualAttempts := []int{}
				for _, p := range getProvisions(c) {
					actualAttempts = append(actualAttempts, p.Spec.Attempt)
				}
				expectedAttempts := []int{0, 2, 3, 4}
				assert.ElementsMatch(t, expectedAttempts, actualAttempts, "unexpected provisions kept")
			},
		},
		{
			name: "Do not adopt failed provision",
			existing: []runtime.Object{
				testInstallConfigSecretAWS(),
				testClusterDeploymentWithDefaultConditions(testClusterDeploymentWithInitializedConditions(testClusterDeployment())),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testProvision(tcp.Failed(), tcp.Attempt(0)),
			},
			expectPendingCreation: true,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if assert.NotNil(t, cd, "missing cluster deployment") {
					assert.Nil(t, cd.Status.ProvisionRef, "expected provision reference to not be set")
					// This code path creates a new provision, but it doesn't get adopted until the *next* reconcile
					testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{{
						Type:    hivev1.ProvisionedCondition,
						Status:  corev1.ConditionFalse,
						Reason:  hivev1.ProvisionedReasonProvisioning,
						Message: "Cluster provision created",
					}})
				}
			},
		},
		{
			name: "Delete-after requeue",
			existing: []runtime.Object{
				testInstallConfigSecretAWS(),
				func() runtime.Object {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeploymentWithProvision())
					cd.CreationTimestamp = metav1.Now()
					if cd.Annotations == nil {
						cd.Annotations = make(map[string]string, 1)
					}
					cd.Annotations[deleteAfterAnnotation] = "8h"
					return cd
				}(),
				testProvision(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			expectExplicitRequeue: true,
			expectedRequeueAfter:  8 * time.Hour,
		},
		{
			name: "Wait after failed provision",
			existing: []runtime.Object{
				testInstallConfigSecretAWS(),
				func() runtime.Object {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeploymentWithProvision())
					cd.CreationTimestamp = metav1.Now()
					if cd.Annotations == nil {
						cd.Annotations = make(map[string]string, 1)
					}
					cd.Annotations[deleteAfterAnnotation] = "8h"
					return cd
				}(),
				testProvision(tcp.WithFailureTime(time.Now())),
				testMetadataConfigMap(),
				testSecret(corev1.SecretTypeOpaque, adminKubeconfigSecret, "kubeconfig", adminKubeconfig),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			expectedRequeueAfter: 1 * time.Minute,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if assert.NotNil(t, cd, "missing clusterdeployment") {
					if assert.NotNil(t, cd.Status.ProvisionRef, "missing provision ref") {
						assert.Equal(t, provisionName, cd.Status.ProvisionRef.Name, "unexpected provision ref name")
					}
					testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{{
						Type:    hivev1.ProvisionedCondition,
						Status:  corev1.ConditionUnknown,
						Reason:  hivev1.InitializedConditionReason,
						Message: "Condition Initialized",
					}})
				}
			},
		},
		{
			name: "Clear out provision after wait time",
			existing: []runtime.Object{
				testInstallConfigSecretAWS(),
				testClusterDeploymentWithInitializedConditions(testClusterDeploymentWithProvision()),
				testProvision(tcp.WithFailureTime(time.Now().Add(-2 * time.Minute))),
				testMetadataConfigMap(),
				testSecret(corev1.SecretTypeOpaque, adminKubeconfigSecret, "kubeconfig", adminKubeconfig),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if assert.NotNil(t, cd, "missing clusterdeployment") {
					assert.Nil(t, cd.Status.ProvisionRef, "expected empty provision ref")
					assert.Equal(t, 1, cd.Status.InstallRestarts, "expected incremented install restart count")
				}
			},
		},
		{
			name: "Delete outstanding provision on delete",
			existing: []runtime.Object{
				func() runtime.Object {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeploymentWithProvision())
					now := metav1.Now()
					cd.DeletionTimestamp = &now
					cd.Status.Conditions = addOrUpdateClusterDeploymentCondition(
						*cd,
						hivev1.ProvisionedCondition,
						corev1.ConditionTrue,
						hivev1.ProvisionedReasonProvisioned,
						"Cluster is provisioned",
					)
					return cd
				}(),
				testProvision(),
				testSecret(corev1.SecretTypeOpaque, adminKubeconfigSecret, "kubeconfig", adminKubeconfig),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			expectedRequeueAfter: defaultRequeueTime,
			validate: func(c client.Client, t *testing.T) {
				provisions := getProvisions(c)
				assert.Empty(t, provisions, "expected provision to be deleted")
				deprovision := getDeprovision(c)
				assert.Nil(t, deprovision, "expect not to create deprovision request until provision removed")
				testassert.AssertConditions(t, getCD(c), []hivev1.ClusterDeploymentCondition{{
					Type:    hivev1.ProvisionedCondition,
					Status:  corev1.ConditionTrue,
					Reason:  hivev1.ProvisionedReasonProvisioned,
					Message: "Cluster is provisioned",
				}})
			},
		},
		{
			name: "Remove finalizer after early-failure provision removed",
			existing: []runtime.Object{
				func() runtime.Object {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeploymentWithProvision())
					now := metav1.Now()
					cd.DeletionTimestamp = &now
					cd.Spec.ClusterMetadata = nil
					return cd
				}(),
				testSecret(corev1.SecretTypeOpaque, adminKubeconfigSecret, "kubeconfig", adminKubeconfig),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				assert.Nil(t, cd, "expected clusterdeployment to be deleted")
			},
		},
		{
			name: "AWS: Create deprovision after late-failure provision removed",
			existing: []runtime.Object{
				func() runtime.Object {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeploymentWithProvision())
					now := metav1.Now()
					cd.DeletionTimestamp = &now
					return cd
				}(),
				testSecret(corev1.SecretTypeOpaque, adminKubeconfigSecret, "kubeconfig", adminKubeconfig),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if assert.NotNil(t, cd, "missing clusterdeployment") {
					assert.Contains(t, cd.Finalizers, hivev1.FinalizerDeprovision, "expected hive finalizer")
				}
				deprovision := getDeprovision(c)
				if assert.NotNil(t, deprovision, "missing deprovision request") {
					if assert.NotNil(t, deprovision.Spec.Platform.AWS) {
						if assert.NotNil(t, deprovision.Spec.Platform.AWS.HostedZoneRole) {
							assert.Equal(t, "account-b-role", *deprovision.Spec.Platform.AWS.HostedZoneRole)
						}
					}
				}
				testassert.AssertConditions(t, getCD(c), []hivev1.ClusterDeploymentCondition{{
					Type:    hivev1.ProvisionedCondition,
					Status:  corev1.ConditionFalse,
					Reason:  hivev1.ProvisionedReasonDeprovisioning,
					Message: "Cluster is being deprovisioned",
				}})
			},
		},
		{
			name: "GCP: Create deprovision after late-failure provision removed",
			existing: []runtime.Object{
				func() runtime.Object {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeploymentWithProvision())
					cd.Spec.Platform.AWS = nil
					cd.Spec.Platform.GCP = &gcp.Platform{
						CredentialsSecretRef: corev1.LocalObjectReference{
							Name: "gcp-credentials",
						},
						Region: "us-central1",
					}
					cd.Spec.ClusterMetadata.Platform.AWS = nil
					cd.Spec.ClusterMetadata.Platform.GCP = &gcp.Metadata{
						NetworkProjectID: pointer.String("some@np.id"),
					}
					cd.Labels[hivev1.HiveClusterPlatformLabel] = "gcp"
					cd.Labels[hivev1.HiveClusterRegionLabel] = "us-central1"
					now := metav1.Now()
					cd.DeletionTimestamp = &now
					return cd
				}(),
				testSecret(corev1.SecretTypeOpaque, adminKubeconfigSecret, "kubeconfig", adminKubeconfig),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if assert.NotNil(t, cd, "missing clusterdeployment") {
					assert.Contains(t, cd.Finalizers, hivev1.FinalizerDeprovision, "expected hive finalizer")
				}
				deprovision := getDeprovision(c)
				if assert.NotNil(t, deprovision, "missing deprovision request") {
					if assert.NotNil(t, deprovision.Spec.Platform.GCP) {
						if assert.NotNil(t, deprovision.Spec.Platform.GCP.NetworkProjectID) {
							assert.Equal(t, "some@np.id", *deprovision.Spec.Platform.GCP.NetworkProjectID)
						}
					}
				}
				testassert.AssertConditions(t, getCD(c), []hivev1.ClusterDeploymentCondition{{
					Type:    hivev1.ProvisionedCondition,
					Status:  corev1.ConditionFalse,
					Reason:  hivev1.ProvisionedReasonDeprovisioning,
					Message: "Cluster is being deprovisioned",
				}})
			},
		},
		{
			name: "Azure: Create deprovision after late-failure provision removed",
			existing: []runtime.Object{
				func() runtime.Object {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeploymentWithProvision())
					cd.Spec.Platform.AWS = nil
					cd.Spec.Platform.Azure = &azure.Platform{
						CredentialsSecretRef: corev1.LocalObjectReference{
							Name: "azure-credentials",
						},
						Region: "westus",
					}
					cd.Spec.ClusterMetadata.Platform.AWS = nil
					cd.Spec.ClusterMetadata.Platform.Azure = &azure.Metadata{
						ResourceGroupName: pointer.String("some-rg"),
					}
					cd.Labels[hivev1.HiveClusterPlatformLabel] = "azure"
					cd.Labels[hivev1.HiveClusterRegionLabel] = "westus"
					now := metav1.Now()
					cd.DeletionTimestamp = &now
					return cd
				}(),
				testSecret(corev1.SecretTypeOpaque, adminKubeconfigSecret, "kubeconfig", adminKubeconfig),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if assert.NotNil(t, cd, "missing clusterdeployment") {
					assert.Contains(t, cd.Finalizers, hivev1.FinalizerDeprovision, "expected hive finalizer")
				}
				deprovision := getDeprovision(c)
				if assert.NotNil(t, deprovision, "missing deprovision request") {
					if assert.NotNil(t, deprovision.Spec.Platform.Azure) {
						if assert.NotNil(t, deprovision.Spec.Platform.Azure.ResourceGroupName) {
							assert.Equal(t, "some-rg", *deprovision.Spec.Platform.Azure.ResourceGroupName)
						}
					}
				}
				testassert.AssertConditions(t, getCD(c), []hivev1.ClusterDeploymentCondition{{
					Type:    hivev1.ProvisionedCondition,
					Status:  corev1.ConditionFalse,
					Reason:  hivev1.ProvisionedReasonDeprovisioning,
					Message: "Cluster is being deprovisioned",
				}})
			},
		},
		{
			name: "SyncSetFailedCondition should be present",
			existing: []runtime.Object{
				testClusterDeploymentWithInitializedConditions(testInstalledClusterDeployment(time.Now())),
				testSecret(corev1.SecretTypeOpaque, adminKubeconfigSecret, "kubeconfig", adminKubeconfig),
				testSecret(corev1.SecretTypeOpaque, adminPasswordSecret, "password", adminPassword),
				&hiveintv1alpha1.ClusterSync{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNamespace,
						Name:      testName,
					},
					Status: hiveintv1alpha1.ClusterSyncStatus{
						Conditions: []hiveintv1alpha1.ClusterSyncCondition{{
							Type:    hiveintv1alpha1.ClusterSyncFailed,
							Status:  corev1.ConditionTrue,
							Reason:  "FailureReason",
							Message: "Failure message",
						}},
					},
				},
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if assert.NotNil(t, cd, "missing clusterdeployment") {
					cond := controllerutils.FindCondition(cd.Status.Conditions, hivev1.SyncSetFailedCondition)
					if assert.NotNil(t, cond, "missing SyncSetFailedCondition status condition") {
						assert.Equal(t, corev1.ConditionTrue, cond.Status, "did not get expected state for SyncSetFailedCondition condition")
					}
				}
			},
		},
		{
			name: "SyncSetFailedCondition value should be corev1.ConditionFalse",
			existing: []runtime.Object{
				func() runtime.Object {
					cd := testClusterDeploymentWithInitializedConditions(testInstalledClusterDeployment(time.Now()))
					cd.Status.Conditions = addOrUpdateClusterDeploymentCondition(*cd,
						hivev1.SyncSetFailedCondition,
						corev1.ConditionTrue,
						"test reason",
						"test message")
					return cd
				}(),
				testSecret(corev1.SecretTypeOpaque, adminKubeconfigSecret, "kubeconfig", adminKubeconfig),
				testSecret(corev1.SecretTypeOpaque, adminPasswordSecret, "password", adminPassword),
				&hiveintv1alpha1.ClusterSync{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNamespace,
						Name:      testName,
					},
					Status: hiveintv1alpha1.ClusterSyncStatus{
						Conditions: []hiveintv1alpha1.ClusterSyncCondition{{
							Type:    hiveintv1alpha1.ClusterSyncFailed,
							Status:  corev1.ConditionFalse,
							Reason:  "SuccessReason",
							Message: "Success message",
						}},
					},
				},
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				cond := controllerutils.FindCondition(cd.Status.Conditions, hivev1.SyncSetFailedCondition)
				if assert.NotNil(t, cond, "missing SyncSetFailedCondition status condition") {
					assert.Equal(t, corev1.ConditionFalse, cond.Status, "did not get expected state for SyncSetFailedCondition condition")
				}
			},
		},
		{
			name: "SyncSet is Paused and ClusterSync object is missing",
			existing: []runtime.Object{
				func() runtime.Object {
					cd := testClusterDeploymentWithInitializedConditions(testInstalledClusterDeployment(time.Now()))
					if cd.Annotations == nil {
						cd.Annotations = make(map[string]string, 1)
					}
					cd.Annotations[constants.SyncsetPauseAnnotation] = "true"
					return cd
				}(),
				testSecret(corev1.SecretTypeOpaque, adminKubeconfigSecret, "kubeconfig", adminKubeconfig),
				testSecret(corev1.SecretTypeOpaque, adminPasswordSecret, "password", adminPassword),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				cond := controllerutils.FindCondition(cd.Status.Conditions, hivev1.SyncSetFailedCondition)
				if assert.NotNil(t, cond, "missing SyncSetFailedCondition status condition") {
					assert.Equal(t, corev1.ConditionTrue, cond.Status, "did not get expected state for SyncSetFailedCondition condition")
					assert.Equal(t, "SyncSetPaused", cond.Reason, "did not get expected reason for SyncSetFailedCondition condition")
				}
			},
		},
		{
			name: "Add cluster platform label",
			existing: []runtime.Object{
				testClusterDeploymentWithInitializedConditions(testClusterDeploymentWithoutPlatformLabel()),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if assert.NotNil(t, cd, "missing clusterdeployment") {
					assert.Equal(t, getClusterPlatform(cd), cd.Labels[hivev1.HiveClusterPlatformLabel], "incorrect cluster platform label")
				}
			},
		},
		{
			name: "Add cluster region label",
			existing: []runtime.Object{
				testClusterDeploymentWithInitializedConditions(testClusterDeploymentWithoutRegionLabel()),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if assert.NotNil(t, cd, "missing clusterdeployment") {
					assert.Equal(t, getClusterRegion(cd), cd.Labels[hivev1.HiveClusterRegionLabel], "incorrect cluster region label")
					assert.Equal(t, getClusterRegion(cd), "us-east-1", "incorrect cluster region label")
				}
			},
		},
		{
			name: "Ensure cluster metadata set from provision",
			existing: []runtime.Object{
				testInstallConfigSecretAWS(),
				func() runtime.Object {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeploymentWithProvision())
					cd.Spec.ClusterMetadata = nil
					return cd
				}(),
				testSuccessfulProvision(tcp.WithMetadata(`{"aws": {"hostedZoneRole": "account-b-role"}}`)),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if assert.NotNil(t, cd, "missing clusterdeployment") {
					if assert.NotNil(t, cd.Spec.ClusterMetadata, "expected cluster metadata to be set") {
						assert.Equal(t, testInfraID, cd.Spec.ClusterMetadata.InfraID, "unexpected infra ID")
						assert.Equal(t, testClusterID, cd.Spec.ClusterMetadata.ClusterID, "unexpected cluster ID")
						assert.Equal(t, adminKubeconfigSecret, cd.Spec.ClusterMetadata.AdminKubeconfigSecretRef.Name, "unexpected admin kubeconfig")
						assert.Equal(t, adminPasswordSecret, cd.Spec.ClusterMetadata.AdminPasswordSecretRef.Name, "unexpected admin password")
					}
				}
			},
		},
		{
			name: "Ensure cluster metadata overwrites from provision",
			existing: []runtime.Object{
				testInstallConfigSecretAWS(),
				func() runtime.Object {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeploymentWithProvision())
					cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{
						InfraID:                  "old-infra-id",
						ClusterID:                "old-cluster-id",
						AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: "old-kubeconfig-secret"},
						AdminPasswordSecretRef:   &corev1.LocalObjectReference{Name: "old-password-secret"},
					}
					return cd
				}(),
				testSuccessfulProvision(tcp.WithMetadata(`{"aws": {"hostedZoneRole": "account-b-role"}}`)),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if assert.NotNil(t, cd, "missing clusterdeployment") {
					if assert.NotNil(t, cd.Spec.ClusterMetadata, "expected cluster metadata to be set") {
						assert.Equal(t, testInfraID, cd.Spec.ClusterMetadata.InfraID, "unexpected infra ID")
						assert.Equal(t, testClusterID, cd.Spec.ClusterMetadata.ClusterID, "unexpected cluster ID")
						assert.Equal(t, adminKubeconfigSecret, cd.Spec.ClusterMetadata.AdminKubeconfigSecretRef.Name, "unexpected admin kubeconfig")
						assert.Equal(t, adminPasswordSecret, cd.Spec.ClusterMetadata.AdminPasswordSecretRef.Name, "unexpected admin password")
					}
				}
			},
		},
		{
			name: "set ClusterImageSet missing condition",
			existing: []runtime.Object{
				testInstallConfigSecretAWS(),
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.Provisioning.ImageSetRef = &hivev1.ClusterImageSetReference{Name: "doesntexist"}
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			expectErr: true,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{
					{
						Type:   hivev1.RequirementsMetCondition,
						Status: corev1.ConditionFalse,
						Reason: clusterImageSetNotFoundReason,
					},
				})
			},
		},
		{
			name: "do not set ClusterImageSet missing condition for installed cluster",
			existing: []runtime.Object{
				testInstallConfigSecretAWS(),
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(
						testInstalledClusterDeployment(time.Now()))
					cd.Spec.Provisioning.ImageSetRef = &hivev1.ClusterImageSetReference{Name: "doesntexist"}
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			expectErr: true,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{
					{
						Type:   hivev1.RequirementsMetCondition,
						Status: corev1.ConditionUnknown,
						Reason: "Initialized",
					},
				})
			},
		},
		{
			name: "clear ClusterImageSet missing condition",
			existing: []runtime.Object{
				testInstallConfigSecretAWS(),
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithDefaultConditions(testClusterDeploymentWithInitializedConditions(testClusterDeployment()))
					cd.Spec.Provisioning.ImageSetRef = &hivev1.ClusterImageSetReference{Name: testClusterImageSetName}
					cd.Status.Conditions = addOrUpdateClusterDeploymentCondition(*cd, hivev1.RequirementsMetCondition,
						corev1.ConditionFalse, clusterImageSetNotFoundReason, "test-message")
					return cd
				}(),
				testClusterImageSet(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			expectPendingCreation: true,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{
					{
						Type:   hivev1.RequirementsMetCondition,
						Status: corev1.ConditionTrue,
						Reason: "AllRequirementsMet",
					},
				})
			},
		},
		{
			name: "clear legacy conditions",
			existing: []runtime.Object{
				testInstallConfigSecretAWS(),
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithDefaultConditions(testClusterDeploymentWithInitializedConditions(testInstalledClusterDeployment(time.Now())))
					cd.Spec.Provisioning.ImageSetRef = &hivev1.ClusterImageSetReference{Name: testClusterImageSetName}
					cd.Status.Conditions = append(cd.Status.Conditions,
						hivev1.ClusterDeploymentCondition{
							Type:    hivev1.ClusterImageSetNotFoundCondition,
							Status:  corev1.ConditionFalse,
							Reason:  clusterImageSetFoundReason,
							Message: "test-message",
						})
					return cd
				}(),
				testClusterImageSet(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				lc := controllerutils.FindCondition(cd.Status.Conditions, hivev1.ClusterImageSetNotFoundCondition)
				assert.Nil(t, lc, "legacy condition was not cleared")
			},
		},
		{
			name: "Add ownership to admin secrets",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.Installed = true
					cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{
						InfraID:                  "fakeinfra",
						AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: adminKubeconfigSecret},
						AdminPasswordSecretRef:   &corev1.LocalObjectReference{Name: adminPasswordSecret},
					}
					cd.Status.WebConsoleURL = "https://example.com"
					cd.Status.APIURL = "https://example.com"
					return cd
				}(),
				testSecret(corev1.SecretTypeOpaque, adminKubeconfigSecret, "kubeconfig", adminKubeconfig),
				testSecret(corev1.SecretTypeOpaque, adminPasswordSecret, "password", adminPassword),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(
					testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testMetadataConfigMap(),
			},
			validate: func(c client.Client, t *testing.T) {
				secretNames := []string{adminKubeconfigSecret, adminPasswordSecret}
				for _, secretName := range secretNames {
					secret := &corev1.Secret{}
					err := c.Get(context.TODO(), client.ObjectKey{Name: secretName, Namespace: testNamespace},
						secret)
					require.NoErrorf(t, err, "not found secret %s", secretName)
					require.NotNilf(t, secret, "expected secret %s", secretName)
					assert.Equalf(t, testClusterDeployment().Name, secret.Labels[constants.ClusterDeploymentNameLabel],
						"incorrect cluster deployment name label for %s", secretName)
					refs := secret.GetOwnerReferences()
					cdAsOwnerRef := false
					for _, ref := range refs {
						if ref.Name == testName {
							cdAsOwnerRef = true
						}
					}
					assert.Truef(t, cdAsOwnerRef, "cluster deployment not owner of %s", secretName)
				}
			},
		},
		{
			name: "delete finalizer when deprovision complete and dnszone gone",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.ManageDNS = true
					cd.Spec.Installed = true
					now := metav1.Now()
					cd.DeletionTimestamp = &now
					return cd
				}(),
				testclusterdeprovision.Build(
					testclusterdeprovision.WithNamespace(testNamespace),
					testclusterdeprovision.WithName(testName),
					testclusterdeprovision.Completed(),
				),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.Nil(t, cd, "expected ClusterDeployment to be deleted")
			},
		},
		{
			name: "release customization on deprovision",
			existing: []runtime.Object{
				testClusterDeploymentCustomization(testName),
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.Installed = true
					cd.Spec.ClusterPoolRef = &hivev1.ClusterPoolReference{
						Namespace:        testNamespace,
						CustomizationRef: &corev1.LocalObjectReference{Name: testName},
					}
					now := metav1.Now()
					cd.DeletionTimestamp = &now
					return cd
				}(),
				testclusterdeprovision.Build(
					testclusterdeprovision.WithNamespace(testNamespace),
					testclusterdeprovision.WithName(testName),
					testclusterdeprovision.Completed(),
				),
			},
			validate: func(c client.Client, t *testing.T) {
				testassert.AssertCDCConditions(t, getCDC(c), []conditionsv1.Condition{{
					Type:    conditionsv1.ConditionAvailable,
					Status:  corev1.ConditionTrue,
					Reason:  "Available",
					Message: "available",
				}})
			},
		},
		{
			name: "deprovision finished",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.ManageDNS = true
					cd.Spec.Installed = true
					now := metav1.Now()
					cd.DeletionTimestamp = &now
					cd.Finalizers = append(cd.Finalizers, "prevent the CD from being deleted so we can validate deprovisioned status")
					return cd
				}(),
				testclusterdeprovision.Build(
					testclusterdeprovision.WithNamespace(testNamespace),
					testclusterdeprovision.WithName(testName),
					testclusterdeprovision.Completed(),
				),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{{
					Type:    hivev1.ProvisionedCondition,
					Status:  corev1.ConditionFalse,
					Reason:  hivev1.ProvisionedReasonDeprovisioned,
					Message: "Cluster is deprovisioned",
				}})
			},
		},
		{
			name: "existing deprovision in progress",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.ManageDNS = true
					cd.Spec.Installed = true
					now := metav1.Now()
					cd.DeletionTimestamp = &now
					return cd
				}(),
				testclusterdeprovision.Build(
					testclusterdeprovision.WithNamespace(testNamespace),
					testclusterdeprovision.WithName(testName),
				),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{{
					Type:    hivev1.ProvisionedCondition,
					Status:  corev1.ConditionFalse,
					Reason:  hivev1.ProvisionedReasonDeprovisioning,
					Message: "Cluster is deprovisioning",
				}})
			},
		},
		{
			name: "deprovision failed",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.ManageDNS = true
					cd.Spec.Installed = true
					now := metav1.Now()
					cd.DeletionTimestamp = &now
					return cd
				}(),
				testclusterdeprovision.Build(
					testclusterdeprovision.WithNamespace(testNamespace),
					testclusterdeprovision.WithName(testName),
					testclusterdeprovision.WithAuthenticationFailure(),
				),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{{
					Type:    hivev1.ProvisionedCondition,
					Status:  corev1.ConditionFalse,
					Reason:  hivev1.ProvisionedReasonDeprovisionFailed,
					Message: "Cluster deprovision failed",
				}})
			},
		},
		{
			name: "wait for deprovision to complete",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.ManageDNS = true
					cd.Spec.Installed = true
					now := metav1.Now()
					cd.DeletionTimestamp = &now
					cd.Status.Conditions = addOrUpdateClusterDeploymentCondition(
						*cd,
						hivev1.ProvisionedCondition,
						corev1.ConditionFalse,
						hivev1.ProvisionedReasonDeprovisioning,
						"Cluster is being deprovisioned",
					)
					return cd
				}(),
				testclusterdeprovision.Build(
					testclusterdeprovision.WithNamespace(testNamespace),
					testclusterdeprovision.WithName(testName),
				),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				assert.Contains(t, cd.Finalizers, hivev1.FinalizerDeprovision, "expected finalizer not to be removed from ClusterDeployment")
				testassert.AssertConditions(t, getCD(c), []hivev1.ClusterDeploymentCondition{{
					Type:    hivev1.ProvisionedCondition,
					Status:  corev1.ConditionFalse,
					Reason:  hivev1.ProvisionedReasonDeprovisioning,
					Message: "Cluster is deprovisioning",
				}})
			},
		},
		{
			name: "wait for dnszone to be gone",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.ManageDNS = true
					cd.Spec.Installed = true
					now := metav1.Now()
					cd.DeletionTimestamp = &now
					return cd
				}(),
				testclusterdeprovision.Build(
					testclusterdeprovision.WithNamespace(testNamespace),
					testclusterdeprovision.WithName(testName),
					testclusterdeprovision.Completed(),
				),
				func() *hivev1.DNSZone {
					dnsZone := testDNSZone()
					now := metav1.Now()
					dnsZone.DeletionTimestamp = &now
					dnsZone.Finalizers = []string{testFinalizer}
					return dnsZone
				}(),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				assert.Contains(t, cd.Finalizers, hivev1.FinalizerDeprovision, "expected finalizer not to be removed from ClusterDeployment")
			},
			expectedRequeueAfter: defaultRequeueTime,
		},
		{
			name: "do not wait for dnszone to be gone when not using managed dns",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.Installed = true
					now := metav1.Now()
					cd.DeletionTimestamp = &now
					return cd
				}(),
				testclusterdeprovision.Build(
					testclusterdeprovision.WithNamespace(testNamespace),
					testclusterdeprovision.WithName(testName),
					testclusterdeprovision.Completed(),
				),
				func() *hivev1.DNSZone {
					dnsZone := testDNSZone()
					now := metav1.Now()
					dnsZone.DeletionTimestamp = &now
					dnsZone.ObjectMeta.Finalizers = []string{testFinalizer}
					return dnsZone
				}(),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.Nil(t, cd, "expected ClusterDeployment to be deleted")
			},
		},
		{
			name: "wait for dnszone to be gone when install failed early",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.ManageDNS = true
					now := metav1.Now()
					cd.DeletionTimestamp = &now
					cd.Spec.ClusterMetadata = nil
					return cd
				}(),
				func() *hivev1.DNSZone {
					dnsZone := testDNSZone()
					now := metav1.Now()
					dnsZone.DeletionTimestamp = &now
					dnsZone.ObjectMeta.Finalizers = []string{testFinalizer}
					return dnsZone
				}(),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				assert.Contains(t, cd.Finalizers, hivev1.FinalizerDeprovision, "expected finalizer not to be removed from ClusterDeployment")
			},
			expectedRequeueAfter: defaultRequeueTime,
		},
		{
			name: "set InstallLaunchErrorCondition when install pod is stuck in pending phase",
			existing: []runtime.Object{
				testInstallConfigSecretAWS(),
				testClusterDeploymentWithInitializedConditions(testClusterDeploymentWithProvision()),
				testProvision(tcp.WithStuckInstallPod()),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{
					{
						Type:   hivev1.InstallLaunchErrorCondition,
						Status: corev1.ConditionTrue,
						Reason: "PodInPendingPhase",
					},
					// This is not terminal; to the outside world, the CD is still provisioning
					{
						Type:    hivev1.ProvisionedCondition,
						Status:  corev1.ConditionFalse,
						Reason:  hivev1.ProvisionedReasonProvisioning,
						Message: "Cluster provision initializing",
					},
				})
			},
		},
		{
			name: "install attempts is less than the limit",
			existing: []runtime.Object{
				testInstallConfigSecretAWS(),
				func() runtime.Object {
					cd := testClusterDeploymentWithDefaultConditions(testClusterDeploymentWithInitializedConditions(testClusterDeployment()))
					cd.Status.InstallRestarts = 1
					cd.Spec.InstallAttemptsLimit = pointer.Int32(2)
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			expectPendingCreation: true,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				testassert.AssertConditionStatus(t, cd, hivev1.ProvisionStoppedCondition, corev1.ConditionFalse)
				testassert.AssertConditions(t, getCD(c), []hivev1.ClusterDeploymentCondition{{
					Type:    hivev1.ProvisionedCondition,
					Status:  corev1.ConditionFalse,
					Reason:  hivev1.ProvisionedReasonProvisioning,
					Message: "Cluster provision created",
				}})
				// HIVE-2198:
				// 1) Ensure the magic trusted CA bundle configmap was created
				// 2) Ensure the provision pod spec in the ClusterProvision has the volume & mount for same
				cacm := &corev1.ConfigMap{}
				if assert.Nil(t, c.Get(context.TODO(),
					types.NamespacedName{Namespace: cd.Namespace, Name: constants.TrustedCAConfigMapName},
					cacm),
					"expected to find ConfigMap %s", constants.TrustedCAConfigMapName) {
					assert.Equal(t, "true", cacm.Labels[injectCABundleKey], "expected label %s to be \"true\"", injectCABundleKey)
				}
				provs := getProvisions(c)
				if assert.Len(t, provs, 1, "expected 1 ClusterProvision to exist") {
					vols := provs[0].Spec.PodSpec.Volumes
					// Find the trusted CA ConfigMap volume
					found := false
					for _, vol := range vols {
						if vol.Name == constants.TrustedCAConfigMapName {
							found = true
							assert.Equal(t, corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: constants.TrustedCAConfigMapName,
									},
									Items: []corev1.KeyToPath{
										{
											Key:  "ca-bundle.crt",
											Path: "ca-bundle.crt",
										},
									},
								},
							}, vol.VolumeSource, "mismatched VolumeSource")
						}
					}
					assert.True(t, found, "did not find the trusted CA ConfigMap Volume")
					for i, container := range provs[0].Spec.PodSpec.Containers {
						found = false
						for _, mount := range container.VolumeMounts {
							if mount.Name == constants.TrustedCAConfigMapName {
								found = true
								assert.Equal(t, constants.TrustedCABundleDir, mount.MountPath, "unexpected MountPath in trusted CA Bundle VolumeMount for container %d", i)
							}
						}
						assert.True(t, found, "did not find the trusted CA ConfigMap VolumeMount in container %d", i)
					}
				}
			},
		},
		{
			name: "ProvisionStopped already",
			existing: []runtime.Object{
				testInstallConfigSecretAWS(),
				func() runtime.Object {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Status.Conditions = controllerutils.SetClusterDeploymentCondition(
						cd.Status.Conditions,
						hivev1.ProvisionStoppedCondition,
						corev1.ConditionTrue,
						"DNSZoneFailedToReconcile",
						"DNSZone failed to reconcile (see the DNSNotReadyCondition condition for details)",
						controllerutils.UpdateConditionAlways)
					cd.Status.Conditions = controllerutils.SetClusterDeploymentCondition(
						cd.Status.Conditions,
						hivev1.ProvisionedCondition,
						corev1.ConditionFalse,
						hivev1.ProvisionedReasonProvisionStopped,
						"Provisioning failed terminally (see the ProvisionStopped condition for details)",
						controllerutils.UpdateConditionAlways)
					cd.Status.Conditions = controllerutils.SetClusterDeploymentCondition(
						cd.Status.Conditions,
						hivev1.DNSNotReadyCondition,
						corev1.ConditionTrue,
						"InsufficientCredentials",
						"",
						controllerutils.UpdateConditionAlways)
					cd.Spec.ManageDNS = true
					return cd
				}(),
				testDNSZoneWithInvalidCredentialsCondition(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{
					{
						Type:   hivev1.ProvisionStoppedCondition,
						Status: corev1.ConditionTrue,
						Reason: "DNSZoneFailedToReconcile",
						Message: "DNSZone failed to reconcile (see the DNSNotReadyCondition condition for details)",
					},
				})
				provisions := getProvisions(c)
				assert.Empty(t, provisions, "provision should not exist")
			},
		},
		{
			name: "install attempts is equal to the limit",
			existing: []runtime.Object{
				testInstallConfigSecretAWS(),
				func() runtime.Object {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Status.InstallRestarts = 2
					cd.Spec.InstallAttemptsLimit = pointer.Int32(2)
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{
					{
						Type:   hivev1.ProvisionStoppedCondition,
						Status: corev1.ConditionTrue,
						Reason: "InstallAttemptsLimitReached",
					},
					{
						Type:    hivev1.ProvisionedCondition,
						Status:  corev1.ConditionFalse,
						Reason:  hivev1.ProvisionedReasonProvisionStopped,
						Message: "Provisioning failed terminally (see the ProvisionStopped condition for details)",
					},
				})
			},
		},
		{
			name: "install attempts is greater than the limit",
			existing: []runtime.Object{
				testInstallConfigSecretAWS(),
				func() runtime.Object {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Status.InstallRestarts = 3
					cd.Spec.InstallAttemptsLimit = pointer.Int32(2)
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{
					{
						Type:   hivev1.ProvisionStoppedCondition,
						Status: corev1.ConditionTrue,
						Reason: "InstallAttemptsLimitReached",
					},
					{
						Type:    hivev1.ProvisionedCondition,
						Status:  corev1.ConditionFalse,
						Reason:  hivev1.ProvisionedReasonProvisionStopped,
						Message: "Provisioning failed terminally (see the ProvisionStopped condition for details)",
					},
				})
			},
		},
		{
			name: "auth condition when platform creds are bad",
			existing: []runtime.Object{
				testClusterDeploymentWithInitializedConditions(testClusterDeployment()),
			},
			platformCredentialsValidation: func(client.Client, *hivev1.ClusterDeployment, log.FieldLogger) (bool, error) {
				return false, errors.New("Post \"https://xxx.xxx.xxx.xxx/sdk\": x509: cannot validate certificate for xxx.xxx.xxx.xxx because it doesn't contain any IP SANs")
			},
			expectErr: true,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")

				testassert.AssertConditionStatus(t, cd, hivev1.AuthenticationFailureClusterDeploymentCondition, corev1.ConditionTrue)
				// Preflight check happens before we declare provisioning
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{
					{
						Type:    hivev1.ProvisionedCondition,
						Status:  corev1.ConditionUnknown,
						Reason:  hivev1.InitializedConditionReason,
						Message: "Condition Initialized",
					},
					{
						Type:    hivev1.AuthenticationFailureClusterDeploymentCondition,
						Status:  corev1.ConditionTrue,
						Reason:  platformAuthFailureReason,
						Message: "Platform credentials failed authentication check: Post \"https://xxx.xxx.xxx.xxx/sdk\": x509: cannot validate certificate for xxx.xxx.xxx.xxx because it doesn't contain any IP SANs",
					},
				})
			},
		},
		{
			name: "no ClusterProvision when platform creds are bad",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					return cd
				}(),
			},
			platformCredentialsValidation: func(client.Client, *hivev1.ClusterDeployment, log.FieldLogger) (bool, error) {
				return false, nil
			},
			expectErr: true,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				// Preflight check happens before we declare provisioning
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{{
					Type:    hivev1.ProvisionedCondition,
					Status:  corev1.ConditionUnknown,
					Reason:  hivev1.InitializedConditionReason,
					Message: "Condition Initialized",
				}})

				provisionList := &hivev1.ClusterProvisionList{}
				err := c.List(context.TODO(), provisionList, client.InNamespace(cd.Namespace))
				require.NoError(t, err, "unexpected error listing ClusterProvisions")

				assert.Zero(t, len(provisionList.Items), "expected no ClusterProvision objects when platform creds are bad")
			},
		},
		{
			name: "clusterinstallref not found",
			existing: []runtime.Object{
				testClusterDeploymentWithInitializedConditions(testClusterInstallRefClusterDeployment("test-fake")),
			},
			expectErr: true,
		},
		{
			name: "clusterinstallref exists, but no imagesetref",
			existing: []runtime.Object{
				testClusterDeploymentWithInitializedConditions(testClusterInstallRefClusterDeployment("test-fake")),
				testFakeClusterInstall("test-fake"),
			},
			expectErr: true,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")

				testassert.AssertConditionStatus(t, cd, hivev1.RequirementsMetCondition, corev1.ConditionFalse)
				// Preflight check happens before we declare provisioning
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{{
					Type:    hivev1.ProvisionedCondition,
					Status:  corev1.ConditionUnknown,
					Reason:  hivev1.InitializedConditionReason,
					Message: "Condition Initialized",
				}})
			},
		},
		{
			name: "clusterinstallref exists, no conditions set",
			existing: []runtime.Object{
				testClusterInstallRefClusterDeployment("test-fake"),
				testFakeClusterInstallWithConditions("test-fake", nil),
				testClusterImageSet(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{{
					Type:    hivev1.ProvisionedCondition,
					Status:  corev1.ConditionUnknown,
					Reason:  hivev1.InitializedConditionReason,
					Message: "Condition Initialized",
				}})
			},
		},
		{
			name: "clusterinstallref exists, requirements met set to false",
			existing: []runtime.Object{
				testClusterInstallRefClusterDeployment("test-fake"),
				testFakeClusterInstallWithConditions("test-fake", []hivev1.ClusterInstallCondition{{
					Type:   hivev1.ClusterInstallRequirementsMet,
					Status: corev1.ConditionFalse,
				}}),
				testClusterImageSet(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{{
					Type:    hivev1.ProvisionedCondition,
					Status:  corev1.ConditionUnknown,
					Reason:  hivev1.InitializedConditionReason,
					Message: "Condition Initialized",
				}})
			},
		},
		{
			name: "clusterinstallref exists, requirements met set to true",
			existing: []runtime.Object{
				testClusterDeploymentWithInitializedConditions(testClusterInstallRefClusterDeployment("test-fake")),
				testFakeClusterInstallWithConditions("test-fake", []hivev1.ClusterInstallCondition{{
					Type:   hivev1.ClusterInstallRequirementsMet,
					Status: corev1.ConditionTrue,
				}}),
				testClusterImageSet(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				testassert.AssertConditionStatus(t, cd, hivev1.ClusterInstallRequirementsMetClusterDeploymentCondition, corev1.ConditionTrue)
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{{
					Type:    hivev1.ProvisionedCondition,
					Status:  corev1.ConditionFalse,
					Reason:  hivev1.ProvisionedReasonProvisioning,
					Message: "Provisioning in progress",
				}})
			},
		},
		{
			name: "clusterinstallref exists, failed true",
			existing: []runtime.Object{
				testClusterDeploymentWithInitializedConditions(testClusterInstallRefClusterDeployment("test-fake")),
				testFakeClusterInstallWithConditions("test-fake", []hivev1.ClusterInstallCondition{{
					Type:   hivev1.ClusterInstallFailed,
					Status: corev1.ConditionTrue,
				}}),
				testClusterImageSet(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				testassert.AssertConditionStatus(t, cd, hivev1.ClusterInstallFailedClusterDeploymentCondition, corev1.ConditionTrue)
				testassert.AssertConditionStatus(t, cd, hivev1.ProvisionFailedCondition, corev1.ConditionTrue)
				// We don't declare provision failed in the Provisioned condition until it's terminal
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{{
					Type:    hivev1.ProvisionedCondition,
					Status:  corev1.ConditionUnknown,
					Reason:  hivev1.InitializedConditionReason,
					Message: "Condition Initialized",
				}})
			},
		},
		{
			name: "clusterinstallref exists, failed false, previously true",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterInstallRefClusterDeployment("test-fake"))
					cd.Status.Conditions = addOrUpdateClusterDeploymentCondition(*cd, hivev1.ProvisionFailedCondition,
						corev1.ConditionTrue, "test-reason", "test-message")
					cd.Status.Conditions = addOrUpdateClusterDeploymentCondition(*cd, hivev1.ClusterInstallFailedClusterDeploymentCondition,
						corev1.ConditionTrue, "test-reason", "test-message")
					return cd
				}(),
				testFakeClusterInstallWithConditions("test-fake", []hivev1.ClusterInstallCondition{{
					Type:   hivev1.ClusterInstallFailed,
					Status: corev1.ConditionFalse,
				}}),
				testClusterImageSet(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				testassert.AssertConditionStatus(t, cd, hivev1.ClusterInstallFailedClusterDeploymentCondition, corev1.ConditionFalse)
				testassert.AssertConditionStatus(t, cd, hivev1.ProvisionFailedCondition, corev1.ConditionFalse)
			},
		},
		{
			name: "clusterinstallref exists, stopped, completed false, failed true",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithDefaultConditions(testClusterDeploymentWithInitializedConditions(testClusterInstallRefClusterDeployment("test-fake")))
					cd.Status.Conditions = addOrUpdateClusterDeploymentCondition(*cd, hivev1.RequirementsMetCondition,
						corev1.ConditionUnknown, clusterImageSetFoundReason, "test-message")
					return cd
				}(),
				testFakeClusterInstallWithConditions("test-fake", []hivev1.ClusterInstallCondition{{
					Type:   hivev1.ClusterInstallStopped,
					Status: corev1.ConditionTrue,
				}, {
					Type:   hivev1.ClusterInstallCompleted,
					Status: corev1.ConditionFalse,
				}, {
					Type:   hivev1.ClusterInstallFailed,
					Status: corev1.ConditionTrue,
				}}),
				testClusterImageSet(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				testassert.AssertConditionStatus(t, cd, hivev1.ClusterInstallStoppedClusterDeploymentCondition, corev1.ConditionTrue)
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{
					{
						Type:    hivev1.ProvisionStoppedCondition,
						Status:  corev1.ConditionTrue,
						Reason:  "InstallAttemptsLimitReached",
						Message: "Install attempts limit reached",
					},
					{
						Type:    hivev1.ProvisionedCondition,
						Status:  corev1.ConditionFalse,
						Reason:  hivev1.ProvisionedReasonProvisionStopped,
						Message: "Provisioning failed terminally (see the ProvisionStopped condition for details)",
					},
				})
			},
		},
		{
			name: "clusterinstallref exists, stopped, completed false, failed not set",
			existing: []runtime.Object{
				testClusterDeploymentWithInitializedConditions(testClusterInstallRefClusterDeployment("test-fake")),
				testFakeClusterInstallWithConditions("test-fake", []hivev1.ClusterInstallCondition{{
					Type:   hivev1.ClusterInstallStopped,
					Status: corev1.ConditionTrue,
				}, {
					Type:   hivev1.ClusterInstallCompleted,
					Status: corev1.ConditionFalse,
				}}),
				testClusterImageSet(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				testassert.AssertConditionStatus(t, cd, hivev1.ClusterInstallStoppedClusterDeploymentCondition, corev1.ConditionTrue)
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{
					{
						Type:   hivev1.ProvisionStoppedCondition,
						Status: corev1.ConditionTrue,
					},
					{
						Type:    hivev1.ProvisionedCondition,
						Status:  corev1.ConditionUnknown,
						Reason:  "Error",
						Message: "Invalid ClusterInstall conditions. Please report this bug.",
					},
				})
			},
		},
		{
			name: "clusterinstallref exists, previously stopped, now progressing",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterInstallRefClusterDeployment("test-fake"))
					cd.Status.Conditions = addOrUpdateClusterDeploymentCondition(*cd, hivev1.ProvisionStoppedCondition,
						corev1.ConditionTrue, installAttemptsLimitReachedReason, "test-message")
					cd.Status.Conditions = addOrUpdateClusterDeploymentCondition(*cd, hivev1.ClusterInstallStoppedClusterDeploymentCondition,
						corev1.ConditionTrue, "test-reason", "test-message")
					return cd
				}(),
				testFakeClusterInstallWithConditions("test-fake", []hivev1.ClusterInstallCondition{{
					Type:   hivev1.ClusterInstallStopped,
					Status: corev1.ConditionFalse,
				}, {
					Type:   hivev1.ClusterInstallCompleted,
					Status: corev1.ConditionFalse,
				}}),
				testClusterImageSet(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				testassert.AssertConditionStatus(t, cd, hivev1.ClusterInstallStoppedClusterDeploymentCondition, corev1.ConditionFalse)
				testassert.AssertConditionStatus(t, cd, hivev1.ProvisionStoppedCondition, corev1.ConditionFalse)
			},
		},
		{
			name: "clusterinstallref exists, cluster metadata available partially",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterInstallRefClusterDeployment("test-fake")
					cd.Spec.ClusterMetadata = nil
					return cd
				}(),
				testFakeClusterInstallWithClusterMetadata("test-fake", hivev1.ClusterMetadata{InfraID: testInfraID}),
				testClusterImageSet(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				assert.Nil(t, cd.Spec.ClusterMetadata)
			},
		},
		{
			name: "clusterinstallref exists, cluster metadata available partially (no AdminPasswordSecretRef)",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterInstallRefClusterDeployment("test-fake")
					cd.Spec.ClusterMetadata = nil
					return cd
				}(),
				testFakeClusterInstallWithClusterMetadata("test-fake", hivev1.ClusterMetadata{
					InfraID:   testInfraID,
					ClusterID: testClusterID,
					AdminKubeconfigSecretRef: corev1.LocalObjectReference{
						Name: adminKubeconfigSecret,
					},
				}),
				testClusterImageSet(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				// Metadata wasn't copied because metadata was incomplete (no AdminPasswordSecretRef)
				assert.Nil(t, cd.Spec.ClusterMetadata)
			},
		},
		{
			name: "clusterinstallref exists, cluster metadata available",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterInstallRefClusterDeployment("test-fake"))
					cd.Spec.ClusterMetadata = nil
					return cd
				}(),
				testFakeClusterInstallWithClusterMetadata("test-fake", hivev1.ClusterMetadata{
					InfraID:   testInfraID,
					ClusterID: testClusterID,
					AdminKubeconfigSecretRef: corev1.LocalObjectReference{
						Name: adminKubeconfigSecret,
					},
					AdminPasswordSecretRef: &corev1.LocalObjectReference{
						Name: adminPasswordSecret,
					},
				}),
				testClusterImageSet(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				require.NotNil(t, cd.Spec.ClusterMetadata)
				assert.Equal(t, testInfraID, cd.Spec.ClusterMetadata.InfraID)
				assert.Equal(t, testClusterID, cd.Spec.ClusterMetadata.ClusterID)
				assert.Equal(t, adminKubeconfigSecret, cd.Spec.ClusterMetadata.AdminKubeconfigSecretRef.Name)
				assert.Equal(t, adminPasswordSecret, cd.Spec.ClusterMetadata.AdminPasswordSecretRef.Name)
			},
		},
		{
			name: "clusterinstallref exists, completed but not stopped",
			existing: []runtime.Object{
				testClusterDeploymentWithInitializedConditions(testClusterInstallRefClusterDeployment("test-fake")),
				testFakeClusterInstallWithConditions("test-fake", []hivev1.ClusterInstallCondition{{
					Type:   hivev1.ClusterInstallCompleted,
					Status: corev1.ConditionTrue,
				}}),
				testClusterImageSet(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				testassert.AssertConditionStatus(t, cd, hivev1.ClusterInstallCompletedClusterDeploymentCondition, corev1.ConditionTrue)
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{{
					Type:    hivev1.ProvisionedCondition,
					Status:  corev1.ConditionUnknown,
					Reason:  "Error",
					Message: "Invalid ClusterInstall conditions. Please report this bug.",
				}})
			},
		},
		{
			name: "clusterinstallref exists, stopped and completed",
			existing: []runtime.Object{
				testClusterDeploymentWithInitializedConditions(testClusterInstallRefClusterDeployment("test-fake")),
				testFakeClusterInstallWithConditions("test-fake", []hivev1.ClusterInstallCondition{{
					Type:   hivev1.ClusterInstallCompleted,
					Status: corev1.ConditionTrue,
				}, {
					Type:   hivev1.ClusterInstallStopped,
					Status: corev1.ConditionTrue,
					Reason: "InstallComplete",
				}}),
				testClusterImageSet(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				testassert.AssertConditionStatus(t, cd, hivev1.ClusterInstallCompletedClusterDeploymentCondition, corev1.ConditionTrue)
				testassert.AssertConditionStatus(t, cd, hivev1.ClusterInstallStoppedClusterDeploymentCondition, corev1.ConditionTrue)
				testassert.AssertConditions(t, cd, []hivev1.ClusterDeploymentCondition{
					{
						Type:   hivev1.ProvisionStoppedCondition,
						Status: corev1.ConditionTrue,
						Reason: "InstallComplete",
					},
					{
						Type:    hivev1.ProvisionedCondition,
						Status:  corev1.ConditionTrue,
						Reason:  hivev1.ProvisionedReasonProvisioned,
						Message: "Cluster is provisioned",
					},
				})
				assert.Equal(t, true, cd.Spec.Installed)
			},
		},
		{
			name: "RetryReasons: matching entry: retry",
			existing: []runtime.Object{
				testClusterDeploymentWithDefaultConditions(testClusterDeploymentWithInitializedConditions(testClusterDeployment())),
				testProvision(tcp.WithFailureReason("aReason")),
				testInstallConfigSecretAWS(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			retryReasons:          &[]string{"foo", "aReason", "bar"},
			expectPendingCreation: true,
			validate: func(c client.Client, t *testing.T) {
				if cd := getCD(c); assert.NotNil(t, cd, "no clusterdeployment found") {
					if cond := controllerutils.FindCondition(cd.Status.Conditions, hivev1.ProvisionStoppedCondition); assert.NotNil(t, cond, "no ProvisionStopped condition") {
						assert.Equal(t, corev1.ConditionFalse, cond.Status, "expected ProvisionStopped to be False")
					}
					if cond := controllerutils.FindCondition(cd.Status.Conditions, hivev1.ProvisionedCondition); assert.NotNil(t, cond, "no Provisioned condition") {
						assert.Equal(t, hivev1.ProvisionedReasonProvisioning, cond.Reason, "expected Provisioned to be Provisioning")
					}
				}
				assert.Len(t, getProvisions(c), 2, "expected 2 ClusterProvisions to exist")
			},
		},
		{
			name: "RetryReasons: matching entry but limit reached: no retry",
			existing: []runtime.Object{
				func() runtime.Object {
					cd := testClusterDeploymentWithDefaultConditions(testClusterDeploymentWithInitializedConditions(testClusterDeployment()))
					cd.Status.InstallRestarts = 2
					cd.Spec.InstallAttemptsLimit = pointer.Int32(2)
					return cd
				}(),
				testProvision(tcp.WithFailureReason("aReason")),
				testInstallConfigSecretAWS(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			retryReasons: &[]string{"foo", "aReason", "bar"},
			validate: func(c client.Client, t *testing.T) {
				if cd := getCD(c); assert.NotNil(t, cd, "no clusterdeployment found") {
					if cond := controllerutils.FindCondition(cd.Status.Conditions, hivev1.ProvisionStoppedCondition); assert.NotNil(t, cond, "no ProvisionStopped condition") {
						assert.Equal(t, corev1.ConditionTrue, cond.Status, "expected ProvisionStopped to be True")
						assert.Equal(t, "InstallAttemptsLimitReached", cond.Reason, "expected ProvisionStopped Reason to be InstallAttemptsLimitReached")
					}
					if cond := controllerutils.FindCondition(cd.Status.Conditions, hivev1.ProvisionedCondition); assert.NotNil(t, cond, "no Provisioned condition") {
						assert.Equal(t, hivev1.ProvisionedReasonProvisionStopped, cond.Reason, "expected Provisioned to be ProvisionStopped")
					}
				}
				assert.Len(t, getProvisions(c), 1, "expected 1 ClusterProvision to exist")
			},
		},
		{
			name: "RetryReasons: matching entry is in most recent provision: retry",
			existing: []runtime.Object{
				testClusterDeploymentWithDefaultConditions(testClusterDeploymentWithInitializedConditions(testClusterDeployment())),
				testProvision(tcp.WithFailureReason("aReason"), tcp.Attempt(0), tcp.WithCreationTimestamp(time.Now().Add(-2*time.Hour))),
				testProvision(tcp.WithFailureReason("bReason"), tcp.Attempt(1), tcp.WithCreationTimestamp(time.Now().Add(-1*time.Hour))),
				testProvision(tcp.WithFailureReason("cReason"), tcp.Attempt(2), tcp.WithCreationTimestamp(time.Now())),
				testInstallConfigSecretAWS(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			retryReasons:          &[]string{"foo", "cReason", "bar"},
			expectPendingCreation: true,
			validate: func(c client.Client, t *testing.T) {
				if cd := getCD(c); assert.NotNil(t, cd, "no clusterdeployment found") {
					if cond := controllerutils.FindCondition(cd.Status.Conditions, hivev1.ProvisionStoppedCondition); assert.NotNil(t, cond, "no ProvisionStopped condition") {
						assert.Equal(t, corev1.ConditionFalse, cond.Status, "expected ProvisionStopped to be False")
					}
					if cond := controllerutils.FindCondition(cd.Status.Conditions, hivev1.ProvisionedCondition); assert.NotNil(t, cond, "no Provisioned condition") {
						assert.Equal(t, hivev1.ProvisionedReasonProvisioning, cond.Reason, "expected Provisioned to be Provisioning")
					}
				}
				assert.Len(t, getProvisions(c), 4, "expected 4 ClusterProvisions to exist")
			},
		},
		{
			name: "RetryReasons: matching entry is in older provision: no retry",
			existing: []runtime.Object{
				testClusterDeploymentWithDefaultConditions(testClusterDeploymentWithInitializedConditions(testClusterDeployment())),
				testProvision(tcp.WithFailureReason("aReason"), tcp.Attempt(0), tcp.WithCreationTimestamp(time.Now().Add(-2*time.Hour))),
				testProvision(tcp.WithFailureReason("bReason"), tcp.Attempt(1), tcp.WithCreationTimestamp(time.Now().Add(-1*time.Hour))),
				testProvision(tcp.WithFailureReason("cReason"), tcp.Attempt(2), tcp.WithCreationTimestamp(time.Now())),
				testInstallConfigSecretAWS(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			retryReasons: &[]string{"foo", "bReason", "bar"},
			validate: func(c client.Client, t *testing.T) {
				if cd := getCD(c); assert.NotNil(t, cd, "no clusterdeployment found") {
					if cond := controllerutils.FindCondition(cd.Status.Conditions, hivev1.ProvisionStoppedCondition); assert.NotNil(t, cond, "no ProvisionStopped condition") {
						assert.Equal(t, corev1.ConditionTrue, cond.Status, "expected ProvisionStopped to be True")
						assert.Equal(t, "FailureReasonNotRetryable", cond.Reason, "expected ProvisionStopped Reason to be FailureReasonNotRetryable")
					}
					if cond := controllerutils.FindCondition(cd.Status.Conditions, hivev1.ProvisionedCondition); assert.NotNil(t, cond, "no Provisioned condition") {
						assert.Equal(t, hivev1.ProvisionedReasonProvisionStopped, cond.Reason, "expected Provisioned to be ProvisionStopped")
					}
				}
				assert.Len(t, getProvisions(c), 3, "expected 3 ClusterProvisions to exist")
			},
		},
		{
			name: "RetryReasons: no matching entry: no retry",
			existing: []runtime.Object{
				testClusterDeploymentWithDefaultConditions(testClusterDeploymentWithInitializedConditions(testClusterDeployment())),
				testProvision(tcp.WithFailureReason("aReason")),
				testInstallConfigSecretAWS(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			retryReasons: &[]string{"foo", "bReason", "bar"},
			validate: func(c client.Client, t *testing.T) {
				if cd := getCD(c); assert.NotNil(t, cd, "no clusterdeployment found") {
					if cond := controllerutils.FindCondition(cd.Status.Conditions, hivev1.ProvisionStoppedCondition); assert.NotNil(t, cond, "no ProvisionStopped condition") {
						assert.Equal(t, corev1.ConditionTrue, cond.Status, "expected ProvisionStopped to be True")
						assert.Equal(t, "FailureReasonNotRetryable", cond.Reason, "expected ProvisionStopped Reason to be FailureReasonNotRetryable")
					}
					if cond := controllerutils.FindCondition(cd.Status.Conditions, hivev1.ProvisionedCondition); assert.NotNil(t, cond, "no Provisioned condition") {
						assert.Equal(t, hivev1.ProvisionedReasonProvisionStopped, cond.Reason, "expected Provisioned to be ProvisionStopped")
					}
				}
				assert.Len(t, getProvisions(c), 1, "expected 1 ClusterProvision to exist")
			},
		},
		{
			name: "RetryReasons: no provision yet: list ignored, provision created",
			existing: []runtime.Object{
				testClusterDeploymentWithDefaultConditions(testClusterDeploymentWithInitializedConditions(testClusterDeployment())),
				testInstallConfigSecretAWS(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			retryReasons:          &[]string{"foo", "bar", "baz"},
			expectPendingCreation: true,
			validate: func(c client.Client, t *testing.T) {
				if cd := getCD(c); assert.NotNil(t, cd, "no clusterdeployment found") {
					if cond := controllerutils.FindCondition(cd.Status.Conditions, hivev1.ProvisionStoppedCondition); assert.NotNil(t, cond, "no ProvisionStopped condition") {
						assert.Equal(t, corev1.ConditionFalse, cond.Status, "expected ProvisionStopped to be False")
					}
					if cond := controllerutils.FindCondition(cd.Status.Conditions, hivev1.ProvisionedCondition); assert.NotNil(t, cond, "no Provisioned condition") {
						assert.Equal(t, hivev1.ProvisionedReasonProvisioning, cond.Reason, "expected Provisioned to be Provisioning")
					}
				}
				assert.Len(t, getProvisions(c), 1, "expected 1 ClusterProvisions to exist")
			},
		},
		{
			name: "RetryReasons: empty list: no retry",
			existing: []runtime.Object{
				testClusterDeploymentWithDefaultConditions(testClusterDeploymentWithInitializedConditions(testClusterDeployment())),
				testProvision(tcp.WithFailureReason("aReason")),
				testInstallConfigSecretAWS(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			// NB: This is *not* the same as the list being absent.
			retryReasons: &[]string{},
			validate: func(c client.Client, t *testing.T) {
				if cd := getCD(c); assert.NotNil(t, cd, "no clusterdeployment found") {
					if cond := controllerutils.FindCondition(cd.Status.Conditions, hivev1.ProvisionStoppedCondition); assert.NotNil(t, cond, "no ProvisionStopped condition") {
						assert.Equal(t, corev1.ConditionTrue, cond.Status, "expected ProvisionStopped to be True")
						assert.Equal(t, "FailureReasonNotRetryable", cond.Reason, "expected ProvisionStopped Reason to be FailureReasonNotRetryable")
					}
					if cond := controllerutils.FindCondition(cd.Status.Conditions, hivev1.ProvisionedCondition); assert.NotNil(t, cond, "no Provisioned condition") {
						assert.Equal(t, hivev1.ProvisionedReasonProvisionStopped, cond.Reason, "expected Provisioned to be ProvisionStopped")
					}
				}
				assert.Len(t, getProvisions(c), 1, "expected 1 ClusterProvision to exist")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logger := log.WithField("controller", "clusterDeployment")
			if test.retryReasons == nil {
				readFile = fakeReadFile("")
			} else {
				b, _ := json.Marshal(*test.retryReasons)
				readFile = fakeReadFile(fmt.Sprintf(`
				{
					"retryReasons": %s
				}
				`, string(b)))
			}
			scheme := scheme.GetScheme()
			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(test.existing...).Build()
			controllerExpectations := controllerutils.NewExpectations(logger)
			mockCtrl := gomock.NewController(t)

			mockRemoteClientBuilder := remoteclientmock.NewMockBuilder(mockCtrl)

			if test.platformCredentialsValidation == nil {
				test.platformCredentialsValidation = func(client.Client, *hivev1.ClusterDeployment, log.FieldLogger) (bool, error) {
					return true, nil
				}
			}
			rcd := &ReconcileClusterDeployment{
				Client:                                  fakeClient,
				scheme:                                  scheme,
				logger:                                  logger,
				expectations:                            controllerExpectations,
				remoteClusterAPIClientBuilder:           func(*hivev1.ClusterDeployment) remoteclient.Builder { return mockRemoteClientBuilder },
				validateCredentialsForClusterDeployment: test.platformCredentialsValidation,
				watchingClusterInstall: map[string]struct{}{
					(schema.GroupVersionKind{Group: "hive.openshift.io", Version: "v1", Kind: "FakeClusterInstall"}).String(): {},
				},
				releaseImageVerifier: test.riVerifier,
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

			if test.pendingCreation {
				controllerExpectations.ExpectCreations(reconcileRequest.String(), 1)
			}

			if test.expectConsoleRouteFetch {
				mockRemoteClientBuilder.EXPECT().Build().Return(testRemoteClusterAPIClient(), nil)
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
			assert.Equal(t, test.expectExplicitRequeue, result.Requeue, "unexpected requeue")

			actualPendingCreation := !controllerExpectations.SatisfiedExpectations(reconcileRequest.String())
			assert.Equal(t, test.expectPendingCreation, actualPendingCreation, "unexpected pending creation")
		})
	}
}

func TestClusterDeploymentReconcileResults(t *testing.T) {
	tests := []struct {
		name                     string
		existing                 []runtime.Object
		exptectedReconcileResult reconcile.Result
	}{
		{
			name: "Requeue after adding finalizer",
			existing: []runtime.Object{
				testClusterDeploymentWithoutFinalizer(),
			},
			exptectedReconcileResult: reconcile.Result{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logger := log.WithField("controller", "clusterDeployment")
			scheme := scheme.GetScheme()
			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(test.existing...).Build()
			controllerExpectations := controllerutils.NewExpectations(logger)
			mockCtrl := gomock.NewController(t)
			mockRemoteClientBuilder := remoteclientmock.NewMockBuilder(mockCtrl)
			rcd := &ReconcileClusterDeployment{
				Client:                        fakeClient,
				scheme:                        scheme,
				logger:                        logger,
				expectations:                  controllerExpectations,
				remoteClusterAPIClientBuilder: func(*hivev1.ClusterDeployment) remoteclient.Builder { return mockRemoteClientBuilder },
			}

			reconcileResult, err := rcd.Reconcile(context.TODO(), reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testName,
					Namespace: testNamespace,
				},
			})

			assert.NoError(t, err, "unexpected error")

			assert.Equal(t, test.exptectedReconcileResult, reconcileResult, "unexpected reconcile result")
		})
	}
}

func TestCalculateNextProvisionTime(t *testing.T) {
	cases := []struct {
		name             string
		failureTime      time.Time
		attempt          int
		expectedNextTime time.Time
	}{
		{
			name:             "first attempt",
			failureTime:      time.Date(2019, time.July, 16, 0, 0, 0, 0, time.UTC),
			attempt:          0,
			expectedNextTime: time.Date(2019, time.July, 16, 0, 1, 0, 0, time.UTC),
		},
		{
			name:             "second attempt",
			failureTime:      time.Date(2019, time.July, 16, 0, 0, 0, 0, time.UTC),
			attempt:          1,
			expectedNextTime: time.Date(2019, time.July, 16, 0, 2, 0, 0, time.UTC),
		},
		{
			name:             "third attempt",
			failureTime:      time.Date(2019, time.July, 16, 0, 0, 0, 0, time.UTC),
			attempt:          2,
			expectedNextTime: time.Date(2019, time.July, 16, 0, 4, 0, 0, time.UTC),
		},
		{
			name:             "eleventh attempt",
			failureTime:      time.Date(2019, time.July, 16, 0, 0, 0, 0, time.UTC),
			attempt:          10,
			expectedNextTime: time.Date(2019, time.July, 16, 17, 4, 0, 0, time.UTC),
		},
		{
			name:             "twelfth attempt",
			failureTime:      time.Date(2019, time.July, 16, 0, 0, 0, 0, time.UTC),
			attempt:          11,
			expectedNextTime: time.Date(2019, time.July, 17, 0, 0, 0, 0, time.UTC),
		},
		{
			name:             "thirteenth attempt",
			failureTime:      time.Date(2019, time.July, 16, 0, 0, 0, 0, time.UTC),
			attempt:          12,
			expectedNextTime: time.Date(2019, time.July, 17, 0, 0, 0, 0, time.UTC),
		},
		{
			name:             "millionth attempt",
			failureTime:      time.Date(2019, time.July, 16, 0, 0, 0, 0, time.UTC),
			attempt:          999999,
			expectedNextTime: time.Date(2019, time.July, 17, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actualNextTime := calculateNextProvisionTime(tc.failureTime, tc.attempt, log.WithField("controller", "clusterDeployment"))
			assert.Equal(t, tc.expectedNextTime.String(), actualNextTime.String(), "unexpected next provision time")
		})
	}
}

func TestDeleteStaleProvisions(t *testing.T) {
	cases := []struct {
		name             string
		existingAttempts []int
		expectedAttempts []int
	}{
		{
			name: "none",
		},
		{
			name:             "one",
			existingAttempts: []int{0},
			expectedAttempts: []int{0},
		},
		{
			name:             "three",
			existingAttempts: []int{0, 1, 2},
			expectedAttempts: []int{0, 1, 2},
		},
		{
			name:             "four",
			existingAttempts: []int{0, 1, 2, 3},
			expectedAttempts: []int{0, 2, 3},
		},
		{
			name:             "five",
			existingAttempts: []int{0, 1, 2, 3, 4},
			expectedAttempts: []int{0, 3, 4},
		},
		{
			name:             "five mixed order",
			existingAttempts: []int{10, 3, 7, 8, 1},
			expectedAttempts: []int{1, 8, 10},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provisions := make([]runtime.Object, len(tc.existingAttempts))
			for i, a := range tc.existingAttempts {
				provisions[i] = testProvision(tcp.Failed(), tcp.Attempt(a))
			}
			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(provisions...).Build()
			rcd := &ReconcileClusterDeployment{
				Client: fakeClient,
				scheme: scheme.GetScheme(),
			}
			rcd.deleteStaleProvisions(getProvisions(fakeClient), log.WithField("test", "TestDeleteStaleProvisions"))
			actualAttempts := []int{}
			for _, p := range getProvisions(fakeClient) {
				actualAttempts = append(actualAttempts, p.Spec.Attempt)
			}
			assert.ElementsMatch(t, tc.expectedAttempts, actualAttempts, "unexpected provisions kept")
		})
	}
}

func TestDeleteOldFailedProvisions(t *testing.T) {
	cases := []struct {
		name                                    string
		totalProvisions                         int
		failedProvisionsMoreThanSevenDaysOld    int
		expectedNumberOfProvisionsAfterDeletion int
	}{
		{
			name:                                    "One failed provision more than 7 days old",
			totalProvisions:                         2,
			failedProvisionsMoreThanSevenDaysOld:    1,
			expectedNumberOfProvisionsAfterDeletion: 1,
		},
		{
			name:                                    "No failed provision more than 7 days old",
			totalProvisions:                         2,
			failedProvisionsMoreThanSevenDaysOld:    0,
			expectedNumberOfProvisionsAfterDeletion: 2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provisions := make([]runtime.Object, tc.totalProvisions)
			for i := 0; i < tc.totalProvisions; i++ {
				if i < tc.failedProvisionsMoreThanSevenDaysOld {
					provisions[i] = testProvision(
						tcp.Failed(),
						tcp.WithCreationTimestamp(time.Now().Add(-7*24*time.Hour)),
						tcp.Attempt(i))
				} else {
					provisions[i] = testProvision(
						tcp.Failed(),
						tcp.WithCreationTimestamp(time.Now()),
						tcp.Attempt(i))
				}
			}
			scheme := scheme.GetScheme()
			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(provisions...).Build()
			rcd := &ReconcileClusterDeployment{
				Client: fakeClient,
				scheme: scheme,
			}
			rcd.deleteOldFailedProvisions(getProvisions(fakeClient), log.WithField("test", "TestDeleteOldFailedProvisions"))
			assert.Len(t, getProvisions(fakeClient), tc.expectedNumberOfProvisionsAfterDeletion, "unexpected provisions kept")
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

func testInstallConfigSecretAWS() *corev1.Secret {
	return testInstallConfigSecret(testAWSIC)
}

func testInstallConfigSecret(icData string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      installConfigSecretName,
		},
		Data: map[string][]byte{
			"install-config.yaml": []byte(icData),
		},
	}
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
			InstallConfigSecretRef: &corev1.LocalObjectReference{Name: installConfigSecretName},
		},
		ClusterMetadata: &hivev1.ClusterMetadata{
			ClusterID:                testClusterID,
			InfraID:                  testInfraID,
			AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: adminKubeconfigSecret},
			AdminPasswordSecretRef:   &corev1.LocalObjectReference{Name: adminPasswordSecret},
			Platform: &hivev1.ClusterPlatformMetadata{
				AWS: &hivev1aws.Metadata{
					HostedZoneRole: pointer.String("account-b-role"),
				},
			},
		},
	}

	if cd.Labels == nil {
		cd.Labels = make(map[string]string, 2)
	}
	cd.Labels[hivev1.HiveClusterPlatformLabel] = "aws"
	cd.Labels[hivev1.HiveClusterRegionLabel] = "us-east-1"

	cd.Status = hivev1.ClusterDeploymentStatus{
		InstallerImage: pointer.String("installer-image:latest"),
		CLIImage:       pointer.String("cli:latest"),
	}

	return cd
}

func testClusterDeploymentWithDefaultConditions(cd *hivev1.ClusterDeployment) *hivev1.ClusterDeployment {
	cd.Status.Conditions = addOrUpdateClusterDeploymentCondition(*cd,
		hivev1.InstallImagesNotResolvedCondition,
		corev1.ConditionFalse,
		imagesResolvedReason,
		imagesResolvedMsg)
	cd.Status.Conditions = addOrUpdateClusterDeploymentCondition(*cd,
		hivev1.AuthenticationFailureClusterDeploymentCondition,
		corev1.ConditionFalse,
		platformAuthSuccessReason,
		"Platform credentials passed authentication check")
	cd.Status.Conditions = addOrUpdateClusterDeploymentCondition(*cd,
		hivev1.ProvisionStoppedCondition,
		corev1.ConditionFalse,
		"ProvisionNotStopped",
		"Provision is not stopped")
	cd.Status.Conditions = addOrUpdateClusterDeploymentCondition(*cd,
		hivev1.RequirementsMetCondition,
		corev1.ConditionTrue,
		"AllRequirementsMet",
		"All pre-provision requirements met")
	return cd
}

func testClusterDeploymentWithInitializedConditions(cd *hivev1.ClusterDeployment) *hivev1.ClusterDeployment {
	for _, condition := range clusterDeploymentConditions {
		cd.Status.Conditions = append(cd.Status.Conditions, hivev1.ClusterDeploymentCondition{
			Status:  corev1.ConditionUnknown,
			Type:    condition,
			Reason:  hivev1.InitializedConditionReason,
			Message: "Condition Initialized",
		})
	}
	return cd
}

func testClusterDeploymentCustomization(name string) *hivev1.ClusterDeploymentCustomization {
	cdc := &hivev1.ClusterDeploymentCustomization{}
	cdc.Name = name
	cdc.Namespace = testNamespace
	return cdc
}

func testInstalledClusterDeployment(installedAt time.Time) *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.Spec.Installed = true
	cd.Status.InstalledTimestamp = &metav1.Time{Time: installedAt}
	cd.Status.APIURL = "http://quite.fake.com"
	cd.Status.WebConsoleURL = "http://quite.fake.com/console"
	return cd
}

func testClusterInstallRefClusterDeployment(name string) *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.Spec.Provisioning = nil
	cd.Spec.ClusterInstallRef = &hivev1.ClusterInstallLocalReference{
		Group:   "hive.openshift.io",
		Version: "v1",
		Kind:    "FakeClusterInstall",
		Name:    name,
	}
	return cd
}

func testClusterDeploymentWithoutFinalizer() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.Finalizers = []string{}
	return cd
}

func testClusterDeploymentWithoutPlatformLabel() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	delete(cd.Labels, hivev1.HiveClusterPlatformLabel)
	return cd
}

func testClusterDeploymentWithoutRegionLabel() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	delete(cd.Labels, hivev1.HiveClusterRegionLabel)
	return cd
}

func testDeletedClusterDeployment() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	now := metav1.Now()
	cd.DeletionTimestamp = &now
	return cd
}

func testExpiredClusterDeployment() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.CreationTimestamp = metav1.Time{Time: metav1.Now().Add(-60 * time.Minute)}
	if cd.Annotations == nil {
		cd.Annotations = make(map[string]string, 1)
	}
	cd.Annotations[deleteAfterAnnotation] = "5m"
	return cd
}

func testClusterDeploymentWithProvision() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.Status.ProvisionRef = &corev1.LocalObjectReference{Name: provisionName}
	return cd
}

func testEmptyFakeClusterInstall(name string) *unstructured.Unstructured {
	fake := &unstructured.Unstructured{}
	fake.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "hive.openshift.io",
		Version: "v1",
		Kind:    "FakeClusterInstall",
	})
	fake.SetNamespace(testNamespace)
	fake.SetName(name)
	unstructured.SetNestedField(fake.UnstructuredContent(), map[string]interface{}{}, "spec")
	unstructured.SetNestedField(fake.UnstructuredContent(), map[string]interface{}{}, "status")
	return fake
}

func testFakeClusterInstall(name string) *unstructured.Unstructured {
	fake := testEmptyFakeClusterInstall(name)
	unstructured.SetNestedField(fake.UnstructuredContent(), map[string]interface{}{
		"name": testClusterImageSetName,
	}, "spec", "imageSetRef")
	return fake
}

func testFakeClusterInstallWithConditions(name string, conditions []hivev1.ClusterInstallCondition) *unstructured.Unstructured {
	fake := testFakeClusterInstall(name)

	value := []interface{}{}
	for _, c := range conditions {
		value = append(value, map[string]interface{}{
			"type":    string(c.Type),
			"status":  string(c.Status),
			"reason":  c.Reason,
			"message": c.Message,
		})
	}

	unstructured.SetNestedField(fake.UnstructuredContent(), value, "status", "conditions")
	return fake
}

func testFakeClusterInstallWithClusterMetadata(name string, metadata hivev1.ClusterMetadata) *unstructured.Unstructured {
	fake := testFakeClusterInstall(name)

	value := map[string]interface{}{
		"clusterID": metadata.ClusterID,
		"infraID":   metadata.InfraID,
		"adminKubeconfigSecretRef": map[string]interface{}{
			"name": metadata.AdminKubeconfigSecretRef.Name,
		},
	}

	if metadata.AdminPasswordSecretRef != nil {
		value["adminPasswordSecretRef"] = map[string]interface{}{
			"name": metadata.AdminPasswordSecretRef.Name,
		}
	}

	unstructured.SetNestedField(fake.UnstructuredContent(), value, "spec", "clusterMetadata")
	return fake
}

func testProvision(opts ...tcp.Option) *hivev1.ClusterProvision {
	cd := testClusterDeployment()
	provision := tcp.FullBuilder(testNamespace, provisionName).Build(tcp.WithClusterDeploymentRef(testName))

	controllerutil.SetControllerReference(cd, provision, scheme.GetScheme())

	for _, opt := range opts {
		opt(provision)
	}

	return provision
}

func testSuccessfulProvision(opts ...tcp.Option) *hivev1.ClusterProvision {
	opts = append(opts, tcp.Successful(
		testClusterID, testInfraID, adminKubeconfigSecret, adminPasswordSecret))
	return testProvision(opts...)
}

func testMetadataConfigMap() *corev1.ConfigMap {
	cm := &corev1.ConfigMap{}
	cm.Name = metadataName
	cm.Namespace = testNamespace
	metadataJSON := `{
		"aws": {
			"identifier": [{"openshiftClusterID": "testFooClusterUUID"}]
		}
	}`
	cm.Data = map[string]string{"metadata.json": metadataJSON}
	return cm
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

func testRemoteClusterAPIClient() client.Client {
	remoteClusterRouteObject := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      remoteClusterRouteObjectName,
			Namespace: remoteClusterRouteObjectNamespace,
		},
	}
	remoteClusterRouteObject.Spec.Host = "bar-api.clusters.example.com:6443/console"
	return testfake.NewFakeClientBuilder().WithRuntimeObjects(remoteClusterRouteObject).Build()
}

func testClusterImageSet() *hivev1.ClusterImageSet {
	cis := &hivev1.ClusterImageSet{}
	cis.Name = testClusterImageSetName
	cis.Spec.ReleaseImage = "test-release-image:latest"
	return cis
}

func testDNSZone() *hivev1.DNSZone {
	zone := &hivev1.DNSZone{}
	zone.Name = testName + "-zone"
	zone.Namespace = testNamespace
	zone.OwnerReferences = append(
		zone.OwnerReferences,
		*metav1.NewControllerRef(
			testClusterDeployment(),
			schema.GroupVersionKind{
				Group:   "hive.openshift.io",
				Version: "v1",
				Kind:    "clusterdeployment",
			},
		),
	)
	return zone
}

func testDNSZoneWithFinalizer() *hivev1.DNSZone {
	zone := testDNSZone()
	zone.ObjectMeta.Finalizers = []string{"hive.openshift.io/dnszone"}
	return zone
}

func testAvailableDNSZone() *hivev1.DNSZone {
	zone := testDNSZoneWithFinalizer()
	zone.Status.Conditions = []hivev1.DNSZoneCondition{
		{
			Type:    hivev1.ZoneAvailableDNSZoneCondition,
			Status:  corev1.ConditionTrue,
			Reason:  dnsReadyReason,
			Message: "DNS Zone available",
			LastTransitionTime: metav1.Time{
				Time: time.Now(),
			},
		},
	}
	return zone
}

func testDNSZoneWithInvalidCredentialsCondition() *hivev1.DNSZone {
	zone := testDNSZone()
	zone.Status.Conditions = []hivev1.DNSZoneCondition{
		{
			Type:   hivev1.InsufficientCredentialsCondition,
			Status: corev1.ConditionTrue,
			LastTransitionTime: metav1.Time{
				Time: time.Now(),
			},
		},
	}
	return zone
}
func testDNSZoneWithAPIOptInRequiredCondition() *hivev1.DNSZone {
	zone := testDNSZone()
	zone.Status.Conditions = []hivev1.DNSZoneCondition{
		{
			Type:   hivev1.APIOptInRequiredCondition,
			Status: corev1.ConditionTrue,
			LastTransitionTime: metav1.Time{
				Time: time.Now(),
			},
		},
	}
	return zone
}

func testDNSZoneWithAuthenticationFailureCondition() *hivev1.DNSZone {
	zone := testDNSZone()
	zone.Status.Conditions = []hivev1.DNSZoneCondition{
		{
			Type:   hivev1.AuthenticationFailureCondition,
			Status: corev1.ConditionTrue,
			LastTransitionTime: metav1.Time{
				Time: time.Now(),
			},
		},
	}
	return zone
}

func testDNSZoneWithDNSErrorCondition() *hivev1.DNSZone {
	zone := testDNSZone()
	zone.Status.Conditions = []hivev1.DNSZoneCondition{
		{
			Type:    hivev1.GenericDNSErrorsCondition,
			Status:  corev1.ConditionTrue,
			Reason:  "CloudError",
			Message: "Some cloud error occurred",
			LastTransitionTime: metav1.Time{
				Time: time.Now(),
			},
		},
	}
	return zone
}

func addOrUpdateClusterDeploymentCondition(cd hivev1.ClusterDeployment,
	condition hivev1.ClusterDeploymentConditionType, status corev1.ConditionStatus,
	reason string, message string) []hivev1.ClusterDeploymentCondition {
	newConditions := cd.Status.Conditions
	changed := false
	for i, cond := range newConditions {
		if cond.Type == condition {
			cond.Status = status
			cond.Reason = reason
			cond.Message = message
			newConditions[i] = cond
			changed = true
			break
		}
	}
	if !changed {
		newConditions = append(newConditions, hivev1.ClusterDeploymentCondition{
			Type:    condition,
			Status:  status,
			Reason:  reason,
			Message: message,
		})
	}
	return newConditions
}

// sanitizeConditions scrubs the condition list for each CD in `cds` so they can reasonably be compared
func sanitizeConditions(cds ...*hivev1.ClusterDeployment) {
	for _, cd := range cds {
		cd.Status.Conditions = controllerutils.SortClusterDeploymentConditions(cd.Status.Conditions)
		for i := range cd.Status.Conditions {
			cd.Status.Conditions[i].LastProbeTime = metav1.Time{}
			cd.Status.Conditions[i].LastTransitionTime = metav1.Time{}
		}
	}
}

func getJob(c client.Client, name string) *batchv1.Job {
	job := &batchv1.Job{}
	err := c.Get(context.TODO(), client.ObjectKey{Name: name, Namespace: testNamespace}, job)
	if err == nil {
		return job
	}
	return nil
}

func TestUpdatePullSecretInfo(t *testing.T) {
	testPullSecret1 := `{"auths": {"registry.svc.ci.okd.org": {"auth": "dXNljlfjldsfSDD"}}}`

	tests := []struct {
		name       string
		existingCD []runtime.Object
		validate   func(*testing.T, *corev1.Secret)
	}{
		{
			name: "update existing merged pull secret with the new pull secret",
			existingCD: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.ClusterMetadata.AdminKubeconfigSecretRef = corev1.LocalObjectReference{Name: adminKubeconfigSecret}
					return cd
				}(),
				testSecret(corev1.SecretTypeOpaque, adminKubeconfigSecret, "kubeconfig", adminKubeconfig),
				testSecret(corev1.SecretTypeDockercfg, pullSecretSecret, corev1.DockerConfigJsonKey, testPullSecret1),
				testSecret(corev1.SecretTypeDockercfg, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(t *testing.T, pullSecretObj *corev1.Secret) {
				pullSecret, ok := pullSecretObj.Data[corev1.DockerConfigJsonKey]
				if !ok {
					t.Error("Error getting pull secret")
				}
				assert.Equal(t, string(pullSecret), testPullSecret1)
			},
		},
		{
			name: "Add a new merged pull secret",
			existingCD: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeploymentWithInitializedConditions(testClusterDeployment())
					cd.Spec.ClusterMetadata.AdminKubeconfigSecretRef = corev1.LocalObjectReference{Name: adminKubeconfigSecret}
					return cd
				}(),
				testSecret(corev1.SecretTypeOpaque, adminKubeconfigSecret, "kubeconfig", adminKubeconfig),
				testSecret(corev1.SecretTypeDockercfg, pullSecretSecret, corev1.DockerConfigJsonKey, testPullSecret1),
			},
			validate: func(t *testing.T, pullSecretObj *corev1.Secret) {
				assert.Equal(t, testClusterDeployment().Name, pullSecretObj.Labels[constants.ClusterDeploymentNameLabel], "incorrect cluster deployment name label")
				assert.Equal(t, constants.SecretTypeMergedPullSecret, pullSecretObj.Labels[constants.SecretTypeLabel], "incorrect secret type label")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scheme := scheme.GetScheme()
			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(test.existingCD...).Build()
			mockCtrl := gomock.NewController(t)
			mockRemoteClientBuilder := remoteclientmock.NewMockBuilder(mockCtrl)
			rcd := &ReconcileClusterDeployment{
				Client:                        fakeClient,
				scheme:                        scheme,
				logger:                        log.WithField("controller", "clusterDeployment"),
				remoteClusterAPIClientBuilder: func(*hivev1.ClusterDeployment) remoteclient.Builder { return mockRemoteClientBuilder },
				validateCredentialsForClusterDeployment: func(client.Client, *hivev1.ClusterDeployment, log.FieldLogger) (bool, error) {
					return true, nil
				},
			}

			_, err := rcd.Reconcile(context.TODO(), reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testName,
					Namespace: testNamespace,
				},
			})
			assert.NoError(t, err, "unexpected error")

			cd := getCDFromClient(rcd.Client)
			mergedSecretName := constants.GetMergedPullSecretName(cd)
			existingPullSecretObj := &corev1.Secret{}
			err = rcd.Get(context.TODO(), types.NamespacedName{Name: mergedSecretName, Namespace: cd.Namespace}, existingPullSecretObj)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if test.validate != nil {
				test.validate(t, existingPullSecretObj)
			}
		})
	}
}

func getCDWithoutPullSecret() *hivev1.ClusterDeployment {
	cd := testEmptyClusterDeployment()

	cd.Spec = hivev1.ClusterDeploymentSpec{
		ClusterName: testClusterName,
		Platform: hivev1.Platform{
			AWS: &hivev1aws.Platform{
				CredentialsSecretRef: corev1.LocalObjectReference{
					Name: "aws-credentials",
				},
				Region: "us-east-1",
			},
		},
		ClusterMetadata: &hivev1.ClusterMetadata{
			ClusterID:                testClusterID,
			InfraID:                  testInfraID,
			AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: adminKubeconfigSecret},
		},
	}
	cd.Status = hivev1.ClusterDeploymentStatus{
		InstallerImage: pointer.String("installer-image:latest"),
	}
	return cd
}

func getCDFromClient(c client.Client) *hivev1.ClusterDeployment {
	cd := &hivev1.ClusterDeployment{}
	err := c.Get(context.TODO(), client.ObjectKey{Name: testName, Namespace: testNamespace}, cd)
	if err == nil {
		return cd
	}
	return nil
}

func createGlobalPullSecretObj(secretType corev1.SecretType, name, key, value string) *corev1.Secret {
	secret := &corev1.Secret{
		Type: secretType,
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: constants.DefaultHiveNamespace,
		},
		Data: map[string][]byte{
			key: []byte(value),
		},
	}
	return secret
}

func TestMergePullSecrets(t *testing.T) {

	tests := []struct {
		name                    string
		localPullSecret         string
		globalPullSecret        string
		mergedPullSecret        string
		existingObjs            []runtime.Object
		expectedErr             bool
		addGlobalSecretToHiveNs bool
	}{
		{
			name:             "merged pull secret should be be equal to local secret",
			localPullSecret:  `{"auths": {"registry.svc.ci.okd.org": {"auth": "dXNljlfjldsfSDD"}}}`,
			mergedPullSecret: `{"auths": {"registry.svc.ci.okd.org": {"auth": "dXNljlfjldsfSDD"}}}`,
			existingObjs: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := getCDWithoutPullSecret()
					cd.Spec.PullSecretRef = &corev1.LocalObjectReference{
						Name: pullSecretSecret,
					}
					return cd
				}(),
			},
		},
		{
			name:             "merged pull secret should be be equal to global pull secret",
			globalPullSecret: `{"auths": {"registry.svc.ci.okd.org": {"auth": "dXNljlfjldsfSDD"}}}`,
			mergedPullSecret: `{"auths": {"registry.svc.ci.okd.org": {"auth": "dXNljlfjldsfSDD"}}}`,
			existingObjs: []runtime.Object{
				getCDWithoutPullSecret(),
			},
			addGlobalSecretToHiveNs: true,
		},
		{
			name:             "Both local secret and global pull secret available",
			localPullSecret:  `{"auths": {"registry.svc.ci.okd.org": {"auth": "dXNljlfjldsfSDD"}}}`,
			globalPullSecret: `{"auths":{"cloud.okd.com":{"auth":"b34xVjWERckjfUyV1pMQTc=","email":"abc@xyz.com"}}}`,
			mergedPullSecret: `{"auths":{"cloud.okd.com":{"auth":"b34xVjWERckjfUyV1pMQTc=","email":"abc@xyz.com"},"registry.svc.ci.okd.org":{"auth":"dXNljlfjldsfSDD"}}}`,
			existingObjs: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := getCDWithoutPullSecret()
					cd.Spec.PullSecretRef = &corev1.LocalObjectReference{
						Name: pullSecretSecret,
					}
					return cd
				}(),
			},
			addGlobalSecretToHiveNs: true,
		},
		{
			name:             "global pull secret does not exist in Hive namespace",
			globalPullSecret: `{"auths": {"registry.svc.ci.okd.org": {"auth": "dXNljlfjldsfSDD"}}}`,
			existingObjs: []runtime.Object{
				getCDWithoutPullSecret(),
			},
			addGlobalSecretToHiveNs: false,
			expectedErr:             true,
		},
		{
			name: "Test should fail as local an global pull secret is not available",
			existingObjs: []runtime.Object{
				getCDWithoutPullSecret(),
			},
			expectedErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.globalPullSecret != "" && test.addGlobalSecretToHiveNs == true {
				globalPullSecretObj := createGlobalPullSecretObj(corev1.SecretTypeDockerConfigJson, globalPullSecret, corev1.DockerConfigJsonKey, test.globalPullSecret)
				test.existingObjs = append(test.existingObjs, globalPullSecretObj)
			}
			if test.localPullSecret != "" {
				localSecretObject := testSecret(corev1.SecretTypeDockercfg, pullSecretSecret, corev1.DockerConfigJsonKey, test.localPullSecret)
				test.existingObjs = append(test.existingObjs, localSecretObject)
			}
			scheme := scheme.GetScheme()
			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(test.existingObjs...).Build()
			mockCtrl := gomock.NewController(t)
			mockRemoteClientBuilder := remoteclientmock.NewMockBuilder(mockCtrl)
			rcd := &ReconcileClusterDeployment{
				Client:                        fakeClient,
				scheme:                        scheme,
				logger:                        log.WithField("controller", "clusterDeployment"),
				remoteClusterAPIClientBuilder: func(*hivev1.ClusterDeployment) remoteclient.Builder { return mockRemoteClientBuilder },
			}

			cd := getCDFromClient(rcd.Client)
			if test.globalPullSecret != "" {
				os.Setenv(constants.GlobalPullSecret, globalPullSecret)
			}
			defer os.Unsetenv(constants.GlobalPullSecret)

			expetedPullSecret, err := rcd.mergePullSecrets(cd, rcd.logger)
			if test.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			if test.mergedPullSecret != "" {
				assert.Equal(t, test.mergedPullSecret, expetedPullSecret)
			}
		})
	}
}

func TestCopyInstallLogSecret(t *testing.T) {

	tests := []struct {
		name                    string
		existingObjs            []runtime.Object
		existingEnvVars         []corev1.EnvVar
		expectedErr             bool
		expectedNumberOfSecrets int
	}{
		{
			name: "copies secret",
			existingObjs: []runtime.Object{
				testSecretWithNamespace(corev1.SecretTypeOpaque, installLogSecret, "hive", "cloud", "cloudsecret"),
			},
			expectedNumberOfSecrets: 1,
			existingEnvVars: []corev1.EnvVar{
				{
					Name:  constants.InstallLogsCredentialsSecretRefEnvVar,
					Value: installLogSecret,
				},
			},
		},
		{
			name:        "missing secret",
			expectedErr: true,
			existingEnvVars: []corev1.EnvVar{
				{
					Name:  constants.InstallLogsCredentialsSecretRefEnvVar,
					Value: installLogSecret,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scheme := scheme.GetScheme()
			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(test.existingObjs...).Build()
			mockCtrl := gomock.NewController(t)
			mockRemoteClientBuilder := remoteclientmock.NewMockBuilder(mockCtrl)
			rcd := &ReconcileClusterDeployment{
				Client:                        fakeClient,
				scheme:                        scheme,
				logger:                        log.WithField("controller", "clusterDeployment"),
				remoteClusterAPIClientBuilder: func(*hivev1.ClusterDeployment) remoteclient.Builder { return mockRemoteClientBuilder },
			}

			for i, envVar := range test.existingEnvVars {
				if err := os.Setenv(envVar.Name, envVar.Value); err == nil {
					defer func(evar corev1.EnvVar) {
						if err := os.Unsetenv(evar.Name); err != nil {
							t.Error(err)
						}
					}(test.existingEnvVars[i])
				} else {
					t.Error(err)
				}
			}

			err := rcd.copyInstallLogSecret(testNamespace, test.existingEnvVars)
			secretList := &corev1.SecretList{}
			listErr := rcd.List(context.TODO(), secretList, client.InNamespace(testNamespace))

			if test.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.NoError(t, listErr, "Listing secrets returned an unexpected error")
			assert.Equal(t, test.expectedNumberOfSecrets, len(secretList.Items), "Number of secrets different than expected")
		})
	}
}

func TestEnsureManagedDNSZone(t *testing.T) {

	goodDNSZone := func() *hivev1.DNSZone {
		return testdnszone.Build(
			dnsZoneBase(),
			testdnszone.WithControllerOwnerReference(testclusterdeployment.Build(
				clusterDeploymentBase(),
			)),
			testdnszone.WithCondition(hivev1.DNSZoneCondition{
				Type: hivev1.ZoneAvailableDNSZoneCondition,
			}),
		)
	}
	tests := []struct {
		name                         string
		existingObjs                 []runtime.Object
		existingEnvVars              []corev1.EnvVar
		clusterDeployment            *hivev1.ClusterDeployment
		expectedUpdated              bool
		expectedResult               reconcile.Result
		expectedErr                  bool
		expectedDNSZone              *hivev1.DNSZone
		expectedDNSNotReadyCondition *hivev1.ClusterDeploymentCondition
	}{
		{
			name: "unsupported platform",
			clusterDeployment: testclusterdeployment.Build(
				testclusterdeployment.WithNamespace(testNamespace),
				testclusterdeployment.WithName(testName),
			),
			expectedErr: true,
			expectedDNSNotReadyCondition: &hivev1.ClusterDeploymentCondition{
				Type:   hivev1.DNSNotReadyCondition,
				Status: corev1.ConditionTrue,
				Reason: dnsUnsupportedPlatformReason,
			},
		},
		{
			name: "create zone",
			clusterDeployment: testclusterdeployment.Build(
				clusterDeploymentBase(),
			),
			expectedUpdated: true,
			expectedDNSZone: goodDNSZone(),
		},
		{
			name: "zone already exists and is owned by clusterdeployment",
			existingObjs: []runtime.Object{
				goodDNSZone(),
			},
			clusterDeployment: testclusterdeployment.Build(
				clusterDeploymentBase(),
				testclusterdeployment.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:   hivev1.DNSNotReadyCondition,
					Status: corev1.ConditionUnknown,
				}),
				testclusterdeployment.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:   hivev1.ProvisionStoppedCondition,
					Status: corev1.ConditionUnknown,
				}),
			),
			expectedUpdated: true,
			expectedResult:  reconcile.Result{Requeue: true, RequeueAfter: defaultDNSNotReadyTimeout},
			expectedDNSNotReadyCondition: &hivev1.ClusterDeploymentCondition{
				Type:   hivev1.DNSNotReadyCondition,
				Status: corev1.ConditionTrue,
				Reason: dnsNotReadyReason,
			},
		},
		{
			name: "zone already exists but is not owned by clusterdeployment",
			existingObjs: []runtime.Object{
				testdnszone.Build(
					dnsZoneBase(),
				),
			},
			clusterDeployment: testclusterdeployment.Build(
				clusterDeploymentBase(),
			),
			expectedUpdated: true,
			expectedErr:     true,
			expectedDNSNotReadyCondition: &hivev1.ClusterDeploymentCondition{
				Type:   hivev1.DNSNotReadyCondition,
				Status: corev1.ConditionTrue,
				Reason: dnsZoneResourceConflictReason,
			},
		},
		{
			name: "zone already exists and is owned by clusterdeployment, but has timed out",
			existingObjs: []runtime.Object{
				goodDNSZone(),
			},
			clusterDeployment: testclusterdeployment.Build(
				clusterDeploymentBase(),
				testclusterdeployment.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:               hivev1.DNSNotReadyCondition,
					Status:             corev1.ConditionTrue,
					Reason:             dnsNotReadyReason,
					LastProbeTime:      metav1.Time{Time: time.Now().Add(-20 * time.Minute)},
					LastTransitionTime: metav1.Time{Time: time.Now().Add(-20 * time.Minute)},
				}),
				testclusterdeployment.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:   hivev1.ProvisionStoppedCondition,
					Status: corev1.ConditionUnknown,
				}),
			),
			expectedUpdated: true,
			expectedResult:  reconcile.Result{Requeue: true},
			expectedDNSNotReadyCondition: &hivev1.ClusterDeploymentCondition{
				Type:   hivev1.DNSNotReadyCondition,
				Status: corev1.ConditionTrue,
				Reason: dnsNotReadyTimedoutReason,
			},
		},
		{
			name: "no-op if already timed out",
			existingObjs: []runtime.Object{
				goodDNSZone(),
			},
			clusterDeployment: testclusterdeployment.Build(
				clusterDeploymentBase(),
				testclusterdeployment.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:   hivev1.DNSNotReadyCondition,
					Status: corev1.ConditionTrue,
					Reason: dnsNotReadyTimedoutReason,
				}),
				testclusterdeployment.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:   hivev1.ProvisionStoppedCondition,
					Status: corev1.ConditionUnknown,
				}),
			),
			expectedDNSNotReadyCondition: &hivev1.ClusterDeploymentCondition{
				Type:   hivev1.DNSNotReadyCondition,
				Status: corev1.ConditionTrue,
				Reason: dnsNotReadyTimedoutReason,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			existingObjs := append(test.existingObjs, test.clusterDeployment)
			scheme := scheme.GetScheme()
			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(existingObjs...).Build()
			mockCtrl := gomock.NewController(t)
			mockRemoteClientBuilder := remoteclientmock.NewMockBuilder(mockCtrl)
			rcd := &ReconcileClusterDeployment{
				Client:                        fakeClient,
				scheme:                        scheme,
				logger:                        log.WithField("controller", "clusterDeployment"),
				remoteClusterAPIClientBuilder: func(*hivev1.ClusterDeployment) remoteclient.Builder { return mockRemoteClientBuilder },
			}

			// act
			updated, result, err := rcd.ensureManagedDNSZone(test.clusterDeployment, rcd.logger)
			actualDNSNotReadyCondition := controllerutils.FindCondition(test.clusterDeployment.Status.Conditions, hivev1.DNSNotReadyCondition)

			// assert
			assert.Equal(t, test.expectedUpdated, updated, "Unexpected 'updated' return")
			assert.Equal(t, test.expectedResult, result, "Unexpected 'result' return")
			if test.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Tests that start off with an existing DNSZone may not want to bother checking that it still exists
			if test.expectedDNSZone != nil {
				actualDNSZone := &hivev1.DNSZone{}
				fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: testNamespace, Name: controllerutils.DNSZoneName(testName)}, actualDNSZone)
				// Just assert the fields we care about. Otherwise we have to muck with e.g. typemeta
				assert.Equal(t, test.expectedDNSZone.Namespace, actualDNSZone.Namespace, "Unexpected DNSZone namespace")
				assert.Equal(t, test.expectedDNSZone.Name, actualDNSZone.Name, "Unexpected DNSZone name")
				// TODO: Add assertions for (which will require setting up the CD and goodDNSZone more thoroughly):
				// - ControllerReference
				// - Labels
				// - Zone
				// - LinkToParentDomain
				// - AWS CredentialsSecretRef, CredentialsAssumeRole, AdditionalTags, Region
			}

			// TODO: Test setDNSDelayMetric paths

			if actualDNSNotReadyCondition != nil {
				actualDNSNotReadyCondition.LastProbeTime = metav1.Time{}      // zero out so it won't be checked.
				actualDNSNotReadyCondition.LastTransitionTime = metav1.Time{} // zero out so it won't be checked.
				actualDNSNotReadyCondition.Message = ""                       // zero out so it won't be checked.
			}
			assert.Equal(t, test.expectedDNSNotReadyCondition, actualDNSNotReadyCondition, "Expected DNSZone DNSNotReady condition doesn't match returned condition")
		})
	}
}

func filterNils(objs ...runtime.Object) (filtered []runtime.Object) {
	for _, obj := range objs {
		if !reflect.ValueOf(obj).IsNil() {
			filtered = append(filtered, obj)
		}
	}
	return
}

func Test_discoverAWSHostedZoneRole(t *testing.T) {
	logger := log.WithField("controller", "clusterDeployment")
	awsCD := func(installed bool, cm *hivev1.ClusterMetadata) *hivev1.ClusterDeployment {
		cd := testEmptyClusterDeployment()
		cd.ObjectMeta = metav1.ObjectMeta{
			Name:      testName,
			Namespace: testNamespace,
		}
		cd.Spec = hivev1.ClusterDeploymentSpec{
			Platform: hivev1.Platform{
				AWS: &hivev1aws.Platform{
					Region: "us-east-1",
				},
			},
			Provisioning: &hivev1.Provisioning{
				InstallConfigSecretRef: &corev1.LocalObjectReference{
					Name: installConfigSecretName,
				},
			},
		}
		if installed {
			cd.Spec.Installed = true
		}
		if cm != nil {
			cd.Spec.ClusterMetadata = cm
		}
		return cd
	}
	tests := []struct {
		name               string
		cd                 *hivev1.ClusterDeployment
		icSecret           *corev1.Secret
		wantReturn         bool
		wantHostedZoneRole *string
	}{
		{
			name: "no-op: non-AWS platform",
			cd: func() *hivev1.ClusterDeployment {
				cd := awsCD(true, nil)
				cd.Spec.Platform = hivev1.Platform{
					Azure: &azure.Platform{},
				}
				return cd
			}(),
		},
		{
			name: "no-op: cluster not installed",
			cd:   awsCD(false, nil),
		},
		{
			name: "no-op: already set",
			cd: awsCD(true, &hivev1.ClusterMetadata{
				Platform: &hivev1.ClusterPlatformMetadata{
					AWS: &hivev1aws.Metadata{
						HostedZoneRole: pointer.String("my-hzr"),
					},
				},
			},
			),
			wantHostedZoneRole: pointer.String("my-hzr"),
		},
		{
			name: "no-op: already set, empty string acceptable",
			cd: awsCD(true, &hivev1.ClusterMetadata{
				Platform: &hivev1.ClusterPlatformMetadata{
					AWS: &hivev1aws.Metadata{
						HostedZoneRole: pointer.String(""),
					},
				},
			},
			),
			wantHostedZoneRole: pointer.String(""),
		},
		{
			name: "set from install-config: field absent",
			cd:   awsCD(true, nil),
			icSecret: testInstallConfigSecret(`
platform:
  aws:
    region: us-east-1
`),
			wantReturn:         true,
			wantHostedZoneRole: pointer.String(""),
		},
		{
			name: "set from install-config: field empty",
			cd:   awsCD(true, nil),
			icSecret: testInstallConfigSecret(`
platform:
  aws:
    region: us-east-1
    hostedZoneRole: ""
`),
			wantReturn:         true,
			wantHostedZoneRole: pointer.String(""),
		},
		{
			name: "set from install-config: field populated",
			cd:   awsCD(true, nil),
			icSecret: testInstallConfigSecret(`
platform:
  aws:
    region: us-east-1
    hostedZoneRole: some-hzr
`),
			wantReturn:         true,
			wantHostedZoneRole: pointer.String("some-hzr"),
		},
		{
			name: "no cd provisioning",
			cd: func() *hivev1.ClusterDeployment {
				cd := awsCD(true, nil)
				cd.Spec.Provisioning = nil
				return cd
			}(),
		},
		{
			name: "no installConfigSecretRef",
			cd: func() *hivev1.ClusterDeployment {
				cd := awsCD(true, nil)
				cd.Spec.Provisioning = &hivev1.Provisioning{}
				return cd
			}(),
		},
		{
			name: "no cd installConfigSecretRef.name",
			cd: func() *hivev1.ClusterDeployment {
				cd := awsCD(true, nil)
				cd.Spec.Provisioning = &hivev1.Provisioning{
					InstallConfigSecretRef: &corev1.LocalObjectReference{},
				}
				return cd
			}(),
		},
		{
			name: "no install-config secret",
			cd:   awsCD(true, nil),
		},
		{
			name: "invalid install-config",
			cd:   awsCD(true, nil),
			icSecret: testInstallConfigSecret(`
platform:
spacing problem!
`),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(filterNils(test.cd, test.icSecret)...).Build()
			r := &ReconcileClusterDeployment{
				Client: fakeClient,
				scheme: scheme.GetScheme(),
			}

			if gotReturn := r.discoverAWSHostedZoneRole(test.cd, logger); gotReturn != test.wantReturn {
				t.Errorf("ReconcileClusterDeployment.discoverAWSHostedZoneRole() = %v, want %v", gotReturn, test.wantReturn)
			}

			// Note that we're *not* getting this from the server -- the func updates the local obj, but the
			// caller is responsible for Update()ing.
			gotHZR := controllerutils.AWSHostedZoneRole(test.cd)
			if test.wantHostedZoneRole == nil {
				assert.Nil(t, gotHZR, "expected hosted zone role to be nil in cd")
			} else {
				if assert.NotNil(t, gotHZR, "expected hosted zone role to be non-nil on cd") {
					assert.Equal(t, *test.wantHostedZoneRole, *gotHZR, "wrong value for hosted zone role on cd")
				}
			}
		})
	}
}

func Test_discoverAzureResourceGroup(t *testing.T) {
	logger := log.WithField("controller", "clusterDeployment")
	azureCD := func(installed bool, cm *hivev1.ClusterMetadata) *hivev1.ClusterDeployment {
		cd := testEmptyClusterDeployment()
		cd.ObjectMeta = metav1.ObjectMeta{
			Name:      testName,
			Namespace: testNamespace,
		}
		// Cluster must be "reachable" for (good) remote client builder
		cd.Status = hivev1.ClusterDeploymentStatus{
			Conditions: []hivev1.ClusterDeploymentCondition{
				{
					Type:   hivev1.UnreachableCondition,
					Status: corev1.ConditionFalse,
				},
			},
		}
		cd.Spec = hivev1.ClusterDeploymentSpec{
			Platform: hivev1.Platform{
				Azure: &azure.Platform{
					Region: "az-region",
				},
			},
			Provisioning: &hivev1.Provisioning{
				InstallConfigSecretRef: &corev1.LocalObjectReference{
					Name: installConfigSecretName,
				},
			},
		}
		if installed {
			cd.Spec.Installed = true
		}
		if cm != nil {
			cd.Spec.ClusterMetadata = cm
		}
		return cd
	}
	tests := []struct {
		name     string
		cd       *hivev1.ClusterDeployment
		icSecret *corev1.Secret
		// This is a tad silly, but:
		// - "true": Configure the remote client (to contain infraObj, if set)
		// - "false" (or unset): Expect remote client builder not to be called at all
		// - "error": Remote client builder is invoked, but errors
		configureRemoteClient string
		// ignored if configureRemoteClient is false
		infraObj   *configv1.Infrastructure
		wantReturn bool
		// Empty means we expect AzureResourceGroup() to error
		wantResourceGroup string
	}{
		{
			name: "no-op: non-Azure platform",
			// This has AWS platform
			cd: testClusterDeployment(),
		},
		{
			name: "no-op: cluster not installed",
			cd:   azureCD(false, nil),
		},
		{
			name: "no-op: already set",
			cd: azureCD(
				true,
				&hivev1.ClusterMetadata{
					Platform: &hivev1.ClusterPlatformMetadata{
						Azure: &azure.Metadata{
							ResourceGroupName: pointer.String("some-rg"),
						},
					},
				},
			),
			wantResourceGroup: "some-rg",
		},
		{
			name: "green path: infrastructure object",
			// absent ClusterMetadata gets created & populated
			cd:                    azureCD(true, nil),
			configureRemoteClient: "true",
			infraObj: &configv1.Infrastructure{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
				Status: configv1.InfrastructureStatus{
					PlatformStatus: &configv1.PlatformStatus{
						Azure: &configv1.AzurePlatformStatus{
							ResourceGroupName: "some-rg",
						},
					},
				},
			},
			wantReturn:        true,
			wantResourceGroup: "some-rg",
		},
		{
			name: "green path: set in install-config",
			// partially configured ClusterMetadata gets populated
			cd: azureCD(true, &hivev1.ClusterMetadata{}),
			icSecret: testInstallConfigSecret(`
platform:
  azure:
    region: az-region
    resourceGroupName: some-rg
`),
			// Make the Infrastructure attempt fail
			configureRemoteClient: "error",
			wantReturn:            true,
			wantResourceGroup:     "some-rg",
		},
		{
			name: "green path: unset in install-config (use default)",
			cd: func() *hivev1.ClusterDeployment {
				cd := azureCD(true, &hivev1.ClusterMetadata{
					InfraID: testInfraID,
					// empty Platform gets populated
					Platform: &hivev1.ClusterPlatformMetadata{},
				})
				return cd
			}(),
			icSecret: testInstallConfigSecret(`
platform:
  azure:
    region: az-region
`),
			configureRemoteClient: "error",
			wantReturn:            true,
			wantResourceGroup:     testInfraID + "-rg",
		},
		{
			name: "no infra obj, no Provisioning in CD",
			cd: func() *hivev1.ClusterDeployment {
				cd := azureCD(true, nil)
				cd.Spec.Provisioning = nil
				return cd
			}(),
			configureRemoteClient: "true",
		},
		{
			name: "no PlatformStatus in infra; no InstallConfigSecretRef in CD",
			cd: func() *hivev1.ClusterDeployment {
				cd := azureCD(true, nil)
				cd.Spec.Provisioning = &hivev1.Provisioning{}
				return cd
			}(),
			configureRemoteClient: "true",
			infraObj: &configv1.Infrastructure{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
				Status: configv1.InfrastructureStatus{},
			},
		},
		{
			name: "no PlatformStatus.Azure in infra; empty InstallConfigSecretRef in CD",
			cd: func() *hivev1.ClusterDeployment {
				cd := azureCD(true, nil)
				cd.Spec.Provisioning = &hivev1.Provisioning{InstallConfigSecretRef: &corev1.LocalObjectReference{}}
				return cd
			}(),
			configureRemoteClient: "true",
			infraObj: &configv1.Infrastructure{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
				Status: configv1.InfrastructureStatus{
					PlatformStatus: &configv1.PlatformStatus{
						// Not Azure
						AWS: &configv1.AWSPlatformStatus{},
					},
				},
			},
		},
		{
			name:                  "no ResourceGroupName in infra; no install-config secret",
			cd:                    azureCD(true, nil),
			configureRemoteClient: "true",
			infraObj: &configv1.Infrastructure{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
				Status: configv1.InfrastructureStatus{
					PlatformStatus: &configv1.PlatformStatus{
						Azure: &configv1.AzurePlatformStatus{},
					},
				},
			},
		},
		{
			name:                  "invalid install-config: bad format",
			cd:                    azureCD(true, nil),
			configureRemoteClient: "error",
			icSecret:              testInstallConfigSecret("not valid yaml"),
		},
		{
			name:                  "invalid install-config: platform mismatch",
			cd:                    azureCD(true, nil),
			configureRemoteClient: "error",
			// Platform mismatch between CD and install-config
			icSecret: testInstallConfigSecretAWS(),
		},
		{
			name:                  "unset in install-config, but no infra ID in CD",
			cd:                    azureCD(true, nil),
			configureRemoteClient: "error",
			icSecret: testInstallConfigSecret(`
platform:
  azure:
    region: az-region
`),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scheme := scheme.GetScheme()
			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(filterNils(test.cd, test.icSecret)...).Build()
			mockCtrl := gomock.NewController(t)
			mockRemoteClientBuilder := remoteclientmock.NewMockBuilder(mockCtrl)
			switch test.configureRemoteClient {
			case "true":
				mockRemoteClientBuilder.EXPECT().Build().Return(
					testfake.NewFakeClientBuilder().WithRuntimeObjects(filterNils(test.infraObj)...).Build(),
					nil,
				)
			case "error":
				mockRemoteClientBuilder.EXPECT().Build().Return(nil, errors.New("couldn't build remote client"))
			}

			r := &ReconcileClusterDeployment{
				Client:                        fakeClient,
				scheme:                        scheme,
				remoteClusterAPIClientBuilder: func(*hivev1.ClusterDeployment) remoteclient.Builder { return mockRemoteClientBuilder },
			}

			if gotReturn := r.discoverAzureResourceGroup(test.cd, logger); gotReturn != test.wantReturn {
				t.Errorf("ReconcileClusterDeployment.discoverAzureResourceGroup() = %v, want %v", gotReturn, test.wantReturn)
			}

			// Note that we're *not* getting this from the server -- the func updates the local obj, but the
			// caller is responsible for Update()ing.
			if gotResourceGroup, err := controllerutils.AzureResourceGroup(test.cd); test.wantResourceGroup == "" {
				assert.Error(t, err, "expected AzureResourceGroup() to error")
			} else {
				if assert.NoError(t, err, "expected AzureResourceGroup() to succeed") {
					assert.Equal(t, test.wantResourceGroup, gotResourceGroup, "resource group mismatch")
				}
			}

			// If the remote client builder errored, expect the cd to be updated with the unreachable condition set
			if test.configureRemoteClient == "error" {
				if cd := getCDFromClient(fakeClient); assert.NotNil(t, cd, "expected to find the ClusterDeployment on the server") {
					found := false
					for _, cond := range cd.Status.Conditions {
						if cond.Type == hivev1.UnreachableCondition {
							found = true
							assert.Equal(t, corev1.ConditionTrue, cond.Status, "expected unreachable condition to be true")
							break
						}
					}
					assert.True(t, found, "expected to find the unreachable condition")
				}
			}
		})
	}
}

func Test_discoverGCPNetworkProjectID(t *testing.T) {
	logger := log.WithField("controller", "clusterDeployment")
	gcpCD := func(installed bool, cm *hivev1.ClusterMetadata) *hivev1.ClusterDeployment {
		cd := testEmptyClusterDeployment()
		cd.ObjectMeta = metav1.ObjectMeta{
			Name:      testName,
			Namespace: testNamespace,
		}
		cd.Spec = hivev1.ClusterDeploymentSpec{
			Platform: hivev1.Platform{
				GCP: &gcp.Platform{
					Region: "us-central1",
				},
			},
			Provisioning: &hivev1.Provisioning{
				InstallConfigSecretRef: &corev1.LocalObjectReference{
					Name: installConfigSecretName,
				},
			},
		}
		if installed {
			cd.Spec.Installed = true
		}
		if cm != nil {
			cd.Spec.ClusterMetadata = cm
		}
		return cd
	}
	tests := []struct {
		name                 string
		cd                   *hivev1.ClusterDeployment
		icSecret             *corev1.Secret
		wantReturn           bool
		wantNetworkProjectID *string
	}{
		{
			name: "no-op: non-GCP platform",
			cd: func() *hivev1.ClusterDeployment {
				cd := gcpCD(true, nil)
				cd.Spec.Platform = hivev1.Platform{
					Azure: &azure.Platform{},
				}
				return cd
			}(),
		},
		{
			name: "no-op: cluster not installed",
			cd:   gcpCD(false, nil),
		},
		{
			name: "no-op: already set",
			cd: gcpCD(true, &hivev1.ClusterMetadata{
				Platform: &hivev1.ClusterPlatformMetadata{
					GCP: &gcp.Metadata{
						NetworkProjectID: pointer.String("my@np.id"),
					},
				},
			},
			),
			wantNetworkProjectID: pointer.String("my@np.id"),
		},
		{
			name: "no-op: already set, empty string acceptable",
			cd: gcpCD(true, &hivev1.ClusterMetadata{
				Platform: &hivev1.ClusterPlatformMetadata{
					GCP: &gcp.Metadata{
						NetworkProjectID: pointer.String(""),
					},
				},
			},
			),
			wantNetworkProjectID: pointer.String(""),
		},
		{
			name: "set from install-config: field absent",
			cd:   gcpCD(true, nil),
			icSecret: testInstallConfigSecret(`
platform:
  gcp:
    region: us-central1
`),
			wantReturn:           true,
			wantNetworkProjectID: pointer.String(""),
		},
		{
			name: "set from install-config: field empty",
			cd:   gcpCD(true, nil),
			icSecret: testInstallConfigSecret(`
platform:
  gcp:
    region: us-east-1
    networkProjectID: ""
`),
			wantReturn:           true,
			wantNetworkProjectID: pointer.String(""),
		},
		{
			name: "set from install-config: field populated",
			cd:   gcpCD(true, nil),
			icSecret: testInstallConfigSecret(`
platform:
  gcp:
    region: us-east-1
    networkProjectID: some@np.id
`),
			wantReturn:           true,
			wantNetworkProjectID: pointer.String("some@np.id"),
		},
		{
			name: "no cd provisioning",
			cd: func() *hivev1.ClusterDeployment {
				cd := gcpCD(true, nil)
				cd.Spec.Provisioning = nil
				return cd
			}(),
		},
		{
			name: "no installConfigSecretRef",
			cd: func() *hivev1.ClusterDeployment {
				cd := gcpCD(true, nil)
				cd.Spec.Provisioning = &hivev1.Provisioning{}
				return cd
			}(),
		},
		{
			name: "no cd installConfigSecretRef.name",
			cd: func() *hivev1.ClusterDeployment {
				cd := gcpCD(true, nil)
				cd.Spec.Provisioning = &hivev1.Provisioning{
					InstallConfigSecretRef: &corev1.LocalObjectReference{},
				}
				return cd
			}(),
		},
		{
			name: "no install-config secret",
			cd:   gcpCD(true, nil),
		},
		{
			name: "invalid install-config",
			cd:   gcpCD(true, nil),
			icSecret: testInstallConfigSecret(`
platform:
spacing problem!
`),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(filterNils(test.cd, test.icSecret)...).Build()
			r := &ReconcileClusterDeployment{
				Client: fakeClient,
				scheme: scheme.GetScheme(),
			}

			if gotReturn := r.discoverGCPNetworkProjectID(test.cd, logger); gotReturn != test.wantReturn {
				t.Errorf("ReconcileClusterDeployment.discoverGCPNetworkProjectID() = %v, want %v", gotReturn, test.wantReturn)
			}

			// Note that we're *not* getting this from the server -- the func updates the local obj, but the
			// caller is responsible for Update()ing.
			gotNPID := controllerutils.GCPNetworkProjectID(test.cd)
			if test.wantNetworkProjectID == nil {
				assert.Nil(t, gotNPID, "expected network project ID to be nil in cd")
			} else {
				if assert.NotNil(t, gotNPID, "expected network project ID to be non-nil on cd") {
					assert.Equal(t, *test.wantNetworkProjectID, *gotNPID, "wrong value for network project ID on cd")
				}
			}
		})
	}
}
func getProvisions(c client.Client) []*hivev1.ClusterProvision {
	provisionList := &hivev1.ClusterProvisionList{}
	if err := c.List(context.TODO(), provisionList); err != nil {
		return nil
	}
	provisions := make([]*hivev1.ClusterProvision, len(provisionList.Items))
	for i := range provisionList.Items {
		provisions[i] = &provisionList.Items[i]
	}
	sort.Slice(provisions, func(i, j int) bool { return provisions[i].Spec.Attempt < provisions[j].Spec.Attempt })
	return provisions
}

func testCompletedImageSetJob() *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      imageSetJobName,
			Namespace: testNamespace,
		},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{{
				Type:   batchv1.JobComplete,
				Status: corev1.ConditionTrue,
			}},
		},
	}
}

func testCompletedFailedImageSetJob() *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      imageSetJobName,
			Namespace: testNamespace,
		},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{{
				Type:    batchv1.JobFailed,
				Status:  corev1.ConditionTrue,
				Reason:  "ImagePullBackoff",
				Message: "The pod failed to start because one the containers did not start",
			}},
		},
	}
}

func dnsZoneBase() testdnszone.Option {
	return func(dnsZone *hivev1.DNSZone) {
		dnsZone.Name = controllerutils.DNSZoneName(testName)
		dnsZone.Namespace = testNamespace
	}
}

func clusterDeploymentBase() testclusterdeployment.Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Namespace = testNamespace
		clusterDeployment.Name = testName
		clusterDeployment.Spec.Platform.AWS = &hivev1aws.Platform{}
	}
}

// testReleaseVerifier returns Verify true for only provided known digests.
type testReleaseVerifier struct {
	known sets.String
}

func (t testReleaseVerifier) Verify(ctx context.Context, releaseDigest string) error {
	if !t.known.Has(releaseDigest) {
		return fmt.Errorf("verification did not succeed")
	}
	return nil
}

func (testReleaseVerifier) Signatures() map[string][][]byte {
	return nil
}

func (testReleaseVerifier) Verifiers() map[string]openpgp.EntityList {
	return nil
}

func (testReleaseVerifier) AddStore(_ store.Store) {
}
