<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`contrib/pkg/clusterpool/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `ClusterClaimOptions`
- `ClusterPoolOptions`
- `NewClaimClusterPoolCommand`
- `NewClusterPoolCommand` — NewClusterPoolCommand is the entrypoint to create the 'clusterpool' subcommand
- `NewCreateClusterPoolCommand`

## Internal Dependencies

- `fmt`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/contrib/pkg/utils`
- `github.com/openshift/hive/pkg/clusterresource`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/creds/aws`
- `github.com/openshift/hive/pkg/creds/azure`
- `github.com/openshift/hive/pkg/creds/gcp`
- `github.com/openshift/hive/pkg/util/scheme`
- `github.com/pkg/errors`
- `github.com/sirupsen/logrus`
- `github.com/spf13/cobra`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/api/meta`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/apimachinery/pkg/util/sets`
- `k8s.io/cli-runtime/pkg/printers`
- `k8s.io/client-go/util/homedir`
- `os`
- `path/filepath`
- `time`

## Capabilities

- **`package`** name(s): **clusterpool**.
- Go **`import`** edges listed below (22 unique path(s)).
- Package ID(s): `github.com/openshift/hive/contrib/pkg/clusterpool`.

## Understanding Score

0.0
