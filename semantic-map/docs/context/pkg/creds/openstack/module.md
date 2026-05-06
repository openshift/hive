# Module atlas

## Responsibility

Implements OpenStack credential extraction and environment configuration for Hive install/uninstall jobs. Reads OpenStack clouds.yaml credentials from files, projects creds and certs secrets to filesystem directories, and installs CA certificates.

## Public Interface/API

- `func GetCreds(credsFile string) ([]byte, error)` -- reads OpenStack credentials from specified file, `~/.config/openstack/clouds.yaml`, or `/etc/openstack`
- `func ConfigureCreds(c client.Client, metadata *installertypes.ClusterMetadata)` -- loads creds and certs secrets, projects to directories, installs CA certificates

## Internal Dependencies

- `github.com/openshift/hive/contrib/pkg/utils` -- LoadSecretOrDie, ProjectToDir, InstallCerts
- `github.com/openshift/hive/pkg/constants` -- credential file names, directory paths
- `github.com/openshift/installer/pkg/types` -- ClusterMetadata type
- `k8s.io/client-go/util/homedir` -- home directory resolution
- `sigs.k8s.io/controller-runtime/pkg/client` -- Kubernetes client

## Capabilities

- Reads OpenStack credentials with fallback order: explicit file, home directory path, /etc/openstack
- Projects creds secret to OpenStack credentials directory
- Projects certs secret to certificates directory and installs them
- Installs cluster proxy trusted CA bundle

## Understanding Score

0.90
