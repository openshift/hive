# Module atlas

## Responsibility

Implements GCP credential extraction and environment configuration for Hive install/uninstall jobs. Reads GCP service account JSON credentials from files or environment variables, projects creds secret to a directory, and sets the `GOOGLE_CREDENTIALS` environment variable.

## Public Interface/API

- `func GetCreds(credsFile string) ([]byte, error)` -- reads GCP credentials from specified file, env var `GOOGLE_CREDENTIALS`, or default `~/.gcp/osServiceAccount.json`
- `func ConfigureCreds(c client.Client, metadata *installertypes.ClusterMetadata)` -- loads creds secret, projects to directory, sets GOOGLE_CREDENTIALS env var, installs CA certs

## Internal Dependencies

- `github.com/openshift/hive/contrib/pkg/utils` -- LoadSecretOrDie, ProjectToDir, InstallCerts
- `github.com/openshift/hive/pkg/constants` -- credential file names, directory paths
- `github.com/openshift/installer/pkg/types` -- ClusterMetadata type
- `k8s.io/client-go/util/homedir` -- home directory resolution
- `sigs.k8s.io/controller-runtime/pkg/client` -- Kubernetes client

## Capabilities

- Reads GCP credentials with fallback order: explicit file, env var, default home path
- Projects secret data to filesystem directory for installer consumption
- Sets GCP auth environment variable
- Installs cluster proxy trusted CA bundle

## Understanding Score

0.90
