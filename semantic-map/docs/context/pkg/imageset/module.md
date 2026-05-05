# Module: pkg/imageset

## Responsibility

Resolves the installer and CLI container images from an OpenShift release payload for use during cluster provisioning. Provides two capabilities: (1) generating a Kubernetes Job that extracts `image-references` and `release-metadata` from a release image and invokes the `hiveutil update-installer-image` command, and (2) the `update-installer-image` cobra command itself, which parses those extracted artifacts, locates the `baremetal-installer` and `cli` image tags, determines the release version, and patches the ClusterDeployment status with `InstallerImage`, `CLIImage`, and `InstallVersion`.

## Public Interface/API

- `ImagesetJobLabel` -- constant label `"hive.openshift.io/imageset"` applied to imageset jobs for counting/selection.
- `GenerateImageSetJob(cd, releaseImage, serviceAccountName, httpProxy, httpsProxy, noProxy, sharedPodConfig) *batchv1.Job` -- creates a two-container (init + hiveutil) Job that copies release manifests into a shared volume and runs `update-installer-image`.
- `GetImageSetJobName(cdName string) string` -- returns the deterministic job name for a ClusterDeployment using `apihelpers.GetResourceName`.
- `NewUpdateInstallerImageCommand() *cobra.Command` -- returns the `update-installer-image` CLI command.
- `UpdateInstallerImageOptions` -- struct holding ClusterDeployment name/namespace, work directory, log level, and an internal controller-runtime client.
  - `Complete() error` -- initializes the logger and kube client from kubeconfig.
  - `Validate() error` -- checks required flags and that the work directory contains `image-references`.
  - `Run() error` -- reads `image-references` ImageStream and `release-metadata`, resolves installer/CLI images, extracts release version, and updates ClusterDeployment status. Sets `InstallerImageResolutionFailedCondition` on error.

## Internal Dependencies

Key Hive-internal imports:
- `apis/helpers` -- resource name generation
- `apis/hive/v1` -- ClusterDeployment, condition types
- `pkg/constants` -- label names, pull secret names, annotation keys (e.g. `OverrideInstallerImageNameAnnotation`, `CopyCLIImageDomainFromInstallerImage`)
- `pkg/controller/images` -- `GetHiveImage()`, `GetHiveImagePullPolicy()`
- `pkg/controller/utils` -- `SetProxyEnvVars`, `SetClusterDeploymentCondition`, `AddLogFieldsEnvVar`
- `pkg/util/scheme` -- shared scheme for the controller-runtime client
- `contrib/pkg/utils` -- `NewLogger`

External:
- `github.com/openshift/api/image/v1` -- `ImageStream` type for parsing `image-references`
- `sigs.k8s.io/controller-runtime/pkg/client` -- kube API interaction
- `github.com/spf13/cobra` -- CLI command wiring

## Capabilities

- **Package**: `imageset`
- Two source files: `generate.go` (job generation), `updateinstaller.go` (cobra command + image resolution logic).
- 29 unique import paths.

## Understanding Score

0.85
