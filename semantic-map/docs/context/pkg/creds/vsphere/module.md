# Module atlas

## Responsibility

Loads and configures vSphere credentials for Hive install/uninstall jobs. Supports two credential formats: post-zonal (array of vCenter JSON objects unmarshalled into `metadata.VSphere.VCenters`) and legacy flat (username/password copied across all pre-populated vCenters). Also installs vSphere-specific and proxy CA certificates.

## Public Interface/API

- `ConfigureCreds(c client.Client, metadata *installertypes.ClusterMetadata)` -- Loads credentials and certificates from Kubernetes secrets. If metadata is non-nil (cleanup path), reads the `CREDS_SECRET_NAME` secret: unmarshals `constants.VSphereVCentersSecretKey` for zonal creds, or falls back to copying flat `username`/`password` across all pre-populated vCenters. Fatals if no vCenters are present after loading. Projects certificates from `CERTS_SECRET_NAME` and installs both vSphere and proxy CA bundles. Skips credential loading when metadata is nil (provisioning path, as creds are embedded in install-config).

## Internal Dependencies

- `github.com/openshift/hive/contrib/pkg/utils` -- `LoadSecretOrDie`, `ProjectToDir`, `InstallCerts`
- `github.com/openshift/hive/pkg/constants` -- `VSphereVCentersSecretKey`, `UsernameSecretKey`, `PasswordSecretKey`, `VSphereCertificatesDir`
- `github.com/openshift/installer/pkg/types` -- `ClusterMetadata` type
- `github.com/openshift/installer/pkg/types/vsphere` -- `Metadata` struct for VCenter data
- `sigs.k8s.io/yaml` -- YAML/JSON unmarshalling of vCenter credentials
- `sigs.k8s.io/controller-runtime/pkg/client` -- Kubernetes client for secret access

## Capabilities

- **`package`** name(s): **vsphere**.
- Single-file package (`vsphere.go`).
- Two credential format paths: post-zonal JSON array and legacy flat username/password.
- Handles both credentials and certificates (two separate secrets).
- Conditionally skips credential loading based on nil metadata (provisioning vs cleanup).
- Package ID(s): `github.com/openshift/hive/pkg/creds/vsphere`.

## Understanding Score

0.85
