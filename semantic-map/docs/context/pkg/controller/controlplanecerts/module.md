# Module atlas

## Responsibility

Ensures that control plane TLS certificates specified in the ClusterDeployment are synced to the remote cluster. Creates a SyncSet containing the certificate secrets and manages their application to the `openshift-config` namespace on the target cluster.

## Public Interface/API

- `ControllerName` — constant (from `hivev1.ControlPlaneCertsControllerName`)
- `Add(mgr manager.Manager) error` — creates and registers the controller with the manager
- `NewReconciler(mgr manager.Manager, rateLimiter flowcontrol.RateLimiter) reconcile.Reconciler`
- `AddToManager(mgr manager.Manager, r reconcile.Reconciler, concurrentReconciles int, rateLimiter workqueue.TypedRateLimiter[reconcile.Request]) error`
- `ReconcileControlPlaneCerts` — reconciler struct embedding `client.Client`
- `ReconcileControlPlaneCerts.Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error)`
- `GenerateControlPlaneCertsSyncSetName(name string) string` — generates the SyncSet name for a given ClusterDeployment

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`, `github.com/openshift/hive/apis/helpers` — ClusterDeployment CRD, resource name helpers
- `github.com/openshift/hive/pkg/constants` — certificate suffix constant
- `github.com/openshift/hive/pkg/controller/metrics` — reconcile observer
- `github.com/openshift/hive/pkg/controller/utils` — controller config, client wrappers
- `github.com/openshift/hive/pkg/remoteclient` — remote cluster client builder
- `github.com/openshift/hive/pkg/resource` — resource applier
- `sigs.k8s.io/controller-runtime` — controller, reconcile, manager, client

## Capabilities

- Watches ClusterDeployment resources
- Reads certificate secrets referenced by ClusterDeployment spec
- Creates/updates a SyncSet to push certificate secrets to the remote cluster's `openshift-config` namespace
- Hashes secret contents (MD5) to detect changes and trigger redeployment
- Patches the remote kube-apiserver to force redeployment when certificates change
- Sets `ControlPlaneCertificateNotFound` condition on ClusterDeployment when referenced secrets are missing
- Periodically re-checks certificate secrets (2-minute interval)

## Understanding Score

0.85
