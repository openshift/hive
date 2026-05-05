# Module atlas

## Responsibility

Provides a controller that reconciles ClusterPool objects, managing a pool of pre-provisioned ClusterDeployments. It creates new ClusterDeployments to maintain the desired pool size, assigns clusters to ClusterClaims, handles pool scaling, hibernation of unclaimed clusters, broken cluster replacement, pool spec updates (rolling out changes to unclaimed clusters), namespace and RBAC management for pool clusters, and inventory validation for platforms like vSphere.

## Public Interface/API

- `Add` -- Creates the controller and adds it to the Manager.
- `AddToManager` -- Adds the controller with field indexes for CD-to-pool and claim-to-pool lookups.
- `ControllerName` -- Constant referencing `hivev1.ClusterpoolControllerName`.
- `ReconcileClusterPool` -- Reconciler struct with expectations for CD creation tracking.
- `ReconcileClusterPool.Reconcile` -- Main reconcile loop. Manages pool size, claim assignment, cluster lifecycle.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ClusterPool, ClusterDeployment, ClusterClaim CRs.
- `github.com/openshift/hive/pkg/clusterresource` -- Generates ClusterDeployment resources from pool templates.
- `github.com/openshift/hive/pkg/controller/utils` -- Conditions, expectations, finalizers, controller config.
- `github.com/openshift/hive/pkg/controller/utils/vsphereutils` -- vSphere field conversion.
- `github.com/openshift/hive/pkg/controller/metrics` -- Reconcile time observation.

## Capabilities

- Reconciles: **ClusterPool**.
- Watches: ClusterPool, ClusterDeployment (indexed by pool), ClusterClaim (indexed by pool name).
- Conditions set: `ClusterPoolMissingDependenciesCondition`, `ClusterPoolCapacityAvailableCondition`, `ClusterPoolAllClustersCurrentCondition`, `ClusterPoolInventoryValidCondition`, `ClusterPoolDeletionPossibleCondition`.
- Key logic: Maintains pool size (creates CDs up to pool.spec.size), assigns unclaimed installed clusters to pending ClusterClaims (sorted by creation time), creates namespaces per CD with pool admin RBAC, hibernates unclaimed clusters, detects and replaces broken clusters, rolls out pool spec changes to unclaimed clusters via hash comparison, manages pool finalizer for cleanup.
- Finalizer: `hive.openshift.io/clusters`.

## Understanding Score

0.80
