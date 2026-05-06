# Module atlas

## Responsibility

Provides a unified Azure client interface wrapping Azure SDK services (DNS zones, DNS record sets, virtual machines, compute images, resource SKUs) with authentication via service principal credentials from Kubernetes Secrets, files, or raw bytes.

## Public Interface/API

**Interfaces:**
- `Client` -- Unified interface for Azure operations: DNS zone CRUD, record set CRUD, VM lifecycle (list, deallocate, start), image listing, resource SKU listing
- `ResourceSKUsPage` -- Paginated results for resource SKU queries
- `RecordSetPage` -- Paginated results for DNS record set queries
- `ImageListResultPage` -- Paginated results for image listing

**Functions:**
- `NewClientFromSecret(secret *corev1.Secret, environmentName string) (Client, error)` -- Create client from a Kubernetes Secret
- `NewClientFromFile(filename string, environmentName string) (Client, error)` -- Create client from a credentials file
- `NewClient(creds []byte, environmentName string) (Client, error)` -- Create client from raw credential bytes

## Internal Dependencies

- `github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute` -- Resource SKUs client
- `github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-12-01/compute` -- VM and images clients
- `github.com/Azure/azure-sdk-for-go/services/dns/mgmt/2018-05-01/dns` -- DNS zones and record sets clients
- `github.com/Azure/go-autorest/autorest`, `azure`, `azure/auth` -- Azure authentication
- `github.com/openshift/installer/pkg/asset/installconfig/azure` -- Credentials struct for JSON parsing
- `github.com/openshift/hive/pkg/constants` -- Azure credentials name constant

## Capabilities

- Manage Azure DNS zones (create, delete, get)
- Manage Azure DNS record sets (list, create/update, delete)
- Manage Azure virtual machines (list all, deallocate, start)
- List compute images by resource group
- List resource SKUs by region
- Authenticate using client secret credentials against configurable Azure cloud environments (Public, Government, etc.)

## Understanding Score

0.9
