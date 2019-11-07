package nameserver

import (
	"k8s.io/apimachinery/pkg/util/sets"
)

//go:generate mockgen -source=./query.go -destination=./mock/query_generated.go -package=mock

// Query is used to perform queries for name servers.
type Query interface {
	// Get the name servers under the specified root domain.
	Get(rootDomain string) (map[string]sets.String, error)

	// Create name servers for the specified domain under the specified root domain.
	Create(rootDomain string, domain string, values sets.String) error

	// Delete the name servers for the specified domain under the specified root domain.
	// If specified values of the name servers only serve as guidance for what to delete.
	// If there are other name servers for the specified domain server, those will be
	// deleted as well.
	Delete(rootDomain string, domain string, values sets.String) error
}
