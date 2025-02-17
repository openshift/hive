package nutanixclient

import (
	"context"
	"fmt"
	"strings"

	nutanixClient "github.com/nutanix-cloud-native/prism-go-client"
	nutanixClientV3 "github.com/nutanix-cloud-native/prism-go-client/v3"
	"github.com/openshift/hive/apis/hive/v1/nutanix"
)

func NewNutanixClient(nutanixProvider *nutanix.Platform, username, password string) (*nutanixClientV3.Client, error) {
	credentials := nutanixClient.Credentials{
		Username: username,
		Password: password,
		Endpoint: nutanixProvider.PrismCentral.Endpoint.Address,
		Port:     string(nutanixProvider.PrismCentral.Endpoint.Port),
	}

	client, err := nutanixClientV3.NewV3Client(credentials)
	if err != nil {
		return nil, err
	}
	return client, err

}

func GetPrismCentralClusterName(client *nutanixClientV3.Client) (string, error) {
	clusterList, err := client.V3.ListAllCluster(context.TODO(), "")
	if err != nil {
		return "", err
	}

	foundPCs := make([]*nutanixClientV3.ClusterIntentResponse, 0)
	for _, cl := range clusterList.Entities {
		if cl.Status != nil && cl.Status.Resources != nil && cl.Status.Resources.Config != nil {
			serviceList := cl.Status.Resources.Config.ServiceList
			for _, svc := range serviceList {
				if svc != nil && strings.ToUpper(*svc) == "PRISM_CENTRAL" {
					foundPCs = append(foundPCs, cl)
				}
			}
		}
	}
	numFoundPCs := len(foundPCs)
	if numFoundPCs == 1 {
		return *foundPCs[0].Spec.Name, nil
	}
	if len(foundPCs) == 0 {
		return "", fmt.Errorf("failed to retrieve Prism Central cluster")
	}
	return "", fmt.Errorf("found more than one Prism Central cluster: %v", numFoundPCs)
}

//import (
//	"context"
//	"fmt"
//	prismgoclient "github.com/nutanix-cloud-native/prism-go-client"
//	v3 "github.com/nutanix-cloud-native/prism-go-client/v3"
//	hivev1 "github.com/openshift/hive/apis/hive/v1"
//	corev1 "k8s.io/api/core/v1"
//)
//
//type Client struct {
//	v3Client *v3.Client
//}
//
//func NewClient(credentials *prismgoclient.Credentials) (*Client, error) {
//	if credentials.URL == "" {
//		credentials.URL = fmt.Sprintf("https://%s:%s", credentials.Endpoint, credentials.Port)
//	}
//
//	client, err := v3.NewV3Client(*credentials)
//	if err != nil {
//		return nil, fmt.Errorf("failed to create Nutanix client: %w", err)
//	}
//	return &Client{v3Client: client}, nil
//}
//
//func NewClientFromSecret(secret *corev1.Secret) (*Client, error) {
//	credentials := &prismgoclient.Credentials{
//		Username: string(secret.Data["username"]),
//		Password: string(secret.Data["password"]),
//		Endpoint: string(secret.Data["endpoint"]),
//		Port:     string(secret.Data["port"]),
//	}
//
//	return NewClient(credentials)
//}
//
//func (c *Client) getDisksList(machinePool *hivev1.MachinePool) []*v3.VMDisk {
//	diskList := make([]*v3.VMDisk, 0)
//	for _, disk := range machinePool.Spec.Platform.Nutanix.Disks {
//		diskList = append(diskList, &v3.VMDisk{
//			DiskSizeBytes: &disk.DiskSizeBytes,
//			DeviceProperties: &v3.VMDiskDeviceProperties{
//				DeviceType: &disk.DeviceType,
//			},
//		})
//	}
//	return diskList
//}
//
//func (c *Client) getBootConfig(machinePool *hivev1.MachinePool) *v3.VMBootConfig {
//	bootConfig := &v3.VMBootConfig{
//		BootType: &machinePool.Spec.Platform.Nutanix.BootType,
//	}
//	return bootConfig
//}
//
//func (c *Client) getNicList(machinePool *hivev1.MachinePool) []*v3.VMNic {
//	nicList := make([]*v3.VMNic, 0)
//	for _, nic := range machinePool.Spec.Platform.Nutanix.NicList {
//		nicList = append(nicList, &v3.VMNic{
//			UUID: &nic,
//		})
//	}
//	return nicList
//}
//
//func (c *Client) CreateVMs(ctx context.Context, machinePool *hivev1.MachinePool) error {
//	// Convert MachinePool data into Nutanix VM creation payload
//	createRequest := v3.VMIntentResource{
//		Spec: &v3.VM{
//			Name: &machinePool.Name,
//			Resources: &v3.VMResources{
//				NumVcpusPerSocket: &machinePool.Spec.Platform.Nutanix.NumVcpusPerSocket,
//				NumSockets:        &machinePool.Spec.Platform.Nutanix.NumSockets,
//				MemorySizeMib:     &machinePool.Spec.Platform.Nutanix.MemorySizeMiB,
//				DiskList:          c.getDisksList(machinePool),
//				BootConfig:        c.getBootConfig(machinePool),
//				NicList:           c.getNicList(machinePool),
//			},
//		},
//	}
//
//	intent := v3.VMIntentInput{
//		Spec: &v3.VM{
//			Name: &machinePool.Name,
//			Resources: &v3.VMResources{
//				NumVcpusPerSocket: &machinePool.Spec.Platform.Nutanix.NumVcpusPerSocket,
//				NumSockets:        &machinePool.Spec.Platform.Nutanix.NumSockets,
//				MemorySizeMib:     &machinePool.Spec.Platform.Nutanix.MemorySizeMiB,
//				DiskList:          c.getDisksList(machinePool),
//				BootConfig:        c.getBootConfig(machinePool),
//				NicList:           c.getNicList(machinePool),
//			},
//		},
//	}
//
//	// Call the Nutanix API to create a VM
//	c.v3Client.V3.CreateVM(ctx, &intent)
//
//	_, err := c.VmApi.CreateVm(createRequest.Spec)
//	if err != nil {
//		return fmt.Errorf("failed to create Nutanix VM: %w", err)
//	}
//
//	return nil
//}
//
//func (c *Client) GetVmByName(vmName string) (*v3.VM, error) {
//	// Retrieve the list of all VMs
//	vms, err := c.VmApi.ListVms(nil, nil, nil, nil, nil)
//	if err != nil {
//		return nil, fmt.Errorf("failed to get Nutanix VM: %w", err)
//	}
//
//	// Iterate through the VMs to find one with the matching name
//	for _, vm := range vms.Entities {
//		if *vm.Spec.Name == vmName {
//			return vm, nil
//		}
//	}
//
//	// If no matching VM is found, return an error
//	return nil, fmt.Errorf("VM with name %s not found", vmName)
//}
//
//func (c *Client) DeleteVMs(machinePool *hivev1.MachinePool) error {
//	// Step 1: List VMs to find the one(s) to delete
//	vmList, _, err := c.VmApi.ListVms(nil)
//	if err != nil {
//		return fmt.Errorf("failed to list Nutanix VMs: %w", err)
//	}
//
//	// Step 2: Find the VM(s) that match the MachinePool name
//	var vmUUIDs []string
//	for _, vm := range vmList.Entities {
//		if *vm.Spec.Name == machinePool.Name {
//			vmUUIDs = append(vmUUIDs, *vm.Metadata.UUID)
//		}
//	}
//
//	if len(vmUUIDs) == 0 {
//		return fmt.Errorf("no VMs found for machine pool: %s", machinePool.Name)
//	}
//
//	// Step 3: Delete each matching VM
//	for _, vmUUID := range vmUUIDs {
//		err := c.VmApi.DeleteVm(context.TODO(), vmUUID)
//		if err != nil {
//			return fmt.Errorf("failed to delete VM with UUID %s: %w", vmUUID, err)
//		}
//	}
//
//	return nil
//}
