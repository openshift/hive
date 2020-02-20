package generic

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	k8slabels "k8s.io/kubernetes/pkg/util/labels"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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

// WithControllerOwnerReference sets the owner reference to the supplied object.
func WithControllerOwnerReference(owner metav1.Object) Option {
	return func(meta metav1.Object) {
		controllerutil.SetControllerReference(owner, meta, scheme.Scheme)
	}
}

// WithLabel sets the specified label on the supplied object.
func WithLabel(key, value string) Option {
	return func(meta metav1.Object) {
		labels := meta.GetLabels()
		labels = k8slabels.AddLabel(labels, key, value)
		meta.SetLabels(labels)
	}
}
