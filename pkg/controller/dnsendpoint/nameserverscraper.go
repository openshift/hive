package dnsendpoint

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/controller/dnsendpoint/nameserver"
)

const (
	defaultScrapePeriod = 2 * time.Hour
)

var (
	metricSubdomainsScraped = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_managed_dns_subdomains_scraped",
		Help: "The number of subdomains of a root managed domain handled by the name server scraper.",
	}, []string{"managed_domain"})
	metricScrapeDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "hive_managed_dns_scrape_seconds",
		Help:    "How long it took to scrape a root domain.",
		Buckets: []float64{1, 10, 30, 60, 120},
	}, []string{"managed_domain"})
)

func init() {
	metrics.Registry.MustRegister(metricSubdomainsScraped)
	metrics.Registry.MustRegister(metricScrapeDuration)
}

type endpointState struct {
	dnsZone  *hivev1.DNSZone
	nsValues sets.Set[string]
}

// A map, keyed by subdomain name, of endpointState objects
type endpointsBySubdomain map[string]endpointState

type rootDomainsInfo struct {
	scraped              bool
	endpointsBySubdomain endpointsBySubdomain
}

// A map, keyed by root domain name, of information about root domains.
// The information about a root domain is:
// - A bool indicating whether the scraper has scraped that root domain yet.
// - A map, keyed by subdomain name, of information about each subdomain.
//
// The subdomain information is:
// - A pointer to a DNSZone. This is populated by the dnsendpoint controller iff it has ever
// reconciled that DNSZone (and the DNSZone hasn't been deleted).
//
// - A set of strings representing the name servers the scraper has discovered for the subdomain
// in the root domain's hosted zone. This is populated by the scraper; and updated by the
// dnsendpoint controller iff it decides the name servers for the subdomain need to be synced
// after inspecting the corresponding DNSZone.
//
//	{
//	   "p1.openshiftapps.com": {
//	      scraped: true
//	      endpoinsBySubdomain: {
//	         "abcd.p1.openshiftapps.com": {
//	            dnsZone: $dnsZone,
//	            nsValues: {
//	               "ns-124.awsdns-15.com.",
//	               ...
//	            }
//	         }
//	         "wxyz.p1.openshiftapps.com": { ... }
//	      }
//	   }
//	   "p2.openshiftapps.com": { ... }
//	}
//
// This is used as a cache with the intent of reducing the number of queries to the cloud provider.
//
// On startup, if the dnsendpoint controller reconciles a DNSZone before its corresponding root
// domain has been scraped, it seeds the cache with the DNSZone and an empty nsValues set, then
// returns without requeueing.
//   - If the scraper encounters entries for that subdomain, it updates the cache and enqueues that
//     DNSZone to the dnsendpoint controller.
//   - If the scraper does not see the subdomain at all, this indicates that the dnsendpoint
//     controller has not yet configured the root hosted zone for this subdomain, so it enqueues the
//     DNSZone for that case as well.
type rootDomainsMap map[string]*rootDomainsInfo

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
		rootDomainsMap[rootDomain] = &rootDomainsInfo{
			scraped:              false,
			endpointsBySubdomain: endpointsBySubdomain{},
		}
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
func (s *nameServerScraper) GetEndpoint(subdomain string) (rootDomain string, nameServers sets.Set[string]) {
	s.mux.Lock()
	defer s.mux.Unlock()
	rootDomain, nsMap := s.rootDomainNameServers(subdomain)
	nameServers = sets.Set[string]{}
	if nsMap != nil {
		if endpoint, ok := nsMap[subdomain]; ok {
			nameServers = endpoint.nsValues
		}
	}
	return rootDomain, nameServers
}

// CheckSeedScrapeStatus
//   - Ensures the cache has been seeded with this dnsZone. This enables the scraper to enqueue the
//     DNSZone to the dnsendpoint controller if necessary.
//   - Returns whether the scraper has run yet (ever, since this controller started) for the *root* domain.
func (s *nameServerScraper) CheckSeedScrapeStatus(dnsZone *hivev1.DNSZone, rootDomain string) bool {
	s.mux.Lock()
	defer s.mux.Unlock()
	subdomain := dnsZone.Spec.Zone
	rdInfo, ok := s.rootDomainsMap[rootDomain]
	// NOTE: This should be impossible, since (at the time of this writing) the caller has already
	// ensured that the rootDomain is tracked. This is defending against future bug injection.
	if !ok || rdInfo == nil {
		s.logger.WithField("rootDomain", rootDomain).Error("no root domain info found -- this is a bug!")
		// This will cause the caller to return with no requeue
		return false
	}
	// Since eps is a struct (not a pointer),
	// - this is a copyout
	// - eps gets an empty struct if the key doesn't exist in the map
	// so we can use it to initialize if necessary, but we need to copy it back in when done
	eps := rdInfo.endpointsBySubdomain[subdomain]
	eps.dnsZone = dnsZone
	rdInfo.endpointsBySubdomain[subdomain] = eps
	return rdInfo.scraped
}

// SyncEndpoint adds or updates the subdomain's endpointState in the scraper's cache
func (s *nameServerScraper) SyncEndpoint(dnsZone *hivev1.DNSZone, subdomain string, nameServers sets.Set[string]) {
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

// Start starts the name server scraper.
func (s *nameServerScraper) Start(ctx context.Context) error {
	defer s.queue.ShutDown()
	go func() {
		for {
			obj, shutdown := s.queue.Get()
			if shutdown {
				s.logger.Info("scraper queue shutting down")
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
					s.logger.WithField("domain", rootDomain).Info("successfully scraped name servers for root domain")
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
	logger := s.logger.WithField("rootDomain", rootDomain)
	logger.Info("scrape root domain")
	start := time.Now()
	currentNameServerMap, err := s.nameServerQuery.Get(rootDomain)
	if err != nil {
		return errors.Wrap(err, "error querying name servers")
	}
	changedDNSZones := []client.Object{}
	func() {
		s.mux.Lock()
		defer s.mux.Unlock()
		oldRDInfo, ok := s.rootDomainsMap[rootDomain]
		if !ok {
			logger.Error("domain is not a managed root domain")
			return
		}
		oldNSMap := oldRDInfo.endpointsBySubdomain
		// Keep track of subdomains for which we have name servers. We'll use this to find out if
		// there are DNSZones that have a) already been reconciled, and b) not been set up yet in
		// the root hosted zone. Those need to be enqueued explicitly to the dnsendpoint controller.
		seenSubdomains := sets.Set[string]{}
		// Sync our cache with NS entries for each subdomain.
		// NOTE: We are caching *all* the subdomains, not just the ones for DNSZones managed by
		// this controller. This is to mitigate timing issues on controller restart: so the
		// dnsendpoint controller can skip redundant calls to the cloud provider to sync name
		// servers it already has.
		for subdomain, nameServers := range currentNameServerMap {
			seenSubdomains.Insert(subdomain)
			oldEndpoints, ok := oldNSMap[subdomain]
			// If the subdomain existed in the cache and its name servers have changed, set up to
			// enqueue any affected DNSZone objects to the dnsendpoint controller.
			if ok && oldEndpoints.dnsZone != nil && !nameServers.Equal(oldEndpoints.nsValues) {
				changedDNSZones = append(changedDNSZones, oldEndpoints.dnsZone)
			}
			// Populate the cache for this subdomain.
			if !ok {
				oldEndpoints = endpointState{}
			}
			oldEndpoints.nsValues = nameServers
			oldNSMap[subdomain] = oldEndpoints
		}
		// Now go through the cache looking for entries that were seeded by the dnsendpoint
		// controller, but for which no entries were found in the root hosted zone. Those need to
		// be sent explicitly to the dnsendpoint controller, which would otherwise not reconcile
		// them again.
		for subdomain, endpointInfo := range oldNSMap {
			if !seenSubdomains.Has(subdomain) && endpointInfo.dnsZone != nil {
				changedDNSZones = append(changedDNSZones, endpointInfo.dnsZone)
			}
		}
		oldRDInfo.scraped = true
	}()
	for _, changedDNSZone := range changedDNSZones {
		logger.WithField("dnsZone", fmt.Sprintf("%s/%s", changedDNSZone.GetNamespace(), changedDNSZone.GetName())).
			Info("notify dnsendpoint controller of name server change")
		s.notifyChange(changedDNSZone)
	}
	metricScrapeDuration.With(prometheus.Labels{"managed_domain": rootDomain}).Observe(time.Since(start).Seconds())
	metricSubdomainsScraped.With(prometheus.Labels{"managed_domain": rootDomain}).Set((float64)(len(currentNameServerMap)))
	return nil
}

func (s *nameServerScraper) rootDomainNameServers(domain string) (string, endpointsBySubdomain) {
	for root, rdInfo := range s.rootDomainsMap {
		if strings.HasSuffix(domain, root) {
			return root, rdInfo.endpointsBySubdomain
		}
	}
	return "", nil
}
