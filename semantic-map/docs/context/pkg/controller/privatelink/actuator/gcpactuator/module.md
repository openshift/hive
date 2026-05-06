<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/privatelink/actuator/gcpactuator/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `GCPLinkActuator`
- `GCPLinkActuator.Cleanup` — Cleanup is the actuator interface for cleaning up the cloud resources.
- `GCPLinkActuator.CleanupRequired` — CleanupRequired is the actuator interface for determining if cleanup is required.
- `GCPLinkActuator.Reconcile` — Reconcile is the actuator interface for reconciling the cloud resources.
- `GCPLinkActuator.ShouldSync` — ShouldSync is the actuator interface to determine if there are changes that need to be made.

## Internal Dependencies

- `context`
- `fmt`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/apis/hive/v1/gcp`
- `github.com/openshift/hive/pkg/controller/privatelink/actuator`
- `github.com/openshift/hive/pkg/controller/privatelink/conditions`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/openshift/hive/pkg/gcpclient`
- `github.com/pkg/errors`
- `github.com/sirupsen/logrus`
- `google.golang.org/api/compute/v1`
- `google.golang.org/api/googleapi`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/types`
- `k8s.io/apimachinery/pkg/util/wait`
- `k8s.io/client-go/util/retry`
- `net/http`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/reconcile`
- `sort`
- `time`

## Capabilities

- **`package`** name(s): **gcpactuator**.
- Go **`import`** edges listed below (21 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/privatelink/actuator/gcpactuator`.

## Understanding Score

0.0
