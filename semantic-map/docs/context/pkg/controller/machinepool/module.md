# Module atlas

## Responsibility

Reconciles MachinePool custom resources by generating and syncing MachineSets to remote clusters. Uses a pluggable Actuator pattern with cloud-specific implementations (AWS, Azure, GCP, IBM Cloud, Nutanix, OpenStack, vSphere) that each translate a MachinePool spec into the appropriate set of MachineSets for the target platform.

## Public Interface/API

- `ControllerName` -- constant `hivev1.MachinePoolControllerName`
- `Add(mgr manager.Manager) error` -- creates controller with watches on MachinePool, ClusterDeployment, MachinePoolNameLease
- `Actuator` -- interface: `GenerateMachineSets(cd, pool, logger) (msets []*machineapi.MachineSet, proceed bool, genError error)`
- `AWSActuator`, `AzureActuator`, `GCPActuator`, `IBMCloudActuator`, `NutanixActuator`, `OpenStackActuator`, `VSphereActuator` -- platform-specific Actuator implementations
- `ReconcileMachinePool` -- reconciler struct
- `ReconcileMachinePool.Reconcile(ctx, request) (reconcile.Result, error)`
- `GetVPCIDForMachinePool(pool, awsClient) (string, error)` -- retrieves VPC ID from AWS subnets
- `IsErrorUpdateEvent(event.UpdateEvent) bool` -- predicate for rate-limited event handling

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/awsclient`, `pkg/azureclient`, `pkg/gcpclient`, `pkg/ibmclient` -- cloud clients
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/metrics` -- ReconcileObserver
- `github.com/openshift/hive/pkg/controller/utils`, `pkg/controller/utils/nutanixutils`
- `github.com/openshift/hive/pkg/remoteclient` -- remote cluster client builder
- `github.com/openshift/installer/pkg/asset/machines/*` -- installer MachineSet generation for each platform
- `github.com/openshift/installer/pkg/types/*` -- installer platform types
- `github.com/openshift/api/machine/v1beta1` -- Machine API types
- `github.com/openshift/cluster-autoscaler-operator/pkg/apis/autoscaling/*` -- ClusterAutoscaler/MachineAutoscaler APIs

## Capabilities

- Generates MachineSets from MachinePool specs using installer machine asset libraries per platform
- Syncs generated MachineSets to remote clusters, handling create/update/delete
- Manages MachinePoolNameLease resources for unique naming with ordinal-based partitioning
- Handles ClusterAutoscaler and MachineAutoscaler scaling annotations
- Sets conditions: NotEnoughReplicas, NoMachinePoolNameLeasesAvailable, InvalidSubnets, UnsupportedConfiguration, MachineSetsGenerated, Synced
- Supports per-pool replica overrides, taints, labels, and platform-specific configuration
- Polls remote cluster state on a configurable interval

## Understanding Score

0.85
