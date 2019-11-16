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
	ListManagedZones(opts ListManagedZonesOptions) (*dns.ManagedZonesListResponse, error)

	ListResourceRecordSets(managedZone string, opts ListResourceRecordSetsOptions) (*dns.ResourceRecordSetsListResponse, error)

	AddResourceRecordSet(managedZone string, recordSet *dns.ResourceRecordSet) error

	DeleteResourceRecordSet(managedZone string, recordSet *dns.ResourceRecordSet) error
}

// ListManagedZonesOptions are the options for listing managed zones.
type ListManagedZonesOptions struct {
	MaxResults int64
	PageToken  string
	DNSName    string
}

// ListResourceRecordSetsOptions are the options for listing resource record sets.
type ListResourceRecordSetsOptions struct {
	MaxResults int64
	PageToken  string
	Name       string
	Type       string
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

func (c *gcpClient) ListManagedZones(opts ListManagedZonesOptions) (*dns.ManagedZonesListResponse, error) {
	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()
	call := c.dnsClient.ManagedZones.List(c.projectName).Context(ctx)
	if opts.MaxResults > 0 {
		call.MaxResults(opts.MaxResults)
	}
	if opts.PageToken != "" {
		call.PageToken(opts.PageToken)
	}
	if opts.DNSName != "" {
		call.DnsName(opts.DNSName)
	}
	return call.Do()
}

func (c *gcpClient) ListResourceRecordSets(managedZone string, opts ListResourceRecordSetsOptions) (*dns.ResourceRecordSetsListResponse, error) {
	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()
	call := c.dnsClient.ResourceRecordSets.List(c.projectName, managedZone).Context(ctx)
	if opts.MaxResults > 0 {
		call.MaxResults(opts.MaxResults)
	}
	if opts.PageToken != "" {
		call.PageToken(opts.PageToken)
	}
	if opts.Name != "" {
		call.Name(opts.Name)
	}
	if opts.Type != "" {
		call.Type(opts.Type)
	}
	return call.Do()
}

func (c *gcpClient) AddResourceRecordSet(managedZone string, recordSet *dns.ResourceRecordSet) error {
	return c.changeResourceRecordSet(
		managedZone,
		&dns.Change{
			Additions: []*dns.ResourceRecordSet{recordSet},
		},
	)
}

func (c *gcpClient) DeleteResourceRecordSet(managedZone string, recordSet *dns.ResourceRecordSet) error {
	return c.changeResourceRecordSet(
		managedZone,
		&dns.Change{
			Deletions: []*dns.ResourceRecordSet{recordSet},
		},
	)
}

func (c *gcpClient) changeResourceRecordSet(managedZone string, change *dns.Change) error {
	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()
	_, err := c.dnsClient.Changes.Create(c.projectName, managedZone, change).Context(ctx).Do()
	return err
}

// NewClient creates our client wrapper object for interacting with GCP.
func NewClient(projectName string, authJSON []byte) (Client, error) {
	c, err := newClient(authJSON)
	if err != nil {
		return nil, err
	}
	c.projectName = projectName
	return c, nil
}

// NewClientWithDefaultProject creates our client wrapper object for interacting with GCP.
func NewClientWithDefaultProject(authJSON []byte) (Client, error) {
	return newClient(authJSON)
}

func newClient(authJSON []byte) (*gcpClient, error) {
	ctx := context.TODO()

	// since we're using a single creds var, we should specify all the required scopes when initializing
	creds, err := google.CredentialsFromJSON(ctx, authJSON, dns.CloudPlatformScope)
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
		projectName:                creds.ProjectID,
		creds:                      creds,
		cloudResourceManagerClient: cloudResourceManagerClient,
		serviceUsageClient:         serviceUsageClient,
		dnsClient:                  dnsClient,
	}, nil
}
