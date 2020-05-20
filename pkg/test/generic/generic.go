package generic

import (
	k8slabels "github.com/openshift/hive/pkg/util/labels"

	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
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

// WithAnnotation adds an annotation with the specified key and value to the supplied object.
// If there is already an annotation with the specified key, it will be replaced.
func WithAnnotation(key, value string) Option {
	return func(meta metav1.Object) {
		annotations := meta.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string, 1)
		}
		annotations[key] = value
		meta.SetAnnotations(annotations)
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

// WithResourceVersion sets the specified resource version on the supplied object.
func WithResourceVersion(resourceVersion string) Option {
	return func(meta metav1.Object) {
		meta.SetResourceVersion(resourceVersion)
	}
}

// WithIncrementedResourceVersion increments by one the resource version on the supplied object.
// If the resource version is not an integer, then the new resource version will be set to 1.
func WithIncrementedResourceVersion() Option {
	return func(meta metav1.Object) {
		rv, err := strconv.Atoi(meta.GetResourceVersion())
		if err != nil {
			rv = 0
		}
		meta.SetResourceVersion(strconv.Itoa(rv + 1))
	}
}

// WithUID sets the Metadata UID on the supplied object.
func WithUID(uid string) Option {
	return func(meta metav1.Object) {
		meta.SetUID(types.UID(uid))
	}
}
