# Module atlas

## Responsibility

Loads and configures Azure credentials for Hive install/uninstall jobs. Reads credentials from a JSON service account file, the `AZURE_AUTH_LOCATION` environment variable, or a default path (`~/.azure/osServiceAccount.json`). Also projects credentials from a Kubernetes secret onto the filesystem and sets the Azure credentials environment variable.

## Public Interface/API

- `GetCreds(credsFile string) ([]byte, error)` -- Reads Azure credentials from the specified file, the `AZURE_AUTH_LOCATION` env var, or the default file. Returns raw credential bytes.
- `ConfigureCreds(c client.Client, metadata *installertypes.ClusterMetadata)` -- Loads a Kubernetes secret and projects it to `constants.AzureCredentialsDir`, sets `AZURE_AUTH_LOCATION` env var, and installs proxy trusted CA bundles.

## Internal Dependencies

- `github.com/openshift/hive/contrib/pkg/utils` -- `LoadSecretOrDie`, `ProjectToDir`, `InstallCerts`
- `github.com/openshift/hive/pkg/constants` -- `AzureCredentialsName`, `AzureCredentialsEnvVar`, `AzureCredentialsDir`
- `github.com/openshift/installer/pkg/types` -- `ClusterMetadata` type
- `k8s.io/client-go/util/homedir` -- resolves home directory for default credentials path
- `sigs.k8s.io/controller-runtime/pkg/client` -- Kubernetes client for secret access

## Capabilities

- **`package`** name(s): **azure**.
- Single-file package (`azure.go`).
- Package ID(s): `github.com/openshift/hive/pkg/creds/azure`.

## Understanding Score

0.85
