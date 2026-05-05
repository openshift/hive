# Module atlas

## Responsibility

Loads and configures AWS credentials for Hive install/uninstall jobs. Supports reading credentials from an INI-format credentials file, environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`), or a default file (`~/.aws/credentials`). Also loads credentials from Kubernetes secrets and sets the corresponding environment variables and config files for the AWS SDK.

## Public Interface/API

- `GetAWSCreds(credsFile, defaultCredsFile string) (string, string, error)` -- Reads AWS access key ID and secret access key from a credentials file, environment variables, or a default credentials file. Returns `(accessKeyID, secretAccessKey, error)`.
- `ConfigureCreds(c client.Client, metadata *installertypes.ClusterMetadata)` -- Loads a Kubernetes secret (identified by `CREDS_SECRET_NAME` env var), sets `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_CONFIG_FILE`, and `AWS_SDK_LOAD_CONFIG` environment variables. Rejects `credential_process` in AWS config for security. Installs proxy trusted CA bundles.

## Internal Dependencies

- `github.com/openshift/hive/contrib/pkg/utils` -- `LoadSecretOrDie`, `ProjectToDir`, `InstallCerts`, `ProjectOnlyTheseKeys` helpers
- `github.com/openshift/hive/pkg/awsclient` -- `ContainsCredentialProcess` to detect and forbid insecure config
- `github.com/openshift/hive/pkg/constants` -- secret key names (`AWSAccessKeyIDSecretKey`, etc.) and mount paths
- `github.com/openshift/installer/pkg/types` -- `ClusterMetadata` type
- `gopkg.in/ini.v1` -- INI file parsing for AWS credentials files
- `sigs.k8s.io/controller-runtime/pkg/client` -- Kubernetes client for secret access

## Capabilities

- **`package`** name(s): **aws**.
- Single-file package (`aws.go`).
- Security: blocks `credential_process` directives in AWS config via `awsConfigForbidCredentialProcess` filter.
- Package ID(s): `github.com/openshift/hive/pkg/creds/aws`.

## Understanding Score

0.85
