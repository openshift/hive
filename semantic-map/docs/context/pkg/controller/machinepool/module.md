# Module atlas

## Responsibility

Provides a controller that reconciles MachinePool objects by generating and syncing MachineSets to remote clusters. Uses an actuator pattern with cloud-specific implementations for AWS, Azure, GCP, IBM Cloud, Nutanix, OpenStack, and vSphere. Each actuator translates a MachinePool spec into platform-specific MachineSet definitions, which are then applied to the remote cluster. Handles autoscaling (ClusterAutoscaler/MachineAutoscaler), lease management for MachinePool ownership, and master machine pool updates.

## Public Interface/API

- `Add` -- Creates the controller and adds it to the Manager.
- `ControllerName` -- Constant referencing `hivev1.MachinePoolControllerName`.
- `Actuator` -- Interface: `GenerateMachineSets(cd, pool, logger) ([]MachineSet, bool, error)`.
- `AWSActuator`, `AzureActuator`, `GCPActuator`, `IBMCloudActuator`, `NutanixActuator`, `OpenStackActuator`, `VSphereActuator` -- Cloud-specific implementations.
- `ReconcileMachinePool` -- Reconciler struct.
- `ReconcileMachinePool.Reconcile` -- Generates MachineSets from MachinePool spec and syncs to remote cluster.
- `GetVPCIDForMachinePool` -- Retrieves AWS VPC ID from subnet configuration.
- `IsErrorUpdateEvent` -- Predicate for rate-limited error event handling.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- MachinePool, ClusterDeployment CRs.
- `github.com/openshift/api/machine/v1beta1`, `machine/v1` -- MachineSet, Machine API types.
- `github.com/openshift/api/config/v1` -- Infrastructure types.
- `github.com/openshift/hive/pkg/constants` -- Environment variable names, label keys.
- `github.com/openshift/hive/pkg/remoteclient` -- Remote cluster client for MachineSet management.
- `github.com/openshift/hive/pkg/awsclient`, `pkg/azureclient`, `pkg/gcpclient`, `pkg/ibmclient` -- Cloud clients for subnet/zone discovery.
- `github.com/openshift/hive/pkg/controller/utils` -- Conditions, controller config, lease exceptions.
- `github.com/openshift/hive/pkg/controller/utils/nutanixutils` -- Nutanix failure domain conversion.
- `github.com/openshift/hive/pkg/controller/metrics` -- Reconcile time observation.
- `github.com/openshift/hive/pkg/util/logrus`, `pkg/util/scheme` -- Logging and scheme utilities.
- `github.com/openshift/machine-api-provider-gcp`, `machine-api-provider-ibmcloud` -- Provider-specific Machine API types.
- `github.com/openshift/installer/pkg/asset/machines/*` -- Installer MachineSet generation per platform.
- `github.com/openshift/installer/pkg/types` -- Installer types per platform.

## Capabilities

- Reconciles: **MachinePool**.
- Watches: MachinePool, ClusterDeployment (mapped to MachinePools).
- Conditions set: Various MachinePool conditions (e.g., `InvalidSubnetsMachinePoolCondition`, `UnsupportedConfigurationMachinePoolCondition`).
- Key logic: Selects actuator by platform, generates MachineSets using installer libraries, syncs to remote cluster (create/update/delete), manages ClusterAutoscaler and MachineAutoscaler for autoscaling pools, handles lease-based ownership for StatefulSet replicas, tracks master MachinePool for control plane updates.

## Understanding Score

0.80
