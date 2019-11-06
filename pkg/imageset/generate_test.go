package imageset

import (
	"strings"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
)

const (
	testCLIImage           = "registry.io/cli-image:latest"
	testCLIImagePullPolicy = corev1.PullNever
)

var (
	testCLIImageSpec = ImageSpec{
		Image:      testCLIImage,
		PullPolicy: testCLIImagePullPolicy,
	}
)

func TestGenerateImageSetJob(t *testing.T) {

	tests := []struct {
		name string
		cd   func(*hivev1.ClusterDeployment)
		// additionalValidation must return true if validation is passed
		additionalValidation func(*batchv1.Job) bool
	}{
		{
			name:                 "cli-image-only",
			additionalValidation: func(job *batchv1.Job) bool { return true },
		},
		{
			name: "installer-image-override",
			cd: func(cd *hivev1.ClusterDeployment) {
				cd.Status.InstallerImage = strPtr("custom-image")
			},
			additionalValidation: func(job *batchv1.Job) bool {
				for _, c := range job.Spec.Template.Spec.Containers {
					if c.Name == "hiveutil" {
						for _, a := range c.Args {
							if strings.Contains(a, "custom-image") {
								return true
							}
						}
					}
				}
				return false
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deployment := testClusterDeployment()
			if test.cd != nil {
				test.cd(deployment)
			}
			job := GenerateImageSetJob(deployment, *testImageSet().Spec.ReleaseImage, "test-service-account", testCLIImageSpec)
			validateJob(t, job)
			if !test.additionalValidation(job) {
				t.Errorf("additional validation failed %s", test.name)
			}
		})
	}
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
