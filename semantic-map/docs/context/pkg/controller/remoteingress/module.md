# Module atlas

## Responsibility

Reconciles ClusterDeployment ingress definitions into SyncSet objects that create IngressController and certificate Secret resources on remote clusters. Manages the `IngressCertificateNotFound` condition on ClusterDeployments when referenced certificate bundle secrets are missing.

## Public Interface/API

- `const ControllerName` -- hivev1.RemoteIngressControllerName
- `func Add(mgr manager.Manager) error` -- creates and registers the controller with default RBAC
- `func NewReconciler(mgr manager.Manager, rateLimiter flowcontrol.RateLimiter) reconcile.Reconciler` -- returns a new reconciler with a resource.Helper for CLI apply
- `func AddToManager(mgr manager.Manager, r reconcile.Reconciler, concurrentReconciles int, rateLimiter workqueue.TypedRateLimiter[reconcile.Request]) error` -- registers controller watching ClusterDeployments
- `func GenerateRemoteIngressSyncSetName(clusterDeploymentName string) string` -- generates predictable SyncSet name
- `type ReconcileRemoteClusterIngress struct` -- reconciler; embeds client.Client
- `func (r *ReconcileRemoteClusterIngress) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error)` -- main reconcile loop

## Internal Dependencies

- `github.com/openshift/hive/apis/helpers` -- resource name generation
- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment, SyncSet, CertificateBundle types
- `github.com/openshift/hive/pkg/constants` -- label keys, annotation keys, suffixes
- `github.com/openshift/hive/pkg/controller/metrics` -- reconcile time observer
- `github.com/openshift/hive/pkg/controller/utils` -- controller config, client wrapper, conditions, ownership, logging
- `github.com/openshift/hive/pkg/resource` -- Helper for kubectl-style apply
- `github.com/openshift/hive/pkg/util/labels` -- label helpers
- `github.com/openshift/api/operator/v1` -- IngressController type
- `sigs.k8s.io/controller-runtime` -- controller, reconcile, client, handler, source

## Capabilities

- Watches ClusterDeployments for ingress spec changes
- Generates SyncSet containing IngressController objects and SecretMappings for TLS certificates
- Sets owner references on derived SyncSets for garbage collection
- Initializes and manages the IngressCertificateNotFound cluster deployment condition
- Respects reconcile pause annotation
- Uses resource.Helper for idempotent apply of SyncSets

## Understanding Score

0.85
