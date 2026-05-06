# Module atlas

## Responsibility

Reconciles FakeClusterInstall resources (from the hiveinternal API) as a test/development cluster install strategy. Simulates the cluster install lifecycle by managing install conditions and generating mock cluster metadata without actually provisioning infrastructure.

## Public Interface/API

- `ControllerName` -- constant `hivev1.FakeClusterInstallControllerName`
- `Add(mgr manager.Manager) error` -- creates controller and adds to manager
- `NewReconciler(mgr manager.Manager, rateLimiter flowcontrol.RateLimiter) reconcile.Reconciler`
- `AddToManager(mgr, r, concurrentReconciles, rateLimiter) error` -- registers controller with FakeClusterInstall watch
- `ReconcileClusterInstall` -- reconciler struct embedding `client.Client`
- `ReconcileClusterInstall.Reconcile(ctx, request) (reconcile.Result, error)` -- manages FakeClusterInstall lifecycle, initializes conditions, generates fake cluster metadata

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/apis/hiveinternal/v1alpha1` -- FakeClusterInstall CRD
- `github.com/openshift/hive/pkg/controller/metrics` -- ReconcileObserver
- `github.com/openshift/hive/pkg/controller/utils` -- conditions, controller config
- `github.com/openshift/library-go/pkg/controller`
- `sigs.k8s.io/controller-runtime` -- controller, reconcile, manager

## Capabilities

- Watches FakeClusterInstall resources
- Initializes standard ClusterInstall conditions (Completed, Failed, Stopped, RequirementsMet)
- Looks up the corresponding ClusterDeployment and manages install simulation
- Generates fake admin kubeconfig and cluster metadata
- Sets install conditions to reflect simulated cluster state

## Understanding Score

0.85
