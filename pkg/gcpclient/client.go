package gcpclient

import (
	"context"
	"time"

	"github.com/openshift/hive/pkg/constants"
	"github.com/pkg/errors"
	"golang.org/x/oauth2/google"
	cloudresourcemanager "google.golang.org/api/cloudresourcemanager/v1"
	compute "google.golang.org/api/compute/v1"
	dns "google.golang.org/api/dns/v1"
	"google.golang.org/api/option"
	serviceusage "google.golang.org/api/serviceusage/v1"
	corev1 "k8s.io/api/core/v1"
)

//go:generate mockgen -source=./client.go -destination=./mock/client_generated.go -package=mock

// Client is a wrapper object for actual GCP libraries to allow for easier mocking/testing.
type Client interface {
	ListManagedZones(opts ListManagedZonesOptions) (*dns.ManagedZonesListResponse, error)

	ListResourceRecordSets(managedZone string, opts ListResourceRecordSetsOptions) (*dns.ResourceRecordSetsListResponse, error)

	AddResourceRecordSet(managedZone string, recordSet *dns.ResourceRecordSet) error

	DeleteResourceRecordSet(managedZone string, recordSet *dns.ResourceRecordSet) error

	GetManagedZone(managedZone string) (*dns.ManagedZone, error)

	CreateManagedZone(managedZone *dns.ManagedZone) (*dns.ManagedZone, error)

	DeleteManagedZone(managedZone string) error

	ListComputeZones(ListComputeZonesOptions) (*compute.ZoneList, error)

	ListComputeImages(ListComputeImagesOptions) (*compute.ImageList, error)
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
	computeClient              *compute.Service
	serviceUsageClient         *serviceusage.Service
	dnsClient                  *dns.Service
}

const (
	defaultCallTimeout = 2 * time.Minute

	// ErrCodeNotFound is what the google api returns when a resource doesn't exist.
	// googleapi does not give us a constant for this.
	ErrCodeNotFound = 404
)

func contextWithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, defaultCallTimeout)
}

func (c *gcpClient) GetManagedZone(managedZone string) (*dns.ManagedZone, error) {
	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()

	return c.dnsClient.ManagedZones.Get(c.projectName, managedZone).Context(ctx).Do()
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

func (c *gcpClient) CreateManagedZone(managedZone *dns.ManagedZone) (*dns.ManagedZone, error) {
	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()
	return c.dnsClient.ManagedZones.Create(c.projectName, managedZone).Context(ctx).Do()
}

func (c *gcpClient) DeleteManagedZone(managedZone string) error {
	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()
	return c.dnsClient.ManagedZones.Delete(c.projectName, managedZone).Context(ctx).Do()
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

// ListComputeZonesOptions are the options for listing compute zones.
type ListComputeZonesOptions struct {
	MaxResults int64
	PageToken  string
	Filter     string
}

func (c *gcpClient) ListComputeZones(opts ListComputeZonesOptions) (*compute.ZoneList, error) {
	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()

	call := c.computeClient.Zones.List(c.projectName).Filter(opts.Filter).Context(ctx)

	if opts.MaxResults > 0 {
		call.MaxResults(opts.MaxResults)
	}
	if opts.PageToken != "" {
		call.PageToken(opts.PageToken)
	}

	return call.Do()
}

// ListComputeImagesOptions are the options for listing compute images.
type ListComputeImagesOptions struct {
	MaxResults int64
	PageToken  string
	Filter     string
}

func (c *gcpClient) ListComputeImages(opts ListComputeImagesOptions) (*compute.ImageList, error) {
	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()

	call := c.computeClient.Images.List(c.projectName).Filter(opts.Filter).Context(ctx)

	if opts.MaxResults > 0 {
		call.MaxResults(opts.MaxResults)
	}
	if opts.PageToken != "" {
		call.PageToken(opts.PageToken)
	}
	return call.Do()
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

// NewClientFromSecret creates our client wrapper object for interacting with GCP.
func NewClientFromSecret(secret *corev1.Secret) (Client, error) {
	authJSON, ok := secret.Data[constants.GCPCredentialsName]
	if !ok {
		return nil, errors.New("creds secret does not contain \"" + constants.GCPCredentialsName + "\" data")
	}
	gcpClient, err := NewClientWithDefaultProject(authJSON)
	return gcpClient, errors.Wrap(err, "error creating GCP client")
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

	computeClient, err := compute.NewService(ctx, option.WithCredentials(creds))
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
		computeClient:              computeClient,
		serviceUsageClient:         serviceUsageClient,
		dnsClient:                  dnsClient,
	}, nil
}
