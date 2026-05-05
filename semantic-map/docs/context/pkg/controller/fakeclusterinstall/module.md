# Module atlas

## Responsibility

Provides a test/development controller that reconciles FakeClusterInstall objects. FakeClusterInstall is a ClusterInstall contract implementation used for testing Hive's ClusterInstall support without actually provisioning a cluster. It simulates installation completion after a configurable delay.

## Public Interface/API

- `Add` -- Creates the controller and adds it to the Manager.
- `AddToManager` -- Adds the controller to a manager.
- `NewReconciler` -- Returns a new reconciler.
- `ControllerName` -- Constant `"fakeclusterinstall"`.
- `ReconcileClusterInstall` -- Reconciler struct.
- `ReconcileClusterInstall.Reconcile` -- Simulates cluster installation by updating FakeClusterInstall conditions after a delay.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment CR.
- `github.com/openshift/hive/apis/hiveinternal/v1alpha1` -- FakeClusterInstall CR.
- `github.com/openshift/hive/pkg/controller/utils` -- Controller config.
- `github.com/openshift/hive/pkg/controller/metrics` -- Reconcile time observation.

## Capabilities

- Reconciles: **FakeClusterInstall** (hiveinternal.v1alpha1).
- Watches: FakeClusterInstall.
- Conditions set: ClusterInstall conditions (Completed, Failed, Stopped, RequirementsMet).
- Key logic: After creation, waits a configurable duration then marks the FakeClusterInstall as completed, populating cluster metadata. Used for integration testing of the ClusterInstall contract flow without real cloud provisioning.

## Understanding Score

0.85
