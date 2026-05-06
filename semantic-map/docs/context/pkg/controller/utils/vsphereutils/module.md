# Module atlas

## Responsibility

Converts deprecated flat vSphere platform fields (VCenter, Datacenter, DefaultDatastore, Folder, Cluster, Network) into the newer Infrastructure-based format by delegating to the OpenShift Installer's vSphere conversion logic. This is a one-way upconversion for backward compatibility.

## Public Interface/API

- `func ConvertDeprecatedFields(platform *hivevsphere.Platform) error` -- populates `platform.Infrastructure` from deprecated flat fields using installer's ConvertInstallConfig; no-op if Infrastructure is already set

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1/vsphere` -- Hive vSphere Platform type
- `github.com/openshift/installer/pkg/types` -- InstallConfig, Platform
- `github.com/openshift/installer/pkg/types/vsphere` -- Installer vSphere Platform type
- `github.com/openshift/installer/pkg/types/vsphere/conversion` -- ConvertInstallConfig

## Capabilities

- Detects whether the newer Infrastructure field is already populated and skips conversion if so
- Constructs a dummy InstallConfig with deprecated fields and delegates to the installer's conversion function
- Writes the converted Infrastructure back to the Hive platform object

## Understanding Score

0.90
