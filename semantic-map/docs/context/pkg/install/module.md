<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/install/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `AWSAssumeRoleConfig` — AWSAssumeRoleConfig creates or updates a secret with an AWS credentials file containing: - Role configuration for AssumeRole, pointing to... - A profile containing the source cred…
- `AWSAssumeRoleSecretName`
- `CopyAWSServiceProviderSecret` — CopyAWSServiceProviderSecret copies the AWS service provider secret to the dest namespace when HiveAWSServiceProviderCredentialsSecretRefEnvVar is set in envVars. The secret name …
- `GenerateInstallerJob` — GenerateInstallerJob creates a job to install an OpenShift cluster given a ClusterDeployment and an installer image.
- `GenerateUninstallerJobForDeprovision` — GenerateUninstallerJobForDeprovision generates an uninstaller job for a given deprovision request
- `GetInstallJobName` — GetInstallJobName returns the expected name of the install job for a cluster provision.
- `GetUninstallJobName` — GetUninstallJobName returns the expected name of the deprovision job for a cluster deployment.
- `InstallerPodSpec` — InstallerPodSpec generates a spec for an installer pod.

## Internal Dependencies

- `context`
- `fmt`
- `github.com/openshift/hive/apis/helpers`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/apis/hive/v1/aws`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/images`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/pkg/errors`
- `github.com/sirupsen/logrus`
- `k8s.io/api/batch/v1`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/api/errors`
- `k8s.io/apimachinery/pkg/api/resource`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/apimachinery/pkg/types`
- `k8s.io/utils/ptr`
- `os`
- `reflect`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/controller/controllerutil`
- `strconv`
- `strings`
- `time`

## Capabilities

- **`package`** name(s): **install**.
- Go **`import`** edges listed below (25 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/install`.

## Understanding Score

0.0
