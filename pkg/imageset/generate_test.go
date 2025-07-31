package imageset

import (
	"testing"

	hiveassert "github.com/openshift/hive/pkg/test/assert"
	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

const (
	testHttpProxy  = "localhost:3112"
	testHttpsProxy = "localhost:4432"
	testNoProxy    = "example.com,foo.com,bar.org"
)

func TestGenerateImageSetJob(t *testing.T) {
	job := GenerateImageSetJob(
		testClusterDeployment(),
		testImageSet().Spec.ReleaseImage,
		"test-service-account",
		testHttpProxy, testHttpsProxy, testNoProxy,
		map[string]string{}, []corev1.Toleration{}, []corev1.LocalObjectReference{})
	assert.Equal(t, GetImageSetJobName(testClusterDeployment().Name), job.Name, "unexpected job name")
	assert.Equal(t, testClusterDeployment().Namespace, job.Namespace, "unexpected job namespace")
	assert.Len(t, job.Spec.Template.Spec.InitContainers, 1, "unexpected number of init containers")
	assert.Len(t, job.Spec.Template.Spec.Containers, 1, "unexpected number of containers")
	assert.True(t, hasVolume(job, "common"), "missing common volume")
	hiveassert.AssertAllContainersHaveEnvVar(t, &job.Spec.Template.Spec, "HTTP_PROXY", testHttpProxy)
	hiveassert.AssertAllContainersHaveEnvVar(t, &job.Spec.Template.Spec, "HTTPS_PROXY", testHttpsProxy)
	hiveassert.AssertAllContainersHaveEnvVar(t, &job.Spec.Template.Spec, "NO_PROXY", testNoProxy)
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
	is.Spec.ReleaseImage = "test-release-image"
	return is
}

func hasVolume(job *batchv1.Job, name string) bool {
	for _, v := range job.Spec.Template.Spec.Volumes {
		if v.Name == name {
			return true
		}
	}
	return false
}
