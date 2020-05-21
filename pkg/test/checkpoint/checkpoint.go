package checkpoint

import (
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/test/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Option defines a function signature for any function that wants to be passed into Build
type Option func(*hivev1.Checkpoint)

// Build runs each of the functions passed in to generate the object.
func Build(opts ...Option) *hivev1.Checkpoint {
	retval := &hivev1.Checkpoint{}
	for _, o := range opts {
		o(retval)
	}

	return retval
}

type Builder interface {
	Build(opts ...Option) *hivev1.Checkpoint

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

func (b *builder) Build(opts ...Option) *hivev1.Checkpoint {
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
	return func(checkpoint *hivev1.Checkpoint) {
		opt(checkpoint)
	}
}

// WithResourceVersion sets the specified resource version on the supplied object.
func WithResourceVersion(resourceVersion string) Option {
	return Generic(generic.WithResourceVersion(resourceVersion))
}

// WithTypeMeta populates the type meta for the object.
func WithTypeMeta() Option {
	return func(checkpoint *hivev1.Checkpoint) {
		checkpoint.APIVersion = hivev1.SchemeGroupVersion.String()
		checkpoint.Kind = "Checkpoint"
	}
}

// WithLastBackupChecksum sets the checksum for all Hive objects backed up in the last backup object.
func WithLastBackupChecksum(lastBackupChecksum string) Option {
	return func(checkpoint *hivev1.Checkpoint) {
		checkpoint.Spec.LastBackupChecksum = lastBackupChecksum
	}
}

// WithLastBackupTime sets the last time a backup object was created.
func WithLastBackupTime(lastBackupTime metav1.Time) Option {
	return func(checkpoint *hivev1.Checkpoint) {
		checkpoint.Spec.LastBackupTime = lastBackupTime
	}
}

// WithLastBackupRef sets the name of the last backup object created.
func WithLastBackupRef(lastBackupRef hivev1.BackupReference) Option {
	return func(checkpoint *hivev1.Checkpoint) {
		checkpoint.Spec.LastBackupRef = lastBackupRef
	}
}
