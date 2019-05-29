package imageset

import (
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
)

const (
	testCLIImage            = "registry.io/cli-image:latest"
	testCLIImagePullPolicy  = corev1.PullNever
	testHiveImage           = "registry.io/hive-image:latest"
	testHiveImagePullPolicy = corev1.PullAlways
	testPullSecretName      = "test-pull-secret"
)

var (
	testCLIImageSpec = ImageSpec{
		Image:      testCLIImage,
		PullPolicy: testCLIImagePullPolicy,
	}
	testHiveImageSpec = ImageSpec{
		Image:      testHiveImage,
		PullPolicy: testHiveImagePullPolicy,
	}
)

func TestGenerateImageSetJob(t *testing.T) {
	job := GenerateImageSetJob(testClusterDeployment(), *testImageSet().Spec.ReleaseImage, "test-service-account", testCLIImageSpec, testHiveImageSpec)
	validateJob(t, job)
}

func testClusterDeployment() *hivev1.ClusterDeployment {
	cd := &hivev1.ClusterDeployment{}
	cd.Name = "test-cluster-deployment"
	cd.Namespace = "test-namespace"
	return cd
}

func testImageSet() *hivev1.ClusterImageSet {
	is := &hivev1.ClusterImageSet{}
	is.Name = "test-image-set"
	is.Spec.ReleaseImage = strPtr("test-release-image")
	return is
}

func validateJob(t *testing.T, job *batchv1.Job) {
	if job.Name != GetImageSetJobName(testClusterDeployment().Name) {
		t.Errorf("unexpected job name: %s", job.Name)
	}
	if job.Namespace != testClusterDeployment().Namespace {
		t.Errorf("unexpected job namespace: %s", job.Namespace)
	}
	if len(job.Spec.Template.Spec.Containers) != 2 {
		t.Errorf("unexpected number of containers")
	}
	if !hasVariable(job, "RELEASE_IMAGE") {
		t.Errorf("missing RELEASE_IMAGE environment variable")
	}
	if !hasVolume(job, "common") {
		t.Errorf("missing common volume")
	}
	if !hasVariable(job, "PULL_SECRET") {
		t.Errorf("missing PULL_SECRET env var")
	}
	if !hasVolume(job, "pullsecret") {
		t.Errorf("missing pull secret volume")
	}
}

func hasVariable(job *batchv1.Job, name string) bool {
	for _, c := range job.Spec.Template.Spec.Containers {
		found := false
		for _, e := range c.Env {
			if e.Name == name {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func hasVolume(job *batchv1.Job, name string) bool {
	for _, v := range job.Spec.Template.Spec.Volumes {
		if v.Name == name {
			return true
		}
	}
	return false
}

func strPtr(str string) *string {
	return &str
}
