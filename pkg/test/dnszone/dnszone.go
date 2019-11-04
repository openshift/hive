package dnszone

import (
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/test/generic"
)

// Option defines a function signature for any function that wants to be passed into Build
type Option func(*hivev1.DNSZone)

// Build runs each of the functions passed in to generate the object.
func Build(opts ...Option) *hivev1.DNSZone {
	retval := &hivev1.DNSZone{}
	for _, o := range opts {
		o(retval)
	}

	return retval
}

// Generic allows common functions applicable to all objects to be used as Options to Build
func Generic(opt generic.Option) Option {
	return func(dnsZone *hivev1.DNSZone) {
		opt(dnsZone)
	}
}
