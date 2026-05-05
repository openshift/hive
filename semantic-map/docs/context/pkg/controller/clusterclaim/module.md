# Module atlas

## Responsibility

Provides a controller that reconciles ClusterClaim objects. It manages the lifecycle of claims against ClusterPools, including: tracking claim assignment to ClusterDeployments, managing claim lifetime (with pool-level defaults and maximums), creating RBAC (Role + RoleBinding) for claim subjects in the cluster namespace, handling assignment conflicts, and cleaning up resources (CD deletion, RBAC removal) when a claim is deleted.

## Public Interface/API

- `Add` -- Creates the controller and adds it to the Manager.
- `AddToManager` -- Adds the controller to a manager.
- `ControllerName` -- Constant referencing `hivev1.ClusterClaimControllerName`.
- `ReconcileClusterClaim` -- Reconciler struct that reconciles ClusterClaim objects.
- `ReconcileClusterClaim.Reconcile` -- Main reconcile loop. Manages claim lifecycle, RBAC, lifetime enforcement, and cleanup.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ClusterClaim, ClusterDeployment, ClusterPool CRs.
- `github.com/openshift/hive/pkg/controller/utils` -- Conditions, finalizers, log helpers.
- `github.com/openshift/hive/pkg/controller/metrics` -- Reconcile time observation.
- `github.com/openshift/hive/pkg/resource` -- DeleteAnyExistingObject for RBAC cleanup.

## Capabilities

- Reconciles: **ClusterClaim**.
- Watches: ClusterClaim, ClusterDeployment (mapped to claim), Role `hive-claim-owner`, RoleBinding `hive-claim-owner`.
- Conditions set: `ClusterClaimPendingCondition`, `ClusterRunningCondition`.
- Key logic: When assigned a cluster, creates `hive-claim-owner` Role (granting full Hive API access + read on kubeconfig/password/metadata secrets) and RoleBinding for claim subjects. Enforces claim lifetime (minimum of pool default/maximum and claim-specified). Auto-deletes claim when lifetime elapses. Handles assignment conflicts by clearing assignment and re-pending.
- Finalizer: `hive.openshift.io/claim`.

## Understanding Score

0.85
