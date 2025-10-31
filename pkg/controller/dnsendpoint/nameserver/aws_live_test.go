package nameserver

import (
	"fmt"
	"math/rand"
	"os"
	"testing"

	"github.com/stretchr/testify/suite"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/hive/pkg/awsclient"
)

// This test will perform a test using real queries with AWS.
// By default, this test will be skipped.
// To enable the test, set the TEST_LIVE_AWS environment variable to the value
// of the root domain that you would like to use for the tests. Note that there
// must be a public hosted zone for that root domain in the default AWS account.
func TestLiveAWS(t *testing.T) {
	rootDomain := os.Getenv("TEST_LIVE_AWS")
	if rootDomain == "" {
		t.SkipNow()
	}
	suite.Run(t, &LiveAWSTestSuite{rootDomain: rootDomain})
}

type LiveAWSTestSuite struct {
	suite.Suite
	rootDomain string
}

func (s *LiveAWSTestSuite) TestGetForNonExistentZone() {
	nameServers, err := s.getCUT().Get("non-existent.zone.live-aws-test.com")
	s.NoError(err, "expected no error")
	s.Empty(nameServers, "expected no name servers")
}

func (s *LiveAWSTestSuite) TestGetForExistentZone() {
	nameServers, err := s.getCUT().Get(s.rootDomain)
	s.NoError(err, "expected no error")
	s.NotEmpty(nameServers, "expected some name servers")
	s.Len(nameServers[s.rootDomain], 4, "expected NS to have 4 values")
}

func (s *LiveAWSTestSuite) TestCreateAndDelete() {
	cases := []struct {
		name         string
		createValues []string
		deleteValues []string
	}{
		{
			name:         "single value",
			createValues: []string{"test-value"},
			deleteValues: []string{"test-value"},
		},
		{
			name:         "single value, outdated delete",
			createValues: []string{"test-value"},
			deleteValues: []string{"bad-value"},
		},
		{
			name:         "multiple values",
			createValues: []string{"test-value-1", "test-value-2", "test-value-3"},
			deleteValues: []string{"test-value-1", "test-value-2", "test-value-3"},
		},
		{
			name:         "multiple values, outdated delete",
			createValues: []string{"test-value-1", "test-value-2", "test-value-3"},
			deleteValues: []string{"test-value-1", "test-value-2"},
		},
		{
			name:         "unknown delete values",
			createValues: []string{"test-value"},
		},
	}
	for _, tc := range cases {
		s.T().Run(tc.name, func(t *testing.T) {
			cut := s.getCUT()
			domain := fmt.Sprintf("live-aws-test-%08d.%s", rand.Intn(100000000), s.rootDomain)
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
		})
	}
}

func (s *LiveAWSTestSuite) TestDeleteOfNonExistentNS() {
	cases := []struct {
		name         string
		deleteValues []string
	}{
		{
			name:         "known values",
			deleteValues: []string{"test-value"},
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

func (s *LiveAWSTestSuite) getCUT() *awsQuery {
	return &awsQuery{
		getAWSClient: func() (awsclient.Client, error) {
			return awsclient.NewClient(nil, "", "", "us-east-1")
		},
	}
}
