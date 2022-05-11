package dnsendpoint

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/controller/dnsendpoint/nameserver"
)

const (
	defaultScrapePeriod = 2 * time.Hour
)

type endpointState struct {
	dnsZone  *hivev1.DNSZone
	nsValues sets.String
}

// A map, keyed by subdomain name, of endpointState objects
type endpointsBySubdomain map[string]endpointState

// A map, keyed by root domain name, of maps of subdomain names to their endpointStates, e.g.
// {
// 		"p1.openshiftapps.com": {
// 			"abcd.p1.openshiftapps.com": {
// 				dnsZone: $dnsZone,
// 				nsValues: {
//  				"ns-124.awsdns-15.com.",
//  				...
// 				}
// 			}
//  		"wxyz.p1.openshiftapps.com": { ... }
// 		}
//  	"p2.openshiftapps.com": { ... }
// }
// This is used as a cache with the intent of reducing the number of queries to the cloud provider.
type rootDomainsMap map[string]endpointsBySubdomain

// Every `defaultScrapePeriod` the scraper queries the cloud provider for the definitive list of
// name servers configured in the root domain's hosted zone for each subdomain. It updates the
// `rootDomainsMap` cache for each subdomain it knows about. If the list of name servers has
// changed for a given subdomain (this should generally never happen unless the user did it
// manually, which they should not do), the scraper enqueues the DNSZone to be reconciled again
// by the dnsendpoint controller, which will effectively revert the changes.
// Root domains come from HiveConfig.spec.managedDomains.domains[] and are seeded into the cache
// when the scraper is created.
// Subdomains come from DNSZone.spec.zone and are seeded into the cache by the dnsendpoint
// controller. Thus, the cache is limited only to the subdomains managed by this controller (hive
// instance), as other hives may be managing DNSZones in the same root domain.
// The source of truth for subdomain NS entries is DNSZone.status.nameServers[]. The dnsendpoint
// controller is responsible for curating the subdomain NS entries in the root hosted zone based on
// that list. When it does so, it updates the cache to avoid an extra reconcile.
type nameServerScraper struct {
	logger          log.FieldLogger
	mux             sync.Mutex
	scrapePeriod    time.Duration
	queue           workqueue.RateLimitingInterface
	rootDomainsMap  rootDomainsMap
	nameServerQuery nameserver.Query
	notifyChange    func(client.Object)
}

func newNameServerScraper(logger log.FieldLogger, nameServerQuery nameserver.Query, rootDomains []string, notifyChange func(client.Object)) *nameServerScraper {
	if len(rootDomains) == 0 {
		return nil
	}
	queue := workqueue.NewNamedRateLimitingQueue(workqueue.NewItemExponentialFailureRateLimiter(10*time.Second, 1*time.Hour), "nameServerScraper")
	rootDomainsMap := make(rootDomainsMap, len(rootDomains))
	for _, rootDomain := range rootDomains {
		queue.Add(rootDomain)
		rootDomainsMap[rootDomain] = nil
	}
	return &nameServerScraper{
		logger:       logger.WithField("scraper", "nameServer"),
		scrapePeriod: defaultScrapePeriod,
		// queue expects *root* domain names
		queue:           queue,
		rootDomainsMap:  rootDomainsMap,
		nameServerQuery: nameServerQuery,
		notifyChange:    notifyChange,
	}
}

// GetEndpoint gets the root domain name and the name servers for the specified subdomain.
func (s *nameServerScraper) GetEndpoint(subdomain string) (rootDomain string, nameServers sets.String) {
	s.mux.Lock()
	defer s.mux.Unlock()
	rootDomain, nsMap := s.rootDomainNameServers(subdomain)
	return rootDomain, nsMap[subdomain].nsValues
}

// SyncEndpoint adds or updates the subdomain's endpointState in the scraper's cache
func (s *nameServerScraper) SyncEndpoint(dnsZone *hivev1.DNSZone, subdomain string, nameServers sets.String) {
	s.mux.Lock()
	defer s.mux.Unlock()
	_, nsMap := s.rootDomainNameServers(subdomain)
	if nsMap == nil {
		return
	}
	nsMap[subdomain] = endpointState{
		dnsZone:  dnsZone,
		nsValues: nameServers,
	}
}

// RemoveEndpoint removes the endpoint with the specified domain.
func (s *nameServerScraper) RemoveEndpoint(subdomain string) {
	s.mux.Lock()
	defer s.mux.Unlock()
	_, nsMap := s.rootDomainNameServers(subdomain)
	delete(nsMap, subdomain)
}

func (s *nameServerScraper) HasBeenScraped(subdomain string) bool {
	s.mux.Lock()
	defer s.mux.Unlock()
	_, nsMap := s.rootDomainNameServers(subdomain)
	return nsMap != nil
}

// Start starts the name server scraper.
func (s *nameServerScraper) Start(ctx context.Context) error {
	defer s.queue.ShutDown()
	go func() {
		for {
			obj, shutdown := s.queue.Get()
			if shutdown {
				return
			}
			func() {
				defer s.queue.Done(obj)
				rootDomain, ok := obj.(string)
				if !ok {
					s.logger.WithField("obj", obj).Error("queued object is not a string")
					s.queue.Forget(obj)
					return
				}
				if err := s.scrape(rootDomain); err == nil {
					s.logger.WithField("domain", rootDomain).Info("scrape name servers for root domain")
					s.queue.Forget(obj)
					s.queue.AddAfter(rootDomain, s.scrapePeriod)
				} else {
					s.logger.WithField("domain", rootDomain).WithError(err).Error("failed to scrape name servers for root domain")
					s.queue.AddRateLimited(obj)
				}
			}()
		}
	}()
	<-ctx.Done()
	return nil
}

func (s *nameServerScraper) scrape(rootDomain string) error {
	currentNameServerMap, err := s.nameServerQuery.Get(rootDomain)
	if err != nil {
		return errors.Wrap(err, "error querying name servers")
	}
	changedDNSZones := []client.Object{}
	func() {
		s.mux.Lock()
		defer s.mux.Unlock()
		oldNSMap, ok := s.rootDomainsMap[rootDomain]
		if !ok {
			s.logger.WithField("domain", rootDomain).Error("domain is not a root domain")
			return
		}
		if oldNSMap == nil {
			oldNSMap = endpointsBySubdomain{}
			s.rootDomainsMap[rootDomain] = oldNSMap
		}
		// Sync our cache with NS entries for each subdomain that has already been seeded. If
		// anything has changed, update the cache based on what came from the cloud provider
		// and (set up to) requeue any affected DNSZone objects to the dnsendpoint controller.
		for subdomain, oldEndpoints := range oldNSMap {
			currentNameServers, ok := currentNameServerMap[subdomain]
			if !ok || !currentNameServers.Equal(oldEndpoints.nsValues) {
				changedDNSZones = append(changedDNSZones, oldEndpoints.dnsZone)
				oldEndpoints.nsValues = currentNameServers
				oldNSMap[subdomain] = oldEndpoints
			}
		}
	}()
	for _, changedDNSZone := range changedDNSZones {
		s.notifyChange(changedDNSZone)
	}
	return nil
}

func (s *nameServerScraper) rootDomainNameServers(domain string) (string, endpointsBySubdomain) {
	for root, nsMap := range s.rootDomainsMap {
		if strings.HasSuffix(domain, root) {
			return root, nsMap
		}
	}
	return "", nil
}
