# Module atlas

## Responsibility

Generates Kubernetes Jobs for resolving installer images from a release image, and provides a CLI command (`update-installer-image`) to update the installer/CLI image and release version on a ClusterDeployment's status.

## Public Interface/API

- `GenerateImageSetJob(cd, releaseImage, serviceAccountName, httpProxy, httpsProxy, noProxy, sharedPodConfig) *batchv1.Job` -- creates a Job that extracts image-references from a release image and updates the ClusterDeployment
- `GetImageSetJobName(cdName string) string` -- deterministic job name for a ClusterDeployment
- `ImagesetJobLabel` const -- label key `"hive.openshift.io/imageset"`
- `NewUpdateInstallerImageCommand() *cobra.Command` -- cobra command for `hiveutil update-installer-image`
- `UpdateInstallerImageOptions` struct -- options for the update-installer-image command (ClusterDeploymentName, ClusterDeploymentNamespace, WorkDir, LogLevel)
  - `Complete() error`, `Validate() error`, `Run() error`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`, `github.com/openshift/hive/apis/helpers`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/images`, `github.com/openshift/hive/pkg/controller/utils`
- `github.com/openshift/hive/pkg/util/scheme`
- `github.com/openshift/hive/contrib/pkg/utils`
- `github.com/openshift/api/image/v1`
- `github.com/spf13/cobra`
- `sigs.k8s.io/controller-runtime/pkg/client`, `sigs.k8s.io/controller-runtime/pkg/manager`
- `k8s.io/api/batch/v1`, `k8s.io/api/core/v1`

## Capabilities

- Generates a two-container Job (init: release image, main: hiveutil) to resolve installer image from release payload
- Parses `image-references` (ImageStream) and `release-metadata` (cincinnati-metadata) from release payload
- Resolves installer image, CLI image, and release version; updates ClusterDeployment status
- Supports installer image override via annotation and spec field
- Sets InstallerImageResolutionFailedCondition on success/failure

## Understanding Score

0.9
