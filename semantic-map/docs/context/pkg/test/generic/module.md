# Module atlas

## Responsibility

Foundation package for all Hive test builders. Defines the generic `Option` type that operates on `hivev1.MetaRuntimeObject` (the intersection of metav1.Object and runtime.Object), providing reusable metadata-manipulation options that are adapted by each resource-specific builder via their `Generic()` function.

## Public Interface/API

- `type Option func(hivev1.MetaRuntimeObject)` -- generic functional option operating on any Hive object
- **Name/Namespace**: `WithName(name)`, `WithNamePostfix(postfix)`, `WithNamespace(namespace)`
- **Annotations**: `WithAnnotationsPopulated()`, `WithAnnotation(key, value)`
- **Labels**: `WithLabel(key, value)`
- **Owner references**: `WithControllerOwnerReference(owner)`, `WithOwnerReference(owner)`
- **Resource version**: `WithResourceVersion(rv)`, `WithIncrementedResourceVersion()`
- **Identity**: `WithUID(uid)`, `WithTypeMeta(typers...)`
- **Finalizers**: `WithFinalizer(finalizer)`, `WithoutFinalizer(finalizer)`
- **Timestamps**: `WithCreationTimestamp(time)`, `Deleted()`
- **Other**: `WithGeneration(generation)`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- MetaRuntimeObject interface
- `github.com/openshift/hive/pkg/util/labels` -- AddLabel utility
- `github.com/openshift/hive/pkg/util/scheme` -- default scheme for TypeMeta resolution
- `github.com/openshift/library-go/pkg/controller` -- EnsureOwnerRef
- `k8s.io/apimachinery/pkg/apis/meta/v1`, `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/apimachinery/pkg/types`, `k8s.io/apimachinery/pkg/util/sets`
- `sigs.k8s.io/controller-runtime/pkg/controller/controllerutil`
- `strconv`, `time`

## Capabilities

- **Package**: `generic`
- Core dependency for all other `pkg/test/*` builder packages
- Every builder's `Generic(opt)` function wraps these options to work with the specific resource type

## Understanding Score

0.9
