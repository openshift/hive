package clusterdeprovision

import (
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mockaws "github.com/openshift/hive/pkg/awsclient/mock"
	mockazure "github.com/openshift/hive/pkg/azureclient/mock"
	ofake "github.com/openshift/hive/pkg/client/fake"
	mockgcp "github.com/openshift/hive/pkg/gcpclient/mock"
	testfake "github.com/openshift/hive/pkg/test/fake"
)

type mocks struct {
	fakeKubeClient  client.Client
	mockCtrl        *gomock.Controller
	mockAWSClient   *mockaws.MockClient
	mockGCPClient   *mockgcp.MockClient
	mockAzureClient *mockazure.MockClient
}

// setupDefaultMocks is an easy way to setup all of the default mocks
func setupDefaultMocks(t *testing.T, failDelete bool, initObjs ...runtime.Object) *mocks {
	oFakeClient := ofake.FakeClientWithCustomErrors{
		Client: testfake.NewFakeClientBuilder().WithRuntimeObjects(initObjs...).Build(),
	}
	if failDelete {
		// There's only one Delete() call in the reconcile flow. Mock it to error if requested.
		oFakeClient.DeleteBehavior = []error{fmt.Errorf("An error")}
	}
	mocks := &mocks{
		fakeKubeClient: oFakeClient,
		mockCtrl:       gomock.NewController(t),
	}

	mocks.mockAWSClient = mockaws.NewMockClient(mocks.mockCtrl)
	mocks.mockGCPClient = mockgcp.NewMockClient(mocks.mockCtrl)
	mocks.mockAzureClient = mockazure.NewMockClient(mocks.mockCtrl)

	return mocks
}
