package nameserver

import (
	"fmt"
	"math/rand"
	"os"
	"os/user"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/util/sets"

	azurerest "github.com/Azure/go-autorest/autorest/azure"
	"github.com/openshift/hive/pkg/azureclient"
	"github.com/openshift/hive/pkg/constants"
)

// This test will perform a test using real queries with Azure.
// By default, this test will be skipped.
// To enable the test, set the TEST_LIVE_AZURE environment variable to the value
// of the root domain and TEST_LIVE_AZURE_RESOURCE_GROUP to the value of the resource group
// that you would like to use for the tests. Note that there must be a public DNS zone
// for that root domain in the default Azure account.
func TestLiveAzure(t *testing.T) {
	rootDomain := os.Getenv("TEST_LIVE_AZURE")
	resourceGroupName := os.Getenv("TEST_LIVE_AZURE_RESOURCE_GROUP")
	if rootDomain == "" {
		t.SkipNow()
	}
	rand.Seed(time.Now().UnixNano())
	suite.Run(t, &LiveAzureTestSuite{
		resourceGroupName: resourceGroupName,
		rootDomain:        rootDomain,
	})
}

type LiveAzureTestSuite struct {
	suite.Suite
	resourceGroupName string
	rootDomain        string
}

func (s *LiveAzureTestSuite) TestGetForNonExistentZone() {
	nameServers, err := s.getCUT().Get("non-existent.zone.live-azure.test.com")
	s.Error(err, "expected error")
	s.Empty(nameServers, "expected no name servers")
}

func (s *LiveAzureTestSuite) TestGetForExistentZone() {
	nameServers, err := s.getCUT().Get(s.rootDomain)
	s.NoError(err, "expected no error")
	s.NotEmpty(nameServers, "expected some name servers")
	s.Len(nameServers[s.rootDomain], 4, "expected NS to have 4 values")
}

func (s *LiveAzureTestSuite) TestCreateAndDelete_SingleValue() {
	s.testCreateAndDelete(&testCreateAndDeleteCase{
		createValues: []string{"test-value"},
		deleteValues: []string{"test-value"},
	})
}

func (s *LiveAzureTestSuite) TestCreateAndDelete_SingleValueOutdatedDelete() {
	s.testCreateAndDelete(&testCreateAndDeleteCase{
		createValues: []string{"test-value"},
		deleteValues: []string{"bad-value"},
	})
}

func (s *LiveAzureTestSuite) TestCreateAndDelete_MultipleValues() {
	s.testCreateAndDelete(&testCreateAndDeleteCase{
		createValues: []string{"test-value-1", "test-value-2", "test-value-3"},
		deleteValues: []string{"test-value-1", "test-value-2", "test-value-3"},
	})
}

func (s *LiveAzureTestSuite) TestCreateAndDelete_MultipleValuesOutdatedDelete() {
	s.testCreateAndDelete(&testCreateAndDeleteCase{
		createValues: []string{"test-value-1", "test-value-2", "test-value-3"},
		deleteValues: []string{"test-value-1", "test-value-2"},
	})
}

func (s *LiveAzureTestSuite) TestCreateAndDelete_UnknownDeleteValues() {
	s.testCreateAndDelete(&testCreateAndDeleteCase{
		createValues: []string{"test-value"},
	})
}

func (s *LiveAzureTestSuite) TestCreateThenUpdate_SameValuesOnUpdate() {
	s.testCreateThenUpdate(&testCreateThenUpdateCase{
		createValues: []string{"test-value"},
		updateValues: []string{"test-value"},
	})
}

func (s *LiveAzureTestSuite) TestCreateThenUpdate_DifferentValuesOnUpdate() {
	s.testCreateThenUpdate(&testCreateThenUpdateCase{
		createValues: []string{"test-value"},
		updateValues: []string{"test-value-2"},
	})
}

func (s *LiveAzureTestSuite) TestDeleteOfNonExistentNS() {
	cases := []struct {
		name         string
		deleteValues []string
	}{
		{
			name:         "known values",
			deleteValues: []string{"test-value."},
		},
		{
			name: "unknown values",
		},
	}
	for _, tc := range cases {
		s.T().Run(tc.name, func(t *testing.T) {
			err := s.getCUT().Delete(s.rootDomain, fmt.Sprintf("non-existent.subdomain.%s", s.rootDomain), sets.New(tc.deleteValues...))
			s.NoError(err, "expected no error")
		})
	}
}

func (s *LiveAzureTestSuite) testCreateAndDelete(tc *testCreateAndDeleteCase) {
	cut := s.getCUT()
	domain := fmt.Sprintf("live-azure-test-%08d.%s", rand.Intn(100000000), s.rootDomain)
	s.T().Logf("domain = %q", domain)
	err := cut.CreateOrUpdate(s.rootDomain, domain, sets.New(tc.createValues...))
	if s.NoError(err, "unexpected error creating NS") {
		defer func() {
			err := cut.Delete(s.rootDomain, domain, sets.New(tc.deleteValues...))
			s.NoError(err, "unexpected error deleting NS")
		}()
	}

	nameServers, err := cut.Get(s.rootDomain)
	s.NoError(err, "unexpected error querying domain")
	s.NotEmpty(nameServers, "expected some name servers")
	actualValues := nameServers[domain]
	s.Equal(sets.New(tc.createValues...), actualValues, "unexpected values for domain")
}

func (s *LiveAzureTestSuite) testCreateThenUpdate(tc *testCreateThenUpdateCase) {
	cut := s.getCUT()
	domain := fmt.Sprintf("live-azure-test-%08d.%s", rand.Intn(100000000), s.rootDomain)
	s.T().Logf("domain = %q", domain)
	err := cut.CreateOrUpdate(s.rootDomain, domain, sets.New(tc.createValues...))
	if s.NoError(err, "unexpected error creating NS") {
		defer func() {
			err := cut.Delete(s.rootDomain, domain, sets.Set[string]{})
			s.NoError(err, "unexpected error deleting NS")
		}()
	}

	// now test updating by re-issuing a Create()
	err = cut.CreateOrUpdate(s.rootDomain, domain, sets.New(tc.updateValues...))
	s.NoError(err, "unexpected error updating NS")

	nameServers, err := cut.Get(s.rootDomain)
	s.NoError(err, "unexpected error querying domain")
	s.NotEmpty(nameServers, "expected some name servers")
	actualValues := nameServers[domain]
	s.Equal(sets.New(tc.updateValues...), actualValues, "unexpected values for domain")
}

func (s *LiveAzureTestSuite) getCUT() *azureQuery {
	usr, err := user.Current()
	if err != nil {
		s.T().Fatalf("could not get the current user: %v", err)
	}
	credsFile := filepath.Join(usr.HomeDir, ".azure", constants.AzureCredentialsName)
	return &azureQuery{
		getAzureClient: func() (azureclient.Client, error) {
			return azureclient.NewClientFromFile(credsFile, azurerest.PublicCloud.Name)
		},
		resourceGroupName: s.resourceGroupName,
	}
}
