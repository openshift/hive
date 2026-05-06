# Module atlas

## Responsibility

Provides a registry map of platform-specific credential configuration functions, keyed by platform name string. Acts as the top-level dispatcher that routes credential setup to the appropriate cloud provider implementation (AWS, Azure, GCP, IBMCloud, Nutanix, OpenStack, VSphere).

## Public Interface/API

- `var ConfigureCreds map[string]func(client.Client, *types.ClusterMetadata)` -- map from platform constant string to provider-specific ConfigureCreds function

## Internal Dependencies

- `github.com/openshift/hive/pkg/constants` -- platform name constants (PlatformAWS, PlatformAzure, etc.)
- `github.com/openshift/hive/pkg/creds/aws` -- AWS ConfigureCreds
- `github.com/openshift/hive/pkg/creds/azure` -- Azure ConfigureCreds
- `github.com/openshift/hive/pkg/creds/gcp` -- GCP ConfigureCreds
- `github.com/openshift/hive/pkg/creds/ibmcloud` -- IBMCloud ConfigureCreds
- `github.com/openshift/hive/pkg/creds/nutanix` -- Nutanix ConfigureCreds
- `github.com/openshift/hive/pkg/creds/openstack` -- OpenStack ConfigureCreds
- `github.com/openshift/hive/pkg/creds/vsphere` -- VSphere ConfigureCreds
- `github.com/openshift/installer/pkg/types` -- ClusterMetadata type
- `sigs.k8s.io/controller-runtime/pkg/client` -- Kubernetes client

## Capabilities

- Single registry mapping all supported cloud platforms to their credential setup functions
- Used by install/uninstall jobs to configure environment for the appropriate cloud provider

## Understanding Score

0.90
