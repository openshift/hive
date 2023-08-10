package generic

import (
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"

	librarygocontroller "github.com/openshift/library-go/pkg/controller"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	k8slabels "github.com/openshift/hive/pkg/util/labels"
	"github.com/openshift/hive/pkg/util/scheme"

	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Option defines a function signature for any function that wants to be passed into Build
type Option func(hivev1.MetaRuntimeObject)

// WithName sets the object.Name field when building an object with Build.
func WithName(name string) Option {
	return func(meta hivev1.MetaRuntimeObject) {
		meta.SetName(name)
	}
}

// WithNamePostfix appends the string passed in to the object.Name field when building an with Build.
func WithNamePostfix(postfix string) Option {
	return func(meta hivev1.MetaRuntimeObject) {
		name := meta.GetName()
		meta.SetName(name + "-" + postfix)
	}
}

// WithNamespace sets the object.Namespace field when building an object with Build.
func WithNamespace(namespace string) Option {
	return func(meta hivev1.MetaRuntimeObject) {
		meta.SetNamespace(namespace)
	}
}

// WithAnnotationsPopulated ensures that object.Annotations is not nil.
func WithAnnotationsPopulated() Option {
	return func(meta hivev1.MetaRuntimeObject) {
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
	return func(meta hivev1.MetaRuntimeObject) {
		annotations := meta.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string, 1)
		}
		annotations[key] = value
		meta.SetAnnotations(annotations)
	}
}

// WithControllerOwnerReference sets the controller owner reference to the supplied object.
func WithControllerOwnerReference(owner metav1.Object) Option {
	return func(meta hivev1.MetaRuntimeObject) {
		controllerutil.SetControllerReference(owner, meta, scheme.GetScheme())
	}
}

// WithOwnerReference sets the owner reference to the supplied object.
// BlockOwnerDeletion is set to true
func WithOwnerReference(owner hivev1.MetaRuntimeObject) Option {
	return func(meta hivev1.MetaRuntimeObject) {
		ownerRef := metav1.NewControllerRef(owner, owner.GetObjectKind().GroupVersionKind())
		ownerRef.Controller = nil
		librarygocontroller.EnsureOwnerRef(meta, *ownerRef)
	}
}

// WithLabel sets the specified label on the supplied object.
func WithLabel(key, value string) Option {
	return func(meta hivev1.MetaRuntimeObject) {
		labels := meta.GetLabels()
		labels = k8slabels.AddLabel(labels, key, value)
		meta.SetLabels(labels)
	}
}

// WithResourceVersion sets the specified resource version on the supplied object.
func WithResourceVersion(resourceVersion string) Option {
	return func(meta hivev1.MetaRuntimeObject) {
		meta.SetResourceVersion(resourceVersion)
	}
}

// WithIncrementedResourceVersion increments by one the resource version on the supplied object.
// If the resource version is not an integer, then the new resource version will be set to 1.
func WithIncrementedResourceVersion() Option {
	return func(meta hivev1.MetaRuntimeObject) {
		rv, err := strconv.Atoi(meta.GetResourceVersion())
		if err != nil {
			rv = 0
		}
		meta.SetResourceVersion(strconv.Itoa(rv + 1))
	}
}

// WithUID sets the Metadata UID on the supplied object.
func WithUID(uid string) Option {
	return func(meta hivev1.MetaRuntimeObject) {
		meta.SetUID(types.UID(uid))
	}
}

// WithTypeMeta populates the type meta for the object.
func WithTypeMeta(typers ...runtime.ObjectTyper) Option {
	return func(meta hivev1.MetaRuntimeObject) {
		for _, typer := range append(typers, scheme.GetScheme()) {
			gvks, _, err := typer.ObjectKinds(meta)
			if err != nil {
				continue
			}
			if len(gvks) == 0 {
				continue
			}
			meta.GetObjectKind().SetGroupVersionKind(gvks[0])
			return
		}
	}
}

// WithFinalizer adds the specified finalizer to the object.
func WithFinalizer(finalizer string) Option {
	return func(meta hivev1.MetaRuntimeObject) {
		finalizers := sets.NewString(meta.GetFinalizers()...)
		finalizers.Insert(finalizer)
		meta.SetFinalizers(finalizers.List())
	}
}

// WithoutFinalizer removes the specified finalizer from the object.
func WithoutFinalizer(finalizer string) Option {
	return func(meta hivev1.MetaRuntimeObject) {
		finalizers := sets.NewString(meta.GetFinalizers()...)
		finalizers.Delete(finalizer)
		meta.SetFinalizers(finalizers.List())
	}
}

// WithCreationTimestamp sets the creation timestamp on the object.
func WithCreationTimestamp(time time.Time) Option {
	return func(meta hivev1.MetaRuntimeObject) {
		meta.SetCreationTimestamp(metav1.NewTime(time))
	}
}

// Deleted sets a deletion timestamp on the object.
func Deleted() Option {
	return func(meta hivev1.MetaRuntimeObject) {
		now := metav1.Now()
		meta.SetDeletionTimestamp(&now)
	}
}

func WithGeneration(generation int64) Option {
	return func(meta hivev1.MetaRuntimeObject) {
		meta.SetGeneration(generation)
	}
}
