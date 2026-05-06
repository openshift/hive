# Module atlas

## Responsibility

Provides a GCP API client abstraction for Hive. Defines the `Client` interface covering DNS managed zones, resource record sets, compute instances, firewalls, forwarding rules, service attachments, subnets, addresses, and networks. Offers constructors from raw JSON, Kubernetes secrets, or files.

## Public Interface/API

- `Client` interface -- full GCP operations surface (DNS zones, record sets, compute zones/images/instances, firewalls, forwarding rules, service attachments, subnets, addresses, networks)
- `NewClient(authJSON []byte) (Client, error)` -- create client from raw credentials JSON
- `NewClientFromSecret(secret *corev1.Secret) (Client, error)` -- create client from a Kubernetes secret
- `NewClientFromFile(filename string) (Client, error)` -- create client from a credentials file
- `ProjectID(authJSON []byte) (string, error)` -- extract project ID from raw credentials
- `ProjectIDFromSecret(secret *corev1.Secret) (string, error)` -- extract project ID from Kubernetes secret
- `ProjectIDFromFile(filename string) (string, error)` -- extract project ID from file
- `InstancesStopCallOption` -- functional option type for StopInstance
- `ListManagedZonesOptions`, `ListResourceRecordSetsOptions`, `ListComputeZonesOptions`, `ListComputeImagesOptions`, `ListComputeInstancesOptions`, `ListAddressesOptions` -- option structs for list operations

## Internal Dependencies

- `github.com/openshift/hive/pkg/constants`
- `golang.org/x/oauth2/google`
- `google.golang.org/api/compute/v1`, `dns/v1`, `cloudresourcemanager/v1`, `serviceusage/v1`
- `google.golang.org/api/googleapi`, `google.golang.org/api/option`
- `k8s.io/api/core/v1`
- `github.com/pkg/errors`

## Capabilities

- Wraps GCP Compute, DNS, Cloud Resource Manager, and Service Usage APIs behind a single interface
- CRUD operations on firewalls, forwarding rules, service attachments, subnets, and addresses
- DNS managed zone and resource record set management
- Compute instance listing, start, and stop with timeout-guarded operations
- Credential loading from raw bytes, Kubernetes secrets, or files

## Understanding Score

0.9
