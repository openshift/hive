package generic

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Option defines a function signature for any function that wants to be passed into Build
type Option func(metav1.Object)

// WithName sets the object.Name field when building an object with Build.
func WithName(name string) Option {
	return func(meta metav1.Object) {
		meta.SetName(name)
	}
}

// WithNamePostfix appends the string passed in to the object.Name field when building an with Build.
func WithNamePostfix(postfix string) Option {
	return func(meta metav1.Object) {
		name := meta.GetName()
		meta.SetName(name + "-" + postfix)
	}
}

// WithNamespace sets the object.Namespace field when building an object with Build.
func WithNamespace(namespace string) Option {
	return func(meta metav1.Object) {
		meta.SetNamespace(namespace)
	}
}

// WithAnnotationsPopulated ensures that object.Annotations is not nil.
func WithAnnotationsPopulated() Option {
	return func(meta metav1.Object) {
		annotations := meta.GetAnnotations()

		// Only set if Nil (don't wipe out existing)
		if annotations == nil {
			meta.SetAnnotations(map[string]string{})
		}
	}
}
