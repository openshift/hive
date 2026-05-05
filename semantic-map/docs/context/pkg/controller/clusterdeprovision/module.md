# Module atlas

## Responsibility

Provides a controller that manages ClusterDeprovision objects. It creates and monitors uninstall jobs to deprovision cloud resources for deleted clusters. Uses an actuator pattern to support cloud-specific authentication validation (currently AWS via `awsactuator.go`). Tracks job completion, handles job hash-based updates, and can optionally disable deprovisions entirely via environment variable.

## Public Interface/API

- `Add` -- Creates the controller and adds it to the Manager.
- `ControllerName` -- Constant referencing `hivev1.ClusterDeprovisionControllerName`.
- `Actuator` -- Interface for cloud-specific deprovision support. Methods: `CanHandle(*ClusterDeprovision)`, `TestCredentials(*ClusterDeprovision)`.
- `ReconcileClusterDeprovision` -- Reconciler struct.
- `ReconcileClusterDeprovision.Reconcile` -- Main reconcile loop. Creates/monitors uninstall jobs, validates credentials via actuators, updates conditions.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeprovision CR.
- `github.com/openshift/hive/pkg/install` -- Uninstall job generation.
- `github.com/openshift/hive/pkg/controller/utils` -- Finalizers, conditions, job utilities, controller config.
- `github.com/openshift/hive/pkg/controller/metrics` -- Reconcile time observation.
- `github.com/openshift/hive/pkg/awsclient` -- AWS credentials testing (via actuator).

## Capabilities

- Reconciles: **ClusterDeprovision**.
- Watches: ClusterDeprovision, Jobs (owner).
- Conditions set: `AuthenticationFailureClusterDeploymentCondition` (via actuator credential testing).
- Key logic: Creates uninstall jobs using `install.GenerateUninstallerJobForDeprovision`, monitors job completion, records uninstall duration metrics, supports deprovisionsDisabled mode. AWS actuator validates credentials via STS GetCallerIdentity before launching jobs.
- Metrics: `hive_cluster_deployment_uninstall_job_duration_seconds` histogram.

## Understanding Score

0.85
