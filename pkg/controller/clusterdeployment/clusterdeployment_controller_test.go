package clusterdeployment

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/pkg/apis/hive/v1/aws"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
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
	sshKeySecret            = "ssh-key"
	pullSecretSecret        = "pull-secret"
	globalPullSecret        = "global-pull-secret"
	adminKubeconfigSecret   = "foo-lqmsh-admin-kubeconfig"
	adminKubeconfig         = `clusters:
- cluster:
    certificate-authority-data: JUNK
    server: https://bar-api.clusters.example.com:6443
  name: bar
`
	adminPasswordSecret             = "foo-lqmsh-admin-password"
	testRemoteClusterCurrentVersion = "4.0.0"
	remoteClusterVersionObjectName  = "version"

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

	getDeprovisionRequest := func(c client.Client) *hivev1.ClusterDeprovisionRequest {
		req := &hivev1.ClusterDeprovisionRequest{}
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
		name                  string
		existing              []runtime.Object
		pendingCreation       bool
		expectErr             bool
		expectedRequeueAfter  time.Duration
		expectPendingCreation bool
		validate              func(client.Client, *testing.T)
	}{
		{
			name: "Add finalizer",
			existing: []runtime.Object{
				testClusterDeploymentWithoutFinalizer(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
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
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
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
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
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
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
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
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
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
				testClusterDeploymentWithProvision(),
				testSuccessfulProvision(),
				testSecret(corev1.SecretTypeOpaque, adminKubeconfigSecret, "kubeconfig", adminKubeconfig),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
				testMetadataConfigMap(),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				assert.Equal(t, "https://bar-api.clusters.example.com:6443", cd.Status.APIURL)
				assert.Equal(t, "https://bar-api.clusters.example.com:6443/console", cd.Status.WebConsoleURL)
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
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if assert.NotNil(t, cd, "missing clusterdeployment") {
					assert.True(t, cd.Spec.Installed, "expected cluster to be installed")
				}
			},
		},
		{
			name: "PVC cleanup for successful install",
			existing: []runtime.Object{
				testInstalledClusterDeployment(time.Now()),
				testInstallLogPVC(),
				testMetadataConfigMap(),
				testSecret(corev1.SecretTypeOpaque, adminKubeconfigSecret, "kubeconfig", adminKubeconfig),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
			},
			validate: func(c client.Client, t *testing.T) {
				pvc := &corev1.PersistentVolumeClaim{}
				err := c.Get(context.TODO(), client.ObjectKey{Name: GetInstallLogsPVCName(testClusterDeployment()), Namespace: testNamespace}, pvc)
				if assert.Error(t, err) {
					assert.True(t, errors.IsNotFound(err))
				}
			},
		},
		{
			name: "PVC preserved for install with restarts",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testInstalledClusterDeployment(time.Now())
					cd.Status.InstallRestarts = 5
					return cd
				}(),
				testInstallLogPVC(),
				testMetadataConfigMap(),
				testSecret(corev1.SecretTypeOpaque, adminKubeconfigSecret, "kubeconfig", adminKubeconfig),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
			},
			validate: func(c client.Client, t *testing.T) {
				pvc := &corev1.PersistentVolumeClaim{}
				err := c.Get(context.TODO(), client.ObjectKey{Name: testInstallLogPVC().Name, Namespace: testNamespace}, pvc)
				assert.NoError(t, err)
			},
		},
		{
			name: "PVC cleanup for install with restarts after 7 days",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testInstalledClusterDeployment(time.Now().Add(-8 * 24 * time.Hour))
					cd.Status.InstallRestarts = 5
					return cd
				}(),
				testInstallLogPVC(),
				testMetadataConfigMap(),
				testSecret(corev1.SecretTypeOpaque, adminKubeconfigSecret, "kubeconfig", adminKubeconfig),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
			},
			validate: func(c client.Client, t *testing.T) {
				pvc := &corev1.PersistentVolumeClaim{}
				err := c.Get(context.TODO(), client.ObjectKey{Name: GetInstallLogsPVCName(testClusterDeployment()), Namespace: testNamespace}, pvc)
				if assert.Error(t, err) {
					assert.True(t, errors.IsNotFound(err))
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
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
			},
		},
		{
			name: "No-op deleted cluster without finalizer",
			existing: []runtime.Object{
				testDeletedClusterDeploymentWithoutFinalizer(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
			},
			validate: func(c client.Client, t *testing.T) {
				deprovision := getDeprovisionRequest(c)
				if deprovision != nil {
					t.Errorf("got unexpected deprovision request")
				}
			},
		},
		{
			name: "Delete expired cluster deployment",
			existing: []runtime.Object{
				testExpiredClusterDeployment(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
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
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if assert.NotNil(t, cd, "missing clusterdeployment") {
					assert.Empty(t, cd.Finalizers, "expected empty finalizers")
				}
				deprovision := getDeprovisionRequest(c)
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
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
			},
			validate: func(c client.Client, t *testing.T) {
				deprovision := getDeprovisionRequest(c)
				assert.NotNil(t, deprovision, "expected deprovision request")
			},
		},
		{
			name: "Resolve installer image from spec.images.installerimage",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
					cd.Status.InstallerImage = nil
					cd.Spec.Images.InstallerImage = "test-installer-image:latest"
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if cd.Status.InstallerImage == nil || *cd.Status.InstallerImage != "test-installer-image:latest" {
					t.Errorf("unexpected status.installerImage")
				}
			},
		},
		{
			name: "Resolve installer image from imageSet.spec.installerimage",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
					cd.Status.InstallerImage = nil
					cd.Spec.Images.InstallerImage = ""
					cd.Spec.ImageSetRef = &hivev1.ClusterImageSetReference{Name: testClusterImageSetName}
					return cd
				}(),
				func() *hivev1.ClusterImageSet {
					cis := testClusterImageSet()
					cis.Spec.InstallerImage = pointer.StringPtr("test-cis-installer-image:latest")
					return cis
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if cd.Status.InstallerImage == nil || *cd.Status.InstallerImage != "test-cis-installer-image:latest" {
					t.Errorf("unexpected status.installerImage")
				}
			},
		},
		{
			name: "Create job to resolve installer image",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
					cd.Status.InstallerImage = nil
					cd.Spec.Images.InstallerImage = ""
					cd.Spec.ImageSetRef = &hivev1.ClusterImageSetReference{Name: testClusterImageSetName}
					cd.Status.ClusterVersionStatus.AvailableUpdates = []openshiftapiv1.Update{}
					return cd
				}(),
				testClusterImageSet(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
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
						if e.Value != *testClusterImageSet().Spec.ReleaseImage {
							t.Errorf("unexpected release image used in job: %s", e.Value)
						}
						break
					}
				}
			},
		},
		{
			name: "Ensure release image from clusterdeployment (when present) is used to generate imageset job",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
					cd.Status.InstallerImage = nil
					cd.Spec.Images.InstallerImage = ""
					cd.Spec.Images.ReleaseImage = "embedded-release-image:latest"
					cd.Spec.ImageSetRef = &hivev1.ClusterImageSetReference{Name: testClusterImageSetName}
					cd.Status.ClusterVersionStatus.AvailableUpdates = []openshiftapiv1.Update{}
					return cd
				}(),
				testClusterImageSet(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
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
					cd.Spec.Images.InstallerImage = ""
					cd.Spec.ImageSetRef = &hivev1.ClusterImageSetReference{Name: testClusterImageSetName}
					return cd
				}(),
				func() *hivev1.ClusterImageSet {
					cis := testClusterImageSet()
					cis.Spec.ReleaseImage = pointer.StringPtr("test-release-image:latest")
					return cis
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
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
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
			},
			validate: func(c client.Client, t *testing.T) {
				zone := getDNSZone(c)
				assert.NotNil(t, zone, "dns zone should exist")
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
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
				testDNSZone(),
			},
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
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
				testDNSZone(),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				assertConditionStatus(t, cd, hivev1.DNSNotReadyCondition, corev1.ConditionTrue)
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
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
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
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
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
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
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
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
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
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
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
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
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
					cd.Spec.ImageSetRef = &hivev1.ClusterImageSetReference{Name: testClusterImageSetName}
					return cd
				}(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
			},
			validate: func(c client.Client, t *testing.T) {
				deprovision := getDeprovisionRequest(c)
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
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
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
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
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
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
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
					cd.Annotations[deleteAfterAnnotation] = "8h"
					return cd
				}(),
				testProvision(),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
			},
			expectedRequeueAfter: 8*time.Hour + 60*time.Second,
		},
		{
			name: "Wait after failed provision",
			existing: []runtime.Object{
				func() runtime.Object {
					cd := testClusterDeploymentWithProvision()
					cd.CreationTimestamp = metav1.Now()
					cd.Annotations[deleteAfterAnnotation] = "8h"
					return cd
				}(),
				testFailedProvisionTime(time.Now()),
				testMetadataConfigMap(),
				testSecret(corev1.SecretTypeOpaque, adminKubeconfigSecret, "kubeconfig", adminKubeconfig),
				testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecret, corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeDockerConfigJson, constants.GetMergedPullSecretName(testClusterDeployment()), corev1.DockerConfigJsonKey, "{}"),
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
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
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
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
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
			},
			expectedRequeueAfter: defaultRequeueTime,
			validate: func(c client.Client, t *testing.T) {
				provisions := getProvisions(c)
				assert.Empty(t, provisions, "expected provision to be deleted")
				deprovision := getDeprovisionRequest(c)
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
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
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
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if assert.NotNil(t, cd, "missing clusterdeployment") {
					assert.Contains(t, cd.Finalizers, hivev1.FinalizerDeprovision, "expected hive finalizer")
				}
				deprovision := getDeprovisionRequest(c)
				assert.NotNil(t, deprovision, "missing deprovision request")
			},
		},
		{
			name: "setSyncSetFailedCondition should be present",
			existing: []runtime.Object{
				testInstalledClusterDeployment(time.Now()),
				createSyncSetInstanceObj(hivev1.ApplyFailureSyncCondition),
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
			name: "setSyncSetFailedCondition value should be corev1.ConditionFalse",
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
				createSyncSetInstanceObj(hivev1.ApplySuccessSyncCondition),
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
			name: "Add cluster platform label",
			existing: []runtime.Object{
				testClusterDeploymentWithoutPlatformLabel(),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if assert.NotNil(t, cd, "missing clusterdeployment") {
					if assert.NotNil(t, cd.Labels, "missing labels map") {
						assert.Equal(t, cd.Labels[hivev1.HiveClusterPlatformLabel], getClusterPlatform(cd), "incorrect cluster platform label")
					}
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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logger := log.WithField("controller", "clusterDeployment")
			fakeClient := fake.NewFakeClient(test.existing...)
			controllerExpectations := controllerutils.NewExpectations(logger)
			rcd := &ReconcileClusterDeployment{
				Client:                        fakeClient,
				scheme:                        scheme.Scheme,
				logger:                        logger,
				expectations:                  controllerExpectations,
				remoteClusterAPIClientBuilder: testRemoteClusterAPIClientBuilder,
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
			rcd := &ReconcileClusterDeployment{
				Client:                        fakeClient,
				scheme:                        scheme.Scheme,
				logger:                        logger,
				expectations:                  controllerExpectations,
				remoteClusterAPIClientBuilder: testRemoteClusterAPIClientBuilder,
			}

			reconcileResult, err := rcd.Reconcile(reconcile.Request{
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

func testEmptyClusterDeployment() *hivev1.ClusterDeployment {
	cd := &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        testName,
			Namespace:   testNamespace,
			Finalizers:  []string{hivev1.FinalizerDeprovision},
			UID:         types.UID("1234"),
			Annotations: map[string]string{},
			Labels:      map[string]string{},
		},
	}
	return cd
}

func testClusterDeployment() *hivev1.ClusterDeployment {
	cd := testEmptyClusterDeployment()

	cd.Spec = hivev1.ClusterDeploymentSpec{
		ClusterName: testClusterName,
		Compute:     []hivev1.MachinePool{},
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
			InstallConfigSecretRef: corev1.LocalObjectReference{Name: "install-config-secret"},
		},
		ClusterMetadata: &hivev1.ClusterMetadata{
			ClusterID:                testClusterID,
			InfraID:                  testInfraID,
			AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: adminKubeconfigSecret},
			AdminPasswordSecretRef:   corev1.LocalObjectReference{Name: adminPasswordSecret},
		},
	}

	cd.Labels[hivev1.HiveClusterPlatformLabel] = "aws"

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
	cd.Annotations[deleteAfterAnnotation] = "5m"
	return cd
}

func testClusterDeploymentWithProvision() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.Status.ProvisionRef = &corev1.LocalObjectReference{Name: provisionName}
	return cd
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

func testInstallLogPVC() *corev1.PersistentVolumeClaim {
	pvc := &corev1.PersistentVolumeClaim{}
	pvc.Name = GetInstallLogsPVCName(testClusterDeployment())
	pvc.Namespace = testNamespace
	return pvc
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
	s := &corev1.Secret{
		Type: secretType,
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

func testRemoteClusterAPIClientBuilder(secretData, controllerName string) (client.Client, error) {
	remoteClusterVersion := &openshiftapiv1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name: remoteClusterVersionObjectName,
		},
	}
	remoteClusterVersion.Status = testRemoteClusterVersionStatus()

	remoteClusterRouteObject := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      remoteClusterRouteObjectName,
			Namespace: remoteClusterRouteObjectNamespace,
		},
	}
	remoteClusterRouteObject.Spec.Host = "bar-api.clusters.example.com:6443/console"

	remoteClient := fake.NewFakeClient(remoteClusterVersion, remoteClusterRouteObject)
	return remoteClient, nil
}

func testRemoteClusterVersionStatus() openshiftapiv1.ClusterVersionStatus {
	status := openshiftapiv1.ClusterVersionStatus{
		History: []openshiftapiv1.UpdateHistory{
			{
				State:   openshiftapiv1.CompletedUpdate,
				Version: testRemoteClusterCurrentVersion,
				Image:   "TESTIMAGE",
			},
		},
		ObservedGeneration: 123456789,
		VersionHash:        "TESTVERSIONHASH",
	}
	return status
}

func testClusterImageSet() *hivev1.ClusterImageSet {
	cis := &hivev1.ClusterImageSet{}
	cis.Name = testClusterImageSetName
	cis.Spec.ReleaseImage = pointer.StringPtr("test-release-image:latest")
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
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
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
				testSecret(corev1.SecretTypeOpaque, sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClient(test.existingCD...)
			rcd := &ReconcileClusterDeployment{
				Client:                        fakeClient,
				scheme:                        scheme.Scheme,
				logger:                        log.WithField("controller", "clusterDeployment"),
				remoteClusterAPIClientBuilder: testRemoteClusterAPIClientBuilder,
			}

			_, err := rcd.Reconcile(reconcile.Request{
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
		Compute:     []hivev1.MachinePool{},
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
			Namespace: constants.HiveNamespace,
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
			rcd := &ReconcileClusterDeployment{
				Client:                        fakeClient,
				scheme:                        scheme.Scheme,
				logger:                        log.WithField("controller", "clusterDeployment"),
				remoteClusterAPIClientBuilder: testRemoteClusterAPIClientBuilder,
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

func createSyncSetInstanceObj(syncCondType hivev1.SyncConditionType) *hivev1.SyncSetInstance {
	ssi := &hivev1.SyncSetInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSyncsetInstanceName,
			Namespace: testNamespace,
		},
	}
	ssi.Spec.ClusterDeploymentRef.Name = testName
	ssi.Status = createSyncSetInstanceStatus(syncCondType)
	return ssi
}

func createSyncSetInstanceStatus(syncCondType hivev1.SyncConditionType) hivev1.SyncSetInstanceStatus {
	conditionTime := metav1.NewTime(time.Now())
	var ssiStatus corev1.ConditionStatus
	var condType hivev1.SyncConditionType
	if syncCondType == hivev1.ApplyFailureSyncCondition {
		ssiStatus = corev1.ConditionTrue
		condType = syncCondType
	} else {
		ssiStatus = corev1.ConditionFalse
		condType = syncCondType
	}
	status := hivev1.SyncSetInstanceStatus{
		Conditions: []hivev1.SyncCondition{
			{
				Type:               condType,
				Status:             ssiStatus,
				LastTransitionTime: conditionTime,
				LastProbeTime:      conditionTime,
			},
		},
	}
	return status
}
