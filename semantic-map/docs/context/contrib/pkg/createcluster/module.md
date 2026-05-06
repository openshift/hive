<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`contrib/pkg/createcluster/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `NewCreateClusterCommand` — NewCreateClusterCommand creates a command that generates and applies cluster deployment artifacts.
- `Options` — Options is the set of options to generate and apply a new cluster deployment
- `Options.Complete` — Complete finishes parsing arguments for the command
- `Options.GenerateObjects` — GenerateObjects generates resources for a new cluster deployment
- `Options.Run` — Run executes the command
- `Options.Validate` — Validate ensures that option values make sense

## Internal Dependencies

- `bytes`
- `encoding/json`
- `fmt`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/apis/hive/v1/azure`
- `github.com/openshift/hive/contrib/pkg/utils`
- `github.com/openshift/hive/pkg/clusterresource`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/creds/aws`
- `github.com/openshift/hive/pkg/creds/azure`
- `github.com/openshift/hive/pkg/creds/gcp`
- `github.com/openshift/hive/pkg/creds/openstack`
- `github.com/openshift/hive/pkg/gcpclient`
- `github.com/openshift/hive/pkg/util/scheme`
- `github.com/openshift/installer/pkg/types`
- `github.com/openshift/installer/pkg/types/nutanix`
- `github.com/openshift/installer/pkg/types/vsphere`
- `github.com/openshift/installer/pkg/types/vsphere/conversion`
- `github.com/openshift/installer/pkg/validate`
- `github.com/pkg/errors`
- `github.com/sirupsen/logrus`
- `github.com/spf13/cobra`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/api/meta`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/apimachinery/pkg/util/sets`
- `k8s.io/cli-runtime/pkg/printers`
- `os`
- `os/user`
- `path/filepath`
- `strconv`
- `strings`
- `time`

## Capabilities

- **`package`** name(s): **createcluster**.
- Go **`import`** edges listed below (34 unique path(s)).
- Package ID(s): `github.com/openshift/hive/contrib/pkg/createcluster`.

## Understanding Score

0.0
