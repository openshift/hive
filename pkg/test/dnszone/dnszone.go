package dnszone

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
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

// WithResourceVersion sets the specified resource version on the supplied object.
func WithResourceVersion(resourceVersion string) Option {
	return Generic(generic.WithResourceVersion(resourceVersion))
}

// WithIncrementedResourceVersion increments by one the resource version on the supplied object.
// If the resource version is not an integer, then the new resource version will be set to 1.
func WithIncrementedResourceVersion() Option {
	return Generic(generic.WithIncrementedResourceVersion())
}

// WithTypeMeta populates the type meta for the object.
func WithTypeMeta() Option {
	return func(dnsZone *hivev1.DNSZone) {
		dnsZone.APIVersion = hivev1.SchemeGroupVersion.String()
		dnsZone.Kind = "DNSZone"
	}
}

// WithControllerOwnerReference sets the owner reference to the supplied object.
func WithControllerOwnerReference(owner metav1.Object) Option {
	return Generic(generic.WithControllerOwnerReference(owner))
}

// WithLabelOwner sets the labels so that the specific ClusterDeployment is the labeled as the owner.
func WithLabelOwner(owner *hivev1.ClusterDeployment) Option {
	return func(dnsZone *hivev1.DNSZone) {
		if dnsZone.Labels == nil {
			dnsZone.Labels = make(map[string]string, 2)
		}
		dnsZone.Labels[constants.ClusterDeploymentNameLabel] = owner.Name
		dnsZone.Labels[constants.DNSZoneTypeLabel] = constants.DNSZoneTypeChild
	}
}
