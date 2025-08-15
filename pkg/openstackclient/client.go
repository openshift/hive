package openstackclient

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v2"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/quotasets"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/secgroups"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/usage"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/openstack/imageservice/v2/images"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/networks"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/ports"
	"github.com/openshift/hive/pkg/constants"

	corev1 "k8s.io/api/core/v1"
)

//go:generate mockgen -source=./client.go -destination=./mock/client_generated.go -package=mock

// Client is a wrapper object for actual OpenStack libraries to allow for easier mocking/testing.
type Client interface {
	// Servers
	ListServers(ctx context.Context, opts *servers.ListOpts) ([]servers.Server, error)
	GetServer(ctx context.Context, serverID string) (*servers.Server, error)
	DeleteServer(ctx context.Context, serverID string) error
	CreateServerSnapshot(ctx context.Context, serverID, snapshotName string) (string, error)
	CreateServerFromOpts(ctx context.Context, opts *servers.CreateOpts) (*servers.Server, error)

	// Images
	GetImage(ctx context.Context, imageID string) (*images.Image, error)
	ListImages(ctx context.Context, opts *images.ListOpts) ([]images.Image, error)
	DeleteImage(ctx context.Context, imageID string) error

	// Networks
	GetNetworkByName(ctx context.Context, networkName string) (*networks.Network, error)
	ListPorts(ctx context.Context) ([]ports.Port, error)

	// Security Group Names
	GetServerSecurityGroupNames(ctx context.Context, serverID string) ([]string, error)

	// Project resources
	GetComputeQuotas(ctx context.Context) (*quotasets.QuotaSet, error)
	GetComputeUsage(ctx context.Context) (*usage.TenantUsage, error)
	GetFlavorDetails(ctx context.Context, flavorID string) (*flavors.Flavor, error)
}

type ResourceRequirements struct {
	Instances int
	VCPUs     int
	RAM       int
}

type Credentials struct {
	AuthURL            string `json:"auth_url"`
	Username           string `json:"username,omitempty"`
	Password           string `json:"password,omitempty"`
	UserID             string `json:"user_id,omitempty"`
	ProjectID          string `json:"project_id,omitempty"`
	ProjectName        string `json:"project_name,omitempty"`
	UserDomainName     string `json:"user_domain_name,omitempty"`
	UserDomainID       string `json:"user_domain_id,omitempty"`
	ProjectDomainName  string `json:"project_domain_name,omitempty"`
	ProjectDomainID    string `json:"project_domain_id,omitempty"`
	RegionName         string `json:"region_name,omitempty"`
	Interface          string `json:"interface,omitempty"`
	IdentityAPIVersion string `json:"identity_api_version,omitempty"`

	TenantID   string `json:"tenant_id,omitempty"`
	TenantName string `json:"tenant_name,omitempty"`
	DomainID   string `json:"domain_id,omitempty"`
	DomainName string `json:"domain_name,omitempty"`
	Region     string `json:"region,omitempty"`
}

type CloudsYAML struct {
	Clouds map[string]CloudConfig `yaml:"clouds"`
}

type CloudConfig struct {
	Auth      CloudAuth `yaml:"auth"`
	Region    string    `yaml:"region_name"`
	Interface string    `yaml:"interface"`
	Version   string    `yaml:"identity_api_version"`
}

type CloudAuth struct {
	AuthURL           string `yaml:"auth_url"`
	Username          string `yaml:"username"`
	Password          string `yaml:"password"`
	ProjectID         string `yaml:"project_id"`
	ProjectName       string `yaml:"project_name"`
	UserDomainName    string `yaml:"user_domain_name"`
	ProjectDomainName string `yaml:"project_domain_name"`
	UserDomainID      string `yaml:"user_domain_id"`
	ProjectDomainID   string `yaml:"project_domain_id"`
}

type openstackClient struct {
	provider      *gophercloud.ProviderClient
	computeClient *gophercloud.ServiceClient
	imageClient   *gophercloud.ServiceClient
	networkClient *gophercloud.ServiceClient
	credentials   *Credentials
}

// Implementation of server methods
func (c *openstackClient) ListServers(ctx context.Context, opts *servers.ListOpts) ([]servers.Server, error) {
	if opts == nil {
		opts = &servers.ListOpts{}
	}
	allPages, err := servers.List(c.computeClient, opts).AllPages()
	if err != nil {
		return nil, err
	}
	return servers.ExtractServers(allPages)
}

func (c *openstackClient) GetServer(ctx context.Context, serverID string) (*servers.Server, error) {
	return servers.Get(c.computeClient, serverID).Extract()
}

func (c *openstackClient) DeleteServer(ctx context.Context, serverID string) error {
	return servers.Delete(c.computeClient, serverID).ExtractErr()
}

// Create a snapshot of the specified server
func (c *openstackClient) CreateServerSnapshot(ctx context.Context, serverID, snapshotName string) (string, error) {
	createImageOpts := servers.CreateImageOpts{
		Name: snapshotName,
	}

	result := servers.CreateImage(c.computeClient, serverID, createImageOpts)
	imageID, err := result.ExtractImageID()
	if err != nil {
		return "", fmt.Errorf("failed to create snapshot: %w", err)
	}

	return imageID, nil
}

func (c *openstackClient) ListImages(ctx context.Context, opts *images.ListOpts) ([]images.Image, error) {
	if opts == nil {
		opts = &images.ListOpts{}
	}
	allPages, err := images.List(c.imageClient, opts).AllPages()
	if err != nil {
		return nil, err
	}
	return images.ExtractImages(allPages)
}

func (c *openstackClient) DeleteImage(ctx context.Context, imageID string) error {
	return images.Delete(c.imageClient, imageID).ExtractErr()
}

// Creates a new server
func (c *openstackClient) CreateServerFromOpts(ctx context.Context, opts *servers.CreateOpts) (*servers.Server, error) {
	server, err := servers.Create(c.computeClient, opts).Extract()
	if err != nil {
		return nil, fmt.Errorf("failed to create server: %w", err)
	}
	return server, nil
}

// Image methods
func (c *openstackClient) GetImage(ctx context.Context, imageID string) (*images.Image, error) {
	return images.Get(c.imageClient, imageID).Extract()
}

// Ports
func (c *openstackClient) ListPorts(ctx context.Context) ([]ports.Port, error) {
	allPages, err := ports.List(c.networkClient, nil).AllPages()
	if err != nil {
		return nil, err
	}

	return ports.ExtractPorts(allPages)
}

// Network specific
func (c *openstackClient) GetNetworkByName(ctx context.Context, networkName string) (*networks.Network, error) {
	listOpts := networks.ListOpts{
		Name: networkName,
	}

	allPages, err := networks.List(c.networkClient, listOpts).AllPages()
	if err != nil {
		return nil, err
	}

	networkList, err := networks.ExtractNetworks(allPages)
	if err != nil {
		return nil, err
	}

	if len(networkList) == 0 {
		return nil, fmt.Errorf("network with name '%s' not found", networkName)
	}

	// Return the first match directly
	return &networkList[0], nil
}

// Get the security group names for a specific server
func (c *openstackClient) GetServerSecurityGroupNames(ctx context.Context, serverID string) ([]string, error) {
	serverSecGroups, err := secgroups.ListByServer(c.computeClient, serverID).AllPages()
	if err != nil {
		return nil, err
	}

	secGroupList, err := secgroups.ExtractSecurityGroups(serverSecGroups)
	if err != nil {
		return nil, err
	}

	var secGroupNames []string
	for _, secGroup := range secGroupList {
		secGroupNames = append(secGroupNames, secGroup.Name)
	}

	return secGroupNames, nil
}

// Gets the compute quotas for the current project
func (c *openstackClient) GetComputeQuotas(ctx context.Context) (*quotasets.QuotaSet, error) {
	projectID := c.credentials.ProjectID
	if projectID == "" {
		projectID = c.credentials.TenantID
	}
	if projectID == "" {
		return nil, fmt.Errorf("no project ID found in credentials")
	}

	quotaSet, err := quotasets.Get(c.computeClient, projectID).Extract()
	if err != nil {
		return nil, fmt.Errorf("failed to get quota set: %w", err)
	}

	return quotaSet, nil
}

// Gets the current compute usage for the project
func (c *openstackClient) GetComputeUsage(ctx context.Context) (*usage.TenantUsage, error) {
	projectID := c.credentials.ProjectID
	if projectID == "" {
		projectID = c.credentials.TenantID
	}
	if projectID == "" {
		return nil, fmt.Errorf("no project ID found in credentials")
	}

	// Get usage for current project
	allPages, err := usage.SingleTenant(c.computeClient, projectID, usage.SingleTenantOpts{}).AllPages()
	if err != nil {
		return nil, fmt.Errorf("failed to get compute usage pages: %w", err)
	}

	// Extract usage from pages
	tenantUsage, err := usage.ExtractSingleTenant(allPages)
	if err != nil {
		return nil, fmt.Errorf("failed to extract compute usage: %w", err)
	}

	// Note: tenantUsage can be nil - caller needs to handle this
	return tenantUsage, nil
}

// Gets detailed information about a specific flavor
func (c *openstackClient) GetFlavorDetails(ctx context.Context, flavorID string) (*flavors.Flavor, error) {
	return flavors.Get(c.computeClient, flavorID).Extract()
}

// Create a client wrapper object for interacting with OpenStack.
func NewClientFromSecret(secret *corev1.Secret) (Client, error) {
	cloudsYaml, ok := secret.Data[constants.OpenStackCredentialsName]
	if !ok {
		return nil, fmt.Errorf("secret does not contain %q data", constants.OpenStackCredentialsName)
	}
	return newClientFromCloudsYAML(cloudsYaml)
}

// Creates a client from clouds.yaml data
func newClientFromCloudsYAML(cloudsYamlData []byte) (Client, error) {
	var clouds CloudsYAML
	if err := yaml.Unmarshal(cloudsYamlData, &clouds); err != nil {
		return nil, fmt.Errorf("failed to parse clouds.yaml: %w", err)
	}

	// Get the "openstack" cloud config
	openstackCloud, ok := clouds.Clouds["openstack"]
	if !ok {
		return nil, fmt.Errorf("no 'openstack' cloud found in clouds.yaml")
	}

	// Convert to Credentials struct
	creds := &Credentials{
		AuthURL:            openstackCloud.Auth.AuthURL,
		Username:           openstackCloud.Auth.Username,
		Password:           openstackCloud.Auth.Password,
		ProjectID:          openstackCloud.Auth.ProjectID,
		ProjectName:        openstackCloud.Auth.ProjectName,
		UserDomainName:     openstackCloud.Auth.UserDomainName,
		ProjectDomainName:  openstackCloud.Auth.ProjectDomainName,
		UserDomainID:       openstackCloud.Auth.UserDomainID,
		ProjectDomainID:    openstackCloud.Auth.ProjectDomainID,
		RegionName:         openstackCloud.Region,
		Interface:          openstackCloud.Interface,
		IdentityAPIVersion: openstackCloud.Version,
	}

	return newClientFromStruct(creds)
}

// getRegion helper function
func getRegion(creds *Credentials) string {
	if creds.RegionName != "" {
		return creds.RegionName
	}
	if creds.Region != "" {
		return creds.Region
	}
	return "RegionOne"
}

func newClientFromStruct(creds *Credentials) (*openstackClient, error) {
	// Validate required credentials
	if creds.AuthURL == "" {
		return nil, fmt.Errorf("missing auth_url in credentials")
	}

	// Authentication options
	authOpts := gophercloud.AuthOptions{
		IdentityEndpoint: creds.AuthURL,
		Username:         creds.Username,
		UserID:           creds.UserID,
		Password:         creds.Password,
		TenantID:         creds.ProjectID,
		TenantName:       creds.ProjectName,
		DomainName:       creds.UserDomainName,
		DomainID:         creds.UserDomainID,
	}

	// Authenticate and get provider client
	provider, err := openstack.AuthenticatedClient(authOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate with OpenStack: %w", err)
	}

	region := getRegion(creds)

	interfaceType := gophercloud.AvailabilityPublic
	switch creds.Interface {
	case "internal":
		interfaceType = gophercloud.AvailabilityInternal
	case "admin":
		interfaceType = gophercloud.AvailabilityAdmin
	}

	endpointOpts := gophercloud.EndpointOpts{
		Region:       region,
		Availability: interfaceType,
	}

	// Create service clients
	computeClient, err := openstack.NewComputeV2(provider, endpointOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create compute client: %w", err)
	}

	imageClient, err := openstack.NewImageServiceV2(provider, endpointOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create image client: %w", err)
	}

	networkClient, err := openstack.NewNetworkV2(provider, endpointOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create network client: %w", err)
	}

	return &openstackClient{
		provider:      provider,
		computeClient: computeClient,
		imageClient:   imageClient,
		networkClient: networkClient,
		credentials:   creds,
	}, nil
}
