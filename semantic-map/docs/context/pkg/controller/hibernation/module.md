# Module atlas

## Responsibility

Manages the hibernation lifecycle for ClusterDeployments by stopping and starting cloud machines (AWS EC2, Azure VMs, GCP instances, IBM Cloud VPC instances), approving CSRs on resume, and tracking cluster readiness via ClusterOperator status checks. Uses a pluggable actuator pattern for cloud-specific machine management.

## Public Interface/API

- `ControllerName` -- constant `hivev1.HibernationControllerName`
- `Add(mgr manager.Manager) error` -- creates controller and adds to manager
- `AddToManager(mgr, r, concurrentReconciles, rateLimiter) error`
- `NewReconciler(mgr, rateLimiter) *hibernationReconciler`
- `RegisterActuator(a HibernationActuator)` -- registers a cloud actuator (called in init() by each cloud actuator)
- `HibernationActuator` -- interface: `CanHandle(cd)`, `StopMachines(cd, client, logger)`, `StartMachines(cd, client, logger)`, `MachinesRunning(cd, client, logger)`, `MachinesStopped(cd, client, logger)`
- `HibernationPreemptibleMachines` -- interface: `ReplaceMachines(cd, remoteClient, logger) (replaced bool, err error)`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`, `apis/hiveinternal/v1alpha1`
- `github.com/openshift/hive/pkg/awsclient`, `pkg/azureclient`, `pkg/gcpclient`, `pkg/ibmclient` -- cloud client factories
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/metrics` -- ReconcileObserver, hibernation transition metrics
- `github.com/openshift/hive/pkg/controller/utils` -- controller config, conditions
- `github.com/openshift/hive/pkg/remoteclient` -- remote cluster client builder
- `github.com/openshift/api/config/v1` -- ClusterOperator checks
- `github.com/openshift/api/machine/v1beta1` -- Machine API for preemptible instance handling

## Capabilities

- Watches ClusterDeployment resources for hibernation power state changes
- AWS actuator: stops/starts EC2 instances, checks running/stopped state, handles preemptible (spot) instances
- Azure actuator: deallocates/starts Azure VMs
- GCP actuator: stops/starts GCP compute instances, replaces preemptible machines
- IBM Cloud actuator: stops/starts VPC instances
- Approves pending CSRs on cluster resume via remote client
- Checks ClusterOperator readiness after resume before declaring cluster ready
- Reports hibernation/resume transition duration metrics
- Manages `ClusterHibernatingCondition` and `ClusterReadyCondition` on ClusterDeployment

## Understanding Score

0.85
