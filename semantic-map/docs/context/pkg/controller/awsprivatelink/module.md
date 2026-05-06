<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/awsprivatelink/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `Add` — Add creates a new AWSPrivateLink Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller and Start it when the Manager is Started.
- `AddToManager` — AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
- `ControllerName`
- `ReadAWSPrivateLinkControllerConfigFile` — ReadAWSPrivateLinkControllerConfigFile reads the configuration from the env and unmarshals. If the env is set to a file but that file doesn't exist it returns a zero-value configu…
- `ReconcileAWSPrivateLink` — ReconcileAWSPrivateLink reconciles a PrivateLink for clusterdeployment object
- `ReconcileAWSPrivateLink.Reconcile` — Reconcile reconciles PrivateLink for ClusterDeployment.

## Internal Dependencies

- `context`
- `encoding/json`
- `github.com/aws/aws-sdk-go-v2/aws`
- `github.com/aws/aws-sdk-go-v2/service/ec2`
- `github.com/aws/aws-sdk-go-v2/service/ec2/types`
- `github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2`
- `github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types`
- `github.com/aws/aws-sdk-go-v2/service/route53`
- `github.com/aws/aws-sdk-go-v2/service/route53/types`
- `github.com/aws/aws-sdk-go-v2/service/sts`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/apis/hive/v1/aws`
- `github.com/openshift/hive/pkg/awsclient`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/metrics`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/pkg/errors`
- `github.com/sirupsen/logrus`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/api/errors`
- `k8s.io/apimachinery/pkg/types`
- `k8s.io/apimachinery/pkg/util/sets`
- `k8s.io/apimachinery/pkg/util/wait`
- `k8s.io/client-go/util/flowcontrol`
- `k8s.io/client-go/util/retry`
- `k8s.io/client-go/util/workqueue`
- `net/url`
- `os`
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

- **`package`** name(s): **awsprivatelink**.
- Go **`import`** edges listed below (38 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/awsprivatelink`.

## Understanding Score

0.0
