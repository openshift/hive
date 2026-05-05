# Module atlas

## Responsibility

Test builder for `hivev1.ClusterRelocate` resources. Provides options for setting the kubeconfig secret reference and cluster deployment label selector.

## Public Interface/API

- `type Option func(*hivev1.ClusterRelocate)` -- functional option type
- `Build(opts ...Option) *hivev1.ClusterRelocate` -- constructs by applying options
- `type Builder interface` -- fluent builder with `Build`, `Options`, `GenericOptions`
- `BasicBuilder() Builder` / `FullBuilder(name, typer) Builder` -- note: cluster-scoped (no namespace)
- `Generic(opt) Option` -- adapts a `generic.Option`
- `WithKubeconfigSecret(namespace, name) Option` -- sets `Spec.KubeconfigSecretRef`
- `WithClusterDeploymentSelector(key, value) Option` -- sets `Spec.ClusterDeploymentSelector` with a single MatchLabels entry

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/test/generic`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/runtime`

## Capabilities

- **Package**: `clusterrelocate`
- Standard Hive test builder pattern; minimal option set

## Understanding Score

0.9
