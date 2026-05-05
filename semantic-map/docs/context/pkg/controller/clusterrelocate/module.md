# Module atlas

## Responsibility

Provides a controller that relocates ClusterDeployments from one Hive instance to another. When a ClusterRelocate CR matches a ClusterDeployment (via label selector), the controller copies the CD and all associated resources (Secrets, ConfigMaps, MachinePools, SyncSets, SyncIdentityProviders) to the target Hive instance specified in the ClusterRelocate's kubeconfig secret. After successful relocation, the CD is deleted from the source instance.

## Public Interface/API

- `Add` -- Creates the controller and adds it to the Manager.
- `ControllerName` -- Constant referencing `hivev1.ClusterRelocateControllerName`.
- `ReconcileClusterRelocate` -- Reconciler struct with remote client builder.
- `ReconcileClusterRelocate.Reconcile` -- Reconciles ClusterDeployments, matching them against ClusterRelocate CRs and performing relocation.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment, ClusterRelocate, MachinePool, SyncSet, SyncIdentityProvider CRs.
- `github.com/openshift/hive/pkg/constants` -- Label keys, annotation names.
- `github.com/openshift/hive/pkg/remoteclient` -- Builds clients for the target Hive instance.
- `github.com/openshift/hive/pkg/controller/utils` -- Conditions, relocate annotations, controller config.
- `github.com/openshift/hive/pkg/controller/metrics` -- Reconcile time observation.

## Capabilities

- Reconciles: **ClusterDeployment** (matched by ClusterRelocate label selectors).
- Watches: ClusterDeployment, ClusterRelocate (mapped to matching CDs).
- Conditions set: `RelocationFailedCondition`.
- Key logic: Finds matching ClusterRelocate for each CD, copies CD + associated resources (Secrets, ConfigMaps, MachinePools, SyncSets, SyncIdentityProviders) to target Hive instance, then deletes the CD from the source. Ignores ConfigMaps injected by kube-controller-manager (`kube-root-ca.crt`, `openshift-service-ca.crt`).
- Metrics: `hive_cluster_relocations` counter, `hive_aborted_cluster_relocations` counter.

## Understanding Score

0.80
