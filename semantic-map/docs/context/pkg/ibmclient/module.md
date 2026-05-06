<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/ibmclient/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `API` — API represents the calls made to the API.
- `Client` — Client makes calls to the IBM Cloud API.
- `Client.GetAuthenticatorAPIKeyDetails` — GetAuthenticatorAPIKeyDetails gets detailed information on the API key used for authentication to the IBM Cloud APIs
- `Client.GetCISInstance` — GetCISInstance gets a specific Cloud Internet Services instance by its CRN.
- `Client.GetDNSRecordsByName` — GetDNSRecordsByName gets DNS records in specific Cloud Internet Services instance by its CRN, zone ID, and DNS record name.
- `Client.GetDNSZoneIDByName` — GetDNSZoneIDByName gets the CIS zone ID from its domain name.
- `Client.GetDNSZones` — GetDNSZones returns all of the active DNS zones managed by CIS.
- `Client.GetDedicatedHostByName` — GetDedicatedHostByName gets dedicated host by name.
- `Client.GetDedicatedHostProfiles` — GetDedicatedHostProfiles gets a list of profiles supported in a region.
- `Client.GetEncryptionKey` — GetEncryptionKey gets data for an encryption key
- `Client.GetResourceGroup` — GetResourceGroup gets a resource group by its name or ID.
- `Client.GetResourceGroups` — GetResourceGroups gets the list of resource groups.
- `Client.GetSubnet` — GetSubnet gets a subnet by its ID.
- `Client.GetVPC` — GetVPC gets a VPC by its ID.
- `Client.GetVPCInstances`
- `Client.GetVPCZonesForRegion` — GetVPCZonesForRegion gets the supported zones for a VPC region.
- `Client.GetVSIProfiles` — GetVSIProfiles gets a list of all VSI profiles.
- `Client.StartInstances`
- `Client.StopInstances`
- `DNSZoneResponse` — DNSZoneResponse represents a DNS zone response.
- `EncryptionKeyResponse` — EncryptionKeyResponse represents an encryption key response.
- `GetAccountID`
- `GetCISInstanceCRN`
- `NewIamAuthenticator` — NewIamAuthenticator returns a new IamAuthenticator for using IBM Cloud services.
- `VPCResourceNotFoundError` — VPCResourceNotFoundError represents an error for a VPC resoruce that is not found.
- `VPCResourceNotFoundError.Error` — Error returns the error message for the VPCResourceNotFoundError error type.

## Internal Dependencies

- `context`
- `fmt`
- `github.com/IBM/go-sdk-core/v5/core`
- `github.com/IBM/networking-go-sdk/dnsrecordsv1`
- `github.com/IBM/networking-go-sdk/zonesv1`
- `github.com/IBM/platform-services-go-sdk/iamidentityv1`
- `github.com/IBM/platform-services-go-sdk/resourcecontrollerv2`
- `github.com/IBM/platform-services-go-sdk/resourcemanagerv2`
- `github.com/IBM/vpc-go-sdk/vpcv1`
- `github.com/openshift/hive/pkg/constants`
- `github.com/pkg/errors`
- `k8s.io/api/core/v1`
- `net/http`
- `strings`
- `time`

## Capabilities

- **`package`** name(s): **ibmclient**.
- Go **`import`** edges listed below (15 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/ibmclient`.

## Understanding Score

0.0
