# Module atlas

## Responsibility

Generates Kubernetes Jobs and PodSpecs for provisioning (installing) and deprovisioning (uninstalling) OpenShift clusters across all supported cloud platforms (AWS, Azure, GCP, OpenStack, VSphere, IBMCloud, Nutanix, BareMetal). Also handles AWS AssumeRole credential configuration.

## Public Interface/API

- `GenerateInstallerJob(provision *hivev1.ClusterProvision, sharedPodConfig) (*batchv1.Job, error)` -- creates install job from a ClusterProvision
- `InstallerPodSpec(cd, provisionName, releaseImage, serviceAccountName, httpProxy, httpsProxy, noProxy, extraEnvVars) (*corev1.PodSpec, error)` -- builds pod spec with init containers (hive, cli, installer) and platform-specific credential mounts
- `GenerateUninstallerJobForDeprovision(req, serviceAccountName, httpProxy, httpsProxy, noProxy, extraEnvVars, sharedPodConfig, rLog) (*batchv1.Job, error)` -- creates deprovision job dispatching to platform-specific completers
- `GetInstallJobName(provision *hivev1.ClusterProvision) string` -- deterministic install job name
- `GetUninstallJobName(name string) string` -- deterministic uninstall job name
- `CopyAWSServiceProviderSecret(client, destNamespace, envVars, owner, scheme) error` -- copies AWS service provider secret to target namespace
- `AWSAssumeRoleConfig(client, role, secretName, secretNamespace, owner, scheme) error` -- creates/updates AWS AssumeRole credentials config secret
- `AWSAssumeRoleSecretName(secretPrefix string) string` -- deterministic secret name for AssumeRole config

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`, `github.com/openshift/hive/apis/hive/v1/aws`
- `github.com/openshift/hive/apis/helpers`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/images`, `github.com/openshift/hive/pkg/controller/utils`
- `sigs.k8s.io/controller-runtime/pkg/client`, `controllerutil`
- `k8s.io/api/batch/v1`, `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/api/resource`, `k8s.io/utils/ptr`

## Capabilities

- Multi-platform install job generation with platform-specific credential mounts, env vars, and volumes
- Multi-platform deprovision job generation with legacy and metadata-json-based flows
- AWS AssumeRole credential file assembly and secret management
- Proxy, trusted CA bundle, and image pull secret injection into pod specs
- Support for additional manifests via ConfigMap or Secret, SSH keys, bound SA signing keys
- Fake cluster install mode for testing

## Understanding Score

0.9
