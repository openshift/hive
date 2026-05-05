# Module atlas

## Responsibility

Provides a controller that manages AWS PrivateLink access for ClusterDeployments. For clusters with `spec.platform.aws.privateLink.enabled`, this controller creates and manages the full chain of AWS resources: VPC Endpoint Service (backed by the cluster's internal NLB), VPC Endpoint in a hub account VPC, a Private Hosted Zone with DNS records pointing to the VPC Endpoint, and VPC associations. It handles both installation-time and post-install reconciliation, cleanup on deletion or when PrivateLink is disabled, and cleanup between provision reattempts.

## Public Interface/API

- `Add` -- Creates the controller and adds it to the Manager.
- `AddToManager` -- Adds the controller to a manager.
- `ControllerName` -- Constant referencing `hivev1.AWSPrivateLinkControllerName`.
- `ReconcileAWSPrivateLink` -- Reconciler struct with AWS client factory and controller config.
- `ReconcileAWSPrivateLink.Reconcile` -- Main reconcile loop. Manages VPC Endpoint Service, VPC Endpoint, Hosted Zone, DNS records, and VPC associations.
- `ReadAWSPrivateLinkControllerConfigFile` -- Reads `AWSPrivateLinkConfig` from file path in env var.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment, ClusterProvision, ClusterDeprovision CRs; AWS PrivateLink config types.
- `github.com/openshift/hive/pkg/awsclient` -- AWS SDK client wrapper.
- `github.com/openshift/hive/pkg/controller/utils` -- Conditions, finalizers, error scrubbing, secret loading, controller config.
- `github.com/openshift/hive/pkg/controller/metrics` -- Reconcile time observation.

## Capabilities

- Reconciles: **ClusterDeployment** (with AWS PrivateLink enabled).
- Watches: ClusterDeployment, ClusterProvision (owner), ClusterDeprovision (owner).
- Conditions set: `AWSPrivateLinkFailedClusterDeploymentCondition`, `AWSPrivateLinkReadyClusterDeploymentCondition`.
- Key logic: Discovers cluster NLB, creates/reconciles VPC Endpoint Service with allowed principals (including AdditionalAllowedPrincipals from spec), creates VPC Endpoint in hub VPC, creates Private Hosted Zone with A or Alias DNS records, manages VPC associations. Uses shouldSync() to rate-limit cloud API calls (2h window for installed, 10m for installing). Cleanup removes all resources in reverse order.
- Finalizer: `hive.openshift.io/aws-private-link`.

## Understanding Score

0.85
