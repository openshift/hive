# Module atlas

## Responsibility

Manages AWS PrivateLink VPC endpoints, endpoint services, and Route53 hosted zone associations for ClusterDeployments that have PrivateLink enabled. Handles the full lifecycle including creation during provisioning, cleanup during deprovisioning, and cleanup between provision attempts.

## Public Interface/API

- `ControllerName` — constant (from `hivev1.AWSPrivateLinkControllerName`)
- `Add(mgr manager.Manager) error` — creates and registers the controller with the manager
- `NewReconciler(mgr manager.Manager, rateLimiter flowcontrol.RateLimiter) (*ReconcileAWSPrivateLink, error)`
- `AddToManager(mgr manager.Manager, r *ReconcileAWSPrivateLink, concurrentReconciles int, rateLimiter workqueue.TypedRateLimiter[reconcile.Request]) error`
- `ReconcileAWSPrivateLink` — reconciler struct embedding `client.Client`; reconciles PrivateLink for ClusterDeployments
- `ReconcileAWSPrivateLink.Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error)`
- `ReadAWSPrivateLinkControllerConfigFile() (*hivev1.AWSPrivateLinkConfig, error)` — reads controller config from env/file

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`, `github.com/openshift/hive/apis/hive/v1/aws` — CRDs and AWS platform types
- `github.com/openshift/hive/pkg/constants` — pause annotation, config env vars
- `github.com/openshift/hive/pkg/awsclient` — AWS client abstraction
- `github.com/openshift/hive/pkg/controller/metrics` — reconcile observer
- `github.com/openshift/hive/pkg/controller/utils` — controller config, client wrappers, finalizer helpers
- `github.com/aws/aws-sdk-go-v2/service/ec2`, `route53`, `elasticloadbalancingv2`, `sts` — AWS SDK services
- `sigs.k8s.io/controller-runtime` — controller, reconcile, manager, client

## Capabilities

- Watches ClusterDeployment, ClusterProvision, and ClusterDeprovision resources
- Creates and manages VPC endpoints and VPC endpoint services in AWS
- Manages Route53 hosted zone associations for private DNS resolution
- Selects VPCs from an inventory using a spread strategy (least-loaded VPC first)
- Manages a finalizer (`hive.openshift.io/aws-private-link`) for cleanup
- Cleans up PrivateLink resources between provision attempts and during deprovisioning
- Sets `AWSPrivateLinkFailed` and `AWSPrivateLinkReady` conditions on ClusterDeployment

## Understanding Score

0.85
