<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/azureclient/mock/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `MockClient` — MockClient is a mock of Client interface.
- `MockClient.CreateOrUpdateRecordSet` — CreateOrUpdateRecordSet mocks base method.
- `MockClient.CreateOrUpdateZone` — CreateOrUpdateZone mocks base method.
- `MockClient.DeallocateVirtualMachine` — DeallocateVirtualMachine mocks base method.
- `MockClient.DeleteRecordSet` — DeleteRecordSet mocks base method.
- `MockClient.DeleteZone` — DeleteZone mocks base method.
- `MockClient.EXPECT` — EXPECT returns an object that allows the caller to indicate expected use.
- `MockClient.GetZone` — GetZone mocks base method.
- `MockClient.ListAllVirtualMachines` — ListAllVirtualMachines mocks base method.
- `MockClient.ListImagesByResourceGroup` — ListImagesByResourceGroup mocks base method.
- `MockClient.ListRecordSetsByZone` — ListRecordSetsByZone mocks base method.
- `MockClient.ListResourceSKUs` — ListResourceSKUs mocks base method.
- `MockClient.StartVirtualMachine` — StartVirtualMachine mocks base method.
- `MockClientMockRecorder` — MockClientMockRecorder is the mock recorder for MockClient.
- `MockClientMockRecorder.CreateOrUpdateRecordSet` — CreateOrUpdateRecordSet indicates an expected call of CreateOrUpdateRecordSet.
- `MockClientMockRecorder.CreateOrUpdateZone` — CreateOrUpdateZone indicates an expected call of CreateOrUpdateZone.
- `MockClientMockRecorder.DeallocateVirtualMachine` — DeallocateVirtualMachine indicates an expected call of DeallocateVirtualMachine.
- `MockClientMockRecorder.DeleteRecordSet` — DeleteRecordSet indicates an expected call of DeleteRecordSet.
- `MockClientMockRecorder.DeleteZone` — DeleteZone indicates an expected call of DeleteZone.
- `MockClientMockRecorder.GetZone` — GetZone indicates an expected call of GetZone.
- `MockClientMockRecorder.ListAllVirtualMachines` — ListAllVirtualMachines indicates an expected call of ListAllVirtualMachines.
- `MockClientMockRecorder.ListImagesByResourceGroup` — ListImagesByResourceGroup indicates an expected call of ListImagesByResourceGroup.
- `MockClientMockRecorder.ListRecordSetsByZone` — ListRecordSetsByZone indicates an expected call of ListRecordSetsByZone.
- `MockClientMockRecorder.ListResourceSKUs` — ListResourceSKUs indicates an expected call of ListResourceSKUs.
- `MockClientMockRecorder.StartVirtualMachine` — StartVirtualMachine indicates an expected call of StartVirtualMachine.
- `MockImageListResultPage` — MockImageListResultPage is a mock of ImageListResultPage interface.
- `MockImageListResultPage.EXPECT` — EXPECT returns an object that allows the caller to indicate expected use.
- `MockImageListResultPage.NextWithContext` — NextWithContext mocks base method.
- `MockImageListResultPage.NotDone` — NotDone mocks base method.
- `MockImageListResultPage.Values` — Values mocks base method.
- `MockImageListResultPageMockRecorder` — MockImageListResultPageMockRecorder is the mock recorder for MockImageListResultPage.
- `MockImageListResultPageMockRecorder.NextWithContext` — NextWithContext indicates an expected call of NextWithContext.
- `MockImageListResultPageMockRecorder.NotDone` — NotDone indicates an expected call of NotDone.
- `MockImageListResultPageMockRecorder.Values` — Values indicates an expected call of Values.
- `MockRecordSetPage` — MockRecordSetPage is a mock of RecordSetPage interface.
- `MockRecordSetPage.EXPECT` — EXPECT returns an object that allows the caller to indicate expected use.
- `MockRecordSetPage.NextWithContext` — NextWithContext mocks base method.
- `MockRecordSetPage.NotDone` — NotDone mocks base method.
- `MockRecordSetPage.Values` — Values mocks base method.
- `MockRecordSetPageMockRecorder` — MockRecordSetPageMockRecorder is the mock recorder for MockRecordSetPage.
- `MockRecordSetPageMockRecorder.NextWithContext` — NextWithContext indicates an expected call of NextWithContext.
- `MockRecordSetPageMockRecorder.NotDone` — NotDone indicates an expected call of NotDone.
- `MockRecordSetPageMockRecorder.Values` — Values indicates an expected call of Values.
- `MockResourceSKUsPage` — MockResourceSKUsPage is a mock of ResourceSKUsPage interface.
- `MockResourceSKUsPage.EXPECT` — EXPECT returns an object that allows the caller to indicate expected use.
- `MockResourceSKUsPage.NextWithContext` — NextWithContext mocks base method.
- `MockResourceSKUsPage.NotDone` — NotDone mocks base method.
- `MockResourceSKUsPage.Values` — Values mocks base method.
- `MockResourceSKUsPageMockRecorder` — MockResourceSKUsPageMockRecorder is the mock recorder for MockResourceSKUsPage.
- `MockResourceSKUsPageMockRecorder.NextWithContext` — NextWithContext indicates an expected call of NextWithContext.
- `MockResourceSKUsPageMockRecorder.NotDone` — NotDone indicates an expected call of NotDone.
- `MockResourceSKUsPageMockRecorder.Values` — Values indicates an expected call of Values.

## Internal Dependencies

- `context`
- `github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute`
- `github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-12-01/compute`
- `github.com/Azure/azure-sdk-for-go/services/dns/mgmt/2018-05-01/dns`
- `github.com/openshift/hive/pkg/azureclient`
- `go.uber.org/mock/gomock`
- `reflect`

## Capabilities

- **`package`** name(s): **mock**.
- Go **`import`** edges listed below (7 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/azureclient/mock`.

## Understanding Score

0.0
