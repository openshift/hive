package nameserver

import (
	"k8s.io/apimachinery/pkg/util/sets"
)

//go:generate mockgen -source=./query.go -destination=./mock/query_generated.go -package=mock

// Query is used to perform queries for name servers.
type Query interface {
	// Get returns a map, keyed by domain name, of sets of name servers configured in the root
	// domain's hosted zone for each domain. Entries are included for the root domain as well as
	// subdomains. For managed DNS, the root domain's hosted zone is created by the user; but the
	// dnsendpoint controller populates it with the NS entries from the status of each DNSZone for
	// the subdomain denoted by that DNSZone's spec.zone.
	Get(rootDomain string) (map[string]sets.Set[string], error)

	// CreateOrUpdate creates or replaces name servers for the specified subdomain under the
	// specified root domain.
	CreateOrUpdate(rootDomain string, domain string, values sets.Set[string]) error

	// Delete the name servers for the specified subdomain under the specified root domain.
	// If specified values of the name servers only serve as guidance for what to delete.
	// If there are other name servers for the specified domain server, those will be
	// deleted as well.
	Delete(rootDomain string, domain string, values sets.Set[string]) error
}
