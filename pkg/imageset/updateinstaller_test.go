package imageset

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	imageapi "github.com/openshift/api/image/v1"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/apis/hive/v1/baremetal"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	testInstallerImage = "registry.io/test-installer-image:latest"
	testCLIImage       = "registry.io/test-cli-image:latest"
)

func TestUpdateInstallerImageCommand(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name                      string
		existingClusterDeployment *hivev1.ClusterDeployment
		expectError               bool
		setupWorkDir              func(t *testing.T, dir string)
		validateClusterDeployment func(t *testing.T, clusterDeployment *hivev1.ClusterDeployment)
	}{
		{
			name:                      "successful execution",
			existingClusterDeployment: testClusterDeployment(),
			setupWorkDir: writeImageReferencesFile(
				map[string]string{
					"installer": testInstallerImage,
					"cli":       testCLIImage,
				},
			),
			validateClusterDeployment: validateSuccessfulExecution,
		},
		{
			name:                      "failure execution",
			existingClusterDeployment: testClusterDeployment(),
			setupWorkDir: writeImageReferencesFile(
				map[string]string{
					"installer": testInstallerImage,
				},
			),
			validateClusterDeployment: validateFailureExecution,
			expectError:               true,
		},
		{
			name:                      "successful execution after failure",
			existingClusterDeployment: testClusterDeploymentWithErrorCondition(),
			setupWorkDir: writeImageReferencesFile(
				map[string]string{
					"installer": testInstallerImage,
					"cli":       testCLIImage,
				},
			),
			validateClusterDeployment: validateSuccessfulExecutionAfterFailure,
		},
		{
			name: "baremetal",
			existingClusterDeployment: func() *hivev1.ClusterDeployment {
				cd := testClusterDeployment()
				cd.Spec.Platform.BareMetal = &baremetal.Platform{}
				return cd
			}(),
			setupWorkDir: writeImageReferencesFile(
				map[string]string{
					"baremetal-installer": testInstallerImage,
					"cli":                 testCLIImage,
				},
			),
			validateClusterDeployment: validateSuccessfulExecution,
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
				ClusterDeploymentName: testClusterDeployment().Name,
				WorkDir:               workDir,
				log:                   log.WithField("test", test.name),
				client:                client,
			}

			test.setupWorkDir(t, workDir)

			err = opt.Run()
			if !test.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if test.expectError && err == nil {
				t.Errorf("expected error but did not get one")
				return
			}

			clusterDeployment := &hivev1.ClusterDeployment{}
			err = client.Get(context.TODO(), types.NamespacedName{Name: testClusterDeployment().Name, Namespace: testClusterDeployment().Namespace}, clusterDeployment)
			if err != nil {
				t.Fatalf("unexpected get error: %v", err)
			}
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

func validateSuccessfulExecution(t *testing.T, clusterDeployment *hivev1.ClusterDeployment) {
	if clusterDeployment.Status.InstallerImage == nil ||
		*clusterDeployment.Status.InstallerImage != testInstallerImage {
		t.Errorf("did not get expected installer image in status")
	}
	if len(clusterDeployment.Status.Conditions) != 0 {
		t.Errorf("conditions is not empty")
	}
}

func validateSuccessfulExecutionAfterFailure(t *testing.T, clusterDeployment *hivev1.ClusterDeployment) {
	if clusterDeployment.Status.InstallerImage == nil ||
		*clusterDeployment.Status.InstallerImage != testInstallerImage {
		t.Errorf("did not get expected installer image in status")
	}
	condition := controllerutils.FindClusterDeploymentCondition(clusterDeployment.Status.Conditions, hivev1.InstallerImageResolutionFailedCondition)
	if condition == nil {
		t.Errorf("no failure condition found")
		return
	}
	if condition.Status != corev1.ConditionFalse {
		t.Errorf("unexpected condition status")
	}
	if condition.Reason != installerImageResolvedReason {
		t.Errorf("unexpected condition reason")
	}
}

func validateFailureExecution(t *testing.T, clusterDeployment *hivev1.ClusterDeployment) {
	condition := controllerutils.FindClusterDeploymentCondition(clusterDeployment.Status.Conditions, hivev1.InstallerImageResolutionFailedCondition)
	if condition == nil {
		t.Errorf("no failure condition found")
		return
	}
	if condition.Status != corev1.ConditionTrue {
		t.Errorf("unexpected condition status")
	}
	if !strings.Contains(condition.Message, "could not get cli image") {
		t.Errorf("condition message does not contain expected error message: %s", condition.Message)
	}
}

func writeImageReferencesFile(images map[string]string) func(*testing.T, string) {
	return func(t *testing.T, dir string) {
		imageStream := &imageapi.ImageStream{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ImageStream",
				APIVersion: imageapi.GroupVersion.String(),
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
}
