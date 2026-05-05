# Module atlas

## Responsibility

Provides a typed Go client wrapper for IBM Cloud APIs used by Hive. Defines the `API` interface and a concrete `Client` struct that wraps IBM Cloud SDK services for VPC (instances, subnets, dedicated hosts, profiles, zones), DNS (CIS zones, DNS records), IAM identity, resource management, and resource controller operations. Handles authentication via API key using IAM authenticators.

## Public Interface/API

**Interface:**
- `API` -- Interface defining all IBM Cloud API operations. 17 methods covering IAM, DNS, VPC, resources, and encryption keys.

**Concrete type:**
- `Client` -- Struct implementing `API`, wrapping IBM Cloud Resource Manager, Resource Controller, and VPC SDK services. Fields: `APIKey string`.

**Types:**
- `DNSZoneResponse` -- Response struct for DNS zones (Name, ID, CISInstanceCRN, CISInstanceName, ResourceGroupID).
- `EncryptionKeyResponse` -- Placeholder struct for encryption key responses (currently empty, TODO).
- `VPCResourceNotFoundError` -- Error type for VPC resources not found; implements `error` interface.

**Constructors:**
- `NewClient(apiKey string) (*Client, error)` -- Creates a client from an API key string, initializing all SDK services.
- `NewClientFromSecret(secret *corev1.Secret) (*Client, error)` -- Creates a client by extracting the API key from a Kubernetes secret.
- `NewIamAuthenticator(apiKey string) (*core.IamAuthenticator, error)` -- Returns an IAM authenticator for IBM Cloud service authentication.

**Standalone functions:**
- `GetCISInstanceCRN(client API, ctx context.Context, baseDomain string) (string, error)` -- Looks up the CIS instance CRN for a given base domain.
- `GetAccountID(client API, ctx context.Context) (string, error)` -- Retrieves the IBM Cloud account ID from the API key details.

## Internal Dependencies

- `github.com/IBM/go-sdk-core/v5/core` -- IBM SDK core, IAM authenticator
- `github.com/IBM/networking-go-sdk/dnsrecordsv1` -- CIS DNS records API
- `github.com/IBM/networking-go-sdk/zonesv1` -- CIS zones API
- `github.com/IBM/platform-services-go-sdk/iamidentityv1` -- IAM identity service (API key details)
- `github.com/IBM/platform-services-go-sdk/resourcecontrollerv2` -- Resource Controller (CIS instances)
- `github.com/IBM/platform-services-go-sdk/resourcemanagerv2` -- Resource Manager (resource groups)
- `github.com/IBM/vpc-go-sdk/vpcv1` -- VPC service (instances, subnets, dedicated hosts, profiles, regions/zones)
- `github.com/openshift/hive/pkg/constants` -- `IBMCloudAPIKeySecretKey` for reading secrets
- `k8s.io/api/core/v1` -- `Secret` type for reading credentials from Kubernetes

## Capabilities

- **`package`** name(s): **ibmclient**.
- Single-file package (`client.go`) with `//go:generate mockgen` directive.
- Dynamically sets VPC service URL per region via `setVPCServiceURLForRegion`.
- 1-minute timeouts on most API calls.
- VPC instance filtering by infra ID name pattern.
- CIS service identified by catalog service ID constant `cisServiceID`.
- `GetEncryptionKey` is a stub (TODO).
- Package ID(s): `github.com/openshift/hive/pkg/ibmclient`.

## Understanding Score

0.85
