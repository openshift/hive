# Module atlas

## Responsibility

Ensures that provisioned clusters are registered in the ArgoCD cluster registry by creating ArgoCD-compatible cluster secrets, and removes them when the cluster is deprovisioned. Activated only when the ArgoCD integration environment variable is set.

## Public Interface/API

- `ControllerName` — constant `"argocdregister"`
- `Add(mgr manager.Manager) error` — creates and registers the controller with the manager
- `NewReconciler(mgr manager.Manager, logger log.FieldLogger, rateLimiter flowcontrol.RateLimiter) reconcile.Reconciler`
- `AddToManager(mgr manager.Manager, r reconcile.Reconciler, concurrentReconciles int, rateLimiter workqueue.TypedRateLimiter[reconcile.Request]) error`
- `ArgoCDRegisterController` — reconciler struct embedding `client.Client`; reconciles ClusterDeployments
- `ArgoCDRegisterController.Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error)`
- `TLSClientConfig` — struct for transport layer security settings (copied from ArgoCD types)
- `ClusterConfig` — struct for ArgoCD cluster configuration attributes (bearer token, TLS, basic auth)

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` — ClusterDeployment CRD
- `github.com/openshift/hive/pkg/constants` — ArgoCD env var
- `github.com/openshift/hive/pkg/controller/metrics` — reconcile observer
- `github.com/openshift/hive/pkg/controller/utils` — controller config, client wrappers
- `sigs.k8s.io/controller-runtime` — controller, reconcile, manager, client
- `k8s.io/client-go/rest`, `k8s.io/client-go/tools/clientcmd` — kubeconfig handling

## Capabilities

- Watches ClusterDeployment resources
- Creates/updates ArgoCD cluster secrets in the ArgoCD namespace for installed clusters
- Removes ArgoCD cluster secrets when clusters are deprovisioned
- Reads admin kubeconfig from cluster secrets to build ArgoCD TLS client config

## Understanding Score

0.85
