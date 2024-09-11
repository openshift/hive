package azureclient

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-12-01/compute"
	"github.com/Azure/azure-sdk-for-go/services/dns/mgmt/2018-05-01/dns"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/Azure/go-autorest/autorest/to"
	installerazure "github.com/openshift/installer/pkg/asset/installconfig/azure"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"

	"github.com/openshift/hive/pkg/constants"
)

//go:generate mockgen -source=./client.go -destination=./mock/client_generated.go -package=mock

// Client is a wrapper object for actual Azure libraries to allow for easier mocking/testing.
type Client interface {
	ListResourceSKUs(ctx context.Context, filter string) (ResourceSKUsPage, error)

	// Zones
	CreateOrUpdateZone(ctx context.Context, resourceGroupName string, zone string) (dns.Zone, error)
	DeleteZone(ctx context.Context, resourceGroupName string, zone string) error
	GetZone(ctx context.Context, resourceGroupName string, zone string) (dns.Zone, error)

	// RecordSets
	ListRecordSetsByZone(ctx context.Context, resourceGroupName string, zone string, suffix string) (RecordSetPage, error)
	CreateOrUpdateRecordSet(ctx context.Context, resourceGroupName string, zone string, recordSetName string, recordType dns.RecordType, recordSet dns.RecordSet) (dns.RecordSet, error)
	DeleteRecordSet(ctx context.Context, resourceGroupName string, zone string, recordSetName string, recordType dns.RecordType) error

	// Virtual Machines
	ListAllVirtualMachines(ctx context.Context, statusOnly string) (compute.VirtualMachineListResultPage, error)
	DeallocateVirtualMachine(ctx context.Context, resourceGroup, name string) (compute.VirtualMachinesDeallocateFuture, error)
	StartVirtualMachine(ctx context.Context, resourceGroup, name string) (compute.VirtualMachinesStartFuture, error)
	GetVMCapabilities(ctx context.Context, instanceType, region string) (map[string]string, error)

	// SKU
	GetVirtualMachineSku(ctx context.Context, name, region string) (*compute.ResourceSku, error)

	// Images
	ListImagesByResourceGroup(ctx context.Context, resourgeGroupName string) (ImageListResultPage, error)
}

// ResourceSKUsPage is a page of results from listing resource SKUs.
type ResourceSKUsPage interface {
	NextWithContext(ctx context.Context) error
	NotDone() bool
	Values() []compute.ResourceSku
}

// RecordSetPage is a page of results from listing record sets.
type RecordSetPage interface {
	NextWithContext(ctx context.Context) error
	NotDone() bool
	Values() []dns.RecordSet
}

type ImageListResultPage interface {
	NextWithContext(ctx context.Context) error
	NotDone() bool
	Values() []compute.Image
}

type azureClient struct {
	resourceSKUsClient    *compute.ResourceSkusClient
	recordSetsClient      *dns.RecordSetsClient
	zonesClient           *dns.ZonesClient
	virtualMachinesClient *compute.VirtualMachinesClient
	imagesClient          *compute.ImagesClient
}

func (c *azureClient) ListResourceSKUs(ctx context.Context, filter string) (ResourceSKUsPage, error) {
	page, err := c.resourceSKUsClient.List(ctx, filter)
	return &page, err
}

func (c *azureClient) CreateOrUpdateZone(ctx context.Context, resourceGroupName string, zone string) (dns.Zone, error) {
	return c.zonesClient.CreateOrUpdate(ctx, resourceGroupName, zone, dns.Zone{
		Location: to.StringPtr("global"),
		ZoneProperties: &dns.ZoneProperties{
			ZoneType: dns.Public,
		},
	}, "", "")
}

func (c *azureClient) DeleteZone(ctx context.Context, resourceGroupName string, zone string) error {
	future, err := c.zonesClient.Delete(ctx, resourceGroupName, zone, "")
	if err != nil {
		return err
	}

	return future.WaitForCompletionRef(ctx, c.zonesClient.Client)
}

func (c *azureClient) DeleteRecordSet(ctx context.Context, resourceGroupName string, zone string, recordSetName string, recordType dns.RecordType) error {
	_, err := c.recordSetsClient.Delete(ctx, resourceGroupName, zone, recordSetName, recordType, "")
	return err
}

func (c *azureClient) ListRecordSetsByZone(ctx context.Context, resourceGroupName string, zone string, suffix string) (RecordSetPage, error) {
	page, err := c.recordSetsClient.ListByDNSZone(ctx, resourceGroupName, zone, nil, suffix)
	return &page, err
}

func (c *azureClient) GetZone(ctx context.Context, resourceGroupName string, zone string) (dns.Zone, error) {
	return c.zonesClient.Get(ctx, resourceGroupName, zone)
}

func (c *azureClient) CreateOrUpdateRecordSet(ctx context.Context, resourceGroupName string, zone string, recordSetName string, recordType dns.RecordType, recordSet dns.RecordSet) (dns.RecordSet, error) {
	return c.recordSetsClient.CreateOrUpdate(ctx, resourceGroupName, zone, recordSetName, recordType, recordSet, "", "")
}

func (c *azureClient) ListAllVirtualMachines(ctx context.Context, statusOnly string) (compute.VirtualMachineListResultPage, error) {
	return c.virtualMachinesClient.ListAll(ctx, statusOnly)
}

func (c *azureClient) DeallocateVirtualMachine(ctx context.Context, resourceGroup, name string) (compute.VirtualMachinesDeallocateFuture, error) {
	return c.virtualMachinesClient.Deallocate(ctx, resourceGroup, name)
}

func (c *azureClient) StartVirtualMachine(ctx context.Context, resourceGroup, name string) (compute.VirtualMachinesStartFuture, error) {
	return c.virtualMachinesClient.Start(ctx, resourceGroup, name)
}

// GetVMCapabilities retrieves the capabilities of an instance type in a specific region. Returns these values
// in a map with the capability name as the key and the corresponding value.
func (c *azureClient) GetVMCapabilities(ctx context.Context, instanceType, region string) (map[string]string, error) {
	typeMeta, err := c.GetVirtualMachineSku(ctx, instanceType, region)
	if err != nil {
		return nil, fmt.Errorf("error connecting to Azure client: %v", err)
	}
	if typeMeta == nil {
		return nil, fmt.Errorf("not found in region %s", region)
	}

	capabilities := make(map[string]string)
	for _, capability := range *typeMeta.Capabilities {
		capabilities[to.String(capability.Name)] = to.String(capability.Value)
	}
	return capabilities, nil
}

// GetVirtualMachineSku retrieves the resource SKU of a specified virtual machine SKU in the specified region.
func (c *azureClient) GetVirtualMachineSku(ctx context.Context, name, region string) (*compute.ResourceSku, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	for page, err := c.resourceSKUsClient.List(ctx, ""); page.NotDone(); err = page.NextWithContext(ctx) {
		if err != nil {
			return nil, errors.Wrap(err, "error fetching SKU pages")
		}
		for _, sku := range page.Values() {
			// Filter out resources that are not virtualMachines
			if !strings.EqualFold("virtualMachines", *sku.ResourceType) {
				continue
			}
			// Filter out resources that do not match the provided name
			if !strings.EqualFold(name, *sku.Name) {
				continue
			}
			// Return the resource from the provided region
			for _, location := range to.StringSlice(sku.Locations) {
				if strings.EqualFold(location, region) {
					return &sku, nil
				}
			}
		}
	}
	return nil, nil
}

func (c *azureClient) ListImagesByResourceGroup(ctx context.Context, resourceGroupName string) (ImageListResultPage, error) {
	page, err := c.imagesClient.ListByResourceGroup(ctx, resourceGroupName)
	return &page, err
}

// NewClientFromSecret creates our client wrapper object for interacting with Azure. The Azure creds are read from the
// specified secret.
func NewClientFromSecret(secret *corev1.Secret, environmentName string) (Client, error) {
	return newClient(authJSONFromSecretSource(secret), environmentName)
}

// NewClientFromFile creates our client wrapper object for interacting with Azure. The Azure creds are read from the
// specified file.
func NewClientFromFile(filename string, environmentName string) (Client, error) {
	return newClient(authJSONFromFileSource(filename), environmentName)
}

// NewClient creates our client wrapper object for interacting with Azure using the Azure creds provided.
func NewClient(creds []byte, environmentName string) (Client, error) {
	return newClient(authJSONFromBytes(creds), environmentName)
}

func newClient(authJSONSource func() ([]byte, error), environmentName string) (*azureClient, error) {
	authJSON, err := authJSONSource()
	if err != nil {
		return nil, err
	}
	var creds installerazure.Credentials
	// json.Unmarshal is case-insensitive. ACM-14248.
	if err := json.Unmarshal(authJSON, &creds); err != nil {
		return nil, err
	}
	// TODO: Installer supports cert and MSI auth as well; we currently only support client secret.
	if creds.ClientID == "" {
		return nil, errors.New("missing clientId in auth")
	}
	if creds.ClientSecret == "" {
		return nil, errors.New("missing clientSecret in auth")
	}
	if creds.TenantID == "" {
		return nil, errors.New("missing tenantId in auth")
	}
	if creds.SubscriptionID == "" {
		return nil, errors.New("missing subscriptionId in auth")
	}

	if environmentName == "" {
		environmentName = azure.PublicCloud.Name
	}

	env, err := azure.EnvironmentFromName(environmentName)
	if err != nil {
		return nil, err
	}

	authorizer, err := getAuthorizer(creds.ClientID, creds.ClientSecret, creds.TenantID, env)
	if err != nil {
		return nil, err
	}

	resourceSKUsClient := compute.NewResourceSkusClientWithBaseURI(env.ResourceManagerEndpoint, creds.SubscriptionID)
	resourceSKUsClient.Authorizer = authorizer

	recordSetsClient := dns.NewRecordSetsClientWithBaseURI(env.ResourceManagerEndpoint, creds.SubscriptionID)
	recordSetsClient.Authorizer = authorizer

	zonesClient := dns.NewZonesClientWithBaseURI(env.ResourceManagerEndpoint, creds.SubscriptionID)
	zonesClient.Authorizer = authorizer

	virtualMachinesClient := compute.NewVirtualMachinesClientWithBaseURI(env.ResourceManagerEndpoint, creds.SubscriptionID)
	virtualMachinesClient.Authorizer = authorizer

	imagesClient := compute.NewImagesClientWithBaseURI(env.ResourceManagerEndpoint, creds.SubscriptionID)
	imagesClient.Authorizer = authorizer

	return &azureClient{
		resourceSKUsClient:    &resourceSKUsClient,
		recordSetsClient:      &recordSetsClient,
		zonesClient:           &zonesClient,
		virtualMachinesClient: &virtualMachinesClient,
		imagesClient:          &imagesClient,
	}, nil
}

func authJSONFromBytes(creds []byte) func() ([]byte, error) {
	return func() ([]byte, error) {
		return creds, nil
	}
}

func authJSONFromSecretSource(secret *corev1.Secret) func() ([]byte, error) {
	return func() ([]byte, error) {
		authJSON, ok := secret.Data[constants.AzureCredentialsName]
		if !ok {
			return nil, errors.New("creds secret does not contain \"" + constants.AzureCredentialsName + "\" data")
		}
		return authJSON, nil
	}
}

func authJSONFromFileSource(filename string) func() ([]byte, error) {
	return func() ([]byte, error) {
		return os.ReadFile(filename)
	}
}

func getAuthorizer(clientID, clientSecret, tenantID string, env azure.Environment) (autorest.Authorizer, error) {
	config := auth.NewClientCredentialsConfig(clientID, clientSecret, tenantID)
	config.Resource = env.ResourceManagerEndpoint
	config.AADEndpoint = env.ActiveDirectoryEndpoint
	return config.Authorizer()
}
