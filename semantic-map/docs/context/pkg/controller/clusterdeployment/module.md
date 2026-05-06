# Module atlas

## Responsibility

The primary controller for the ClusterDeployment lifecycle. Orchestrates cluster installation, provisioning, deprovisioning, DNS zone management, install config validation, release image verification, and ClusterInstall contract support. This is the largest and most central controller in Hive.

## Public Interface/API

- `ControllerName` — constant (from `hivev1.ClusterDeploymentControllerName`)
- `Add(mgr manager.Manager) error` — creates and registers the controller with the manager
- `NewReconciler(mgr manager.Manager, logger log.FieldLogger, rateLimiter flowcontrol.RateLimiter) reconcile.Reconciler`
- `AddToManager(mgr manager.Manager, r reconcile.Reconciler, concurrentReconciles int, rateLimiter workqueue.TypedRateLimiter[reconcile.Request]) error`
- `ReconcileClusterDeployment` — reconciler struct embedding `manager.Manager` and `client.Client`
- `ReconcileClusterDeployment.Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error)`
- `ReconcileClusterDeployment.SetWatcher` — injects a dynamic watcher for ClusterInstall contracts
- `ValidateInstallConfig(cd *hivev1.ClusterDeployment, installConfigSecret *corev1.Secret) (*installertypes.InstallConfig, error)` — validates install-config.yaml against ClusterDeployment platform
- `LoadReleaseImageVerifier(config *rest.Config) (verify.Interface, error)` — loads signature-based release image verifier
- `ClusterProvisionManager` — struct for provision management logic

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`, `hivecontracts/v1alpha1`, `hiveinternal/v1alpha1` — core CRDs
- `github.com/openshift/hive/pkg/constants` — env vars and constants
- `github.com/openshift/hive/pkg/controller/metrics` — reconcile observer, metrics config
- `github.com/openshift/hive/pkg/controller/utils` — controller config, client wrappers, expectations
- `github.com/openshift/hive/pkg/remoteclient` — remote cluster client builder
- `github.com/openshift/hive/pkg/imageset` — image set resolution
- `github.com/openshift/hive/pkg/install` — install job generation
- `github.com/openshift/hive/pkg/gcpclient`, `ibmclient` — cloud credential validation
- `github.com/openshift/installer/pkg/types` — InstallConfig types for validation
- `github.com/openshift/library-go/pkg/verify` — release image signature verification
- `sigs.k8s.io/controller-runtime` — controller, reconcile, manager, client, metrics

## Capabilities

- Watches ClusterDeployment, ClusterProvision, ClusterDeprovision, DNSZone, Job, Pod, ClusterSync, and ClusterInstall resources
- Manages the full cluster install lifecycle: DNS zones, install config validation, provision creation, install job monitoring
- Handles cluster deprovisioning via ClusterDeprovision creation
- Validates platform credentials (AWS, GCP, Azure, vSphere, IBM Cloud, Nutanix, OpenStack)
- Validates install-config.yaml for platform/region consistency
- Supports ClusterInstall contracts for external install mechanisms
- Manages protected delete (prevents accidental cluster removal)
- Verifies release images via signature stores when configured
- Emits Prometheus metrics: install job duration, install delay, DNS delay, provision failures, cluster lifecycle counters
- Manages many ClusterDeployment conditions (DNS, provision, install, authentication, requirements, etc.)

## Understanding Score

0.85
