# Module atlas

## Responsibility

Relocates ClusterDeployments from one Hive instance to another by copying cluster resources (secrets, configmaps, machine pools, syncsets, identity providers) to a remote Hive instance based on ClusterRelocate label selectors.

## Public Interface/API

- `ControllerName` — constant (from `hivev1.ClusterRelocateControllerName`)
- `Add(mgr manager.Manager) error` — creates and registers the controller with the manager
- `ReconcileClusterRelocate` — reconciler struct embedding `client.Client`
- `ReconcileClusterRelocate.Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error)`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` — ClusterDeployment, ClusterRelocate, MachinePool, SyncSet, SyncIdentityProvider CRDs
- `github.com/openshift/hive/pkg/constants` — constants
- `github.com/openshift/hive/pkg/controller/metrics` — reconcile observer
- `github.com/openshift/hive/pkg/controller/utils` — controller config, client wrappers, relocating helpers
- `github.com/openshift/hive/pkg/remoteclient` — remote Hive client builder from kubeconfig
- `sigs.k8s.io/controller-runtime` — controller, reconcile, manager, client, metrics

## Capabilities

- Watches ClusterDeployment and ClusterRelocate resources
- Matches ClusterDeployments to ClusterRelocate resources via label selectors
- Copies Secrets, ConfigMaps, MachinePools, SyncSets, and SyncIdentityProviders to the remote Hive instance
- Ignores system ConfigMaps (`kube-root-ca.crt`, `openshift-service-ca.crt`) during copy
- Sets `RelocationFailed` condition on ClusterDeployment
- Emits `hive_cluster_relocations` and `hive_aborted_cluster_relocations` counter metrics

## Understanding Score

0.85
