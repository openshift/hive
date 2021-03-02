package dnszone

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
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

type Builder interface {
	Build(opts ...Option) *hivev1.DNSZone

	Options(opts ...Option) Builder

	GenericOptions(opts ...generic.Option) Builder
}

func BasicBuilder() Builder {
	return &builder{}
}

func FullBuilder(namespace, name string, typer runtime.ObjectTyper) Builder {
	b := &builder{}
	return b.GenericOptions(
		generic.WithTypeMeta(typer),
		generic.WithResourceVersion("1"),
		generic.WithNamespace(namespace),
		generic.WithName(name),
	)
}

type builder struct {
	options []Option
}

func (b *builder) Build(opts ...Option) *hivev1.DNSZone {
	return Build(append(b.options, opts...)...)
}

func (b *builder) Options(opts ...Option) Builder {
	return &builder{
		options: append(b.options, opts...),
	}
}

func (b *builder) GenericOptions(opts ...generic.Option) Builder {
	options := make([]Option, len(opts))
	for i, o := range opts {
		options[i] = Generic(o)
	}
	return b.Options(options...)
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
func WithTypeMeta(typers ...runtime.ObjectTyper) Option {
	return Generic(generic.WithTypeMeta(typers...))
}

// WithControllerOwnerReference sets the controller owner reference to the supplied object.
func WithControllerOwnerReference(owner metav1.Object) Option {
	return Generic(generic.WithControllerOwnerReference(owner))
}

// WithOwnerReference sets the owner reference to the supplied object.
func WithOwnerReference(owner hivev1.MetaRuntimeObject) Option {
	return Generic(generic.WithOwnerReference(owner))
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

// WithCondition adds the specified condition to the DNSZone
func WithCondition(cond hivev1.DNSZoneCondition) Option {
	return func(dnsZone *hivev1.DNSZone) {
		for i, c := range dnsZone.Status.Conditions {
			if c.Type == cond.Type {
				dnsZone.Status.Conditions[i] = cond
				return
			}
		}
		dnsZone.Status.Conditions = append(dnsZone.Status.Conditions, cond)
	}
}

func WithZone(zone string) Option {
	return func(dnsZone *hivev1.DNSZone) {
		dnsZone.Spec.Zone = zone
	}
}

// WithGCPPlatform will set the GCP spce and status fields non-nil and populate
// the status fields with the provided name for the zone.
func WithGCPPlatform(zoneName string) Option {
	return func(dnsZone *hivev1.DNSZone) {
		dnsZone.Spec.GCP = &hivev1.GCPDNSZoneSpec{}
		dnsZone.Status.GCP = &hivev1.GCPDNSZoneStatus{
			ZoneName: &zoneName,
		}
	}
}
