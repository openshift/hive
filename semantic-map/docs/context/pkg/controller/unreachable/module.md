# Module atlas

## Responsibility

Provides a controller that monitors the reachability of remote cluster API servers. For each installed ClusterDeployment, the controller periodically attempts to establish a client connection to the remote cluster and sets the `UnreachableCondition` based on whether the connection succeeds or fails.

## Public Interface/API

- `Add` -- Creates the controller and adds it to the Manager.
- `AddToManager` -- Adds the controller to a manager.
- `NewReconciler` -- Returns a new reconciler.
- `ControllerName` -- Constant `"unreachable"`.
- `ReconcileRemoteMachineSet` -- Reconciler struct (note: name is a misnomer, it reconciles reachability, not MachineSets).
- `ReconcileRemoteMachineSet.Reconcile` -- Checks API client connectivity and maintains UnreachableCondition.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment CR.
- `github.com/openshift/hive/pkg/remoteclient` -- Remote cluster client builder.
- `github.com/openshift/hive/pkg/controller/utils` -- Conditions, controller config.
- `github.com/openshift/hive/pkg/controller/metrics` -- Reconcile time observation.

## Capabilities

- Reconciles: **ClusterDeployment** (installed only).
- Watches: ClusterDeployment.
- Conditions set: `UnreachableCondition`.
- Key logic: Attempts to connect to the remote cluster API server using the admin kubeconfig. If successful, sets UnreachableCondition to False. If connection fails, sets it to True. Requeues periodically for continuous monitoring. Skips paused or relocating clusters.

## Understanding Score

0.85
