package imageset

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"testing"

	imageapi "github.com/openshift/api/image/v1"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openshift/hive/apis"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/apis/hive/v1/baremetal"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	testInstallerImage     = "registry.io/test-installer-image:latest"
	testCLIImage           = "registry.io/test-cli-image:latest"
	testReleaseVersion     = "v0.0.0-test-version"
	installerImageOverride = "quay.io/foo/bar:baz"
)

func TestUpdateInstallerImageCommand(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name    string
		images  map[string]string
		version string

		existingClusterDeployment *hivev1.ClusterDeployment
		expectError               bool
		validateClusterDeployment func(t *testing.T, clusterDeployment *hivev1.ClusterDeployment)
	}{
		{
			name:                      "successful execution",
			existingClusterDeployment: testClusterDeployment(),
			images: map[string]string{
				"installer": testInstallerImage,
				"cli":       testCLIImage,
			},
			validateClusterDeployment: validateSuccessfulExecution(testInstallerImage, ""),
		},
		{
			name:                      "failure execution missing cli",
			existingClusterDeployment: testClusterDeployment(),
			images: map[string]string{
				"installer": testInstallerImage,
			},
			validateClusterDeployment: validateFailureExecution,
			expectError:               true,
		},
		{
			name:                      "successful execution after failure",
			existingClusterDeployment: testClusterDeploymentWithErrorCondition(),
			images: map[string]string{
				"installer": testInstallerImage,
				"cli":       testCLIImage,
			},
			validateClusterDeployment: validateSuccessfulExecution(testInstallerImage, installerImageResolvedReason),
		},
		{
			name: "successful execution baremetal platform",
			existingClusterDeployment: func() *hivev1.ClusterDeployment {
				cd := testClusterDeployment()
				cd.Spec.Platform.BareMetal = &baremetal.Platform{}
				return cd
			}(),
			images: map[string]string{
				"baremetal-installer": testInstallerImage,
				"cli":                 testCLIImage,
			},
			validateClusterDeployment: validateSuccessfulExecution(testInstallerImage, ""),
		},
		{
			name:                      "successful execution with version in release metadata",
			existingClusterDeployment: testClusterDeployment(),
			images: map[string]string{
				"installer": testInstallerImage,
				"cli":       testCLIImage,
			},
			version:                   testReleaseVersion,
			validateClusterDeployment: validateSuccessfulExecution(testInstallerImage, ""),
		},
		{
			name:                      "installer image override",
			existingClusterDeployment: testClusterDeploymentWithInstallerImageOverride(installerImageOverride),
			images: map[string]string{
				"installer": testInstallerImage,
				"cli":       testCLIImage,
			},
			validateClusterDeployment: validateSuccessfulExecution(installerImageOverride, ""),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := fake.NewFakeClient(test.existingClusterDeployment)
			workDir, err := ioutil.TempDir("", "test-update")
			if err != nil {
				t.Fatalf("error creating test directory: %v", err)
			}
			opt := UpdateInstallerImageOptions{
				ClusterDeploymentName:      testClusterDeployment().Name,
				ClusterDeploymentNamespace: "test-namespace",
				WorkDir:                    workDir,
				log:                        log.WithField("test", test.name),
				client:                     client,
			}

			writeImageReferencesFile(t, workDir, test.images)
			writeReleaseMetadataFile(t, workDir, test.version)

			err = opt.Run()
			if test.expectError {
				assert.Error(t, err, "expected error but did not get one")
			} else {
				assert.NoError(t, err, "unexpected error")
			}

			clusterDeployment := &hivev1.ClusterDeployment{}
			err = client.Get(context.TODO(), types.NamespacedName{Name: testClusterDeployment().Name, Namespace: testClusterDeployment().Namespace}, clusterDeployment)
			assert.NoError(t, err, "unexpected get error")
			test.validateClusterDeployment(t, clusterDeployment)
		})
	}
}

func testClusterDeploymentWithErrorCondition() *hivev1.ClusterDeployment {
	cis := testClusterDeployment()
	cis.Status.Conditions = []hivev1.ClusterDeploymentCondition{
		{
			Type:               hivev1.InstallerImageResolutionFailedCondition,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: metav1.Now(),
			Reason:             "FailedToResolve",
			Message:            "sample failure message",
		},
	}
	return cis
}

func testClusterDeploymentWithInstallerImageOverride(override string) *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.Spec.Provisioning = &hivev1.Provisioning{
		InstallerImageOverride: override,
	}
	return cd
}

// If expectedReason is empty, we won't check it.
func validateSuccessfulExecution(expectedInstallerImage, expectedReason string) func(*testing.T, *hivev1.ClusterDeployment) {
	return func(t *testing.T, clusterDeployment *hivev1.ClusterDeployment) {
		if assert.NotNil(t, clusterDeployment.Status.InstallerImage, "did not get an installer image in status") {
			assert.Equal(t, expectedInstallerImage, *clusterDeployment.Status.InstallerImage, "did not get expected installer image in status")
		}
		condition := controllerutils.FindClusterDeploymentCondition(clusterDeployment.Status.Conditions, hivev1.InstallerImageResolutionFailedCondition)
		if assert.NotNil(t, condition, "could not find InstallerImageResolutionFailed condition") {
			assert.Equal(t, corev1.ConditionFalse, condition.Status, "unexpected condition status")
			if expectedReason != "" {
				assert.Equal(t, expectedReason, condition.Reason, "unexpected condition reason")
			}
		}
	}
}

func validateFailureExecution(t *testing.T, clusterDeployment *hivev1.ClusterDeployment) {
	condition := controllerutils.FindClusterDeploymentCondition(clusterDeployment.Status.Conditions, hivev1.InstallerImageResolutionFailedCondition)
	if assert.NotNil(t, condition, "no failure condition found") {
		assert.Equal(t, corev1.ConditionTrue, condition.Status, "unexpected condition status")
		assert.Contains(t, condition.Message, "could not get cli image", "condition message does not contain expected error message")
	}
}

func writeImageReferencesFile(t *testing.T, dir string, images map[string]string) {
	imageStream := &imageapi.ImageStream{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ImageStream",
			APIVersion: imageapi.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: testReleaseVersion,
		},
	}
	imageStream.Spec.Tags = make([]imageapi.TagReference, len(images))
	for k, v := range images {
		imageStream.Spec.Tags = append(imageStream.Spec.Tags,
			imageapi.TagReference{
				Name: k,
				From: &corev1.ObjectReference{
					Kind: "DockerImage",
					Name: v,
				},
			},
		)
	}
	imageStreamData, err := yaml.Marshal(imageStream)
	require.NoError(t, err, "failed to marshal image stream")
	err = ioutil.WriteFile(filepath.Join(dir, imageReferencesFilename), imageStreamData, 0644)
	require.NoError(t, err, "failed to write image references file")
}

func writeReleaseMetadataFile(t *testing.T, dir string, version string) {
	rm := &cincinnatiMetadata{
		Kind:    "cincinnati-metadata-v0",
		Version: version,
	}
	rmRaw, err := json.Marshal(rm)
	require.NoError(t, err, "failed to marshal release metadata")
	err = ioutil.WriteFile(filepath.Join(dir, releaseMetadataFilename), rmRaw, 0644)
	require.NoError(t, err, "failed to write release metadata file")
}
