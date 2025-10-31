package openstackclient

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/secgroups"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/openstack/imageservice/v2/images"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/networks"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/ports"
	"github.com/gophercloud/utils/openstack/clientconfig"

	"gopkg.in/yaml.v2"

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

	// Instance state management
	PauseServer(ctx context.Context, serverID string) error
	UnpauseServer(ctx context.Context, serverID string) error

	// Networks
	GetNetworkByName(ctx context.Context, networkName string) (*networks.Network, error)
	ListPorts(ctx context.Context) ([]ports.Port, error)

	// Tags
	GetServerTags(ctx context.Context, serverID string) ([]string, error)
	SetServerTags(ctx context.Context, serverID string, tags []string) error

	// Security Group Names
	GetServerSecurityGroupNames(ctx context.Context, serverID string) ([]string, error)
}

type openstackClient struct {
	provider      *gophercloud.ProviderClient
	computeClient *gophercloud.ServiceClient
	imageClient   *gophercloud.ServiceClient
	networkClient *gophercloud.ServiceClient
}

var _ Client = &openstackClient{}

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

// PauseServer pauses a server using the raw OpenStack API
func (c *openstackClient) PauseServer(ctx context.Context, serverID string) error {
	url := c.computeClient.ServiceURL("servers", serverID, "action")

	reqBody := map[string]interface{}{
		"pause": nil,
	}

	_, err := c.computeClient.Post(url, reqBody, nil, &gophercloud.RequestOpts{
		OkCodes: []int{202},
	})
	return err
}

// UnpauseServer unpauses a server using the raw OpenStack API
func (c *openstackClient) UnpauseServer(ctx context.Context, serverID string) error {
	url := c.computeClient.ServiceURL("servers", serverID, "action")

	reqBody := map[string]interface{}{
		"unpause": nil,
	}

	_, err := c.computeClient.Post(url, reqBody, nil, &gophercloud.RequestOpts{
		OkCodes: []int{202},
	})
	return err
}

func (c *openstackClient) GetServerTags(ctx context.Context, serverID string) ([]string, error) {
	url := c.computeClient.ServiceURL("servers", serverID, "tags")

	var result struct {
		Tags []string `json:"tags"`
	}

	_, err := c.computeClient.Get(url, &result, nil)
	if err != nil {
		// Just return empty tags if the API call fails for any reason
		return []string{}, nil
	}

	return result.Tags, nil
}

func (c *openstackClient) SetServerTags(ctx context.Context, serverID string, tags []string) error {
	url := c.computeClient.ServiceURL("servers", serverID, "tags")

	reqBody := map[string][]string{
		"tags": tags,
	}

	_, err := c.computeClient.Put(url, reqBody, nil, &gophercloud.RequestOpts{
		OkCodes: []int{200},
	})
	return err
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

// Create a client wrapper object for interacting with OpenStack.
func NewClientFromSecret(secret *corev1.Secret) (Client, error) {
	cloudsYaml, ok := secret.Data[constants.OpenStackCredentialsName]
	if !ok {
		return nil, fmt.Errorf("secret does not contain %q data", constants.OpenStackCredentialsName)
	}

	// Use upstream structs instead of custom ones
	var clouds clientconfig.Clouds
	if err := yaml.Unmarshal(cloudsYaml, &clouds); err != nil {
		return nil, fmt.Errorf("failed to parse clouds.yaml: %w", err)
	}

	// Get the first cloud (or specific cloud name)
	var cloud *clientconfig.Cloud
	for _, c := range clouds.Clouds {
		cloud = &c
		break
	}

	if cloud == nil {
		return nil, fmt.Errorf("no cloud configuration found in clouds.yaml")
	}

	authInfo := &clientconfig.AuthInfo{
		AuthURL:           cloud.AuthInfo.AuthURL,
		Username:          cloud.AuthInfo.Username,
		Password:          cloud.AuthInfo.Password,
		ProjectID:         cloud.AuthInfo.ProjectID,
		ProjectName:       cloud.AuthInfo.ProjectName,
		UserDomainName:    cloud.AuthInfo.UserDomainName,
		ProjectDomainName: cloud.AuthInfo.ProjectDomainName,
	}

	opts := &clientconfig.ClientOpts{
		AuthInfo:     authInfo,
		RegionName:   cloud.RegionName,
		EndpointType: cloud.Interface,
	}

	provider, err := clientconfig.AuthenticatedClient(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create authenticated client: %w", err)
	}

	computeClient, err := clientconfig.NewServiceClient("compute", opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create compute client: %w", err)
	}

	// Specify microversion in order to allow instance tagging
	computeClient.Microversion = "2.26"

	imageClient, err := clientconfig.NewServiceClient("image", opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create image client: %w", err)
	}

	networkClient, err := clientconfig.NewServiceClient("network", opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create network client: %w", err)
	}

	return &openstackClient{
		provider:      provider,
		computeClient: computeClient,
		imageClient:   imageClient,
		networkClient: networkClient,
	}, nil
}
