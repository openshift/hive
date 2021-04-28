package clusterdeployment

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/diff"
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
	"github.com/openshift/hive/apis/hive/v1/baremetal"
	hivecontractsv1alpha1 "github.com/openshift/hive/apis/hivecontracts/v1alpha1"
	hiveintv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/remoteclient"
	remoteclientmock "github.com/openshift/hive/pkg/remoteclient/mock"
	testclusterdeployment "github.com/openshift/hive/pkg/test/clusterdeployment"
	testclusterdeprovision "github.com/openshift/hive/pkg/test/clusterdeprovision"
	testdnszone "github.com/openshift/hive/pkg/test/dnszone"
)

const (
	testName                = "foo-lqmsh"
	testClusterName         = "bar"
	testClusterID           = "testFooClusterUUID"
	testInfraID             = "testFooInfraID"
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
}

func TestClusterDeploymentReconcile(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	openshiftapiv1.Install(scheme.Scheme)
	routev1.Install(scheme.Scheme)

	// Utility function to get the test CD from the fake client
	getCD := func(c client.Client) *hivev1.ClusterDeployment {
		cd := &hivev1.ClusterDeployment{}
		err := c.Get(context.TODO(), client.ObjectKey{Name: testName, Namespace: testNamespace}, cd)
		if err == nil {
			return cd
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

	tests := []struct {
		name                          string
		existing                      []runtime.Object
		pendingCreation               bool
		expectErr                     bool
		expectedRequeueAfter          time.Duration
		expectPendingCreation         bool
		expectConsoleRouteFetch       bool
		validate                      func(client.Client, *testing.T)
		reconcilerSetup               func(*ReconcileClusterDeployment)
		platformCredentialsValidation func(client.Client, *hivev1.ClusterDeployment, log.FieldLogger) (bool, error)
	}{
		{
			name: "Add finalizer",
			existing: []runtime.Object{
				testClusterDeploymentWithoutFinalizer(),
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
				testClusterDeployment(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			expectPendingCreation: true,
			validate: func(c client.Client, t *testing.T) {
				provisions := getProvisions(c)
				assert.Len(t, provisions, 1, "expected provision to exist")
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
			},
		},
		{
			name: "Adopt provision",
			existing: []runtime.Object{
				testClusterDeployment(),
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
				}
			},
		},
		{
			name: "No-op Running provision",
			existing: []runtime.Object{
				testClusterDeploymentWithProvision(),
				testProvision(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if assert.NotNil(t, cd, "no clusterdeployment found") {
					if e, a := testClusterDeploymentWithProvision(), cd; !assert.True(t, apiequality.Semantic.DeepEqual(e, a), "unexpected change in clusterdeployment") {
						t.Logf("diff = %s", diff.ObjectReflectDiff(e, a))
					}
				}
				provisions := getProvisions(c)
				if assert.Len(t, provisions, 1, "expected provision to exist") {
					if e, a := testProvision(), provisions[0]; !assert.True(t, apiequality.Semantic.DeepEqual(e, a), "unexpected change in provision") {
						t.Logf("diff = %s", diff.ObjectReflectDiff(e, a))
					}
				}
			},
		},
		{
			name: "Parse server URL from admin kubeconfig",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
					cd.Spec.Installed = true
					cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{
						InfraID:                  "fakeinfra",
						AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: adminKubeconfigSecret},
					}
					cd.Status.Conditions = append(cd.Status.Conditions,
						hivev1.ClusterDeploymentCondition{
							Type:   hivev1.UnreachableCondition,
							Status: corev1.ConditionFalse,
						},
					)
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
					cd := testClusterDeployment()
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
				testClusterDeploymentWithProvision(),
				testSuccessfulProvision(),
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
				}
			},
		},
		{
			name: "Completed provision with protected delete",
			existing: []runtime.Object{
				testClusterDeploymentWithProvision(),
				testSuccessfulProvision(),
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
				}
			},
		},
		{
			name: "clusterdeployment must specify pull secret when there is no global pull secret ",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
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
			name: "No-op deleted cluster without finalizer",
			existing: []runtime.Object{
				testDeletedClusterDeploymentWithoutFinalizer(),
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
			},
		},
		{
			name: "Skip deprovision for deleted BareMetal cluster",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
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
				assert.Equal(t, 0, len(cd.Finalizers))
			},
		},
		{
			name: "Delete expired cluster deployment",
			existing: []runtime.Object{
				testExpiredClusterDeployment(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if cd != nil {
					t.Errorf("got unexpected cluster deployment (expected deleted)")
				}
			},
		},
		{
			name: "Test PreserveOnDelete",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testDeletedClusterDeployment()
					cd.Spec.Installed = true
					cd.Spec.PreserveOnDelete = true
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if assert.NotNil(t, cd, "missing clusterdeployment") {
					assert.Empty(t, cd.Finalizers, "expected empty finalizers")
				}
				deprovision := getDeprovision(c)
				assert.Nil(t, deprovision, "expected no deprovision request")
			},
		},
		{
			name: "Test creation of uninstall job when PreserveOnDelete is true but cluster deployment is not installed",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testDeletedClusterDeployment()
					cd.Spec.PreserveOnDelete = true
					cd.Spec.Installed = false
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				deprovision := getDeprovision(c)
				require.NotNil(t, deprovision, "expected deprovision request")
				assert.Equal(t, testClusterDeployment().Name, deprovision.Labels[constants.ClusterDeploymentNameLabel], "incorrect cluster deployment name label")
			},
		},
		{
			name: "Create job to resolve installer image",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
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
				assert.Equal(t, testClusterDeployment().Name, job.Labels[constants.ClusterDeploymentNameLabel], "incorrect cluster deployment name label")
				assert.Equal(t, constants.JobTypeImageSet, job.Labels[constants.JobTypeLabel], "incorrect job type label")
			},
		},
		{
			name: "failed image should set InstallImagesNotResolved condition on clusterdeployment",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
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
				assertConditionStatus(t, cd, hivev1.InstallImagesNotResolvedCondition, corev1.ConditionTrue)
				assertConditionReason(t, cd, hivev1.InstallImagesNotResolvedCondition, "JobToResolveImagesFailed")

			},
		},
		{
			name: "clear InstallImagesNotResolved condition on success",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
					cd.Status.InstallerImage = pointer.StringPtr("test-installer-image")
					cd.Status.CLIImage = pointer.StringPtr("test-cli-image")
					cd.Spec.Provisioning.ImageSetRef = &hivev1.ClusterImageSetReference{Name: testClusterImageSetName}
					cd.Status.Conditions = []hivev1.ClusterDeploymentCondition{{
						Type:    hivev1.InstallImagesNotResolvedCondition,
						Status:  corev1.ConditionTrue,
						Reason:  "test-reason",
						Message: "test-message",
					}}
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
				assertConditionStatus(t, cd, hivev1.InstallImagesNotResolvedCondition, corev1.ConditionFalse)
			},
		},
		{
			name: "Delete imageset job when complete",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
					cd.Status.InstallerImage = pointer.StringPtr("test-installer-image")
					cd.Status.CLIImage = pointer.StringPtr("test-cli-image")
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
					cd := testClusterDeployment()
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
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
					cd.Status.InstallerImage = pointer.StringPtr("test-installer-image:latest")
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
					cd := testClusterDeployment()
					cd.Spec.ManageDNS = true
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
			},
		},
		{
			name: "Wait when DNSZone is not available yet",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
					cd.Spec.ManageDNS = true
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testDNSZone(),
			},
			expectedRequeueAfter: defaultDNSNotReadyTimeout + defaultRequeueTime,
			validate: func(c client.Client, t *testing.T) {
				provisions := getProvisions(c)
				assert.Empty(t, provisions, "provision should not exist")
			},
		},
		{
			name: "Set condition when DNSZone is not available yet",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
					cd.Spec.ManageDNS = true
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testDNSZone(),
			},
			expectedRequeueAfter: defaultDNSNotReadyTimeout + defaultRequeueTime,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				assertConditionStatus(t, cd, hivev1.DNSNotReadyCondition, corev1.ConditionTrue)
			},
		},
		{
			name: "Set condition when DNSZone cannot be created due to credentials missing permissions",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
					cd.Spec.ManageDNS = true
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testDNSZoneWithInvalidCredentialsCondition(),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				assertConditionStatus(t, cd, hivev1.DNSNotReadyCondition, corev1.ConditionTrue)
				assertConditionReason(t, cd, hivev1.DNSNotReadyCondition, "InsufficientCredentials")
			},
		},
		{
			name: "Set condition when DNSZone cannot be created due to authentication failure",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
					cd.Spec.ManageDNS = true
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testDNSZoneWithAuthenticationFailureCondition(),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				assertConditionStatus(t, cd, hivev1.DNSNotReadyCondition, corev1.ConditionTrue)
				assertConditionReason(t, cd, hivev1.DNSNotReadyCondition, "AuthenticationFailure")
			},
		},
		{
			name: "Clear condition when DNSZone is available",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
					cd.Spec.ManageDNS = true
					cd.Status.Conditions = append(cd.Status.Conditions, hivev1.ClusterDeploymentCondition{
						Type:   hivev1.DNSNotReadyCondition,
						Status: corev1.ConditionTrue,
					})
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testAvailableDNSZone(),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				assertConditionStatus(t, cd, hivev1.DNSNotReadyCondition, corev1.ConditionFalse)
			},
		},
		{
			name: "Do not use unowned DNSZone",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
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
					cond := controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.DNSNotReadyCondition)
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
					cd := testClusterDeployment()
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
					cond := controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.DNSNotReadyCondition)
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
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
					cd.Spec.ManageDNS = true
					cd.Annotations = map[string]string{dnsReadyAnnotation: "NOW"}
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
			},
		},
		{
			name: "Set DNS delay metric",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
					cd.Spec.ManageDNS = true
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testAvailableDNSZone(),
			},
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
					cd := testDeletedClusterDeployment()
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
					cd := testDeletedClusterDeployment()
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
				func() runtime.Object {
					cd := testClusterDeployment()
					cd.Status.InstallRestarts = 4
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testFailedProvisionAttempt(0),
				testFailedProvisionAttempt(1),
				testFailedProvisionAttempt(2),
				testFailedProvisionAttempt(3),
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
			name: "Adopt provision",
			existing: []runtime.Object{
				testClusterDeployment(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testProvision(),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if assert.NotNil(t, cd, "missing cluster deployment") {
					if assert.NotNil(t, cd.Status.ProvisionRef, "provision reference not set") {
						assert.Equal(t, provisionName, cd.Status.ProvisionRef.Name, "unexpected provision referenced")
					}
				}
			},
		},
		{
			name: "Do not adopt failed provision",
			existing: []runtime.Object{
				testClusterDeployment(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testFailedProvisionAttempt(0),
			},
			expectPendingCreation: true,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if assert.NotNil(t, cd, "missing cluster deployment") {
					assert.Nil(t, cd.Status.ProvisionRef, "expected provision reference to not be set")
				}
			},
		},
		{
			name: "Delete-after requeue",
			existing: []runtime.Object{
				func() runtime.Object {
					cd := testClusterDeploymentWithProvision()
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
			expectedRequeueAfter: 8 * time.Hour,
		},
		{
			name: "Wait after failed provision",
			existing: []runtime.Object{
				func() runtime.Object {
					cd := testClusterDeploymentWithProvision()
					cd.CreationTimestamp = metav1.Now()
					if cd.Annotations == nil {
						cd.Annotations = make(map[string]string, 1)
					}
					cd.Annotations[deleteAfterAnnotation] = "8h"
					return cd
				}(),
				testFailedProvisionTime(time.Now()),
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
				}
			},
		},
		{
			name: "Clear out provision after wait time",
			existing: []runtime.Object{
				testClusterDeploymentWithProvision(),
				testFailedProvisionTime(time.Now().Add(-2 * time.Minute)),
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
					cd := testClusterDeploymentWithProvision()
					now := metav1.Now()
					cd.DeletionTimestamp = &now
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
			},
		},
		{
			name: "Remove finalizer after early-failure provision removed",
			existing: []runtime.Object{
				func() runtime.Object {
					cd := testClusterDeploymentWithProvision()
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
				if assert.NotNil(t, cd, "missing clusterdeployment") {
					assert.Empty(t, cd.Finalizers, "expected empty finalizers")
				}
			},
		},
		{
			name: "Create deprovision after late-failure provision removed",
			existing: []runtime.Object{
				func() runtime.Object {
					cd := testClusterDeploymentWithProvision()
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
				assert.NotNil(t, deprovision, "missing deprovision request")
			},
		},
		{
			name: "SyncSetFailedCondition should be present",
			existing: []runtime.Object{
				testInstalledClusterDeployment(time.Now()),
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
					cond := controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.SyncSetFailedCondition)
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
					cd := testInstalledClusterDeployment(time.Now())
					cd.Status.Conditions = append(
						cd.Status.Conditions,
						hivev1.ClusterDeploymentCondition{
							Type:   hivev1.SyncSetFailedCondition,
							Status: corev1.ConditionTrue,
						},
					)
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
				cond := controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.SyncSetFailedCondition)
				if assert.NotNil(t, cond, "missing SyncSetFailedCondition status condition") {
					assert.Equal(t, corev1.ConditionFalse, cond.Status, "did not get expected state for SyncSetFailedCondition condition")
				}
			},
		},
		{
			name: "SyncSet is Paused and ClusterSync object is missing",
			existing: []runtime.Object{
				func() runtime.Object {
					cd := testInstalledClusterDeployment(time.Now())
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
				cond := controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.SyncSetFailedCondition)
				if assert.NotNil(t, cond, "missing SyncSetFailedCondition status condition") {
					assert.Equal(t, corev1.ConditionTrue, cond.Status, "did not get expected state for SyncSetFailedCondition condition")
					assert.Equal(t, "SyncSetPaused", cond.Reason, "did not get expected reason for SyncSetFailedCondition condition")
				}
			},
		},
		{
			name: "Add cluster platform label",
			existing: []runtime.Object{
				testClusterDeploymentWithoutPlatformLabel(),
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
				testClusterDeploymentWithoutRegionLabel(),
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
				func() runtime.Object {
					cd := testClusterDeploymentWithProvision()
					cd.Spec.ClusterMetadata = nil
					return cd
				}(),
				testSuccessfulProvision(),
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
				func() runtime.Object {
					cd := testClusterDeploymentWithProvision()
					cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{
						InfraID:                  "old-infra-id",
						ClusterID:                "old-cluster-id",
						AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: "old-kubeconfig-secret"},
						AdminPasswordSecretRef:   corev1.LocalObjectReference{Name: "old-password-secret"},
					}
					return cd
				}(),
				testSuccessfulProvision(),
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
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
					cd.Spec.Provisioning.ImageSetRef = &hivev1.ClusterImageSetReference{Name: "doesntexist"}
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			expectErr: true,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.Equal(t, 1, len(cd.Status.Conditions))
				require.Equal(t, corev1.ConditionTrue, cd.Status.Conditions[0].Status)
				require.Equal(t, clusterImageSetNotFoundReason, cd.Status.Conditions[0].Reason)
			},
		},
		{
			name: "clear ClusterImageSet missing condition",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
					cd.Spec.Provisioning.ImageSetRef = &hivev1.ClusterImageSetReference{Name: testClusterImageSetName}
					cd.Status.Conditions = []hivev1.ClusterDeploymentCondition{{
						Type:    hivev1.ClusterImageSetNotFoundCondition,
						Status:  corev1.ConditionTrue,
						Reason:  "test-reason",
						Message: "test-message",
					}}
					return cd
				}(),
				testClusterImageSet(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			expectPendingCreation: true,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.Equal(t, 1, len(cd.Status.Conditions))
				require.Equal(t, corev1.ConditionFalse, cd.Status.Conditions[0].Status)
				require.Equal(t, clusterImageSetFoundReason, cd.Status.Conditions[0].Reason)
			},
		},
		{
			name: "Add ownership to admin secrets",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
					cd.Spec.Installed = true
					cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{
						InfraID:                  "fakeinfra",
						AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: adminKubeconfigSecret},
						AdminPasswordSecretRef:   corev1.LocalObjectReference{Name: adminPasswordSecret},
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
					cd := testClusterDeployment()
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
				require.NotNil(t, cd, "could not get ClusterDeployment")
				assert.NotContains(t, cd.Finalizers, hivev1.FinalizerDeprovision, "expected finalizer to be removed from ClusterDeployment")
			},
		},
		{
			name: "wait for deprovision to complete",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
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
				assert.Contains(t, cd.Finalizers, hivev1.FinalizerDeprovision, "expected finalizer to be removed from ClusterDeployment")
			},
		},
		{
			name: "wait for dnszone to be gone",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
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
					return dnsZone
				}(),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				assert.Contains(t, cd.Finalizers, hivev1.FinalizerDeprovision, "expected finalizer to be removed from ClusterDeployment")
			},
			expectedRequeueAfter: defaultRequeueTime,
		},
		{
			name: "do not wait for dnszone to be gone when not using managed dns",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
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
					return dnsZone
				}(),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				assert.NotContains(t, cd.Finalizers, hivev1.FinalizerDeprovision, "expected finalizer to be removed from ClusterDeployment")
			},
		},
		{
			name: "wait for dnszone to be gone when install failed early",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
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
					return dnsZone
				}(),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				assert.Contains(t, cd.Finalizers, hivev1.FinalizerDeprovision, "expected finalizer to be removed from ClusterDeployment")
			},
			expectedRequeueAfter: defaultRequeueTime,
		},
		{
			name: "set InstallLaunchErrorCondition when install pod is stuck in pending phase",
			existing: []runtime.Object{
				testClusterDeploymentWithProvision(),
				testProvisionWithStuckInstallPod(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				assertConditionStatus(t, cd, hivev1.InstallLaunchErrorCondition, corev1.ConditionTrue)
				assertConditionReason(t, cd, hivev1.InstallLaunchErrorCondition, "PodInPendingPhase")
			},
		},
		{
			name: "install attempts is less than the limit",
			existing: []runtime.Object{
				func() runtime.Object {
					cd := testClusterDeployment()
					cd.Status.InstallRestarts = 1
					cd.Spec.InstallAttemptsLimit = pointer.Int32Ptr(2)
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			expectPendingCreation: true,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				assert.NotContains(t, cd.Status.Conditions, hivev1.ProvisionStoppedCondition, "ClusterDeployment should not have ProvisionStopped condition")
			},
		},
		{
			name: "install attempts is equal to the limit",
			existing: []runtime.Object{
				func() runtime.Object {
					cd := testClusterDeployment()
					cd.Status.InstallRestarts = 2
					cd.Spec.InstallAttemptsLimit = pointer.Int32Ptr(2)
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				assertConditionStatus(t, cd, hivev1.ProvisionStoppedCondition, corev1.ConditionTrue)
				assertConditionReason(t, cd, hivev1.ProvisionStoppedCondition, "InstallAttemptsLimitReached")
			},
		},
		{
			name: "install attempts is greater than the limit",
			existing: []runtime.Object{
				func() runtime.Object {
					cd := testClusterDeployment()
					cd.Status.InstallRestarts = 3
					cd.Spec.InstallAttemptsLimit = pointer.Int32Ptr(2)
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				assertConditionStatus(t, cd, hivev1.ProvisionStoppedCondition, corev1.ConditionTrue)
				assertConditionReason(t, cd, hivev1.ProvisionStoppedCondition, "InstallAttemptsLimitReached")
			},
		},
		{
			name: "auth condition when platform creds are bad",
			existing: []runtime.Object{
				testClusterDeployment(),
			},
			platformCredentialsValidation: func(client.Client, *hivev1.ClusterDeployment, log.FieldLogger) (bool, error) {
				return false, nil
			},
			expectErr: true,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")

				assertConditionStatus(t, cd, hivev1.AuthenticationFailureClusterDeploymentCondition, corev1.ConditionTrue)
			},
		},
		{
			name: "no ClusterProvision when platform creds are bad",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
					cd.Status.Conditions = []hivev1.ClusterDeploymentCondition{
						{
							Status:  corev1.ConditionTrue,
							Type:    hivev1.AuthenticationFailureClusterDeploymentCondition,
							Reason:  platformAuthFailureReason,
							Message: "Platform credentials failed authentication check",
						},
					}
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

				provisionList := &hivev1.ClusterProvisionList{}
				err := c.List(context.TODO(), provisionList, client.InNamespace(cd.Namespace))
				require.NoError(t, err, "unexpected error listing ClusterProvisions")

				assert.Zero(t, len(provisionList.Items), "expected no ClusterProvision objects when platform creds are bad")
			},
		},
		{
			name: "clusterinstallref not found",
			existing: []runtime.Object{
				testClusterInstallRefClusterDeployment("test-fake"),
			},
			expectErr: true,
		},
		{
			name: "clusterinstallref exists, but no imagesetref",
			existing: []runtime.Object{
				testClusterInstallRefClusterDeployment("test-fake"),
				testFakeClusterInstall("test-fake"),
			},
			expectErr: true,
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")

				assertConditionStatus(t, cd, hivev1.ClusterImageSetNotFoundCondition, corev1.ConditionTrue)
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
			},
		},
		{
			name: "clusterinstallref exists, requirements met set to false",
			existing: []runtime.Object{
				testClusterInstallRefClusterDeployment("test-fake"),
				testFakeClusterInstallWithConditions("test-fake", []hivecontractsv1alpha1.ClusterInstallCondition{{
					Type:   hivecontractsv1alpha1.ClusterInstallRequirementsMet,
					Status: corev1.ConditionFalse,
				}}),
				testClusterImageSet(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
			},
		},
		{
			name: "clusterinstallref exists, requirements met set to true",
			existing: []runtime.Object{
				testClusterInstallRefClusterDeployment("test-fake"),
				testFakeClusterInstallWithConditions("test-fake", []hivecontractsv1alpha1.ClusterInstallCondition{{
					Type:   hivecontractsv1alpha1.ClusterInstallRequirementsMet,
					Status: corev1.ConditionTrue,
				}}),
				testClusterImageSet(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				assertConditionStatus(t, cd, hivev1.ClusterInstallRequirementsMetClusterDeploymentCondition, corev1.ConditionTrue)
			},
		},
		{
			name: "clusterinstallref exists, failed true",
			existing: []runtime.Object{
				testClusterInstallRefClusterDeployment("test-fake"),
				testFakeClusterInstallWithConditions("test-fake", []hivecontractsv1alpha1.ClusterInstallCondition{{
					Type:   hivecontractsv1alpha1.ClusterInstallFailed,
					Status: corev1.ConditionTrue,
				}}),
				testClusterImageSet(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				assertConditionStatus(t, cd, hivev1.ClusterInstallFailedClusterDeploymentCondition, corev1.ConditionTrue)
				assertConditionStatus(t, cd, hivev1.ProvisionFailedCondition, corev1.ConditionTrue)
			},
		},
		{
			name: "clusterinstallref exists, failed false, previously true",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterInstallRefClusterDeployment("test-fake")
					cd.Status.Conditions = append(cd.Status.Conditions, []hivev1.ClusterDeploymentCondition{{
						Type:   hivev1.ProvisionFailedCondition,
						Status: corev1.ConditionTrue,
					}, {
						Type:   hivev1.ClusterInstallFailedClusterDeploymentCondition,
						Status: corev1.ConditionTrue,
					}}...)
					return cd
				}(),
				testFakeClusterInstallWithConditions("test-fake", []hivecontractsv1alpha1.ClusterInstallCondition{{
					Type:   hivecontractsv1alpha1.ClusterInstallFailed,
					Status: corev1.ConditionFalse,
				}}),
				testClusterImageSet(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				assertConditionStatus(t, cd, hivev1.ClusterInstallFailedClusterDeploymentCondition, corev1.ConditionFalse)
				assertConditionStatus(t, cd, hivev1.ProvisionFailedCondition, corev1.ConditionFalse)
			},
		},
		{
			name: "clusterinstallref exists, stopped, completed not set",
			existing: []runtime.Object{
				testClusterInstallRefClusterDeployment("test-fake"),
				testFakeClusterInstallWithConditions("test-fake", []hivecontractsv1alpha1.ClusterInstallCondition{{
					Type:   hivecontractsv1alpha1.ClusterInstallStopped,
					Status: corev1.ConditionTrue,
				}}),
				testClusterImageSet(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				assertConditionStatus(t, cd, hivev1.ClusterInstallStoppedClusterDeploymentCondition, corev1.ConditionTrue)
				assertConditionStatus(t, cd, hivev1.ProvisionStoppedCondition, corev1.ConditionTrue)
				assertConditionReason(t, cd, hivev1.ProvisionStoppedCondition, "InstallAttemptsLimitReached")
			},
		},
		{
			name: "clusterinstallref exists, stopped, completed false",
			existing: []runtime.Object{
				testClusterInstallRefClusterDeployment("test-fake"),
				testFakeClusterInstallWithConditions("test-fake", []hivecontractsv1alpha1.ClusterInstallCondition{{
					Type:   hivecontractsv1alpha1.ClusterInstallStopped,
					Status: corev1.ConditionTrue,
				}, {
					Type:   hivecontractsv1alpha1.ClusterInstallCompleted,
					Status: corev1.ConditionFalse,
				}}),
				testClusterImageSet(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				assertConditionStatus(t, cd, hivev1.ClusterInstallStoppedClusterDeploymentCondition, corev1.ConditionTrue)
				assertConditionStatus(t, cd, hivev1.ProvisionStoppedCondition, corev1.ConditionTrue)
				assertConditionReason(t, cd, hivev1.ProvisionStoppedCondition, "InstallAttemptsLimitReached")
			},
		},
		{
			name: "clusterinstallref exists, previously stopped, now progressing",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterInstallRefClusterDeployment("test-fake")
					cd.Status.Conditions = append(cd.Status.Conditions, []hivev1.ClusterDeploymentCondition{{
						Type:   hivev1.ProvisionStoppedCondition,
						Status: corev1.ConditionTrue,
						Reason: "InstallAttemptsLimitReached",
					}, {
						Type:   hivev1.ClusterInstallStoppedClusterDeploymentCondition,
						Status: corev1.ConditionTrue,
					}}...)
					return cd
				}(),
				testFakeClusterInstallWithConditions("test-fake", []hivecontractsv1alpha1.ClusterInstallCondition{{
					Type:   hivecontractsv1alpha1.ClusterInstallStopped,
					Status: corev1.ConditionFalse,
				}, {
					Type:   hivecontractsv1alpha1.ClusterInstallCompleted,
					Status: corev1.ConditionFalse,
				}}),
				testClusterImageSet(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				assertConditionStatus(t, cd, hivev1.ClusterInstallStoppedClusterDeploymentCondition, corev1.ConditionFalse)
				assertConditionStatus(t, cd, hivev1.ProvisionStoppedCondition, corev1.ConditionFalse)
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
			name: "clusterinstallref exists, cluster metadata available",
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
					AdminPasswordSecretRef: corev1.LocalObjectReference{
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
			name: "clusterinstallref exists, completed",
			existing: []runtime.Object{
				testClusterInstallRefClusterDeployment("test-fake"),
				testFakeClusterInstallWithConditions("test-fake", []hivecontractsv1alpha1.ClusterInstallCondition{{
					Type:   hivecontractsv1alpha1.ClusterInstallCompleted,
					Status: corev1.ConditionTrue,
				}}),
				testClusterImageSet(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				require.NotNil(t, cd, "could not get ClusterDeployment")
				assertConditionStatus(t, cd, hivev1.ClusterInstallCompletedClusterDeploymentCondition, corev1.ConditionTrue)
				assert.Equal(t, true, cd.Spec.Installed)
				assert.NotNil(t, cd.Status.InstalledTimestamp)
			},
		},
		{
			name: "clusterinstallref exists, stopped and completed",
			existing: []runtime.Object{
				testClusterInstallRefClusterDeployment("test-fake"),
				testFakeClusterInstallWithConditions("test-fake", []hivecontractsv1alpha1.ClusterInstallCondition{{
					Type:   hivecontractsv1alpha1.ClusterInstallCompleted,
					Status: corev1.ConditionTrue,
				}, {
					Type:   hivecontractsv1alpha1.ClusterInstallStopped,
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
				assertConditionStatus(t, cd, hivev1.ClusterInstallCompletedClusterDeploymentCondition, corev1.ConditionTrue)
				assertConditionStatus(t, cd, hivev1.ClusterInstallStoppedClusterDeploymentCondition, corev1.ConditionTrue)
				assertConditionStatus(t, cd, hivev1.ProvisionStoppedCondition, corev1.ConditionTrue)
				assertConditionReason(t, cd, hivev1.ProvisionStoppedCondition, "InstallComplete")
				assert.Equal(t, true, cd.Spec.Installed)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logger := log.WithField("controller", "clusterDeployment")
			fakeClient := fake.NewFakeClient(test.existing...)
			controllerExpectations := controllerutils.NewExpectations(logger)
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockRemoteClientBuilder := remoteclientmock.NewMockBuilder(mockCtrl)

			if test.platformCredentialsValidation == nil {
				test.platformCredentialsValidation = func(client.Client, *hivev1.ClusterDeployment, log.FieldLogger) (bool, error) {
					return true, nil
				}
			}
			rcd := &ReconcileClusterDeployment{
				Client:                                  fakeClient,
				scheme:                                  scheme.Scheme,
				logger:                                  logger,
				expectations:                            controllerExpectations,
				remoteClusterAPIClientBuilder:           func(*hivev1.ClusterDeployment) remoteclient.Builder { return mockRemoteClientBuilder },
				validateCredentialsForClusterDeployment: test.platformCredentialsValidation,
				watchingClusterInstall: map[string]struct{}{
					(schema.GroupVersionKind{Group: "hive.openshift.io", Version: "v1", Kind: "FakeClusterInstall"}).String(): {},
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

			actualPendingCreation := !controllerExpectations.SatisfiedExpectations(reconcileRequest.String())
			assert.Equal(t, test.expectPendingCreation, actualPendingCreation, "unexpected pending creation")
		})
	}
}

func TestClusterDeploymentReconcileResults(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

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
			fakeClient := fake.NewFakeClient(test.existing...)
			controllerExpectations := controllerutils.NewExpectations(logger)
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockRemoteClientBuilder := remoteclientmock.NewMockBuilder(mockCtrl)
			rcd := &ReconcileClusterDeployment{
				Client:                        fakeClient,
				scheme:                        scheme.Scheme,
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
	apis.AddToScheme(scheme.Scheme)
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
				provisions[i] = testFailedProvisionAttempt(a)
			}
			fakeClient := fake.NewFakeClient(provisions...)
			rcd := &ReconcileClusterDeployment{
				Client: fakeClient,
				scheme: scheme.Scheme,
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
	apis.AddToScheme(scheme.Scheme)
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
					provisions[i] = testOldFailedProvision(time.Now().Add(-7*24*time.Hour), i)
				} else {
					provisions[i] = testOldFailedProvision(time.Now(), i)
				}
			}
			fakeClient := fake.NewFakeClient(provisions...)
			rcd := &ReconcileClusterDeployment{
				Client: fakeClient,
				scheme: scheme.Scheme,
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

func testDeletedClusterDeploymentWithoutFinalizer() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	now := metav1.Now()
	cd.DeletionTimestamp = &now
	cd.Finalizers = []string{}
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

func testFakeClusterInstallWithConditions(name string, conditions []hivecontractsv1alpha1.ClusterInstallCondition) *unstructured.Unstructured {
	fake := testFakeClusterInstall(name)

	value := []interface{}{}
	for _, c := range conditions {
		value = append(value, map[string]interface{}{
			"type":    c.Type,
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
		"adminPasswordSecretRef": map[string]interface{}{
			"name": metadata.AdminPasswordSecretRef.Name,
		},
	}

	unstructured.SetNestedField(fake.UnstructuredContent(), value, "spec", "clusterMetadata")
	return fake
}

func testProvision() *hivev1.ClusterProvision {
	cd := testClusterDeployment()
	provision := &hivev1.ClusterProvision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      provisionName,
			Namespace: testNamespace,
			Labels: map[string]string{
				constants.ClusterDeploymentNameLabel: testName,
			},
		},
		Spec: hivev1.ClusterProvisionSpec{
			ClusterDeploymentRef: corev1.LocalObjectReference{
				Name: testName,
			},
			Stage: hivev1.ClusterProvisionStageInitializing,
		},
	}

	controllerutil.SetControllerReference(cd, provision, scheme.Scheme)

	return provision
}

func testSuccessfulProvision() *hivev1.ClusterProvision {
	provision := testProvision()
	provision.Spec.Stage = hivev1.ClusterProvisionStageComplete
	provision.Spec.ClusterID = pointer.StringPtr(testClusterID)
	provision.Spec.InfraID = pointer.StringPtr(testInfraID)
	provision.Spec.AdminKubeconfigSecretRef = &corev1.LocalObjectReference{Name: adminKubeconfigSecret}
	provision.Spec.AdminPasswordSecretRef = &corev1.LocalObjectReference{Name: adminPasswordSecret}
	return provision
}

func testFailedProvisionAttempt(attempt int) *hivev1.ClusterProvision {
	provision := testProvision()
	provision.Name = fmt.Sprintf("%s-%02d", provision.Name, attempt)
	provision.Spec.Attempt = attempt
	provision.Spec.Stage = hivev1.ClusterProvisionStageFailed
	return provision
}

func testFailedProvisionTime(time time.Time) *hivev1.ClusterProvision {
	provision := testProvision()
	provision.Spec.Stage = hivev1.ClusterProvisionStageFailed
	provision.Status.Conditions = []hivev1.ClusterProvisionCondition{
		{
			Type:               hivev1.ClusterProvisionFailedCondition,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: metav1.NewTime(time),
		},
	}
	return provision
}

func testProvisionWithStuckInstallPod() *hivev1.ClusterProvision {
	provision := testProvision()
	provision.Status.Conditions = []hivev1.ClusterProvisionCondition{
		{
			Type:    hivev1.InstallPodStuckCondition,
			Status:  corev1.ConditionTrue,
			Reason:  "PodInPendingPhase",
			Message: "pod is in pending phase",
		},
	}
	return provision
}

func testOldFailedProvision(time time.Time, attempt int) *hivev1.ClusterProvision {
	provision := testProvision()
	provision.Name = fmt.Sprintf("%s-%02d", provision.Name, attempt)
	provision.CreationTimestamp.Time = time
	provision.Spec.Stage = hivev1.ClusterProvisionStageFailed
	return provision
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

	return fake.NewFakeClient(remoteClusterRouteObject)
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

func testAvailableDNSZone() *hivev1.DNSZone {
	zone := testDNSZone()
	zone.Status.Conditions = []hivev1.DNSZoneCondition{
		{
			Type:   hivev1.ZoneAvailableDNSZoneCondition,
			Status: corev1.ConditionTrue,
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

func assertConditionStatus(t *testing.T, cd *hivev1.ClusterDeployment, condType hivev1.ClusterDeploymentConditionType, status corev1.ConditionStatus) {
	found := false
	for _, cond := range cd.Status.Conditions {
		if cond.Type == condType {
			found = true
			assert.Equal(t, string(status), string(cond.Status), "condition found with unexpected status")
		}
	}
	assert.True(t, found, "did not find expected condition type: %v", condType)
}

func assertConditionReason(t *testing.T, cd *hivev1.ClusterDeployment, condType hivev1.ClusterDeploymentConditionType, reason string) {
	for _, cond := range cd.Status.Conditions {
		if cond.Type == condType {
			assert.Equal(t, reason, cond.Reason, "condition found with unexpected reason")
			return
		}
	}
	t.Errorf("did not find expected condition type: %v", condType)
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
	apis.AddToScheme(scheme.Scheme)
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
					cd := testClusterDeployment()
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
					cd := testClusterDeployment()
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
			fakeClient := fake.NewFakeClient(test.existingCD...)
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockRemoteClientBuilder := remoteclientmock.NewMockBuilder(mockCtrl)
			rcd := &ReconcileClusterDeployment{
				Client:                        fakeClient,
				scheme:                        scheme.Scheme,
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
		InstallerImage: pointer.StringPtr("installer-image:latest"),
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
	apis.AddToScheme(scheme.Scheme)

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
			fakeClient := fake.NewFakeClient(test.existingObjs...)
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockRemoteClientBuilder := remoteclientmock.NewMockBuilder(mockCtrl)
			rcd := &ReconcileClusterDeployment{
				Client:                        fakeClient,
				scheme:                        scheme.Scheme,
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
	apis.AddToScheme(scheme.Scheme)

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
			fakeClient := fake.NewFakeClient(test.existingObjs...)
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockRemoteClientBuilder := remoteclientmock.NewMockBuilder(mockCtrl)
			rcd := &ReconcileClusterDeployment{
				Client:                        fakeClient,
				scheme:                        scheme.Scheme,
				logger:                        log.WithField("controller", "clusterDeployment"),
				remoteClusterAPIClientBuilder: func(*hivev1.ClusterDeployment) remoteclient.Builder { return mockRemoteClientBuilder },
			}

			for _, envVar := range test.existingEnvVars {
				if err := os.Setenv(envVar.Name, envVar.Value); err == nil {
					defer func() {
						if err := os.Unsetenv(envVar.Name); err != nil {
							t.Error(err)
						}
					}()
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
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name                         string
		existingObjs                 []runtime.Object
		existingEnvVars              []corev1.EnvVar
		clusterDeployment            *hivev1.ClusterDeployment
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
		},
		{
			name: "zone already exists and is owned by clusterdeployment",
			existingObjs: []runtime.Object{
				testdnszone.Build(
					dnsZoneBase(),
					testdnszone.WithControllerOwnerReference(testclusterdeployment.Build(
						clusterDeploymentBase(),
					)),
				),
			},
			clusterDeployment: testclusterdeployment.Build(
				clusterDeploymentBase(),
			),
			expectedErr: true,
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
			expectedErr: true,
			expectedDNSNotReadyCondition: &hivev1.ClusterDeploymentCondition{
				Type:   hivev1.DNSNotReadyCondition,
				Status: corev1.ConditionTrue,
				Reason: dnsZoneResourceConflictReason,
			},
		},
		{
			name: "zone already exists and is owned by clusterdeployment, but has timed out",
			existingObjs: []runtime.Object{
				testdnszone.Build(
					dnsZoneBase(),
					testdnszone.WithControllerOwnerReference(testclusterdeployment.Build(
						clusterDeploymentBase(),
					)),
				),
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
			),
			expectedErr: true,
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
			fakeClient := fake.NewFakeClient(test.existingObjs...)
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockRemoteClientBuilder := remoteclientmock.NewMockBuilder(mockCtrl)
			rcd := &ReconcileClusterDeployment{
				Client:                        fakeClient,
				scheme:                        scheme.Scheme,
				logger:                        log.WithField("controller", "clusterDeployment"),
				remoteClusterAPIClientBuilder: func(*hivev1.ClusterDeployment) remoteclient.Builder { return mockRemoteClientBuilder },
			}

			// act
			actualDNSZone, err := rcd.ensureManagedDNSZone(test.clusterDeployment, rcd.logger)
			actualDNSNotReadyCondition := controllerutils.FindClusterDeploymentCondition(test.clusterDeployment.Status.Conditions, hivev1.DNSNotReadyCondition)

			// assert
			if test.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, test.expectedDNSZone, actualDNSZone, "Expected DNSZone doesn't match returned zone")

			if actualDNSNotReadyCondition != nil {
				actualDNSNotReadyCondition.LastProbeTime = metav1.Time{}      // zero out so it won't be checked.
				actualDNSNotReadyCondition.LastTransitionTime = metav1.Time{} // zero out so it won't be checked.
				actualDNSNotReadyCondition.Message = ""                       // zero out so it won't be checked.
			}
			assert.Equal(t, test.expectedDNSNotReadyCondition, actualDNSNotReadyCondition, "Expected DNSZone DNSNotReady condition doesn't match returned condition")
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
