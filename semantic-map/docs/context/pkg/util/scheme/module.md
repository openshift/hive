# Module atlas

## Responsibility

Provides a singleton `runtime.Scheme` pre-registered with all API types used by Hive (hive CRDs, Kubernetes core, OpenShift, Velero, Prometheus, cluster-autoscaler, etc.). Initialized once at package init time to avoid repeated scheme setup across the codebase.

## Public Interface/API

- `GetScheme() *runtime.Scheme` -- returns the shared, pre-populated Scheme

## Internal Dependencies

- `github.com/openshift/hive/apis` -- hive API scheme registration
- `github.com/openshift/hive/apis/hive/v1` -- hivev1
- `github.com/openshift/hive/apis/hivecontracts/v1alpha1` -- hive contracts
- `github.com/openshift/hive/apis/hiveinternal/v1alpha1` -- hive internal APIs
- `github.com/heptio/velero/pkg/apis/velero/v1` -- Velero backup types
- `github.com/openshift/api/apps/v1`, `authorization/v1`, `config/v1`, `machine/v1alpha1`, `machine/v1beta1`, `operator/v1`, `route/v1` -- OpenShift API types
- `github.com/openshift/cluster-autoscaler-operator/pkg/apis/autoscaling/v1`, `v1beta1` -- cluster autoscaler
- `github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1` -- Prometheus monitoring
- `k8s.io/api/admission/v1`, `admissionregistration/v1`, `apps/v1`, `batch/v1`, `core/v1`, `rbac/v1` -- Kubernetes core types
- `k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1` -- CRD types
- `k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1` -- cluster registry
- `k8s.io/kube-aggregator/pkg/apis/apiregistration/v1` -- API service registration

## Capabilities

- Centralized scheme registration for all Hive-relevant API types
- Singleton pattern avoids redundant scheme setup
- Includes OpenShift, Kubernetes, Velero, Prometheus, and hive-specific types

## Understanding Score

0.85
