package installmanager

import (
	"testing"

	"github.com/golang/mock/gomock"
	mockaws "github.com/openshift/hive/pkg/awsclient/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakekubeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

type mocks struct {
	fakeKubeClient client.Client
	mockCtrl       *gomock.Controller
	mockAWSClient  *mockaws.MockClient
}

// setupDefaultMocks is an easy way to setup all of the default mocks
func setupDefaultMocks(t *testing.T, initObjs ...runtime.Object) *mocks {
	mocks := &mocks{
		fakeKubeClient: fakekubeclient.NewFakeClient(initObjs...),
		mockCtrl:       gomock.NewController(t),
	}

	mocks.mockAWSClient = mockaws.NewMockClient(mocks.mockCtrl)

	return mocks
}

func testClusterProvision() *hivev1.ClusterProvision {
	return &hivev1.ClusterProvision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testProvisionName,
			Namespace: testNamespace,
		},
		Spec: hivev1.ClusterProvisionSpec{
			ClusterDeploymentRef: corev1.LocalObjectReference{
				Name: testDeploymentName,
			},
			Stage: hivev1.ClusterProvisionStageProvisioning,
		},
	}
}
func testClusterProvisionWithInfraIDSet() *hivev1.ClusterProvision {
	return &hivev1.ClusterProvision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testProvisionName,
			Namespace: testNamespace,
		},
		Spec: hivev1.ClusterProvisionSpec{
			ClusterDeploymentRef: corev1.LocalObjectReference{
				Name: testDeploymentName,
			},
			Stage:   hivev1.ClusterProvisionStageProvisioning,
			InfraID: pointer.StringPtr("dummy-infra-id"),
		},
	}
}
