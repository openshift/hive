package gcpclient

import (
	"context"
	"time"

	"golang.org/x/oauth2/google"
	cloudresourcemanager "google.golang.org/api/cloudresourcemanager/v1"
	dns "google.golang.org/api/dns/v1"
	"google.golang.org/api/option"
	serviceusage "google.golang.org/api/serviceusage/v1"
)

//go:generate mockgen -source=./client.go -destination=./mock/client_generated.go -package=mock

// Client is a wrapper object for actual GCP libraries to allow for easier mocking/testing.
type Client interface {
	ListManagedZones() (*dns.ManagedZonesListResponse, error)
}

type gcpClient struct {
	projectName                string
	creds                      *google.Credentials
	cloudResourceManagerClient *cloudresourcemanager.Service
	serviceUsageClient         *serviceusage.Service
	dnsClient                  *dns.Service
}

const (
	defaultCallTimeout = 2 * time.Minute
)

func contextWithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, defaultCallTimeout)
}

func (c *gcpClient) ListManagedZones() (*dns.ManagedZonesListResponse, error) {
	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()

	return c.dnsClient.ManagedZones.List(c.projectName).Context(ctx).Do()
}

// NewClient creates our client wrapper object for interacting with GCP.
func NewClient(projectName string, authJSON []byte) (Client, error) {
	ctx := context.TODO()

	// since we're using a single creds var, we should specify all the required scopes when initializing
	creds, err := google.CredentialsFromJSON(context.TODO(), authJSON, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, err
	}

	cloudResourceManagerClient, err := cloudresourcemanager.NewService(ctx, option.WithCredentials(creds))
	if err != nil {
		return nil, err
	}

	serviceUsageClient, err := serviceusage.NewService(ctx, option.WithCredentials(creds))
	if err != nil {
		return nil, err
	}

	dnsClient, err := dns.NewService(ctx, option.WithCredentials(creds))
	if err != nil {
		return nil, err
	}

	return &gcpClient{
		projectName:                projectName,
		creds:                      creds,
		cloudResourceManagerClient: cloudResourceManagerClient,
		serviceUsageClient:         serviceUsageClient,
		dnsClient:                  dnsClient,
	}, nil
}
