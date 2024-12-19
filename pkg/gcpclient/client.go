package gcpclient

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/oauth2/google"

	cloudresourcemanager "google.golang.org/api/cloudresourcemanager/v1"
	compute "google.golang.org/api/compute/v1"
	dns "google.golang.org/api/dns/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	serviceusage "google.golang.org/api/serviceusage/v1"

	corev1 "k8s.io/api/core/v1"

	"github.com/openshift/hive/pkg/constants"
)

//go:generate mockgen -source=./client.go -destination=./mock/client_generated.go -package=mock

// Client is a wrapper object for actual GCP libraries to allow for easier mocking/testing.
type Client interface {
	ListManagedZones(opts ListManagedZonesOptions) (*dns.ManagedZonesListResponse, error)

	ListResourceRecordSets(managedZone string, opts ListResourceRecordSetsOptions) (*dns.ResourceRecordSetsListResponse, error)

	AddResourceRecordSet(managedZone string, recordSet *dns.ResourceRecordSet) error

	DeleteResourceRecordSet(managedZone string, recordSet *dns.ResourceRecordSet) error

	DeleteResourceRecordSets(managedZone string, recordSet []*dns.ResourceRecordSet) error

	UpdateResourceRecordSet(managedZone string, addRecordSet, removeRecordSet *dns.ResourceRecordSet) error

	GetManagedZone(managedZone string) (*dns.ManagedZone, error)

	CreateManagedZone(managedZone *dns.ManagedZone) (*dns.ManagedZone, error)

	DeleteManagedZone(managedZone string) error

	ListComputeZones(ListComputeZonesOptions) (*compute.ZoneList, error)

	ListComputeImages(ListComputeImagesOptions) (*compute.ImageList, error)

	ListComputeInstances(ListComputeInstancesOptions, func(*compute.InstanceAggregatedList) error) error

	StopInstance(*compute.Instance, ...InstancesStopCallOption) error

	StartInstance(*compute.Instance) error

	CreateFirewall(firewall string, allowed []*compute.FirewallAllowed, direction string, network string, sourceRanges []string, targetTags []string) (*compute.Firewall, error)

	DeleteFirewall(firewall string) error

	GetFirewall(firewall string) (*compute.Firewall, error)

	CreateForwardingRule(forwardingRule string, ipAddress string, region string, subnet string, target string) (*compute.ForwardingRule, error)

	DeleteForwardingRule(forwardingRule string, region string) error

	GetForwardingRule(forwardingRule string, region string) (*compute.ForwardingRule, error)

	CreateServiceAttachment(serviceAttachment string, region string, forwardingRule string, subnets []string, acceptList []string) (*compute.ServiceAttachment, error)

	GetServiceAttachment(serviceAttachment string, region string) (*compute.ServiceAttachment, error)

	DeleteServiceAttachment(serviceAttachment string, region string) error

	GetNetwork(name string) (*compute.Network, error)

	CreateSubnet(subnet string, network string, region string, ipCidr string, purpose string) (*compute.Subnetwork, error)

	DeleteSubnet(subnet string, region string) error

	GetSubnet(name string, region string, project string) (*compute.Subnetwork, error)

	CreateAddress(addressName string, region string, subnetwork string) (*compute.Address, error)

	DeleteAddress(addressName string, region string) error

	GetAddress(addressName string, region string) (*compute.Address, error)

	ListAddresses(region string, opts ListAddressesOptions) (*compute.AddressList, error)

	GetProjectName() string
}

type InstancesStopCallOption func(stopCall *compute.InstancesStopCall) *compute.InstancesStopCall

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

type ListComputeInstancesOptions struct {
	Filter string
	Fields string
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
	return c.DeleteResourceRecordSets(managedZone, []*dns.ResourceRecordSet{recordSet})
}

func (c *gcpClient) DeleteResourceRecordSets(managedZone string, recordSets []*dns.ResourceRecordSet) error {
	return c.changeResourceRecordSet(
		managedZone,
		&dns.Change{
			Deletions: recordSets,
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

func (c *gcpClient) ListComputeInstances(opts ListComputeInstancesOptions, pagesFn func(*compute.InstanceAggregatedList) error) error {
	req := c.computeClient.Instances.AggregatedList(c.projectName)
	if len(opts.Fields) > 0 {
		req.Fields(googleapi.Field(opts.Fields))
	}
	if len(opts.Filter) > 0 {
		req = req.Filter(opts.Filter)
	}
	err := req.Pages(context.TODO(), pagesFn)
	if err != nil {
		return errors.Wrapf(err, "failed to fetch compute instances")
	}
	return nil
}

func (c *gcpClient) StopInstance(instance *compute.Instance, opts ...InstancesStopCallOption) error {
	zone := instanceZone(instance)
	stopCall := c.computeClient.Instances.Stop(c.projectName, zone, instance.Name)
	for _, o := range opts {
		stopCall = o(stopCall)
	}
	_, err := stopCall.Do()
	if err != nil && !isNotModified(err) {
		return errors.Wrapf(err, "failed to stop instance %s in zone %s", instance.Name, zone)
	}
	return nil
}

func (c *gcpClient) StartInstance(instance *compute.Instance) error {
	zone := instanceZone(instance)
	_, err := c.computeClient.Instances.Start(c.projectName, zone, instance.Name).Do()
	if err != nil && !isNotModified(err) {
		return errors.Wrapf(err, "failed to start instance %s in zone %s", instance.Name, zone)
	}
	return nil
}

func (c *gcpClient) CreateFirewall(
	firewall string,
	allowed []*compute.FirewallAllowed,
	direction string,
	network string,
	sourceRanges []string,
	targetTags []string) (*compute.Firewall, error) {

	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()

	newFirewall := &compute.Firewall{
		Name:        firewall,
		Description: "",
		Direction:   direction,
		Network:     network,
	}

	if len(allowed) > 0 {
		newFirewall.Allowed = allowed
	}

	if len(sourceRanges) > 0 {
		newFirewall.SourceRanges = sourceRanges
	}

	if len(targetTags) > 0 {
		newFirewall.TargetTags = targetTags
	}

	op, err := c.computeClient.Firewalls.Insert(c.projectName, newFirewall).Context(ctx).Do()
	if err != nil {
		return nil, err
	}

	done, err := c.waitforGlobalOperation(op)
	if err != nil || !done {
		return nil, err
	}

	return c.computeClient.Firewalls.Get(c.projectName, firewall).Context(ctx).Do()
}

func (c *gcpClient) DeleteFirewall(firewall string) error {
	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()

	op, err := c.computeClient.Firewalls.Delete(c.projectName, firewall).Context(ctx).Do()
	if err != nil {
		return err
	}

	done, err := c.waitforGlobalOperation(op)
	if err != nil {
		return err
	}
	if !done {
		return errors.New("timed out waiting on deletion of firewall")
	}
	return nil
}

func (c *gcpClient) GetFirewall(firewall string) (*compute.Firewall, error) {
	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()

	return c.computeClient.Firewalls.Get(c.projectName, firewall).Context(ctx).Do()
}

func (c *gcpClient) CreateForwardingRule(
	forwardingRule string,
	ipAddress string,
	region string,
	subnet string,
	target string) (*compute.ForwardingRule, error) {

	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()

	newForwardingRule := &compute.ForwardingRule{
		Name:                 forwardingRule,
		AllowPscGlobalAccess: true,
		Description:          "",
		IPAddress:            ipAddress,
		Region:               region,
		Subnetwork:           subnet,
		Target:               target,
	}

	op, err := c.computeClient.ForwardingRules.Insert(c.projectName, region, newForwardingRule).Context(ctx).Do()
	if err != nil {
		return nil, err
	}

	done, err := c.waitforRegionOperation(op, region)
	if err != nil || !done {
		return nil, err
	}

	return c.computeClient.ForwardingRules.Get(c.projectName, region, forwardingRule).Context(ctx).Do()
}

func (c *gcpClient) DeleteForwardingRule(forwardingRule string, region string) error {
	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()

	op, err := c.computeClient.ForwardingRules.Delete(c.projectName, region, forwardingRule).Context(ctx).Do()
	if err != nil {
		return err
	}

	done, err := c.waitforRegionOperation(op, region)
	if err != nil {
		return err
	}
	if !done {
		return errors.New("timed out waiting on deletion of forwarding rule")
	}
	return nil

}

func (c *gcpClient) GetForwardingRule(forwardingRule string, region string) (*compute.ForwardingRule, error) {
	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()

	return c.computeClient.ForwardingRules.Get(c.projectName, region, forwardingRule).Context(ctx).Do()
}

func (c *gcpClient) CreateServiceAttachment(serviceAttachment string, region string, forwardingRule string, subnets []string, acceptList []string) (*compute.ServiceAttachment, error) {
	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()

	newServiceAttachment := &compute.ServiceAttachment{
		Name:                   serviceAttachment,
		ConnectionPreference:   "ACCEPT_MANUAL",
		Description:            "",
		NatSubnets:             subnets,
		ProducerForwardingRule: forwardingRule,
	}

	if len(acceptList) > 0 {
		consumerAcceptList := []*compute.ServiceAttachmentConsumerProjectLimit{}
		for _, item := range acceptList {
			consumerProjectLimit := &compute.ServiceAttachmentConsumerProjectLimit{
				ProjectIdOrNum:  item,
				ConnectionLimit: 10,
			}
			consumerAcceptList = append(consumerAcceptList, consumerProjectLimit)
		}
		newServiceAttachment.ConsumerAcceptLists = consumerAcceptList
	}

	op, err := c.computeClient.ServiceAttachments.Insert(c.projectName, region, newServiceAttachment).Context(ctx).Do()
	if err != nil {
		return nil, err
	}

	done, err := c.waitforRegionOperation(op, region)
	if err != nil || !done {
		return nil, err
	}

	return c.computeClient.ServiceAttachments.Get(c.projectName, region, serviceAttachment).Context(ctx).Do()
}

func (c *gcpClient) DeleteServiceAttachment(serviceAttachment string, region string) error {
	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()

	op, err := c.computeClient.ServiceAttachments.Delete(c.projectName, region, serviceAttachment).Context(ctx).Do()
	if err != nil {
		return err
	}

	done, err := c.waitforRegionOperation(op, region)
	if err != nil {
		return err
	}
	if !done {
		return errors.New("timed out waiting on deletion of service attachment")
	}
	return nil
}

func (c *gcpClient) GetServiceAttachment(serviceAttachment string, region string) (*compute.ServiceAttachment, error) {
	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()

	return c.computeClient.ServiceAttachments.Get(c.projectName, region, serviceAttachment).Context(ctx).Do()
}

func (c *gcpClient) GetNetwork(network string) (*compute.Network, error) {
	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()

	return c.computeClient.Networks.Get(c.projectName, network).Context(ctx).Do()
}

func (c *gcpClient) CreateAddress(addressName string, region string, subnetwork string) (*compute.Address, error) {
	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()

	newAddress := &compute.Address{
		Name:        addressName,
		AddressType: "INTERNAL",
		Description: "",
		IpVersion:   "IPV4",
		Subnetwork:  subnetwork,
	}

	op, err := c.computeClient.Addresses.Insert(c.projectName, region, newAddress).Context(ctx).Do()
	if err != nil {
		return nil, err
	}

	done, err := c.waitforRegionOperation(op, region)
	if err != nil || !done {
		return nil, err
	}

	return c.computeClient.Addresses.Get(c.projectName, region, addressName).Context(ctx).Do()
}

func (c *gcpClient) DeleteAddress(addressName string, region string) error {
	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()

	op, err := c.computeClient.Addresses.Delete(c.projectName, region, addressName).Context(ctx).Do()
	if err != nil {
		return err
	}

	done, err := c.waitforRegionOperation(op, region)
	if err != nil {
		return err
	}
	if !done {
		return errors.New("timed out waiting on deletion of address")
	}
	return nil
}

func (c *gcpClient) GetAddress(addressName string, region string) (*compute.Address, error) {
	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()

	return c.computeClient.Addresses.Get(c.projectName, region, addressName).Context(ctx).Do()
}

// ListAddressesOptions are the options for listing compute images.
type ListAddressesOptions struct {
	MaxResults int64
	PageToken  string
	Filter     string
}

func (c *gcpClient) ListAddresses(region string, opts ListAddressesOptions) (*compute.AddressList, error) {
	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()

	call := c.computeClient.Addresses.List(c.projectName, region).Context(ctx).Filter(opts.Filter)

	if opts.MaxResults > 0 {
		call.MaxResults(opts.MaxResults)
	}
	if opts.PageToken != "" {
		call.PageToken(opts.PageToken)
	}

	return call.Do()
}

func (c *gcpClient) CreateSubnet(
	subnet string,
	ipCidr string,
	network string,
	region string,
	purpose string) (*compute.Subnetwork, error) {

	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()

	newSubnet := &compute.Subnetwork{
		Name:        subnet,
		Description: "",
		IpCidrRange: ipCidr,
		Network:     network,
		Purpose:     purpose,
		Region:      region,
	}

	op, err := c.computeClient.Subnetworks.Insert(c.projectName, region, newSubnet).Context(ctx).Do()
	if err != nil {
		return nil, err
	}

	done, err := c.waitforRegionOperation(op, region)
	if err != nil || !done {
		return nil, err
	}

	return c.computeClient.Subnetworks.Get(c.projectName, region, subnet).Context(ctx).Do()
}

func (c *gcpClient) DeleteSubnet(subnet string, region string) error {
	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()

	op, err := c.computeClient.Subnetworks.Delete(c.projectName, region, subnet).Context(ctx).Do()
	if err != nil {
		return err
	}

	done, err := c.waitforRegionOperation(op, region)
	if err != nil {
		return err
	}
	if !done {
		return errors.New("timed out waiting on deletion subnet")
	}
	return nil
}

func (c *gcpClient) GetSubnet(subnet string, region string, project string) (*compute.Subnetwork, error) {
	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()

	if project == "" {
		project = c.projectName
	}

	return c.computeClient.Subnetworks.Get(project, region, subnet).Context(ctx).Do()
}

func (c *gcpClient) GetProjectName() string {
	return c.projectName
}

func (c *gcpClient) waitforGlobalOperation(op *compute.Operation) (bool, error) {
	// See if the op was already done
	if op.Status == "DONE" {
		if op.Error != nil && len(op.Error.Errors) > 0 {
			return true, fmt.Errorf("operation completed with error: %s", op.Error.Errors[0].Code)
		}
		return true, nil
	}

	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()

	// Wait for the op to finish (up to 2 minutes)
	op, err := c.computeClient.GlobalOperations.Wait(c.projectName, op.Name).Context(ctx).Do()
	if err != nil {
		return false, err
	}

	done := op.Status == "DONE"
	if op.Error != nil && len(op.Error.Errors) > 0 {
		return done, fmt.Errorf("operation completed with error: %s", op.Error.Errors[0].Code)
	}

	return done, nil
}

func (c *gcpClient) waitforRegionOperation(op *compute.Operation, region string) (bool, error) {
	// See if the op was already done
	if op.Status == "DONE" {
		if op.Error != nil && len(op.Error.Errors) > 0 {
			return true, fmt.Errorf("operation completed with error: %s", op.Error.Errors[0].Code)
		}
		return true, nil
	}

	ctx, cancel := contextWithTimeout(context.TODO())
	defer cancel()

	// Wait for the op to finish (up to 2 minutes)
	op, err := c.computeClient.RegionOperations.Wait(c.projectName, region, op.Name).Context(ctx).Do()
	if err != nil {
		return false, err
	}

	done := op.Status == "DONE"
	if op.Error != nil && len(op.Error.Errors) > 0 {
		return done, fmt.Errorf("operation completed with error: %s", op.Error.Errors[0].Code)
	}

	return done, nil
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
		return os.ReadFile(filename)
	}
}

// instanceZone extracts the zone name from a Zone field in a compute Instance.
// The field is originally returned in the form of a url:
// https://www.googleapis.com/compute/v1/projects/project-id/zones/us-central1-a
func instanceZone(instance *compute.Instance) string {
	parts := strings.Split(instance.Zone, "/")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}
	return ""
}

// isNotModified returns true if the error is a StatusNotModified error, which means
// the requested operation has already taken place.
func isNotModified(err error) bool {
	if err == nil {
		return false
	}
	ae, ok := err.(*googleapi.Error)
	return ok && ae.Code == http.StatusNotModified
}
