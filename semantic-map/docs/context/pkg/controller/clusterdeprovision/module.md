# Module atlas

## Responsibility

Manages cluster deprovisioning by creating and monitoring uninstall jobs for ClusterDeprovision resources. Uses an actuator pattern to support multiple cloud providers for credential testing before launching deprovision jobs.

## Public Interface/API

- `ControllerName` — constant (from `hivev1.ClusterDeprovisionControllerName`)
- `Add(mgr manager.Manager) error` — creates and registers the controller with the manager
- `Actuator` — interface for cloud provider deprovision support with `CanHandle(*hivev1.ClusterDeprovision) bool` and `TestCredentials(*hivev1.ClusterDeprovision, client.Client, log.FieldLogger) error`
- `ReconcileClusterDeprovision` — reconciler struct embedding `client.Client`
- `ReconcileClusterDeprovision.Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error)`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` — ClusterDeprovision CRD
- `github.com/openshift/hive/pkg/constants` — env vars (deprovisionsDisabled)
- `github.com/openshift/hive/pkg/controller/metrics` — reconcile observer
- `github.com/openshift/hive/pkg/controller/utils` — controller config, client wrappers, shared pod config
- `github.com/openshift/hive/pkg/install` — deprovision job generation
- `github.com/openshift/hive/pkg/awsclient` — AWS client for credential testing
- `github.com/aws/aws-sdk-go-v2/service/sts` — STS for AWS credential verification
- `sigs.k8s.io/controller-runtime` — controller, reconcile, manager, client, metrics

## Capabilities

- Watches ClusterDeprovision and uninstall Job resources
- Creates uninstall jobs from ClusterDeprovision specs
- Verifies cloud credentials via the Actuator interface before launching jobs
- Includes an AWS actuator (registered via `init()`) that tests credentials via STS GetCallerIdentity
- Supports disabling deprovisions via environment variable
- Tracks job hash annotations to detect when jobs need to be recreated
- Emits `hive_cluster_deployment_uninstall_job_duration_seconds` histogram metric

## Understanding Score

0.85
