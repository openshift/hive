package clusterdeprovision

import (
	"testing"

	"github.com/golang/mock/gomock"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	fakekubeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	mockaws "github.com/openshift/hive/pkg/awsclient/mock"
	mockazure "github.com/openshift/hive/pkg/azureclient/mock"
	mockgcp "github.com/openshift/hive/pkg/gcpclient/mock"
)

type mocks struct {
	fakeKubeClient  client.Client
	mockCtrl        *gomock.Controller
	mockAWSClient   *mockaws.MockClient
	mockGCPClient   *mockgcp.MockClient
	mockAzureClient *mockazure.MockClient
}

// setupDefaultMocks is an easy way to setup all of the default mocks
func setupDefaultMocks(t *testing.T, initObjs ...runtime.Object) *mocks {
	mocks := &mocks{
		fakeKubeClient: fakekubeclient.NewFakeClient(initObjs...),
		mockCtrl:       gomock.NewController(t),
	}

	mocks.mockAWSClient = mockaws.NewMockClient(mocks.mockCtrl)
	mocks.mockGCPClient = mockgcp.NewMockClient(mocks.mockCtrl)
	mocks.mockAzureClient = mockazure.NewMockClient(mocks.mockCtrl)

	return mocks
}
