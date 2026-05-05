# Module atlas

## Responsibility

Provides a **singleton `runtime.Scheme`** pre-registered with every API type that Hive needs across its controllers, webhooks, and utilities. The scheme is built once during package `init()` and shared via `GetScheme()`, removing the need for each consumer to independently construct and register types. This includes all Hive CRD types, core Kubernetes types (apps, batch, core, RBAC, admission, admissionregistration, apiextensions, apiregistration), OpenShift types (apps, authorization, config, machine, operator/ingress, route), cluster-autoscaler-operator types, Velero backup types, Prometheus monitoring types, and cluster-registry types.

## Public Interface/API

- `GetScheme() *runtime.Scheme` -- returns the package-level singleton scheme containing all registered types.

## Internal Dependencies

Kubernetes core types:
- `k8s.io/api/admission/v1`, `k8s.io/api/admissionregistration/v1`, `k8s.io/api/apps/v1`, `k8s.io/api/batch/v1`, `k8s.io/api/core/v1`, `k8s.io/api/rbac/v1`
- `k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1`
- `k8s.io/kube-aggregator/pkg/apis/apiregistration/v1`

OpenShift types:
- `github.com/openshift/api/apps/v1`, `github.com/openshift/api/authorization/v1`, `github.com/openshift/api/config/v1`, `github.com/openshift/api/machine/v1alpha1`, `github.com/openshift/api/machine/v1beta1`, `github.com/openshift/api/operator/v1`, `github.com/openshift/api/route/v1`
- `github.com/openshift/cluster-autoscaler-operator/pkg/apis/autoscaling/v1`, `github.com/openshift/cluster-autoscaler-operator/pkg/apis/autoscaling/v1beta1`

Hive API types:
- `github.com/openshift/hive/apis` (bulk registration), `github.com/openshift/hive/apis/hive/v1`, `github.com/openshift/hive/apis/hivecontracts/v1alpha1`, `github.com/openshift/hive/apis/hiveinternal/v1alpha1`

Third-party:
- `github.com/heptio/velero/pkg/apis/velero/v1`
- `github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1`

## Capabilities

- **`package`** name(s): **scheme**.
- Go **`import`** edges listed below (25 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/util/scheme`.
- Single file (`scheme.go`), no tests. Note: `machinev1alpha1.OpenstackProviderSpec` is registered manually due to a known issue with `machinev1alpha1.Install()`.

## Understanding Score

0.85
