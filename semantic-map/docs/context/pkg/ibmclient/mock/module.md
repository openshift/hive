<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/ibmclient/mock/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `MockAPI` — MockAPI is a mock of API interface.
- `MockAPI.EXPECT` — EXPECT returns an object that allows the caller to indicate expected use.
- `MockAPI.GetAuthenticatorAPIKeyDetails` — GetAuthenticatorAPIKeyDetails mocks base method.
- `MockAPI.GetCISInstance` — GetCISInstance mocks base method.
- `MockAPI.GetDNSRecordsByName` — GetDNSRecordsByName mocks base method.
- `MockAPI.GetDNSZoneIDByName` — GetDNSZoneIDByName mocks base method.
- `MockAPI.GetDNSZones` — GetDNSZones mocks base method.
- `MockAPI.GetDedicatedHostByName` — GetDedicatedHostByName mocks base method.
- `MockAPI.GetDedicatedHostProfiles` — GetDedicatedHostProfiles mocks base method.
- `MockAPI.GetEncryptionKey` — GetEncryptionKey mocks base method.
- `MockAPI.GetResourceGroup` — GetResourceGroup mocks base method.
- `MockAPI.GetResourceGroups` — GetResourceGroups mocks base method.
- `MockAPI.GetSubnet` — GetSubnet mocks base method.
- `MockAPI.GetVPC` — GetVPC mocks base method.
- `MockAPI.GetVPCInstances` — GetVPCInstances mocks base method.
- `MockAPI.GetVPCZonesForRegion` — GetVPCZonesForRegion mocks base method.
- `MockAPI.GetVSIProfiles` — GetVSIProfiles mocks base method.
- `MockAPI.StartInstances` — StartInstances mocks base method.
- `MockAPI.StopInstances` — StopInstances mocks base method.
- `MockAPIMockRecorder` — MockAPIMockRecorder is the mock recorder for MockAPI.
- `MockAPIMockRecorder.GetAuthenticatorAPIKeyDetails` — GetAuthenticatorAPIKeyDetails indicates an expected call of GetAuthenticatorAPIKeyDetails.
- `MockAPIMockRecorder.GetCISInstance` — GetCISInstance indicates an expected call of GetCISInstance.
- `MockAPIMockRecorder.GetDNSRecordsByName` — GetDNSRecordsByName indicates an expected call of GetDNSRecordsByName.
- `MockAPIMockRecorder.GetDNSZoneIDByName` — GetDNSZoneIDByName indicates an expected call of GetDNSZoneIDByName.
- `MockAPIMockRecorder.GetDNSZones` — GetDNSZones indicates an expected call of GetDNSZones.
- `MockAPIMockRecorder.GetDedicatedHostByName` — GetDedicatedHostByName indicates an expected call of GetDedicatedHostByName.
- `MockAPIMockRecorder.GetDedicatedHostProfiles` — GetDedicatedHostProfiles indicates an expected call of GetDedicatedHostProfiles.
- `MockAPIMockRecorder.GetEncryptionKey` — GetEncryptionKey indicates an expected call of GetEncryptionKey.
- `MockAPIMockRecorder.GetResourceGroup` — GetResourceGroup indicates an expected call of GetResourceGroup.
- `MockAPIMockRecorder.GetResourceGroups` — GetResourceGroups indicates an expected call of GetResourceGroups.
- `MockAPIMockRecorder.GetSubnet` — GetSubnet indicates an expected call of GetSubnet.
- `MockAPIMockRecorder.GetVPC` — GetVPC indicates an expected call of GetVPC.
- `MockAPIMockRecorder.GetVPCInstances` — GetVPCInstances indicates an expected call of GetVPCInstances.
- `MockAPIMockRecorder.GetVPCZonesForRegion` — GetVPCZonesForRegion indicates an expected call of GetVPCZonesForRegion.
- `MockAPIMockRecorder.GetVSIProfiles` — GetVSIProfiles indicates an expected call of GetVSIProfiles.
- `MockAPIMockRecorder.StartInstances` — StartInstances indicates an expected call of StartInstances.
- `MockAPIMockRecorder.StopInstances` — StopInstances indicates an expected call of StopInstances.

## Internal Dependencies

- `context`
- `github.com/IBM/networking-go-sdk/dnsrecordsv1`
- `github.com/IBM/platform-services-go-sdk/iamidentityv1`
- `github.com/IBM/platform-services-go-sdk/resourcecontrollerv2`
- `github.com/IBM/platform-services-go-sdk/resourcemanagerv2`
- `github.com/IBM/vpc-go-sdk/vpcv1`
- `github.com/openshift/hive/pkg/ibmclient`
- `go.uber.org/mock/gomock`
- `reflect`

## Capabilities

- **`package`** name(s): **mock**.
- Go **`import`** edges listed below (9 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/ibmclient/mock`.

## Understanding Score

0.0
