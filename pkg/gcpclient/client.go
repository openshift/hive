package gcpclient

import (
	"context"
	"io/ioutil"
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

	UpdateResourceRecordSet(managedZone string, addRecordSet, removeRecordSet *dns.ResourceRecordSet) error

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

func (c *gcpClient) UpdateResourceRecordSet(managedZone string, addRecordSet, removeRecordSet *dns.ResourceRecordSet) error {
	change := &dns.Change{}
	if addRecordSet != nil && len(addRecordSet.Rrdatas) > 0 {
		change.Additions = []*dns.ResourceRecordSet{addRecordSet}
	}
	if removeRecordSet != nil && len(removeRecordSet.Rrdatas) > 0 {
		change.Deletions = []*dns.ResourceRecordSet{removeRecordSet}
	}
	return c.changeResourceRecordSet(
		managedZone,
		change,
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

// NewClient creates our client wrapper object for interacting with GCP. The supplied byte slice contains the GCP creds.
func NewClient(authJSON []byte) (Client, error) {
	return newClient(authJSONPassthroughSource(authJSON))
}

// NewClientFromSecret creates our client wrapper object for interacting with GCP. The GCP creds are read from the
// specified secret.
func NewClientFromSecret(secret *corev1.Secret) (Client, error) {
	return newClient(authJSONFromSecretSource(secret))
}

// NewClientFromFile creates our client wrapper object for interacting with GCP. The GCP creds are read from the
// specified file.
func NewClientFromFile(filename string) (Client, error) {
	return newClient(authJSONFromFileSource(filename))
}

// ProjectID returns the GCP project ID specified in the GCP creds. The supplied byte slice contains the GCP creds.
func ProjectID(authJSON []byte) (string, error) {
	return projectID(authJSONPassthroughSource(authJSON))
}

// ProjectIDFromSecret returns the GCP project ID specified in the GCP creds. The GCP creds are read from the
// specified secret.
func ProjectIDFromSecret(secret *corev1.Secret) (string, error) {
	return projectID(authJSONFromSecretSource(secret))
}

// ProjectIDFromFile returns the GCP project ID specified in the GCP creds. The GCP creds are read from the
// specified file.
func ProjectIDFromFile(filename string) (string, error) {
	return projectID(authJSONFromFileSource(filename))
}

func projectID(authJSONSource func() ([]byte, error)) (string, error) {
	authJSON, err := authJSONSource()
	if err != nil {
		return "", err
	}
	creds, err := google.CredentialsFromJSON(context.Background(), authJSON)
	if err != nil {
		return "", err
	}
	return creds.ProjectID, nil
}

func newClient(authJSONSource func() ([]byte, error)) (*gcpClient, error) {
	ctx := context.TODO()

	authJSON, err := authJSONSource()
	if err != nil {
		return nil, err
	}
	// since we're using a single creds var, we should specify all the required scopes when initializing
	creds, err := google.CredentialsFromJSON(ctx, authJSON, dns.CloudPlatformScope)
	if err != nil {
		return nil, err
	}

	options := []option.ClientOption{
		option.WithCredentials(creds),
		option.WithUserAgent("openshift.io hive/v1"),
	}
	cloudResourceManagerClient, err := cloudresourcemanager.NewService(ctx, options...)
	if err != nil {
		return nil, err
	}

	computeClient, err := compute.NewService(ctx, options...)
	if err != nil {
		return nil, err
	}

	serviceUsageClient, err := serviceusage.NewService(ctx, options...)
	if err != nil {
		return nil, err
	}

	dnsClient, err := dns.NewService(ctx, options...)
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

func authJSONPassthroughSource(authJSON []byte) func() ([]byte, error) {
	return func() ([]byte, error) {
		return authJSON, nil
	}
}

func authJSONFromSecretSource(secret *corev1.Secret) func() ([]byte, error) {
	return func() ([]byte, error) {
		authJSON, ok := secret.Data[constants.GCPCredentialsName]
		if !ok {
			return nil, errors.New("creds secret does not contain \"" + constants.GCPCredentialsName + "\" data")
		}
		return authJSON, nil
	}
}

func authJSONFromFileSource(filename string) func() ([]byte, error) {
	return func() ([]byte, error) {
		return ioutil.ReadFile(filename)
	}
}
