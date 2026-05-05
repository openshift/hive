# Module atlas

## Responsibility

Test builder for `hivev1.ClusterPool` resources. Provides a rich set of options covering platform setup (AWS, OpenStack), pool sizing, claim lifetimes, image sets, install configuration, running count, inventory/customization, and conditions.

## Public Interface/API

- `type Option func(*hivev1.ClusterPool)` -- functional option type
- `Build(opts ...Option) *hivev1.ClusterPool` -- constructs by applying options
- `type Builder interface` -- fluent builder with `Build`, `Options`, `GenericOptions`
- `BasicBuilder() Builder` / `FullBuilder(namespace, name, typer) Builder`
- `Generic(opt) Option` -- adapts a `generic.Option`
- **Platform**: `ForAWS(credsSecret, region)`, `ForOpenstack(credsSecret)`, `WithPlatform(platform)`
- **Pool config**: `WithSize(n)`, `WithMaxSize(n)`, `WithMaxConcurrent(n)`, `WithRunningCount(n)`
- **References**: `WithPullSecret(name)`, `WithImageSet(name)`, `WithInstallConfigSecretTemplateRef(name)`, `WithCustomizationRef(name)`
- **Install**: `WithInstallAttemptsLimit(n)`, `WithInstallerEnv(envVars)`
- **Claim lifetimes**: `WithDefaultClaimLifetime(d)`, `WithMaximumClaimLifetime(d)`
- **Labels/Conditions**: `WithClusterDeploymentLabels(labels)`, `WithCondition(cond)`
- **Other**: `WithBaseDomain(domain)`, `WithInventory(cdcs)`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` and platform sub-packages (`aws`, `openstack`)
- `github.com/openshift/hive/pkg/test/generic`
- `k8s.io/api/core/v1`, `k8s.io/apimachinery/pkg/apis/meta/v1`, `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/utils/ptr`
- `time`

## Capabilities

- **Package**: `clusterpool`
- Standard Hive test builder pattern

## Understanding Score

0.9
