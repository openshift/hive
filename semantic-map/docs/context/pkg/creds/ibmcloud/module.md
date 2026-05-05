# Module atlas

## Responsibility

Loads and configures IBM Cloud credentials for Hive install/uninstall jobs. Reads the API key from a Kubernetes secret and sets the `IC_API_KEY` (or equivalent) environment variable for IBM Cloud SDK consumption.

## Public Interface/API

- `ConfigureCreds(c client.Client, metadata *installertypes.ClusterMetadata)` -- Loads a Kubernetes secret, extracts the IBM Cloud API key from `constants.IBMCloudAPIKeySecretKey`, sets the `constants.IBMCloudAPIKeyEnvVar` environment variable, and installs proxy trusted CA bundles.

## Internal Dependencies

- `github.com/openshift/hive/contrib/pkg/utils` -- `LoadSecretOrDie`, `InstallCerts`
- `github.com/openshift/hive/pkg/constants` -- `IBMCloudAPIKeySecretKey`, `IBMCloudAPIKeyEnvVar`, `TrustedCABundleDir`
- `github.com/openshift/installer/pkg/types` -- `ClusterMetadata` type
- `sigs.k8s.io/controller-runtime/pkg/client` -- Kubernetes client for secret access

## Capabilities

- **`package`** name(s): **ibmcloud**.
- Single-file package (`ibmcloud.go`).
- Simplest credential provider -- only sets a single API key env var (no file projection).
- Package ID(s): `github.com/openshift/hive/pkg/creds/ibmcloud`.

## Understanding Score

0.85
