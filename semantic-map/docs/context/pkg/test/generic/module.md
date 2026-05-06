# Module atlas

## Responsibility

Provides generic functional options that operate on any `hivev1.MetaRuntimeObject`, serving as the shared foundation for all type-specific test builders in `pkg/test/`. Each type-specific builder adapts these options via its own `Generic()` wrapper.

## Public Interface/API

- `type Option func(hivev1.MetaRuntimeObject)` -- generic functional option type operating on any Hive MetaRuntimeObject
- `WithName(name string) Option`
- `WithNamePostfix(postfix string) Option` -- appends "-{postfix}" to existing name
- `WithNamespace(namespace string) Option`
- `WithAnnotationsPopulated() Option` -- ensures Annotations map is non-nil
- `WithAnnotation(key, value string) Option`
- `WithControllerOwnerReference(owner metav1.Object) Option`
- `WithOwnerReference(owner hivev1.MetaRuntimeObject) Option` -- non-controller owner ref with BlockOwnerDeletion=true
- `WithLabel(key, value string) Option`
- `WithResourceVersion(resourceVersion string) Option`
- `WithIncrementedResourceVersion() Option` -- increments numeric resource version by 1
- `WithUID(uid string) Option`
- `WithTypeMeta(typers ...runtime.ObjectTyper) Option` -- resolves and sets GVK from registered scheme
- `WithFinalizer(finalizer string) Option`
- `WithoutFinalizer(finalizer string) Option`
- `WithCreationTimestamp(time time.Time) Option`
- `Deleted() Option` -- sets DeletionTimestamp to now
- `WithGeneration(generation int64) Option`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/util/labels`
- `github.com/openshift/hive/pkg/util/scheme`
- `github.com/openshift/library-go/pkg/controller`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/apimachinery/pkg/types`
- `k8s.io/apimachinery/pkg/util/sets`
- `sigs.k8s.io/controller-runtime/pkg/controller/controllerutil`

## Capabilities

- Common metadata manipulation (name, namespace, labels, annotations, UID, generation)
- Resource version management (set, increment)
- TypeMeta resolution from registered ObjectTyper schemes
- Owner reference management (controller and non-controller)
- Finalizer add/remove
- Creation/deletion timestamp manipulation
- Foundation layer used by all `pkg/test/*` type-specific builders

## Understanding Score

0.9
