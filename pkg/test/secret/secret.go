package secret

import (
	"github.com/openshift/hive/pkg/test/generic"
	corev1 "k8s.io/api/core/v1"
)

// Option defines a function signature for any function that wants to be passed into Build
type Option func(*corev1.Secret)

// Build runs each of the functions passed in to generate the object.
func Build(opts ...Option) *corev1.Secret {
	retval := &corev1.Secret{}
	for _, o := range opts {
		o(retval)
	}

	return retval
}

// Generic allows common functions applicable to all objects to be used as Options to Build
func Generic(opt generic.Option) Option {
	return func(obj *corev1.Secret) {
		opt(obj)
	}
}

// WithDataKeyValue adds the key and value to the secret's data section.
func WithDataKeyValue(key string, value []byte) Option {
	return func(obj *corev1.Secret) {
		if obj.Data == nil {
			obj.Data = map[string][]byte{}
		}
		obj.Data[key] = value
	}
}

// WithType sets the secret's type value.
func WithType(t corev1.SecretType) Option {
	return func(obj *corev1.Secret) {
		obj.Type = t
	}
}
