# Module atlas

## Responsibility

Loads and configures GCP credentials for Hive install/uninstall jobs. Reads service account JSON from a specified file, the `GOOGLE_CREDENTIALS` environment variable, or a default path (`~/.gcp/osServiceAccount.json`). Also projects credentials from a Kubernetes secret onto the filesystem and sets the `GOOGLE_CREDENTIALS` environment variable.

## Public Interface/API

- `GetCreds(credsFile string) ([]byte, error)` -- Reads GCP service account credentials from the specified file, `GOOGLE_CREDENTIALS` env var, or the default file. Returns raw credential bytes.
- `ConfigureCreds(c client.Client, metadata *installertypes.ClusterMetadata)` -- Loads a Kubernetes secret and projects it to `constants.GCPCredentialsDir`, sets `GOOGLE_CREDENTIALS` env var, and installs proxy trusted CA bundles.

## Internal Dependencies

- `github.com/openshift/hive/contrib/pkg/utils` -- `LoadSecretOrDie`, `ProjectToDir`, `InstallCerts`
- `github.com/openshift/hive/pkg/constants` -- `GCPCredentialsName`, `GCPCredentialsDir`
- `github.com/openshift/installer/pkg/types` -- `ClusterMetadata` type
- `k8s.io/client-go/util/homedir` -- resolves home directory for default credentials path
- `sigs.k8s.io/controller-runtime/pkg/client` -- Kubernetes client for secret access

## Capabilities

- **`package`** name(s): **gcp**.
- Single-file package (`gcp.go`).
- Package ID(s): `github.com/openshift/hive/pkg/creds/gcp`.

## Understanding Score

0.85
