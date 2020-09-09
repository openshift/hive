package azureclient

import (
	"context"
	"encoding/json"
	"io/ioutil"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2018-10-01/compute"
	"github.com/Azure/azure-sdk-for-go/services/dns/mgmt/2018-05-01/dns"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"

	"github.com/openshift/hive/pkg/constants"
)

//go:generate mockgen -source=./client.go -destination=./mock/client_generated.go -package=mock

// Client is a wrapper object for actual Azure libraries to allow for easier mocking/testing.
type Client interface {
	ListResourceSKUs(ctx context.Context) (ResourceSKUsPage, error)

	// Zones
	CreateOrUpdateZone(ctx context.Context, resourceGroupName string, zone string) (dns.Zone, error)
	DeleteZone(ctx context.Context, resourceGroupName string, zone string) error
	GetZone(ctx context.Context, resourceGroupName string, zone string) (dns.Zone, error)

	// RecordSets
	ListRecordSetsByZone(ctx context.Context, resourceGroupName string, zone string, suffix string) (RecordSetPage, error)
	CreateOrUpdateRecordSet(ctx context.Context, resourceGroupName string, zone string, recordSetName string, recordType dns.RecordType, recordSet dns.RecordSet) (dns.RecordSet, error)
	DeleteRecordSet(ctx context.Context, resourceGroupName string, zone string, recordSetName string, recordType dns.RecordType) error
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

type azureClient struct {
	resourceSKUsClient *compute.ResourceSkusClient
	recordSetsClient   *dns.RecordSetsClient
	zonesClient        *dns.ZonesClient
}

func (c *azureClient) ListResourceSKUs(ctx context.Context) (ResourceSKUsPage, error) {
	page, err := c.resourceSKUsClient.List(ctx)
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

// NewClientFromSecret creates our client wrapper object for interacting with Azure. The Azure creds are read from the
// specified secret.
func NewClientFromSecret(secret *corev1.Secret) (Client, error) {
	return newClient(authJSONFromSecretSource(secret))
}

// NewClientFromFile creates our client wrapper object for interacting with Azure. The Azure creds are read from the
// specified file.
func NewClientFromFile(filename string) (Client, error) {
	return newClient(authJSONFromFileSource(filename))
}

// NewClient creates our client wrapper object for interacting with Azure using the Azure creds provided.
func NewClient(creds []byte) (Client, error) {
	return newClient(authJSONFromBytes(creds))
}

func newClient(authJSONSource func() ([]byte, error)) (*azureClient, error) {
	authJSON, err := authJSONSource()
	if err != nil {
		return nil, err
	}
	var authMap map[string]string
	if err := json.Unmarshal(authJSON, &authMap); err != nil {
		return nil, err
	}
	clientID, ok := authMap["clientId"]
	if !ok {
		return nil, errors.New("missing clientId in auth")
	}
	clientSecret, ok := authMap["clientSecret"]
	if !ok {
		return nil, errors.New("missing clientSecret in auth")
	}
	tenantID, ok := authMap["tenantId"]
	if !ok {
		return nil, errors.New("missing tenantId in auth")
	}
	subscriptionID, ok := authMap["subscriptionId"]
	if !ok {
		return nil, errors.New("missing subscriptionId in auth")
	}

	config := auth.NewClientCredentialsConfig(clientID, clientSecret, tenantID)

	authorizer, err := config.Authorizer()
	if err != nil {
		return nil, err
	}

	resourceSKUsClient := compute.NewResourceSkusClientWithBaseURI(azure.PublicCloud.ResourceManagerEndpoint, subscriptionID)
	resourceSKUsClient.Authorizer = authorizer

	recordSetsClient := dns.NewRecordSetsClientWithBaseURI(azure.PublicCloud.ResourceManagerEndpoint, subscriptionID)
	recordSetsClient.Authorizer = authorizer

	zonesClient := dns.NewZonesClientWithBaseURI(azure.PublicCloud.ResourceManagerEndpoint, subscriptionID)
	zonesClient.Authorizer = authorizer

	return &azureClient{
		resourceSKUsClient: &resourceSKUsClient,
		recordSetsClient:   &recordSetsClient,
		zonesClient:        &zonesClient,
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
		return ioutil.ReadFile(filename)
	}
}
