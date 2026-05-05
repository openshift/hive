# Module atlas

## Responsibility

Provides a controller that reaps namespaces created for ClusterPool clusters after all ClusterDeployments in the namespace have been deleted. It watches Namespaces and ClusterDeployments, and when a namespace (identified by the `hive.openshift.io/cluster-pool-namespace` label) no longer contains any ClusterDeployments and has existed for at least 5 minutes, the controller deletes it. It also handles deletion of CDs marked for removal by the clusterclaim controller.

## Public Interface/API

- `Add` -- Creates the controller and adds it to the Manager.
- `AddToManager` -- Adds the controller to a manager.
- `NewReconciler` -- Returns a new reconciler.
- `ControllerName` -- Constant referencing `hivev1.ClusterpoolNamespaceControllerName`.
- `ReconcileClusterPoolNamespace` -- Reconciler struct.
- `ReconcileClusterPoolNamespace.Reconcile` -- Deletes namespaces that no longer contain ClusterDeployments.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment CR.
- `github.com/openshift/hive/pkg/constants` -- Label keys for pool namespace identification.
- `github.com/openshift/hive/pkg/controller/utils` -- Controller config, log helpers, cluster removal annotation checks.
- `github.com/openshift/hive/pkg/controller/metrics` -- Reconcile time observation.

## Capabilities

- Reconciles: **Namespace** (those with `hive.openshift.io/cluster-pool-namespace` label).
- Watches: Namespace, ClusterDeployment (mapped to namespace).
- Conditions set: None.
- Key logic: Lists CDs in namespace; if none exist (or all are marked for removal/deleting) and namespace has existed >= 5 minutes, deletes the namespace. Deletes CDs that are marked for removal by the clusterclaim controller. Requeues after 1 minute when CDs are still being deleted.

## Understanding Score

0.85
