package dnsendpoint

import (
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/hive/pkg/controller/dnsendpoint/nameserver"
)

const (
	defaultScrapePeriod = 2 * time.Hour
)

type endpointState struct {
	objectKey client.ObjectKey
	nsValues  sets.String
}

type nameServersMap map[string]endpointState

type rootDomainsMap map[string]nameServersMap

type nameServerScraper struct {
	logger          log.FieldLogger
	mux             sync.Mutex
	scrapePeriod    time.Duration
	queue           workqueue.RateLimitingInterface
	nameServers     rootDomainsMap
	nameServerQuery nameserver.Query
	notifyChange    func(client.ObjectKey)
}

func newNameServerScraper(logger log.FieldLogger, nameServerQuery nameserver.Query, domains []string, notifyChange func(client.ObjectKey)) *nameServerScraper {
	if len(domains) == 0 {
		return nil
	}
	queue := workqueue.NewNamedRateLimitingQueue(workqueue.NewItemExponentialFailureRateLimiter(10*time.Second, 1*time.Hour), "nameServerScraper")
	nameServers := make(rootDomainsMap, len(domains))
	for _, domain := range domains {
		queue.Add(domain)
		nameServers[domain] = nil
	}
	return &nameServerScraper{
		logger:          logger.WithField("scraper", "nameServer"),
		scrapePeriod:    defaultScrapePeriod,
		queue:           queue,
		nameServers:     nameServers,
		nameServerQuery: nameServerQuery,
		notifyChange:    notifyChange,
	}
}

// GetEndpoint gets the name servers for the specified domain.
func (s *nameServerScraper) GetEndpoint(domain string) (rootDomain string, nameServers sets.String) {
	s.mux.Lock()
	defer s.mux.Unlock()
	rootDomain, nsMap := s.rootDomainNameServers(domain)
	return rootDomain, nsMap[domain].nsValues
}

// AddEndpoint adds an endpoint with the specified domain.
func (s *nameServerScraper) AddEndpoint(objectKey client.ObjectKey, domain string, nameServers sets.String) {
	s.mux.Lock()
	defer s.mux.Unlock()
	_, nsMap := s.rootDomainNameServers(domain)
	if nsMap == nil {
		return
	}
	nsMap[domain] = endpointState{
		objectKey: objectKey,
		nsValues:  nameServers,
	}
}

// RemoveEndpoint removes the endpoint with the specified domain.
func (s *nameServerScraper) RemoveEndpoint(domain string) {
	s.mux.Lock()
	defer s.mux.Unlock()
	_, nsMap := s.rootDomainNameServers(domain)
	delete(nsMap, domain)
}

func (s *nameServerScraper) HasBeenScraped(domain string) bool {
	s.mux.Lock()
	defer s.mux.Unlock()
	_, nsMap := s.rootDomainNameServers(domain)
	return nsMap != nil
}

// Start starts the name server scraper.
func (s *nameServerScraper) Start(stop <-chan struct{}) error {
	defer s.queue.ShutDown()
	go func() {
		for {
			obj, shutdown := s.queue.Get()
			if shutdown {
				return
			}
			func() {
				defer s.queue.Done(obj)
				domain, ok := obj.(string)
				if !ok {
					s.logger.WithField("obj", obj).Error("queued object is not a string")
					s.queue.Forget(obj)
					return
				}
				if err := s.scrape(domain); err == nil {
					s.logger.WithField("domain", domain).Info("scrape name servers for domain")
					s.queue.Forget(obj)
					s.queue.AddAfter(domain, s.scrapePeriod)
				} else {
					s.logger.WithField("domain", domain).WithError(err).Error("failed to scrape name servers for domain")
					s.queue.AddRateLimited(obj)
				}
			}()
		}
	}()
	<-stop
	return nil
}

func (s *nameServerScraper) scrape(rootDomain string) error {
	currentNameServers, err := s.nameServerQuery.Get(rootDomain)
	if err != nil {
		return errors.Wrap(err, "error querying name servers")
	}
	changedEndpoints := []client.ObjectKey{}
	func() {
		s.mux.Lock()
		defer s.mux.Unlock()
		oldNameServers, ok := s.nameServers[rootDomain]
		if !ok {
			s.logger.WithField("domain", rootDomain).Error("domain is not a root domain")
			return
		}
		if oldNameServers == nil {
			oldNameServers = nameServersMap{}
			s.nameServers[rootDomain] = oldNameServers
		}
		for domain, oldNameServer := range oldNameServers {
			currentNameServer, ok := currentNameServers[domain]
			if !ok || !currentNameServer.Equal(oldNameServer.nsValues) {
				changedEndpoints = append(changedEndpoints, oldNameServer.objectKey)
				oldNameServer.nsValues = currentNameServer
				oldNameServers[domain] = oldNameServer
			}
		}
	}()
	for _, e := range changedEndpoints {
		s.notifyChange(e)
	}
	return nil
}

func (s *nameServerScraper) rootDomainNameServers(domain string) (string, nameServersMap) {
	for root, nsMap := range s.nameServers {
		if strings.HasSuffix(domain, root) {
			return root, nsMap
		}
	}
	return "", nil
}
