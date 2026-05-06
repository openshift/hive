# Module atlas

## Responsibility

Implements Nutanix credential extraction for Hive install/uninstall jobs. Reads username and password from a Kubernetes secret, sets corresponding environment variables, populates ClusterMetadata with credentials, and installs Nutanix-specific and proxy CA certificates.

## Public Interface/API

- `func ConfigureCreds(c client.Client, metadata *installertypes.ClusterMetadata)` -- loads creds and certs secrets, sets `NUTANIX_USERNAME` and `NUTANIX_PASSWORD` env vars, populates metadata, projects certs to filesystem, installs CA bundles

## Internal Dependencies

- `github.com/openshift/hive/contrib/pkg/utils` -- LoadSecretOrDie, ProjectToDir, InstallCerts
- `github.com/openshift/hive/pkg/constants` -- secret key names, env var names, certificate directory paths
- `github.com/openshift/installer/pkg/types` -- ClusterMetadata type
- `github.com/openshift/installer/pkg/types/nutanix` -- Nutanix Metadata type
- `sigs.k8s.io/controller-runtime/pkg/client` -- Kubernetes client

## Capabilities

- Extracts username and password from creds secret
- Sets Nutanix-specific environment variables
- Populates installer ClusterMetadata with credentials (spoofs metadata if nil)
- Loads and installs Nutanix-specific certificates from a separate secret
- Installs cluster proxy trusted CA bundle

## Understanding Score

0.90
