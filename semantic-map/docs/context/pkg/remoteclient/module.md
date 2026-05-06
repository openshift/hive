<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/remoteclient/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `Builder` — Builder is used to build API clients to the remote cluster
- `ConnectToRemoteCluster` — ConnectToRemoteCluster connects to a remote cluster using the specified builder. If the ClusterDeployment is marked as unreachable, then no connection will be made. If there are p…
- `InitialURL` — InitialURL returns the initial API URL for the ClusterDeployment.
- `IsPrimaryURLActive` — IsPrimaryURLActive returns true if the remote cluster is reachable via the primary API URL.
- `SetUnreachableCondition` — SetUnreachableCondition sets the Unreachable condition on the ClusterDeployment based on the specified error encountered when attempting to connect to the remote cluster.
- `Unreachable` — Unreachable returns true if Hive has not been able to reach the remote cluster. Note that this function will not attempt to reach the remote cluster. It only checks the current co…

## Internal Dependencies

- `context`
- `fmt`
- `github.com/openshift/api/config/v1`
- `github.com/openshift/api/route/v1`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/openshift/hive/pkg/test/fake`
- `github.com/openshift/hive/pkg/util/scheme`
- `github.com/pkg/errors`
- `github.com/sirupsen/logrus`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/apimachinery/pkg/util/net`
- `k8s.io/client-go/discovery`
- `k8s.io/client-go/dynamic`
- `k8s.io/client-go/kubernetes`
- `k8s.io/client-go/rest`
- `k8s.io/client-go/restmapper`
- `net`
- `net/http`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `time`

## Capabilities

- **`package`** name(s): **remoteclient**.
- Go **`import`** edges listed below (23 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/remoteclient`.

## Understanding Score

0.0
