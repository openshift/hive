# Module: pkg/install

## Responsibility

Generates Kubernetes Job specifications for provisioning and deprovisioning OpenShift clusters. This is a pure job-spec factory -- it does not run jobs itself. It produces complete `batchv1.Job` objects with all necessary containers, init containers, volumes, environment variables, and cloud-credential mounts for each supported platform (AWS, Azure, GCP, OpenStack, vSphere, IBM Cloud, Nutanix, BareMetal). It also handles AWS AssumeRole credential configuration and AWS service provider secret copying.

## Public Interface/API

- `GenerateInstallerJob(provision *hivev1.ClusterProvision, sharedPodConfig) (*batchv1.Job, error)` -- wraps the ClusterProvision's pre-built PodSpec into a Job with proper labels, deadlines (3h provision), and backoff.
- `InstallerPodSpec(cd, provisionName, releaseImage, serviceAccountName, httpProxy, httpsProxy, noProxy, extraEnvVars) (*corev1.PodSpec, error)` -- builds the full installer PodSpec including: hive init container (copies hiveutil binaries), CLI init container (copies `oc`), installer main container that runs `hiveutil install-manager`. Handles per-platform credential/cert volume mounts, manifests, SSH keys, bound SA signing keys, fake cluster mode.
- `GenerateUninstallerJobForDeprovision(req, serviceAccountName, httpProxy, httpsProxy, noProxy, extraEnvVars, sharedPodConfig, rLog) (*batchv1.Job, error)` -- builds a deprovision Job. Dispatches to platform-specific `complete*DeprovisionJob` functions that set up the right `hiveutil deprovision` or `hiveutil aws-tag-deprovision` arguments and cloud-specific volumes. Supports both legacy (positional args) and metadata-json-based deprovision flows. 1h deadline.
- `GetInstallJobName(provision) string` -- deterministic install job name.
- `GetUninstallJobName(name string) string` -- deterministic uninstall job name.
- `CopyAWSServiceProviderSecret(client, destNamespace, envVars, owner, scheme) error` -- copies the AWS service provider secret from hive namespace to a target namespace when the env var is configured.
- `AWSAssumeRoleConfig(client, role, secretName, secretNamespace, owner, scheme) error` -- creates or updates a Secret containing an AWS credentials file configured for AssumeRole with a source profile.
- `AWSAssumeRoleSecretName(secretPrefix string) string` -- returns the conventional secret name for assume-role config.

## Internal Dependencies

Key Hive-internal imports:
- `apis/helpers` -- resource name generation
- `apis/hive/v1`, `apis/hive/v1/aws` -- ClusterDeployment, ClusterProvision, ClusterDeprovision, AssumeRole types
- `pkg/constants` -- credential mount paths, label names, env var names, annotation keys for all platforms
- `pkg/controller/images` -- hive image and pull policy
- `pkg/controller/utils` -- proxy env vars, service provider secret name, fake cluster detection, log fields

External:
- `k8s.io/api/batch/v1`, `k8s.io/api/core/v1` -- Job, PodSpec, Secret types
- `sigs.k8s.io/controller-runtime/pkg/client`, `controllerutil` -- API client, owner references

## Capabilities

- **Package**: `install`
- Single source file: `generate.go` (~984 lines).
- Platform-specific deprovision completers: `completeAWSDeprovisionJob`, `completeAzureDeprovisionJob`, `completeGCPDeprovisionJob`, `completeOpenStackDeprovisionJob`, `completeVSphereDeprovisionJob`, `completeIBMCloudDeprovisionJob`, `completeNutanixCloudDeprovisionJob`.
- 25 unique import paths.

## Understanding Score

0.87
