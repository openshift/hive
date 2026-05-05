# Module atlas

## Responsibility

Loads and configures Nutanix credentials for Hive install/uninstall jobs. Reads username and password from a Kubernetes secret, sets corresponding environment variables, populates the `ClusterMetadata.Nutanix` struct, and installs Nutanix-specific and proxy CA certificates.

## Public Interface/API

- `ConfigureCreds(c client.Client, metadata *installertypes.ClusterMetadata)` -- Loads credentials and certificates from Kubernetes secrets (`CREDS_SECRET_NAME`, `CERTS_SECRET_NAME`). Sets `constants.NutanixUsernameEnvVar` and `constants.NutanixPasswordEnvVar` env vars. Copies credentials into `metadata.Nutanix.Username/Password`. Projects certificate secret to `constants.NutanixCertificatesDir` and installs certs. Spoofs empty metadata if nil (for legacy code paths).

## Internal Dependencies

- `github.com/openshift/hive/contrib/pkg/utils` -- `LoadSecretOrDie`, `ProjectToDir`, `InstallCerts`
- `github.com/openshift/hive/pkg/constants` -- secret keys (`UsernameSecretKey`, `PasswordSecretKey`), env vars, and certificate directories
- `github.com/openshift/installer/pkg/types` -- `ClusterMetadata`, `ClusterPlatformMetadata`
- `github.com/openshift/installer/pkg/types/nutanix` -- `Metadata` struct for platform-specific metadata
- `sigs.k8s.io/controller-runtime/pkg/client` -- Kubernetes client for secret access

## Capabilities

- **`package`** name(s): **nutanix**.
- Single-file package (`nutanix.go`).
- Handles both credentials and certificates (two separate secrets).
- Creates a default `Nutanix` metadata struct if caller provides nil metadata (legacy support).
- Package ID(s): `github.com/openshift/hive/pkg/creds/nutanix`.

## Understanding Score

0.85
