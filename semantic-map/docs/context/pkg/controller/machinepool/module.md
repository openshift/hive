<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/machinepool/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `AWSActuator` — AWSActuator encapsulates the pieces necessary to be able to generate a list of MachineSets to sync to the remote cluster.
- `AWSActuator.GenerateMachineSets` — GenerateMachineSets satisfies the Actuator interface and will take a clusterDeployment and return a list of MachineSets to sync to the remote cluster.
- `Actuator` — Actuator is the interface that must be implemented to standardize generating and returning the list of MachineSets to by synced to the remote cluster.
- `Add` — Add creates a new MachinePool Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller and Start it when the Manager is Started.
- `AzureActuator` — AzureActuator encapsulates the pieces necessary to be able to generate a list of MachineSets to sync to the remote cluster.
- `AzureActuator.GenerateMachineSets` — GenerateMachineSets satisfies the Actuator interface and will take a clusterDeployment and return a list of MachineSets to sync to the remote cluster.
- `ControllerName`
- `GCPActuator` — GCPActuator encapsulates the pieces necessary to be able to generate a list of MachineSets to sync to the remote cluster.
- `GCPActuator.GenerateMachineSets` — GenerateMachineSets satisfies the Actuator interface and will take a clusterDeployment and return a list of MachineSets to sync to the remote cluster.
- `GetVPCIDForMachinePool` — GetVPCIDForMachinePool retrieves the VPC ID of the first subnet configured in pool.Spec.Platform.AWS.Subnets using the provided AWS Client.
- `IBMCloudActuator` — IBMCloudActuator encapsulates the pieces necessary to be able to generate a list of MachineSets to sync to the remote cluster
- `IBMCloudActuator.GenerateMachineSets` — GenerateMachineSets satisfies the Actuator interface and will take a clusterDeployment and return a list of MachineSets to sync to the remote cluster.
- `IsErrorUpdateEvent` — IsErrorUpdateEvent returns true when the update event for MachinePool is from error state.
- `NutanixActuator` — NutanixActuator encapsulates the pieces necessary to be able to generate a list of MachineSets to sync to the remote cluster
- `NutanixActuator.GenerateMachineSets` — GenerateMachineSets satisfies the Actuator interface and will take a clusterDeployment and return a list of MachineSets to sync to the remote cluster.
- `OpenStackActuator` — OpenStackActuator encapsulates the pieces necessary to be able to generate a list of MachineSets to sync to the remote cluster.
- `OpenStackActuator.GenerateMachineSets` — GenerateMachineSets satisfies the Actuator interface and will take a clusterDeployment and return a list of MachineSets to sync to the remote cluster.
- `ReconcileMachinePool` — ReconcileMachinePool reconciles the MachineSets generated from a MachinePool object
- `ReconcileMachinePool.Reconcile` — Reconcile reads that state of the cluster for a MachinePool object and makes changes to the remote cluster MachineSets based on the state read
- `VSphereActuator` — VSphereActuator encapsulates the pieces necessary to be able to generate a list of MachineSets to sync to the remote cluster
- `VSphereActuator.GenerateMachineSets` — GenerateMachineSets satisfies the Actuator interface and will take a clusterDeployment and return a list of MachineSets to sync to the remote cluster.

## Internal Dependencies

- `bytes`
- `context`
- `encoding/json`
- `fmt`
- `github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute`
- `github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-12-01/compute`
- `github.com/Azure/go-autorest/autorest/azure`
- `github.com/Azure/go-autorest/autorest/to`
- `github.com/aws/aws-sdk-go-v2/aws`
- `github.com/aws/aws-sdk-go-v2/service/ec2`
- `github.com/aws/aws-sdk-go-v2/service/ec2/types`
- `github.com/gophercloud/utils/v2/openstack/clientconfig`
- `github.com/openshift/api/config/v1`
- `github.com/openshift/api/machine/v1`
- `github.com/openshift/api/machine/v1beta1`
- `github.com/openshift/cluster-autoscaler-operator/pkg/apis/autoscaling/v1`
- `github.com/openshift/cluster-autoscaler-operator/pkg/apis/autoscaling/v1beta1`
- `github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/providerconfig`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/awsclient`
- `github.com/openshift/hive/pkg/azureclient`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/metrics`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/openshift/hive/pkg/controller/utils/nutanixutils`
- `github.com/openshift/hive/pkg/gcpclient`
- `github.com/openshift/hive/pkg/ibmclient`
- `github.com/openshift/hive/pkg/remoteclient`
- `github.com/openshift/hive/pkg/util/logrus`
- `github.com/openshift/hive/pkg/util/scheme`
- `github.com/openshift/installer/pkg/asset/installconfig`
- `github.com/openshift/installer/pkg/asset/installconfig/aws`
- `github.com/openshift/installer/pkg/asset/installconfig/azure`
- `github.com/openshift/installer/pkg/asset/machines/aws`
- `github.com/openshift/installer/pkg/asset/machines/azure`
- `github.com/openshift/installer/pkg/asset/machines/gcp`
- `github.com/openshift/installer/pkg/asset/machines/ibmcloud`
- `github.com/openshift/installer/pkg/asset/machines/nutanix`
- `github.com/openshift/installer/pkg/asset/machines/openstack`
- `github.com/openshift/installer/pkg/asset/machines/vsphere`
- `github.com/openshift/installer/pkg/types`
- `github.com/openshift/installer/pkg/types/aws`
- `github.com/openshift/installer/pkg/types/azure`
- `github.com/openshift/installer/pkg/types/gcp`
- `github.com/openshift/installer/pkg/types/ibmcloud`
- `github.com/openshift/installer/pkg/types/nutanix`
- `github.com/openshift/installer/pkg/types/openstack`
- `github.com/openshift/library-go/pkg/operator/resource/resourcemerge`
- `github.com/openshift/machine-api-provider-gcp/pkg/apis/gcpprovider/v1beta1`
- `github.com/openshift/machine-api-provider-ibmcloud/pkg/apis/ibmcloudprovider/v1`
- `github.com/pkg/errors`
- `github.com/sirupsen/logrus`
- `gopkg.in/yaml.v2`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/api/errors`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/apis/meta/v1/unstructured`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/apimachinery/pkg/types`
- `k8s.io/apimachinery/pkg/util/errors`
- `k8s.io/apimachinery/pkg/util/json`
- `k8s.io/apimachinery/pkg/util/sets`
- `k8s.io/client-go/util/workqueue`
- `k8s.io/utils/ptr`
- `math/rand`
- `os`
- `reflect`
- `regexp`
- `sigs.k8s.io/cluster-api-provider-azure/api/v1beta1`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/controller`
- `sigs.k8s.io/controller-runtime/pkg/event`
- `sigs.k8s.io/controller-runtime/pkg/handler`
- `sigs.k8s.io/controller-runtime/pkg/manager`
- `sigs.k8s.io/controller-runtime/pkg/reconcile`
- `sigs.k8s.io/controller-runtime/pkg/source`
- `sort`
- `strconv`
- `strings`
- `time`

## Capabilities

- **`package`** name(s): **machinepool**.
- Go **`import`** edges listed below (80 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/machinepool`.

## Understanding Score

0.0
