package nameserver

import (
	"fmt"
	"math/rand"
	"os"
	"os/user"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/gcpclient"
)

// This test will perform a test using real queries with GCP.
// By default, this test will be skipped.
// To enable the test, set the TEST_LIVE_GCP environment variable to the value
// of the root domain that you would like to use for the tests. Note that there
// must be a public managed zone for that root domain in the default GCP account.
func TestLiveGCP(t *testing.T) {
	rootDomain := os.Getenv("TEST_LIVE_GCP")
	if rootDomain == "" {
		t.SkipNow()
	}
	suite.Run(t, &LiveGCPTestSuite{rootDomain: rootDomain})
}

type LiveGCPTestSuite struct {
	suite.Suite
	rootDomain string
}

func (s *LiveGCPTestSuite) TestGetForNonExistentZone() {
	nameServers, err := s.getCUT().Get("non-existent.zone.live-gcp-test.com")
	s.NoError(err, "expected no error")
	s.Empty(nameServers, "expected no name servers")
}

func (s *LiveGCPTestSuite) TestGetForExistentZone() {
	nameServers, err := s.getCUT().Get(s.rootDomain)
	s.NoError(err, "expected no error")
	s.NotEmpty(nameServers, "expected some name servers")
	s.Len(nameServers[s.rootDomain], 4, "expected NS to have 4 values")
}

func (s *LiveGCPTestSuite) TestCreateAndDelete_SingleValue() {
	s.testCreateAndDelete(&testCreateAndDeleteCase{
		createValues: []string{"test-value"},
		deleteValues: []string{"test-value"},
	})
}

func (s *LiveGCPTestSuite) TestCreateAndDelete_SingleValueOutdatedDelete() {
	s.testCreateAndDelete(&testCreateAndDeleteCase{
		createValues: []string{"test-value"},
		deleteValues: []string{"bad-value"},
	})
}

func (s *LiveGCPTestSuite) TestCreateAndDelete_MultipleValues() {
	s.testCreateAndDelete(&testCreateAndDeleteCase{
		createValues: []string{"test-value-1", "test-value-2", "test-value-3"},
		deleteValues: []string{"test-value-1", "test-value-2", "test-value-3"},
	})
}

func (s *LiveGCPTestSuite) TestCreateAndDelete_MultipleValuesOutdatedDelete() {
	s.testCreateAndDelete(&testCreateAndDeleteCase{
		createValues: []string{"test-value-1", "test-value-2", "test-value-3"},
		deleteValues: []string{"test-value-1", "test-value-2"},
	})
}

func (s *LiveGCPTestSuite) TestCreateAndDelete_UnknownDeleteValues() {
	s.testCreateAndDelete(&testCreateAndDeleteCase{
		createValues: []string{"test-value"},
	})
}

func (s *LiveGCPTestSuite) TestCreateThenUpdate_SameValuesOnUpdate() {
	s.testCreateThenUpdate(&testCreateThenUpdateCase{
		createValues: []string{"test-value"},
		updateValues: []string{"test-value"},
	})
}

func (s *LiveGCPTestSuite) TestCreateThenUpdate_DifferentValuesOnUpdate() {
	s.testCreateThenUpdate(&testCreateThenUpdateCase{
		createValues: []string{"test-value"},
		updateValues: []string{"test-value-2"},
	})
}

func (s *LiveGCPTestSuite) testCreateAndDelete(tc *testCreateAndDeleteCase) {
	cut := s.getCUT()
	domain := fmt.Sprintf("live-gcp-test-%08d.%s", rand.Intn(100000000), s.rootDomain)
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
	s.Equal(sets.NewString(tc.createValues...), actualValues, "unexpected values for domain")
}

func (s *LiveGCPTestSuite) TestDeleteOfNonExistentNS() {
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

func (s *LiveGCPTestSuite) getCUT() *gcpQuery {
	usr, err := user.Current()
	if err != nil {
		s.T().Fatalf("could not get the current user: %v", err)
	}
	credsFile := filepath.Join(usr.HomeDir, ".gcp", constants.GCPCredentialsName)
	return &gcpQuery{
		getGCPClient: func() (gcpclient.Client, error) {
			return gcpclient.NewClientFromFile(credsFile)
		},
	}
}

func (s *LiveGCPTestSuite) testCreateThenUpdate(tc *testCreateThenUpdateCase) {
	cut := s.getCUT()
	domain := fmt.Sprintf("live-gcp-test-%08d.%s", rand.Intn(100000000), s.rootDomain)
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
	s.Equal(sets.NewString(tc.updateValues...), actualValues, "unexpected values for domain")
}
