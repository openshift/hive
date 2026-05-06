<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/imageset/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `GenerateImageSetJob` — GenerateImageSetJob creates a job to determine the installer image for a ClusterImageSet given a release image
- `GetImageSetJobName` — GetImageSetJobName returns the expected name of the imageset job for a ClusterImageSet.
- `ImagesetJobLabel`
- `NewUpdateInstallerImageCommand` — NewUpdateInstallerImageCommand returns a command to update the installer image on a cluster deployment.
- `UpdateInstallerImageOptions` — UpdateInstallerImageOptions contains options for running the command to update the installer image
- `UpdateInstallerImageOptions.Complete` — Complete sets remaining fields on the UpdateInstallerImageOptions based on command options and arguments.
- `UpdateInstallerImageOptions.Run` — Run updates the given ClusterDeployment based on the image-references file.
- `UpdateInstallerImageOptions.Validate` — Validate ensures the given options and arguments are valid.

## Internal Dependencies

- `context`
- `encoding/json`
- `fmt`
- `github.com/openshift/api/image/v1`
- `github.com/openshift/hive/apis/helpers`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/contrib/pkg/utils`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/images`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/openshift/hive/pkg/util/scheme`
- `github.com/pkg/errors`
- `github.com/sirupsen/logrus`
- `github.com/spf13/cobra`
- `k8s.io/api/batch/v1`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/types`
- `k8s.io/client-go/rest`
- `k8s.io/client-go/tools/clientcmd`
- `os`
- `path/filepath`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/client/apiutil`
- `sigs.k8s.io/controller-runtime/pkg/manager`
- `sigs.k8s.io/yaml`
- `strconv`
- `strings`
- `time`

## Capabilities

- **`package`** name(s): **imageset**.
- Go **`import`** edges listed below (29 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/imageset`.

## Understanding Score

0.0
