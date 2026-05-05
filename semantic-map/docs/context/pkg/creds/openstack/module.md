# Module atlas

## Responsibility

Loads and configures OpenStack credentials for Hive install/uninstall jobs. Reads `clouds.yaml` from a specified file, `~/.config/openstack/clouds.yaml`, or `/etc/openstack/clouds.yaml`. Also projects credentials and certificates from Kubernetes secrets onto the filesystem.

## Public Interface/API

- `GetCreds(credsFile string) ([]byte, error)` -- Reads OpenStack credentials from the specified file, `~/.config/openstack/clouds.yaml`, or `/etc/openstack`. Returns raw file bytes. Searches fallback paths if no file is specified.
- `ConfigureCreds(c client.Client, metadata *installertypes.ClusterMetadata)` -- Projects the credentials secret to `constants.OpenStackCredentialsDir` and the certificates secret to `constants.OpenStackCertificatesDir`. Installs OpenStack-specific and proxy trusted CA bundles.

## Internal Dependencies

- `github.com/openshift/hive/contrib/pkg/utils` -- `LoadSecretOrDie`, `ProjectToDir`, `InstallCerts`
- `github.com/openshift/hive/pkg/constants` -- `OpenStackCredentialsName`, `OpenStackCredentialsDir`, `OpenStackCertificatesDir`
- `github.com/openshift/installer/pkg/types` -- `ClusterMetadata` type
- `k8s.io/client-go/util/homedir` -- resolves home directory for default credentials path
- `sigs.k8s.io/controller-runtime/pkg/client` -- Kubernetes client for secret access

## Capabilities

- **`package`** name(s): **openstack**.
- Single-file package (`openstack.go`).
- Handles both credentials and certificates (two separate secrets).
- Uses a multi-path fallback search for `clouds.yaml` in `GetCreds`.
- Package ID(s): `github.com/openshift/hive/pkg/creds/openstack`.

## Understanding Score

0.85
