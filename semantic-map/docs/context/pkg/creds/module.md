# Module atlas

## Responsibility

Root credential-configuration registry for all supported cloud platforms. Maps platform name constants to platform-specific `ConfigureCreds` functions, allowing callers to look up and invoke the correct credential-loading logic by platform string (e.g. `"aws"`, `"gcp"`).

## Public Interface/API

- `ConfigureCreds` -- `map[string]func(client.Client, *types.ClusterMetadata)` mapping platform name constants to per-platform credential configuration functions.

## Internal Dependencies

- `github.com/openshift/hive/pkg/constants` -- platform name constants used as map keys
- `github.com/openshift/hive/pkg/creds/aws` -- AWS credential configuration
- `github.com/openshift/hive/pkg/creds/azure` -- Azure credential configuration
- `github.com/openshift/hive/pkg/creds/gcp` -- GCP credential configuration
- `github.com/openshift/hive/pkg/creds/ibmcloud` -- IBM Cloud credential configuration
- `github.com/openshift/hive/pkg/creds/nutanix` -- Nutanix credential configuration
- `github.com/openshift/hive/pkg/creds/openstack` -- OpenStack credential configuration
- `github.com/openshift/hive/pkg/creds/vsphere` -- vSphere credential configuration
- `github.com/openshift/installer/pkg/types` -- `ClusterMetadata` type used in function signatures
- `sigs.k8s.io/controller-runtime/pkg/client` -- `Client` interface for loading Kubernetes secrets

## Capabilities

- **`package`** name(s): **creds**.
- Single-file package (`creds.go`) containing only the `ConfigureCreds` map variable.
- Acts as a dispatch table: callers select credential setup by platform string without importing individual cloud packages.
- Package ID(s): `github.com/openshift/hive/pkg/creds`.

## Understanding Score

0.85
