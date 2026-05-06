<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/privatelink/actuator/awsactuator/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `AWSHubActuator`
- `AWSHubActuator.Cleanup` — Cleanup is the actuator interface for cleaning up the cloud resources.
- `AWSHubActuator.CleanupRequired` — CleanupRequired is the actuator interface for determining if cleanup is required.
- `AWSHubActuator.Reconcile` — Reconcile is the actuator interface for reconciling the cloud resources.
- `AWSHubActuator.ReconcileHostedZoneRecords`
- `AWSHubActuator.ShouldSync` — ShouldSync is the actuator interface to determine if there are changes that need to be made.
- `AssumeRole`
- `ReadAWSPrivateLinkControllerConfigFile` — ReadAWSPrivateLinkControllerConfigFile reads the configuration from the env and unmarshals. If the env is set to a file but that file doesn't exist it returns a zero-value configu…

## Internal Dependencies

- `context`
- `encoding/json`
- `github.com/aws/aws-sdk-go-v2/aws`
- `github.com/aws/aws-sdk-go-v2/service/ec2`
- `github.com/aws/aws-sdk-go-v2/service/ec2/types`
- `github.com/aws/aws-sdk-go-v2/service/route53`
- `github.com/aws/aws-sdk-go-v2/service/route53/types`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/apis/hive/v1/aws`
- `github.com/openshift/hive/pkg/awsclient`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/privatelink/actuator`
- `github.com/openshift/hive/pkg/controller/privatelink/conditions`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/pkg/errors`
- `github.com/sirupsen/logrus`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/api/errors`
- `k8s.io/apimachinery/pkg/types`
- `k8s.io/apimachinery/pkg/util/sets`
- `k8s.io/apimachinery/pkg/util/wait`
- `k8s.io/client-go/rest`
- `k8s.io/client-go/tools/clientcmd`
- `k8s.io/client-go/util/retry`
- `net/url`
- `os`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/reconcile`
- `sort`
- `strings`
- `time`

## Capabilities

- **`package`** name(s): **awsactuator**.
- Go **`import`** edges listed below (31 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/privatelink/actuator/awsactuator`.

## Understanding Score

0.0
