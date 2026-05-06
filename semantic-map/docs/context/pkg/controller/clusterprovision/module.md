# Module atlas

## Responsibility

Manages ClusterProvision resources through the install lifecycle. Creates install jobs, monitors their progress, parses install logs for known failure patterns, and reports provision outcomes via status conditions and metrics.

## Public Interface/API

- `ControllerName` — constant (from `hivev1.ClusterProvisionControllerName`)
- `Add(mgr manager.Manager) error` — creates and registers the controller with the manager
- `ReconcileClusterProvision` — reconciler struct embedding `client.Client` with expectations and shared pod config
- `ReconcileClusterProvision.Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error)`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`, `metricsconfig` — ClusterProvision CRD, metrics config
- `github.com/openshift/hive/pkg/constants` — constants
- `github.com/openshift/hive/pkg/controller/metrics` — reconcile observer, dynamic label metrics
- `github.com/openshift/hive/pkg/controller/utils` — controller config, client wrappers, expectations, shared pod config
- `github.com/openshift/hive/pkg/install` — install job generation
- `github.com/openshift/hive/pkg/util/labels` — label utilities
- `github.com/openshift/installer/pkg/types` — InstallConfig types
- `sigs.k8s.io/controller-runtime` — controller, reconcile, manager, client

## Capabilities

- Watches ClusterProvision, Job, and ClusterDeployment resources
- Creates install jobs from ClusterProvision specs using shared pod config
- Tracks job creation expectations to avoid duplicate job creation
- Monitors install job completion and failure
- Parses install logs using configurable regex patterns (from `install-log-regexes` and `additional-install-log-regexes` ConfigMaps)
- Reports install failure reasons and messages based on log pattern matching
- Emits Prometheus metrics: `hive_cluster_provision_results_total`, `hive_install_errors`, install success/failure duration histograms

## Understanding Score

0.85
