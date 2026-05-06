# Module atlas

## Responsibility

Implements IBM Cloud credential extraction for Hive install/uninstall jobs. Reads the IBM Cloud API key from a Kubernetes secret and sets the corresponding environment variable.

## Public Interface/API

- `func ConfigureCreds(c client.Client, metadata *installertypes.ClusterMetadata)` -- loads creds secret, sets `IC_API_KEY` env var from the `ibmcloud_api_key` secret key, installs CA certs

## Internal Dependencies

- `github.com/openshift/hive/contrib/pkg/utils` -- LoadSecretOrDie, InstallCerts
- `github.com/openshift/hive/pkg/constants` -- IBMCloudAPIKeySecretKey, IBMCloudAPIKeyEnvVar, TrustedCABundleDir
- `github.com/openshift/installer/pkg/types` -- ClusterMetadata type
- `sigs.k8s.io/controller-runtime/pkg/client` -- Kubernetes client

## Capabilities

- Extracts IBM Cloud API key from secret data and exports as environment variable
- Installs cluster proxy trusted CA bundle

## Understanding Score

0.90
