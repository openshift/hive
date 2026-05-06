<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`contrib/pkg/adm/managedns/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `NewEnableManageDNSCommand` — NewEnableManageDNSCommand creates a command that generates and applies artifacts to enable managed DNS globally for the Hive cluster.
- `NewManageDNSCommand` — NewManageDNSCommand is the entrypoint to create the 'manage-dns' subcommand
- `Options` — Options is the set of options to generate and apply a new cluster deployment
- `Options.Complete` — Complete finishes parsing arguments for the command
- `Options.Run` — Run executes the command
- `Options.Validate` — Validate ensures that option values make sense

## Internal Dependencies

- `context`
- `fmt`
- `github.com/google/uuid`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/contrib/pkg/utils`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/creds/aws`
- `github.com/openshift/hive/pkg/creds/azure`
- `github.com/openshift/hive/pkg/creds/gcp`
- `github.com/openshift/hive/pkg/resource`
- `github.com/openshift/hive/pkg/util/scheme`
- `github.com/sirupsen/logrus`
- `github.com/spf13/cobra`
- `k8s.io/api/apps/v1`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/apis/meta/v1/unstructured`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/apimachinery/pkg/types`
- `k8s.io/apimachinery/pkg/watch`
- `k8s.io/client-go/tools/cache`
- `k8s.io/client-go/tools/watch`
- `k8s.io/kubectl/pkg/polymorphichelpers`
- `os/user`
- `path/filepath`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/client/config`
- `time`

## Capabilities

- **`package`** name(s): **managedns**.
- Go **`import`** edges listed below (28 unique path(s)).
- Package ID(s): `github.com/openshift/hive/contrib/pkg/adm/managedns`.

## Understanding Score

0.0
