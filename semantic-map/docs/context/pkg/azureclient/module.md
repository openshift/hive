# Module atlas

## Responsibility

Provides the Azure API client abstraction used by Hive: a `Client` interface wrapping DNS zones, DNS record sets, virtual machines, resource SKUs, and images with credential loading from Secrets, files, or byte slices.

## Public Interface/API

- `Client` — interface: DNS zone CRUD, record set CRUD, VM list/deallocate/start, resource SKU listing, image listing
- `NewClientFromSecret(secret, environmentName) (Client, error)` — from a Kubernetes Secret
- `NewClientFromFile(filename, environmentName) (Client, error)` — from a credentials file
- `NewClient(creds, environmentName) (Client, error)` — from raw JSON bytes
- `ResourceSKUsPage`, `RecordSetPage`, `ImageListResultPage` — paginated result interfaces

## Internal Dependencies

- `github.com/Azure/azure-sdk-for-go` — DNS, Compute SDK clients (external)
- `github.com/Azure/go-autorest/autorest` — Azure authentication/authorization (external)
- `github.com/openshift/installer/pkg/asset/installconfig/azure` — Credentials struct for JSON parsing (external)
- `github.com/openshift/hive/pkg/constants` — `AzureCredentialsName` key constant

## Capabilities

- Creates authenticated Azure clients using client secret credentials (clientId/clientSecret/tenantId/subscriptionId)
- Supports configurable Azure environments (public cloud, government, etc.)
- DNS zone and record set management for managed DNS
- Virtual machine lifecycle operations for hibernation support
- Resource SKU listing for machine pool validation

## Understanding Score

0.85
