# Module atlas

## Responsibility

Provides an IBM Cloud API client abstraction for Hive. Defines the `API` interface covering VPC, DNS (CIS), resource groups, IAM, dedicated hosts, encryption keys, and instance lifecycle. The `Client` struct implements `API` using IBM Go SDKs.

## Public Interface/API

- `API` interface -- operations for VPC, CIS DNS, IAM, resource groups, dedicated hosts, encryption keys, instance start/stop
- `Client` struct -- concrete implementation of `API`; field `APIKey string`
- `NewClient(apiKey string) (*Client, error)` -- create client from API key
- `NewClientFromSecret(secret *corev1.Secret) (*Client, error)` -- create client from Kubernetes secret
- `NewIamAuthenticator(apiKey string) (*core.IamAuthenticator, error)` -- build IAM authenticator
- `GetCISInstanceCRN(client API, ctx, baseDomain) (string, error)` -- look up CIS instance CRN by base domain
- `GetAccountID(client API, ctx) (string, error)` -- retrieve account ID from API key details
- `DNSZoneResponse` struct -- Name, ID, CISInstanceCRN, CISInstanceName, ResourceGroupID
- `EncryptionKeyResponse` struct -- placeholder for encryption key data
- `VPCResourceNotFoundError` struct -- error type for missing VPC resources

## Internal Dependencies

- `github.com/openshift/hive/pkg/constants`
- `github.com/IBM/go-sdk-core/v5/core`
- `github.com/IBM/networking-go-sdk/dnsrecordsv1`, `zonesv1`
- `github.com/IBM/platform-services-go-sdk/iamidentityv1`, `resourcecontrollerv2`, `resourcemanagerv2`
- `github.com/IBM/vpc-go-sdk/vpcv1`
- `github.com/pkg/errors`
- `k8s.io/api/core/v1`

## Capabilities

- VPC instance listing, start, and stop by infraID and region
- DNS zone enumeration and record lookup via CIS
- Resource group and CIS instance queries
- IAM API key details retrieval
- Dedicated host and VSI profile listing
- Subnet and VPC lookups across all regions

## Understanding Score

0.9
