package azureclient

import (
	"context"
	"encoding/json"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2018-10-01/compute"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/azure/auth"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"

	"github.com/openshift/hive/pkg/constants"
)

//go:generate mockgen -source=./client.go -destination=./mock/client_generated.go -package=mock

// Client is a wrapper object for actual Azure libraries to allow for easier mocking/testing.
type Client interface {
	ListResourceSKUs(ctx context.Context) (ResourceSKUsPage, error)
}

// ResourceSKUsPage is a page of results from listing resource SKUs.
type ResourceSKUsPage interface {
	NextWithContext(ctx context.Context) error
	NotDone() bool
	Values() []compute.ResourceSku
}

type azureClient struct {
	config         auth.ClientCredentialsConfig
	subscriptionID string
}

func (c *azureClient) ListResourceSKUs(ctx context.Context) (ResourceSKUsPage, error) {
	skusClient := compute.NewResourceSkusClient(c.subscriptionID)
	config := c.config
	config.Resource = azure.PublicCloud.ResourceManagerEndpoint
	authorizer, err := config.Authorizer()
	if err != nil {
		return nil, err
	}
	skusClient.Authorizer = authorizer
	page, err := skusClient.List(ctx)
	return &page, err
}

// NewClientFromSecret creates our client wrapper object for interacting with Azure. The Azure creds are read from the
// specified secret.
func NewClientFromSecret(secret *corev1.Secret) (Client, error) {
	return newClient(authJSONFromSecretSource(secret))
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
	return &azureClient{
		config:         auth.NewClientCredentialsConfig(clientID, clientSecret, tenantID),
		subscriptionID: subscriptionID,
	}, nil
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
