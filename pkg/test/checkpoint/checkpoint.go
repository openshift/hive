package checkpoint

import (
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/test/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// Generic allows common functions applicable to all objects to be used as Options to Build
func Generic(opt generic.Option) Option {
	return func(checkpoint *hivev1.Checkpoint) {
		opt(checkpoint)
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
