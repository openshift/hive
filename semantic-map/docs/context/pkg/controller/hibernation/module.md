<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/hibernation/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `Add` — Add creates a new Hibernation controller and adds it to the manager with default RBAC.
- `AddToManager` — AddToManager adds a new Controller to the controller manager
- `ControllerName`
- `HibernationActuator` — HibernationActuator is the interface that the hibernation controller uses to interact with cloud providers.
- `HibernationPreemptibleMachines` — HibernationPreemptibleMachines is the interface that the hibernation controller for preemptible instances on cloud providers.
- `NewReconciler` — NewReconciler returns a new Reconciler
- `RegisterActuator` — RegisterActuator register an actuator with this controller. The actuator determines whether it can handle a particular cluster deployment via the CanHandle function.

## Internal Dependencies

- `context`
- `crypto/x509`
- `encoding/pem`
- `fmt`
- `github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-12-01/compute`
- `github.com/Azure/go-autorest/autorest/azure`
- `github.com/Azure/go-autorest/autorest/to`
- `github.com/IBM/vpc-go-sdk/vpcv1`
- `github.com/aws/aws-sdk-go-v2/aws`
- `github.com/aws/aws-sdk-go-v2/service/ec2`
- `github.com/aws/aws-sdk-go-v2/service/ec2/types`
- `github.com/openshift/api/config/v1`
- `github.com/openshift/api/machine/v1beta1`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/apis/hiveinternal/v1alpha1`
- `github.com/openshift/hive/pkg/awsclient`
- `github.com/openshift/hive/pkg/azureclient`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/metrics`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/openshift/hive/pkg/gcpclient`
- `github.com/openshift/hive/pkg/ibmclient`
- `github.com/openshift/hive/pkg/remoteclient`
- `github.com/pkg/errors`
- `github.com/prometheus/client_golang/prometheus`
- `github.com/sirupsen/logrus`
- `google.golang.org/api/compute/v1`
- `k8s.io/api/certificates/v1`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/api/errors`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/types`
- `k8s.io/apimachinery/pkg/util/errors`
- `k8s.io/apimachinery/pkg/util/sets`
- `k8s.io/client-go/kubernetes`
- `k8s.io/client-go/util/flowcontrol`
- `k8s.io/client-go/util/workqueue`
- `k8s.io/klog`
- `reflect`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/controller`
- `sigs.k8s.io/controller-runtime/pkg/handler`
- `sigs.k8s.io/controller-runtime/pkg/manager`
- `sigs.k8s.io/controller-runtime/pkg/reconcile`
- `sigs.k8s.io/controller-runtime/pkg/source`
- `sort`
- `strconv`
- `strings`
- `time`

## Capabilities

- **`package`** name(s): **hibernation**.
- Go **`import`** edges listed below (49 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/hibernation`.

## Understanding Score

0.0
