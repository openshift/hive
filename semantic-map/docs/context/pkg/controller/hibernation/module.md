# Module atlas

## Responsibility

Provides a controller that manages cluster hibernation (stop/start) for ClusterDeployments. Uses an actuator pattern with cloud-specific implementations for AWS, Azure, GCP, and IBM Cloud. The controller stops cluster machines when `spec.powerState` is set to `Hibernating`, starts them when set to `Running`, monitors machine state transitions, handles CSR approval on cluster resume, and manages preemptible/spot instance replacement.

## Public Interface/API

- `Add` -- Creates the controller and adds it to the Manager.
- `AddToManager` -- Adds the controller to the manager.
- `NewReconciler` -- Returns a new reconciler.
- `ControllerName` -- Constant `"hibernation"`.
- `HibernationActuator` -- Interface: `CanHandle`, `StopMachines`, `StartMachines`, `MachinesRunning`, `MachinesStopped`.
- `HibernationPreemptibleMachines` -- Optional interface for preemptible instance replacement: `ReplaceMachines`.
- `RegisterActuator` -- Registers a cloud actuator at init time.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment CR.
- `github.com/openshift/hive/pkg/awsclient` -- AWS EC2 client for instance stop/start.
- `github.com/openshift/hive/pkg/azureclient` -- Azure compute client for VM stop/start.
- `github.com/openshift/hive/pkg/gcpclient` -- GCP compute client for instance stop/start.
- `github.com/openshift/hive/pkg/ibmclient` -- IBM Cloud VPC client for instance stop/start.
- `github.com/openshift/hive/pkg/remoteclient` -- Remote cluster client for CSR approval and node monitoring.
- `github.com/openshift/hive/pkg/controller/utils` -- Conditions, controller config.
- `github.com/openshift/hive/pkg/controller/metrics` -- Hibernate/resume duration metrics.

## Capabilities

- Reconciles: **ClusterDeployment** (installed, with powerState changes).
- Watches: ClusterDeployment.
- Conditions set: `HibernatingCondition`, `ReadyCondition` on ClusterDeployment. Updates `status.powerState`.
- Key logic: On hibernation request, stops all machines via cloud actuator, waits for stopped state. On resume, starts machines, waits for running state, monitors remote cluster nodes, approves pending CSRs for kubelet serving certificates. For preemptible instances (AWS spot, GCP preemptible), replaces terminated machines. Tracks transition timing metrics.
- Metrics: `hive_cluster_hibernation_transition_seconds`, `hive_stopping_clusters_seconds`, `hive_resuming_clusters_seconds`, `hive_waiting_for_co_clusters_seconds`.

## Understanding Score

0.80
