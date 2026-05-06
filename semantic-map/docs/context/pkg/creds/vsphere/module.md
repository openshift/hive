# Module atlas

## Responsibility

Implements vSphere credential extraction for Hive install/uninstall jobs. Supports both the newer multi-vcenter credential format (JSON/YAML array of VCenters in the secret) and the legacy flat username/password format. Projects certificates to the filesystem and installs CA bundles. Only loads credentials when metadata is non-nil (provisioning skips this; cleanup uses it).

## Public Interface/API

- `func ConfigureCreds(c client.Client, metadata *installertypes.ClusterMetadata)` -- loads creds and certs secrets, populates metadata.VSphere.VCenters with credentials (supports both multi-vcenter YAML and flat username/password shapes), projects certs to filesystem, installs CA bundles

## Internal Dependencies

- `github.com/openshift/hive/contrib/pkg/utils` -- LoadSecretOrDie, ProjectToDir, InstallCerts
- `github.com/openshift/hive/pkg/constants` -- secret key names (VSphereVCentersSecretKey, UsernameSecretKey, PasswordSecretKey), certificate directory paths
- `github.com/openshift/installer/pkg/types` -- ClusterMetadata type
- `github.com/openshift/installer/pkg/types/vsphere` -- VCenters, Metadata types
- `sigs.k8s.io/yaml` -- YAML unmarshaling for multi-vcenter format
- `sigs.k8s.io/controller-runtime/pkg/client` -- Kubernetes client

## Capabilities

- Supports two credential secret shapes: multi-vcenter YAML array and flat username/password
- For flat credentials, copies the same username/password across all VCenters in metadata
- Fatally exits if no VCenters are available after credential loading
- Skips credential loading when metadata is nil (install path)
- Always loads certificates secret and installs platform-specific and proxy CA bundles

## Understanding Score

0.90
