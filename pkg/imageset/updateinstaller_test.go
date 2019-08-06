package imageset

import (
	"context"
	"io/ioutil"
	"path"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	testInstallerImage = "registry.io/test-installer-image:latest"
	testErrorMessage   = "failed to obtain installer image because of an error"
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
			setupWorkDir:              setupSuccessfulExecutionWorkDir,
			validateClusterDeployment: validateSuccessfulExecution,
		},
		{
			name:                      "failure execution",
			existingClusterDeployment: testClusterDeployment(),
			setupWorkDir:              setupFailureExecutionWorkDir,
			validateClusterDeployment: validateFailureExecution,
		},
		{
			name:                      "successful execution after failure",
			existingClusterDeployment: testClusterDeploymentWithErrorCondition(),
			setupWorkDir:              setupSuccessfulExecutionWorkDir,
			validateClusterDeployment: validateSuccessfulExecutionAfterFailure,
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
			Message:            testErrorMessage,
		},
	}
	return cis
}

func setupWorkDir(t *testing.T, dir, success, installerImage, cliImage, errorMessage string) {
	err := ioutil.WriteFile(path.Join(dir, "success"), []byte(success), 0666)
	if err != nil {
		t.Fatalf("error writing file: %v", err)
	}
	if len(installerImage) != 0 {
		err = ioutil.WriteFile(path.Join(dir, "installer-image.txt"), []byte(testInstallerImage), 0666)
		if err != nil {
			t.Fatalf("error writing file: %v", err)
		}
	}
	if len(cliImage) != 0 {
		err = ioutil.WriteFile(path.Join(dir, "cli-image.txt"), []byte(testCLIImage), 0666)
		if err != nil {
			t.Fatalf("error writing file: %v", err)
		}
	}
	if len(errorMessage) != 0 {
		err = ioutil.WriteFile(path.Join(dir, "error.log"), []byte(errorMessage), 0666)
		if err != nil {
			t.Fatalf("error writing file: %v", err)
		}
	}

}

func setupSuccessfulExecutionWorkDir(t *testing.T, dir string) {
	setupWorkDir(t, dir, "1", testInstallerImage, testCLIImage, "")
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

func setupFailureExecutionWorkDir(t *testing.T, dir string) {
	setupWorkDir(t, dir, "0", "", "", testErrorMessage)
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
	if !strings.Contains(condition.Message, testErrorMessage) {
		t.Errorf("condition message does not contain expected error message: %s", condition.Message)
	}
}
