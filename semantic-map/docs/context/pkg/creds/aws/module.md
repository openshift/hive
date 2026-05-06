# Module atlas

## Responsibility

Implements AWS credential extraction and environment configuration for Hive install/uninstall jobs. Reads AWS access keys from secrets, environment variables, or credential files, sets `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_CONFIG_FILE`, and `AWS_SDK_LOAD_CONFIG` environment variables, and installs trusted CA bundles. Validates that AWS config files do not contain `credential_process` directives.

## Public Interface/API

- `func GetAWSCreds(credsFile, defaultCredsFile string) (string, string, error)` -- reads AWS access key ID and secret access key from a credentials file, env vars, or default file
- `func ConfigureCreds(c client.Client, metadata *installertypes.ClusterMetadata)` -- loads creds secret, sets AWS env vars, writes config file, installs CA certs

## Internal Dependencies

- `github.com/openshift/hive/contrib/pkg/utils` -- LoadSecretOrDie, ProjectToDir, ProjectOnlyTheseKeys, InstallCerts
- `github.com/openshift/hive/pkg/awsclient` -- ContainsCredentialProcess validation
- `github.com/openshift/hive/pkg/constants` -- secret key names, mount paths
- `github.com/openshift/installer/pkg/types` -- ClusterMetadata type
- `gopkg.in/ini.v1` -- INI file parsing for AWS credentials
- `sigs.k8s.io/controller-runtime/pkg/client` -- Kubernetes client

## Capabilities

- Reads AWS credentials from INI-format files with `[default]` section
- Falls back through: explicit file path, environment variables, default file path
- Forbids `credential_process` in AWS config files for security
- Sets environment variables for AWS SDK usage in child processes
- Installs cluster proxy trusted CA bundle

## Understanding Score

0.90
