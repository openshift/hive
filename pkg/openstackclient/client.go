package openstackclient

import (
	"context"
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v2"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/quotasets"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/secgroups"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/tags"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/usage"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/openstack/imageservice/v2/images"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/networks"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/ports"
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
	CreateServerFromOpts(ctx context.Context, opts *ServerCreateOpts) (*servers.Server, error)

	// Tags
	SetServerTags(ctx context.Context, serverID string, serverTags []string) error
	GetServerTags(ctx context.Context, serverID string) ([]string, error)

	// Images - only snapshot checking
	GetImage(ctx context.Context, imageID string) (*images.Image, error)

	// Networks
	GetNetworkByName(ctx context.Context, networkName string) (*Network, error)
	ListPorts(ctx context.Context) ([]Port, error)

	// Security Groups
	GetServerSecurityGroups(ctx context.Context, serverID string) ([]string, error)

	// Project resources
	GetComputeQuotas(ctx context.Context) (*ComputeQuotas, error)
	GetComputeUsage(ctx context.Context) (*ComputeUsage, error)
	GetFlavorDetails(ctx context.Context, flavorID string) (*FlavorDetails, error)
}

type Network struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type Port struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ComputeQuotas struct {
	Instances int `json:"instances"`
	Cores     int `json:"cores"`
	RAM       int `json:"ram"`
}

type ComputeUsage struct {
	InstancesUsed int `json:"totalInstancesUsed"`
	CoresUsed     int `json:"totalCoresUsed"`
	RAMUsed       int `json:"totalRAMUsed"`
}

type FlavorDetails struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	VCPUs int    `json:"vcpus"`
	RAM   int    `json:"ram"`
	Disk  int    `json:"disk"`
}

type ResourceRequirements struct {
	Instances int
	VCPUs     int
	RAM       int
}

// Options for creating a server
type ServerCreateOpts struct {
	Name           string            `json:"name"`
	ImageRef       string            `json:"imageRef"`
	FlavorRef      string            `json:"flavorRef"`
	NetworkID      string            `json:"networkID"`
	PortID         string            `json:"portID"`
	SecurityGroups []string          `json:"securityGroups"`
	Metadata       map[string]string `json:"metadata"`
}

// OpenStack credentials structure
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
	server, err := servers.Get(c.computeClient, serverID).Extract()
	return server, err
}

func (c *openstackClient) DeleteServer(ctx context.Context, serverID string) error {
	return servers.Delete(c.computeClient, serverID).ExtractErr()
}

// Create a snapshot of the specified server
func (c *openstackClient) CreateServerSnapshot(ctx context.Context, serverID, snapshotName string) (string, error) {
	// Create image options
	createImageOpts := servers.CreateImageOpts{
		Name: snapshotName,
		Metadata: map[string]string{
			"snapshot_type": "server_snapshot",
			"source_server": serverID,
		},
	}

	result := servers.CreateImage(c.computeClient, serverID, createImageOpts)
	imageID, err := result.ExtractImageID()
	if err != nil {
		return "", fmt.Errorf("failed to create snapshot: %w", err)
	}

	return imageID, nil
}

// Creates a new server
func (c *openstackClient) CreateServerFromOpts(ctx context.Context, opts *ServerCreateOpts) (*servers.Server, error) {
	// Build networks slice for the NIC
	networks := []servers.Network{
		{
			UUID: opts.NetworkID,
			Port: opts.PortID,
		},
	}

	// Convert custom options to satisfy GopherCloud
	createOpts := &servers.CreateOpts{
		Name:           opts.Name,
		ImageRef:       opts.ImageRef,
		FlavorRef:      opts.FlavorRef,
		Networks:       networks,
		SecurityGroups: opts.SecurityGroups,
		Metadata:       opts.Metadata,
	}

	server, err := servers.Create(c.computeClient, createOpts).Extract()
	if err != nil {
		return nil, fmt.Errorf("failed to create server: %w", err)
	}

	return server, nil
}

// Image methods
func (c *openstackClient) GetImage(ctx context.Context, imageID string) (*images.Image, error) {
	image, err := images.Get(c.imageClient, imageID).Extract()
	return image, err
}

// Ports
func (c *openstackClient) ListPorts(ctx context.Context) ([]Port, error) {
	allPages, err := ports.List(c.networkClient, nil).AllPages()
	if err != nil {
		return nil, err
	}

	portList, err := ports.ExtractPorts(allPages)
	if err != nil {
		return nil, err
	}

	var result []Port
	for _, port := range portList {
		result = append(result, Port{
			ID:   port.ID,
			Name: port.Name,
		})
	}

	return result, nil
}

// Network specific
func (c *openstackClient) GetNetworkByName(ctx context.Context, networkName string) (*Network, error) {
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

	// Return the first match
	net := networkList[0]
	return &Network{
		ID:     net.ID,
		Name:   net.Name,
		Status: net.Status,
	}, nil
}

// Get the security group names for a specific server
func (c *openstackClient) GetServerSecurityGroups(ctx context.Context, serverID string) ([]string, error) {
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

// Adding tags
func (c *openstackClient) SetServerTags(ctx context.Context, serverID string, serverTags []string) error {
	// Delete all existing tags first
	err := tags.DeleteAll(c.computeClient, serverID).ExtractErr()
	if err != nil {
		return fmt.Errorf("failed to clear existing tags for server %s: %w", serverID, err)
	}

	// Add new tags one by one if any exist
	for _, tag := range serverTags {
		err = tags.Add(c.computeClient, serverID, tag).ExtractErr()
		if err != nil {
			return fmt.Errorf("failed to set tag '%s' for server %s: %w", tag, serverID, err)
		}
	}

	return nil
}

// Gets all tags for a server
func (c *openstackClient) GetServerTags(ctx context.Context, serverID string) ([]string, error) {
	serverTags, err := tags.List(c.computeClient, serverID).Extract()
	if err != nil {
		return nil, fmt.Errorf("failed to get tags for server %s: %w", serverID, err)
	}
	return serverTags, nil
}

// Gets the compute quotas for the current project
func (c *openstackClient) GetComputeQuotas(ctx context.Context) (*ComputeQuotas, error) {
	projectID := c.credentials.ProjectID
	if projectID == "" {
		projectID = c.credentials.TenantID // fallback to legacy field
	}
	if projectID == "" {
		return nil, fmt.Errorf("no project ID found in credentials")
	}

	quotaSet, err := quotasets.Get(c.computeClient, projectID).Extract()
	if err != nil {
		return nil, fmt.Errorf("failed to get quota set: %w", err)
	}

	return &ComputeQuotas{
		Instances: quotaSet.Instances,
		Cores:     quotaSet.Cores,
		RAM:       quotaSet.RAM,
	}, nil
}

// Gets the current compute usage for the project
func (c *openstackClient) GetComputeUsage(ctx context.Context) (*ComputeUsage, error) {
	projectID := c.credentials.ProjectID
	if projectID == "" {
		projectID = c.credentials.TenantID // fallback to legacy field
	}
	if projectID == "" {
		return nil, fmt.Errorf("no project ID found in credentials")
	}

	// Create usage options
	usageOpts := usage.SingleTenantOpts{}

	// Get usage for current project
	allPages, err := usage.SingleTenant(c.computeClient, projectID, usageOpts).AllPages()
	if err != nil {
		return nil, fmt.Errorf("failed to get compute usage pages: %w", err)
	}

	// Extract usage from pages
	tenantUsage, err := usage.ExtractSingleTenant(allPages)
	if err != nil {
		return nil, fmt.Errorf("failed to extract compute usage: %w", err)
	}

	// Check if we got valid usage data
	if tenantUsage == nil {
		return &ComputeUsage{
			InstancesUsed: 0,
			CoresUsed:     0,
			RAMUsed:       0,
		}, nil
	}

	return &ComputeUsage{
		InstancesUsed: len(tenantUsage.ServerUsages),
		CoresUsed:     int(tenantUsage.TotalVCPUsUsage),
		RAMUsed:       int(tenantUsage.TotalMemoryMBUsage),
	}, nil
}

// Gets detailed information about a specific flavor
func (c *openstackClient) GetFlavorDetails(ctx context.Context, flavorID string) (*FlavorDetails, error) {
	flavor, err := flavors.Get(c.computeClient, flavorID).Extract()
	if err != nil {
		return nil, fmt.Errorf("failed to get flavor details: %w", err)
	}

	return &FlavorDetails{
		ID:    flavor.ID,
		Name:  flavor.Name,
		VCPUs: flavor.VCPUs,
		RAM:   flavor.RAM,
		Disk:  flavor.Disk,
	}, nil
}

// Create a client wrapper object for interacting with OpenStack.
func NewClientFromSecret(secret *corev1.Secret) (Client, error) {
	cloudsYaml, ok := secret.Data["clouds.yaml"]
	if !ok {
		return nil, fmt.Errorf("secret does not contain \"clouds.yaml\" data")
	}
	return newClientFromCloudsYAML(cloudsYaml)
}

// newClientFromCloudsYAML creates a client from clouds.yaml data
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

// newClientFromStruct creates a client directly from a Credentials struct (no JSON conversion needed)
func newClientFromStruct(creds *Credentials) (*openstackClient, error) {
	// Validate required credentials
	if creds.AuthURL == "" {
		return nil, fmt.Errorf("missing auth_url in credentials")
	}

	// Create authentication options
	authOpts := gophercloud.AuthOptions{
		IdentityEndpoint: creds.AuthURL,
		Username:         creds.Username,
		UserID:           creds.UserID,
		Password:         creds.Password,
		TenantID:         creds.ProjectID, // Use ProjectID as TenantID
		TenantName:       creds.ProjectName,
		DomainName:       creds.UserDomainName,
	}

	if creds.TenantID != "" {
		authOpts.TenantID = creds.TenantID
	}
	if creds.TenantName != "" {
		authOpts.TenantName = creds.TenantName
	}
	if creds.DomainID != "" {
		authOpts.DomainID = creds.DomainID
	}
	if creds.DomainName != "" {
		authOpts.DomainName = creds.DomainName
	}
	if creds.UserDomainID != "" {
		authOpts.DomainID = creds.UserDomainID
	}
	if creds.ProjectDomainID != "" {
		authOpts.DomainID = creds.ProjectDomainID
	}
	if creds.ProjectDomainName != "" {
		authOpts.DomainName = creds.ProjectDomainName
	}

	// Authenticate and get provider client
	provider, err := openstack.AuthenticatedClient(authOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate with OpenStack: %w", err)
	}

	// Set region
	region := creds.RegionName
	if region == "" {
		region = creds.Region
	}
	if region == "" {
		region = "RegionOne" 
	}

	interfaceType := gophercloud.AvailabilityPublic
	if creds.Interface != "" {
		switch creds.Interface {
		case "public":
			interfaceType = gophercloud.AvailabilityPublic
		case "internal":
			interfaceType = gophercloud.AvailabilityInternal
		case "admin":
			interfaceType = gophercloud.AvailabilityAdmin
		}
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
