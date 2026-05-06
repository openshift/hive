<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/privatelink/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `Add` — Add creates a new PrivateLink Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller and Start it when the Manager is Started.
- `AddToManager` — AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
- `ControllerName`
- `CreateActuator` — CreateActuator creates an actuator based on the cloud platform and actuator type.
- `PrivateLink` — PrivateLink is the PrivateLink to be reconciled by the PrivateLinkReconciler
- `PrivateLink.Reconcile`
- `PrivateLinkReconciler` — PrivateLinkReconciler reconciles a PrivateLink for clusterdeployment object
- `PrivateLinkReconciler.Reconcile`
- `ReadPrivateLinkControllerConfigFile` — ReadPrivateLinkControllerConfigFile reads the configuration from the env and unmarshals. If the env is set to a file but that file doesn't exist it returns a zero-value configurat…

## Internal Dependencies

- `context`
- `encoding/json`
- `fmt`
- `github.com/openshift/api/config/v1`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/metrics`
- `github.com/openshift/hive/pkg/controller/privatelink/actuator`
- `github.com/openshift/hive/pkg/controller/privatelink/actuator/awsactuator`
- `github.com/openshift/hive/pkg/controller/privatelink/actuator/gcpactuator`
- `github.com/openshift/hive/pkg/controller/privatelink/conditions`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/pkg/errors`
- `github.com/sirupsen/logrus`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/api/errors`
- `k8s.io/apimachinery/pkg/types`
- `k8s.io/apimachinery/pkg/util/wait`
- `k8s.io/client-go/util/flowcontrol`
- `k8s.io/client-go/util/retry`
- `k8s.io/client-go/util/workqueue`
- `os`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/controller`
- `sigs.k8s.io/controller-runtime/pkg/handler`
- `sigs.k8s.io/controller-runtime/pkg/manager`
- `sigs.k8s.io/controller-runtime/pkg/reconcile`
- `sigs.k8s.io/controller-runtime/pkg/source`
- `strconv`
- `time`

## Capabilities

- **`package`** name(s): **privatelink**.
- Go **`import`** edges listed below (30 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/privatelink`.

## Understanding Score

0.0
