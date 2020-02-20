package persistentvolumeclaim

import (
	"github.com/openshift/hive/pkg/test/generic"
	corev1 "k8s.io/api/core/v1"
)

// Option defines a function signature for any function that wants to be passed into Build
type Option func(*corev1.PersistentVolumeClaim)

// Build runs each of the functions passed in to generate the object.
func Build(opts ...Option) *corev1.PersistentVolumeClaim {
	retval := &corev1.PersistentVolumeClaim{}
	for _, o := range opts {
		o(retval)
	}

	return retval
}

// Generic allows common functions applicable to all objects to be used as Options to Build
func Generic(opt generic.Option) Option {
	return func(checkpoint *corev1.PersistentVolumeClaim) {
		opt(checkpoint)
	}
}
