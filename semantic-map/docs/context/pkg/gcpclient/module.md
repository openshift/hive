# Module atlas

## Responsibility

Provides a typed Go client wrapper for Google Cloud Platform APIs used by Hive. Defines the `Client` interface and a concrete `gcpClient` implementation that wraps GCP Compute, DNS, Cloud Resource Manager, and Service Usage libraries. Handles authentication from raw JSON, Kubernetes secrets, or files. Provides CRUD operations on GCP resources including DNS managed zones and record sets, compute instances, firewalls, forwarding rules, service attachments, subnets, and addresses.

## Public Interface/API

**Interface:**
- `Client` -- Interface defining all GCP API operations. 32 methods covering DNS, Compute, and networking resources.

**Option types:**
- `InstancesStopCallOption` -- `func(*compute.InstancesStopCall) *compute.InstancesStopCall` for customizing instance stop calls.
- `ListManagedZonesOptions` -- Options for listing DNS managed zones (MaxResults, PageToken, DNSName).
- `ListResourceRecordSetsOptions` -- Options for listing DNS resource record sets (MaxResults, PageToken, Name, Type).
- `ListComputeZonesOptions` -- Options for listing compute zones (MaxResults, PageToken, Filter).
- `ListComputeImagesOptions` -- Options for listing compute images (MaxResults, PageToken, Filter).
- `ListComputeInstancesOptions` -- Options for listing compute instances (Filter, Fields).
- `ListAddressesOptions` -- Options for listing compute addresses (MaxResults, PageToken, Filter).

**Constructors:**
- `NewClient(authJSON []byte) (Client, error)` -- Creates a client from raw JSON credentials.
- `NewClientFromSecret(secret *corev1.Secret) (Client, error)` -- Creates a client from a Kubernetes secret.
- `NewClientFromFile(filename string) (Client, error)` -- Creates a client from a credentials file.

**Utility functions:**
- `ProjectID(authJSON []byte) (string, error)` -- Extracts GCP project ID from raw JSON credentials.
- `ProjectIDFromSecret(secret *corev1.Secret) (string, error)` -- Extracts project ID from a Kubernetes secret.
- `ProjectIDFromFile(filename string) (string, error)` -- Extracts project ID from a credentials file.

## Internal Dependencies

- `github.com/openshift/hive/pkg/constants` -- `GCPCredentialsName` secret key
- `golang.org/x/oauth2/google` -- `CredentialsFromJSON` for GCP authentication
- `google.golang.org/api/cloudresourcemanager/v1` -- Cloud Resource Manager service
- `google.golang.org/api/compute/v1` -- Compute Engine service (instances, firewalls, subnets, addresses, forwarding rules, service attachments)
- `google.golang.org/api/dns/v1` -- Cloud DNS service (managed zones, resource record sets)
- `google.golang.org/api/googleapi` -- `Field` type for query field selection, HTTP error handling
- `google.golang.org/api/option` -- Client options (credentials, user agent)
- `google.golang.org/api/serviceusage/v1` -- Service Usage API
- `k8s.io/api/core/v1` -- `Secret` type for reading credentials from Kubernetes

## Capabilities

- **`package`** name(s): **gcpclient**.
- Single-file package (`client.go`) with `//go:generate mockgen` directive.
- Default 2-minute timeout on all API calls via `contextWithTimeout`.
- Async operation polling via `waitforGlobalOperation` and `waitforRegionOperation`.
- Three credential source patterns: raw bytes, Kubernetes secret, filesystem file.
- User agent set to `openshift.io hive/v1`.
- Package ID(s): `github.com/openshift/hive/pkg/gcpclient`.

## Understanding Score

0.85
