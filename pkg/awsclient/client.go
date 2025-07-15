package awsclient

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"

	"github.com/openshift/hive/pkg/constants"
)

var (
	metricAWSAPICalls = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hive_aws_api_calls_total",
			Help: "Number of API calls made to AWS, partitioned by function.",
		},
		[]string{"function"},
	)
)

func init() {
	metrics.Registry.MustRegister(metricAWSAPICalls)
}

//go:generate mockgen -source=./client.go -destination=./mock/client_generated.go -package=mock

// Client is a wrapper object for actual AWS SDK clients to allow for easier testing.
type Client interface {
	// EC2
	DescribeAvailabilityZones(*ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error)
	DescribeSubnets(*ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error)
	DescribeSubnetsPages(*ec2.DescribeSubnetsInput, func(*ec2.DescribeSubnetsOutput, bool) bool) error
	DescribeRouteTables(*ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error)
	DescribeRouteTablesPages(*ec2.DescribeRouteTablesInput, func(*ec2.DescribeRouteTablesOutput, bool) bool) error
	CreateRoute(*ec2.CreateRouteInput) (*ec2.CreateRouteOutput, error)
	DeleteRoute(*ec2.DeleteRouteInput) (*ec2.DeleteRouteOutput, error)
	DescribeInstances(*ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error)
	StopInstances(*ec2.StopInstancesInput) (*ec2.StopInstancesOutput, error)
	TerminateInstances(*ec2.TerminateInstancesInput) (*ec2.TerminateInstancesOutput, error)
	StartInstances(*ec2.StartInstancesInput) (*ec2.StartInstancesOutput, error)
	DescribeSecurityGroups(*ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error)
	AuthorizeSecurityGroupIngress(*ec2.AuthorizeSecurityGroupIngressInput) (*ec2.AuthorizeSecurityGroupIngressOutput, error)
	RevokeSecurityGroupIngress(*ec2.RevokeSecurityGroupIngressInput) (*ec2.RevokeSecurityGroupIngressOutput, error)
	DescribeVpcs(*ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error)
	CreateVpcPeeringConnection(*ec2.CreateVpcPeeringConnectionInput) (*ec2.CreateVpcPeeringConnectionOutput, error)
	DescribeVpcPeeringConnections(*ec2.DescribeVpcPeeringConnectionsInput) (*ec2.DescribeVpcPeeringConnectionsOutput, error)
	AcceptVpcPeeringConnection(*ec2.AcceptVpcPeeringConnectionInput) (*ec2.AcceptVpcPeeringConnectionOutput, error)
	DeleteVpcPeeringConnection(*ec2.DeleteVpcPeeringConnectionInput) (*ec2.DeleteVpcPeeringConnectionOutput, error)
	WaitUntilVpcPeeringConnectionExists(*ec2.DescribeVpcPeeringConnectionsInput) error
	WaitUntilVpcPeeringConnectionDeleted(*ec2.DescribeVpcPeeringConnectionsInput) error
	CreateVpcEndpointServiceConfiguration(*ec2.CreateVpcEndpointServiceConfigurationInput) (*ec2.CreateVpcEndpointServiceConfigurationOutput, error)
	DescribeVpcEndpointServiceConfigurations(*ec2.DescribeVpcEndpointServiceConfigurationsInput) (*ec2.DescribeVpcEndpointServiceConfigurationsOutput, error)
	ModifyVpcEndpointServiceConfiguration(*ec2.ModifyVpcEndpointServiceConfigurationInput) (*ec2.ModifyVpcEndpointServiceConfigurationOutput, error)
	DeleteVpcEndpointServiceConfigurations(*ec2.DeleteVpcEndpointServiceConfigurationsInput) (*ec2.DeleteVpcEndpointServiceConfigurationsOutput, error)
	DescribeVpcEndpointServicePermissions(*ec2.DescribeVpcEndpointServicePermissionsInput) (*ec2.DescribeVpcEndpointServicePermissionsOutput, error)
	ModifyVpcEndpointServicePermissions(*ec2.ModifyVpcEndpointServicePermissionsInput) (*ec2.ModifyVpcEndpointServicePermissionsOutput, error)
	DescribeVpcEndpointServices(*ec2.DescribeVpcEndpointServicesInput) (*ec2.DescribeVpcEndpointServicesOutput, error)
	DescribeVpcEndpoints(*ec2.DescribeVpcEndpointsInput) (*ec2.DescribeVpcEndpointsOutput, error)
	DescribeVpcEndpointsPages(*ec2.DescribeVpcEndpointsInput, func(*ec2.DescribeVpcEndpointsOutput, bool) bool) error

	DescribeNetworkInterfaces(input *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error)
	CreateVpcEndpoint(*ec2.CreateVpcEndpointInput) (*ec2.CreateVpcEndpointOutput, error)
	DeleteVpcEndpoints(*ec2.DeleteVpcEndpointsInput) (*ec2.DeleteVpcEndpointsOutput, error)

	// ELBV2
	DescribeLoadBalancers(*elasticloadbalancingv2.DescribeLoadBalancersInput) (*elasticloadbalancingv2.DescribeLoadBalancersOutput, error)

	// S3 Manager
	Upload(ctx context.Context, input *s3.PutObjectInput) (*s3.PutObjectOutput, error)

	// Custom
	GetS3API() *s3.Client

	// Route53
	CreateHostedZone(input *route53.CreateHostedZoneInput) (*route53.CreateHostedZoneOutput, error)
	GetHostedZone(*route53.GetHostedZoneInput) (*route53.GetHostedZoneOutput, error)
	ListTagsForResource(*route53.ListTagsForResourceInput) (*route53.ListTagsForResourceOutput, error)
	ChangeTagsForResource(input *route53.ChangeTagsForResourceInput) (*route53.ChangeTagsForResourceOutput, error)
	DeleteHostedZone(input *route53.DeleteHostedZoneInput) (*route53.DeleteHostedZoneOutput, error)
	ListResourceRecordSets(input *route53.ListResourceRecordSetsInput) (*route53.ListResourceRecordSetsOutput, error)
	ListHostedZonesByName(input *route53.ListHostedZonesByNameInput) (*route53.ListHostedZonesByNameOutput, error)
	ListHostedZonesByVPC(input *route53.ListHostedZonesByVPCInput) (*route53.ListHostedZonesByVPCOutput, error)
	ChangeResourceRecordSets(*route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error)
	CreateVPCAssociationAuthorization(*route53.CreateVPCAssociationAuthorizationInput) (*route53.CreateVPCAssociationAuthorizationOutput, error)
	DeleteVPCAssociationAuthorization(*route53.DeleteVPCAssociationAuthorizationInput) (*route53.DeleteVPCAssociationAuthorizationOutput, error)
	AssociateVPCWithHostedZone(*route53.AssociateVPCWithHostedZoneInput) (*route53.AssociateVPCWithHostedZoneOutput, error)
	DisassociateVPCFromHostedZone(input *route53.DisassociateVPCFromHostedZoneInput) (*route53.DisassociateVPCFromHostedZoneOutput, error)
	// ResourceTagging
	GetResourcesPages(input *resourcegroupstaggingapi.GetResourcesInput, fn func(*resourcegroupstaggingapi.GetResourcesOutput, bool) bool) error

	// STS
	GetCallerIdentity(input *sts.GetCallerIdentityInput) (*sts.GetCallerIdentityOutput, error)
}

type awsClient struct {
	ec2Client     *ec2.Client
	elbv2Client   *elasticloadbalancingv2.Client
	route53Client *route53.Client
	s3Client      *s3.Client
	stsClient     *sts.Client
	tagClient     *resourcegroupstaggingapi.Client
}

func (c *awsClient) DescribeAvailabilityZones(input *ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeAvailabilityZones").Inc()
	return c.ec2Client.DescribeAvailabilityZones(context.TODO(), input)
}

func (c *awsClient) DescribeSubnets(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeSubnets").Inc()
	return c.ec2Client.DescribeSubnets(context.TODO(), input)
}

func (c *awsClient) DescribeSubnetsPages(input *ec2.DescribeSubnetsInput, fn func(*ec2.DescribeSubnetsOutput, bool) bool) error {
	metricAWSAPICalls.WithLabelValues("DescribeSubnetsPages").Inc()
	
	paginator := ec2.NewDescribeSubnetsPaginator(c.ec2Client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(context.TODO())
		if err != nil {
			return err
		}
		if !fn(page, !paginator.HasMorePages()) {
			break
		}
	}
	return nil
}

func (c *awsClient) DescribeRouteTables(input *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeRouteTables").Inc()
	return c.ec2Client.DescribeRouteTables(context.TODO(), input)
}

func (c *awsClient) DescribeRouteTablesPages(input *ec2.DescribeRouteTablesInput, fn func(*ec2.DescribeRouteTablesOutput, bool) bool) error {
	metricAWSAPICalls.WithLabelValues("DescribeRouteTablesPages").Inc()
	
	paginator := ec2.NewDescribeRouteTablesPaginator(c.ec2Client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(context.TODO())
		if err != nil {
			return err
		}
		if !fn(page, !paginator.HasMorePages()) {
			break
		}
	}
	return nil
}

func (c *awsClient) CreateRoute(input *ec2.CreateRouteInput) (*ec2.CreateRouteOutput, error) {
	metricAWSAPICalls.WithLabelValues("CreateRoute").Inc()
	return c.ec2Client.CreateRoute(context.TODO(), input)
}

func (c *awsClient) DeleteRoute(input *ec2.DeleteRouteInput) (*ec2.DeleteRouteOutput, error) {
	metricAWSAPICalls.WithLabelValues("DeleteRoute").Inc()
	return c.ec2Client.DeleteRoute(context.TODO(), input)
}

func (c *awsClient) DescribeInstances(input *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeInstances").Inc()
	return c.ec2Client.DescribeInstances(context.TODO(), input)
}

func (c *awsClient) StopInstances(input *ec2.StopInstancesInput) (*ec2.StopInstancesOutput, error) {
	metricAWSAPICalls.WithLabelValues("StopInstances").Inc()
	return c.ec2Client.StopInstances(context.TODO(), input)
}

func (c *awsClient) TerminateInstances(input *ec2.TerminateInstancesInput) (*ec2.TerminateInstancesOutput, error) {
	metricAWSAPICalls.WithLabelValues("TerminateInstances").Inc()
	return c.ec2Client.TerminateInstances(context.TODO(), input)
}

func (c *awsClient) StartInstances(input *ec2.StartInstancesInput) (*ec2.StartInstancesOutput, error) {
	metricAWSAPICalls.WithLabelValues("StartInstances").Inc()
	return c.ec2Client.StartInstances(context.TODO(), input)
}

func (c *awsClient) DescribeSecurityGroups(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeSecurityGroups").Inc()
	return c.ec2Client.DescribeSecurityGroups(context.TODO(), input)
}

func (c *awsClient) AuthorizeSecurityGroupIngress(input *ec2.AuthorizeSecurityGroupIngressInput) (*ec2.AuthorizeSecurityGroupIngressOutput, error) {
	metricAWSAPICalls.WithLabelValues("AuthorizeSecurityGroupIngress").Inc()
	return c.ec2Client.AuthorizeSecurityGroupIngress(context.TODO(), input)
}

func (c *awsClient) RevokeSecurityGroupIngress(input *ec2.RevokeSecurityGroupIngressInput) (*ec2.RevokeSecurityGroupIngressOutput, error) {
	metricAWSAPICalls.WithLabelValues("RevokeSecurityGroupIngress").Inc()
	return c.ec2Client.RevokeSecurityGroupIngress(context.TODO(), input)
}

func (c *awsClient) CreateVpcPeeringConnection(input *ec2.CreateVpcPeeringConnectionInput) (*ec2.CreateVpcPeeringConnectionOutput, error) {
	metricAWSAPICalls.WithLabelValues("CreateVpcPeeringConnection").Inc()
	return c.ec2Client.CreateVpcPeeringConnection(context.TODO(), input)
}

func (c *awsClient) DescribeVpcPeeringConnections(input *ec2.DescribeVpcPeeringConnectionsInput) (*ec2.DescribeVpcPeeringConnectionsOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeVpcPeeringConnections").Inc()
	return c.ec2Client.DescribeVpcPeeringConnections(context.TODO(), input)
}

func (c *awsClient) AcceptVpcPeeringConnection(input *ec2.AcceptVpcPeeringConnectionInput) (*ec2.AcceptVpcPeeringConnectionOutput, error) {
	metricAWSAPICalls.WithLabelValues("AcceptVpcPeeringConnection").Inc()
	return c.ec2Client.AcceptVpcPeeringConnection(context.TODO(), input)
}

func (c *awsClient) DeleteVpcPeeringConnection(input *ec2.DeleteVpcPeeringConnectionInput) (*ec2.DeleteVpcPeeringConnectionOutput, error) {
	metricAWSAPICalls.WithLabelValues("DeleteVpcPeeringConnection").Inc()
	return c.ec2Client.DeleteVpcPeeringConnection(context.TODO(), input)
}

func (c *awsClient) WaitUntilVpcPeeringConnectionExists(input *ec2.DescribeVpcPeeringConnectionsInput) error {
	metricAWSAPICalls.WithLabelValues("WaitUntilVpcPeeringConnectionExists").Inc()
	waiter := ec2.NewVpcPeeringConnectionExistsWaiter(c.ec2Client)
	return waiter.Wait(context.TODO(), input, awsWaiterMaxDuration)
}

func (c *awsClient) WaitUntilVpcPeeringConnectionDeleted(input *ec2.DescribeVpcPeeringConnectionsInput) error {
	metricAWSAPICalls.WithLabelValues("WaitUntilVpcPeeringConnectionDeleted").Inc()
	waiter := ec2.NewVpcPeeringConnectionDeletedWaiter(c.ec2Client)
	return waiter.Wait(context.TODO(), input, awsWaiterMaxDuration)
}

func (c *awsClient) CreateVpcEndpointServiceConfiguration(input *ec2.CreateVpcEndpointServiceConfigurationInput) (*ec2.CreateVpcEndpointServiceConfigurationOutput, error) {
	metricAWSAPICalls.WithLabelValues("CreateVpcEndpointServiceConfiguration").Inc()
	return c.ec2Client.CreateVpcEndpointServiceConfiguration(context.TODO(), input)
}

func (c *awsClient) DescribeVpcEndpointServiceConfigurations(input *ec2.DescribeVpcEndpointServiceConfigurationsInput) (*ec2.DescribeVpcEndpointServiceConfigurationsOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeVpcEndpointServiceConfigurations").Inc()
	return c.ec2Client.DescribeVpcEndpointServiceConfigurations(context.TODO(), input)
}

func (c *awsClient) ModifyVpcEndpointServiceConfiguration(input *ec2.ModifyVpcEndpointServiceConfigurationInput) (*ec2.ModifyVpcEndpointServiceConfigurationOutput, error) {
	metricAWSAPICalls.WithLabelValues("ModifyVpcEndpointServiceConfiguration").Inc()
	return c.ec2Client.ModifyVpcEndpointServiceConfiguration(context.TODO(), input)
}

func (c *awsClient) DeleteVpcEndpointServiceConfigurations(input *ec2.DeleteVpcEndpointServiceConfigurationsInput) (*ec2.DeleteVpcEndpointServiceConfigurationsOutput, error) {
	metricAWSAPICalls.WithLabelValues("DeleteVpcEndpointServiceConfigurations").Inc()
	return c.ec2Client.DeleteVpcEndpointServiceConfigurations(context.TODO(), input)
}

func (c *awsClient) DescribeVpcEndpointServicePermissions(input *ec2.DescribeVpcEndpointServicePermissionsInput) (*ec2.DescribeVpcEndpointServicePermissionsOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeVpcEndpointServicePermissions").Inc()
	return c.ec2Client.DescribeVpcEndpointServicePermissions(context.TODO(), input)
}

func (c *awsClient) ModifyVpcEndpointServicePermissions(input *ec2.ModifyVpcEndpointServicePermissionsInput) (*ec2.ModifyVpcEndpointServicePermissionsOutput, error) {
	metricAWSAPICalls.WithLabelValues("ModifyVpcEndpointServicePermissions").Inc()
	return c.ec2Client.ModifyVpcEndpointServicePermissions(context.TODO(), input)
}

func (c *awsClient) DescribeVpcEndpointServices(input *ec2.DescribeVpcEndpointServicesInput) (*ec2.DescribeVpcEndpointServicesOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeVpcEndpointServices").Inc()
	return c.ec2Client.DescribeVpcEndpointServices(context.TODO(), input)
}

func (c *awsClient) DescribeVpcEndpoints(input *ec2.DescribeVpcEndpointsInput) (*ec2.DescribeVpcEndpointsOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeVpcEndpoints").Inc()
	return c.ec2Client.DescribeVpcEndpoints(context.TODO(), input)
}

func (c *awsClient) DescribeVpcEndpointsPages(input *ec2.DescribeVpcEndpointsInput, fn func(*ec2.DescribeVpcEndpointsOutput, bool) bool) error {
	metricAWSAPICalls.WithLabelValues("DescribeVpcEndpointsPages").Inc()
	
	paginator := ec2.NewDescribeVpcEndpointsPaginator(c.ec2Client, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(context.TODO())
		if err != nil {
			return err
		}
		if !fn(page, !paginator.HasMorePages()) {
			break
		}
	}
	return nil
}

func (c *awsClient) DescribeNetworkInterfaces(input *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeNetworkInterfaces").Inc()
	return c.ec2Client.DescribeNetworkInterfaces(context.TODO(), input)
}

func (c *awsClient) CreateVpcEndpoint(input *ec2.CreateVpcEndpointInput) (*ec2.CreateVpcEndpointOutput, error) {
	metricAWSAPICalls.WithLabelValues("CreateVpcEndpoint").Inc()
	return c.ec2Client.CreateVpcEndpoint(context.TODO(), input)
}

func (c *awsClient) DeleteVpcEndpoints(input *ec2.DeleteVpcEndpointsInput) (*ec2.DeleteVpcEndpointsOutput, error) {
	metricAWSAPICalls.WithLabelValues("DeleteVpcEndpoints").Inc()
	return c.ec2Client.DeleteVpcEndpoints(context.TODO(), input)
}

func (c *awsClient) DescribeVpcs(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeVpcs").Inc()
	return c.ec2Client.DescribeVpcs(context.TODO(), input)
}

func (c *awsClient) DescribeLoadBalancers(input *elasticloadbalancingv2.DescribeLoadBalancersInput) (*elasticloadbalancingv2.DescribeLoadBalancersOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeLoadBalancers").Inc()
	return c.elbv2Client.DescribeLoadBalancers(context.TODO(), input)
}

func (c *awsClient) GetS3API() *s3.Client {
	return c.s3Client
}

func (c *awsClient) Upload(ctx context.Context, input *s3.PutObjectInput) (*s3.PutObjectOutput, error) {
	return c.s3Client.PutObject(ctx, input)
}

func (c *awsClient) ListHostedZonesByName(input *route53.ListHostedZonesByNameInput) (*route53.ListHostedZonesByNameOutput, error) {
	metricAWSAPICalls.WithLabelValues("ListHostedZonesByName").Inc()
	return c.route53Client.ListHostedZonesByName(context.TODO(), input)
}

func (c *awsClient) ListHostedZonesByVPC(input *route53.ListHostedZonesByVPCInput) (*route53.ListHostedZonesByVPCOutput, error) {
	metricAWSAPICalls.WithLabelValues("ListHostedZonesByVPC").Inc()
	return c.route53Client.ListHostedZonesByVPC(context.TODO(), input)
}

func (c *awsClient) CreateHostedZone(input *route53.CreateHostedZoneInput) (*route53.CreateHostedZoneOutput, error) {
	metricAWSAPICalls.WithLabelValues("CreateHostedZone").Inc()
	return c.route53Client.CreateHostedZone(context.TODO(), input)
}

func (c *awsClient) GetHostedZone(input *route53.GetHostedZoneInput) (*route53.GetHostedZoneOutput, error) {
	metricAWSAPICalls.WithLabelValues("GetHostedZone").Inc()
	return c.route53Client.GetHostedZone(context.TODO(), input)
}

func (c *awsClient) ListTagsForResource(input *route53.ListTagsForResourceInput) (*route53.ListTagsForResourceOutput, error) {
	metricAWSAPICalls.WithLabelValues("ListTagsForResource").Inc()
	return c.route53Client.ListTagsForResource(context.TODO(), input)
}

func (c *awsClient) ChangeTagsForResource(input *route53.ChangeTagsForResourceInput) (*route53.ChangeTagsForResourceOutput, error) {
	metricAWSAPICalls.WithLabelValues("ChangeTagsForResource").Inc()
	return c.route53Client.ChangeTagsForResource(context.TODO(), input)
}

func (c *awsClient) DeleteHostedZone(input *route53.DeleteHostedZoneInput) (*route53.DeleteHostedZoneOutput, error) {
	metricAWSAPICalls.WithLabelValues("DeleteHostedZone").Inc()
	return c.route53Client.DeleteHostedZone(context.TODO(), input)
}

func (c *awsClient) GetResourcesPages(input *resourcegroupstaggingapi.GetResourcesInput, fn func(*resourcegroupstaggingapi.GetResourcesOutput, bool) bool) error {
	metricAWSAPICalls.WithLabelValues("GetResourcesPages").Inc()
	
	paginator := resourcegroupstaggingapi.NewGetResourcesPaginator(c.tagClient, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(context.TODO())
		if err != nil {
			return err
		}
		if !fn(page, !paginator.HasMorePages()) {
			break
		}
	}
	return nil
}

func (c *awsClient) ListResourceRecordSets(input *route53.ListResourceRecordSetsInput) (*route53.ListResourceRecordSetsOutput, error) {
	metricAWSAPICalls.WithLabelValues("ListResourceRecordSets").Inc()
	return c.route53Client.ListResourceRecordSets(context.TODO(), input)
}

func (c *awsClient) ChangeResourceRecordSets(input *route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error) {
	metricAWSAPICalls.WithLabelValues("ChangeResourceRecordSets").Inc()
	return c.route53Client.ChangeResourceRecordSets(context.TODO(), input)
}

func (c *awsClient) AssociateVPCWithHostedZone(input *route53.AssociateVPCWithHostedZoneInput) (*route53.AssociateVPCWithHostedZoneOutput, error) {
	metricAWSAPICalls.WithLabelValues("AssociateVPCWithHostedZone").Inc()
	return c.route53Client.AssociateVPCWithHostedZone(context.TODO(), input)
}

func (c *awsClient) CreateVPCAssociationAuthorization(input *route53.CreateVPCAssociationAuthorizationInput) (*route53.CreateVPCAssociationAuthorizationOutput, error) {
	metricAWSAPICalls.WithLabelValues("CreateVPCAssociationAuthorization").Inc()
	return c.route53Client.CreateVPCAssociationAuthorization(context.TODO(), input)
}

func (c *awsClient) DeleteVPCAssociationAuthorization(input *route53.DeleteVPCAssociationAuthorizationInput) (*route53.DeleteVPCAssociationAuthorizationOutput, error) {
	metricAWSAPICalls.WithLabelValues("DeleteVPCAssociationAuthorization").Inc()
	return c.route53Client.DeleteVPCAssociationAuthorization(context.TODO(), input)
}

func (c *awsClient) DisassociateVPCFromHostedZone(input *route53.DisassociateVPCFromHostedZoneInput) (*route53.DisassociateVPCFromHostedZoneOutput, error) {
	metricAWSAPICalls.WithLabelValues("DisassociateVPCFromHostedZone").Inc()
	return c.route53Client.DisassociateVPCFromHostedZone(context.TODO(), input)
}

func (c *awsClient) GetCallerIdentity(input *sts.GetCallerIdentityInput) (*sts.GetCallerIdentityOutput, error) {
	metricAWSAPICalls.WithLabelValues("GetCallerIdentity").Inc()
	return c.stsClient.GetCallerIdentity(context.TODO(), input)
}

// Options provides the means to control how a client is created and what
// configuration values will be loaded.
type Options struct {
	// Region helps create the clients with correct endpoints.
	Region string

	// CredentialsSource defines how the credentials will be loaded.
	// It supports various methods of sourcing credentials. But if none
	// of the supported sources are configured such that they can be used,
	// credentials are loaded from the environment.
	// If multiple sources are configured, the first source is used.
	CredentialsSource CredentialsSource
}

// CredentialsSource defines how the credentials will be loaded.
// It supports various methods of sourcing credentials. But if none
// of the supported sources are configured such that they can be used,
// credentials are loaded from the environment.
// If multiple sources are configured, the first source is used.
type CredentialsSource struct {
	// Secret credentials source loads the credentials from a secret.
	// It supports static credentials in the secret provided by aws_access_key_id,
	// and aws_access_secret key. It also supports loading credentials from AWS
	// cli config provided in aws_config key.
	// This source is used only when the Secret name is not empty.
	Secret *SecretCredentialsSource

	// AssumeRole credentials source uses AWS session configured using credentials
	// in the SecretRef, and then uses that to assume the role provided in Role.
	// AWS client is created using the assumed credentials.
	// If the secret in SecretRef is empty, environment is used to create AWS session.
	// This source is used only when the RoleARN is not empty in Role.
	AssumeRole *AssumeRoleCredentialsSource

	// when none set, use environment to load the credentials
}

// Secret credentials source loads the credentials from a secret.
// It supports static credentials in the secret provided by aws_access_key_id,
// and aws_access_secret key. It also supports loading credentials from AWS
// cli config provided in aws_config key.
// This source is used only when the Secret name is not empty.
type SecretCredentialsSource struct {
	Namespace string
	Ref       *corev1.LocalObjectReference
}

// AssumeRole credentials source uses AWS session configured using credentials
// in the SecretRef, and then uses that to assume the role provided in Role.
// AWS client is created using the assumed credentials.
// If the secret in SecretRef is empty, environment is used to create AWS session.
// This source is used only when the RoleARN is not empty in Role.
type AssumeRoleCredentialsSource struct {
	SecretRef corev1.SecretReference
	Role      *hivev1aws.AssumeRole
}

// awsWaiterMaxDuration is the maximum duration to wait for AWS resources
const awsWaiterMaxDuration = 600 * time.Second

// New creates an AWS client using the provided options. kubeClient is used whenever
// a k8s resource like secret needs to be fetched. Look at doc for Options for various
// configurations.
//
// Some examples are,
// 1. Configure an AWS client using credentials in Secret for ClusterDeployment.
//
//	options := Options{
//	    Region: cd.Spec.Platform.AWS.Region,
//	    CredentialsSource: CredentialsSource{
//	        Secret: &SecretCredentialsSource{
//	            Namespace: cd.Namespace,
//	            Ref:       cd.Spec.Platform.AWS.CredentialsSecretRef,
//	        },
//	    },
//	}
//	client, err := New(kubeClient, options)
//
// 2. Configure an AWS client using Assume role chain for ClusterDeployment.
//
//	options := Options{
//	    Region: cd.Spec.Platform.AWS.Region,
//	    CredentialsSource: CredentialsSource{
//	        AssumeRole: &AssumeRoleCredentialsSource{
//	            SecretRef: corev1.SecretReference{
//	                Name:      AWSServiceProviderSecretName,
//	                Namespace: AWSServiceProviderSecretNS,
//	            },
//	        Role: cd.Spec.Platform.AWS.CredentialsAssumeRole,
//	        },
//	    },
//	}
//	client, err := New(kubeClient, options)
func New(kubeClient client.Client, options Options) (Client, error) {
	source := options.CredentialsSource
	switch {
	case source.Secret != nil && source.Secret.Ref != nil && source.Secret.Ref.Name != "":
		return NewClient(kubeClient, source.Secret.Ref.Name, source.Secret.Namespace, options.Region)
	case source.AssumeRole != nil && source.AssumeRole.Role != nil && source.AssumeRole.Role.RoleARN != "":
		return newClientAssumeRole(kubeClient,
			source.AssumeRole.SecretRef.Name, source.AssumeRole.SecretRef.Namespace,
			source.AssumeRole.Role,
			options.Region,
		)
	}

	return NewClientFromSecret(nil, options.Region)
}

func newClientAssumeRole(kubeClient client.Client,
	serviceProviderSecretName, serviceProviderSecretNamespace string,
	role *hivev1aws.AssumeRole,
	region string,
) (Client, error) {
	var secret *corev1.Secret
	if serviceProviderSecretName != "" {
		secret = &corev1.Secret{}
		err := kubeClient.Get(context.TODO(),
			types.NamespacedName{
				Name:      serviceProviderSecretName,
				Namespace: serviceProviderSecretNamespace,
			},
			secret)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get service provider secret")
		}
	}

	cfg, err := NewConfigFromSecret(secret, region)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create AWS config")
	}

	// Create STS client to assume role
	stsClient := sts.NewFromConfig(cfg)
	
	// Create assume role credentials provider
	cfg.Credentials = stscreds.NewAssumeRoleProvider(stsClient, role.RoleARN, func(o *stscreds.AssumeRoleOptions) {
		if role.ExternalID != "" {
			o.ExternalID = aws.String(role.ExternalID)
		}
	})

	return newClientFromConfig(cfg)
}

// NewClient creates our client wrapper object for the actual AWS clients we use.
// For authentication the underlying clients will use either the cluster AWS credentials
// secret if defined (i.e. in the root cluster),
// otherwise the IAM profile of the master where the actuator will run. (target clusters)
//
// Pass a nil client, and empty secret name and namespace to load credentials from the standard
// AWS environment variables.
func NewClient(kubeClient client.Client, secretName, namespace, region string) (Client, error) {

	// Special case to not use a secret to gather credentials.
	if secretName == "" {
		return NewClientFromSecret(nil, region)
	}

	secret := &corev1.Secret{}
	err := kubeClient.Get(context.TODO(),
		types.NamespacedName{
			Name:      secretName,
			Namespace: namespace,
		},
		secret)
	if err != nil {
		return nil, err
	}

	return NewClientFromSecret(secret, region)
}

// NewClientFromSecret creates our client wrapper object for the actual AWS clients we use.
// For authentication the underlying clients will use either the cluster AWS credentials
// secret if defined (i.e. in the root cluster),
// otherwise the IAM profile of the master where the actuator will run. (target clusters)
//
// Pass a nil secret to load credentials from the standard AWS environment variables.
func NewClientFromSecret(secret *corev1.Secret, region string) (Client, error) {
	cfg, err := NewConfigFromSecret(secret, region)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create AWS config")
	}
	return newClientFromConfig(cfg)
}

func newClientFromConfig(cfg aws.Config) (Client, error) {
	return &awsClient{
		ec2Client:     ec2.NewFromConfig(cfg),
		elbv2Client:   elasticloadbalancingv2.NewFromConfig(cfg),
		s3Client:      s3.NewFromConfig(cfg),
		route53Client: route53.NewFromConfig(cfg),
		stsClient:     sts.NewFromConfig(cfg),
		tagClient:     resourcegroupstaggingapi.NewFromConfig(cfg),
	}, nil
}

// NewConfigFromSecret creates a new AWS config using the configuration in the secret. If the secret
// was nil, it initializes a new config using configuration of the environment.
func NewConfigFromSecret(secret *corev1.Secret, region string) (aws.Config, error) {
	ctx := context.TODO()

	// Start with default config
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return aws.Config{}, err
	}

	// Apply custom endpoint resolver for China regions
	cfg.EndpointResolverWithOptions = aws.EndpointResolverWithOptionsFunc(awsChinaEndpointResolver)

	// Special case to not use a secret to gather credentials.
	if secret != nil {
		configData, err := awsCLIConfigFromSecret(secret)
		if err != nil {
			return aws.Config{}, err
		}
		
		// Create temp file for config
		f, err := os.CreateTemp("", "hive-aws-config")
		if err != nil {
			return aws.Config{}, err
		}
		defer f.Close()
		defer os.Remove(f.Name())
		
		if _, err := f.Write(configData); err != nil {
			return aws.Config{}, err
		}
		
		// Load config from temp file
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
			config.WithSharedConfigFiles([]string{f.Name()}),
			config.WithSharedConfigProfile("default"),
		)
		if err != nil {
			return aws.Config{}, err
		}
		
		// Apply custom endpoint resolver for China regions
		cfg.EndpointResolverWithOptions = aws.EndpointResolverWithOptionsFunc(awsChinaEndpointResolver)
	}

	return cfg, nil
}

var credentialProcessRE = regexp.MustCompile(`(?i)\bcredential_process\b`)

func ContainsCredentialProcess(config []byte) bool {
	return len(credentialProcessRE.Find(config)) != 0
}

// awsCLIConfigFromSecret returns an AWS CLI config using the data available in the secret.
func awsCLIConfigFromSecret(secret *corev1.Secret) ([]byte, error) {
	if config, ok := secret.Data[constants.AWSConfigSecretKey]; ok {
		if ContainsCredentialProcess(config) {
			return nil, errors.New("credential_process is insecure and thus forbidden")
		}
		return config, nil
	}

	buf := &bytes.Buffer{}
	fmt.Fprint(buf, "[default]\n")
	for k, v := range secret.Data {
		if strings.ToLower(k) == "credential_process" {
			return nil, errors.New("credential_process is insecure and thus forbidden")
		}
		fmt.Fprintf(buf, "%s = %s\n", k, v)
	}
	return buf.Bytes(), nil
}

func awsChinaEndpointResolver(service, region string, options ...interface{}) (aws.Endpoint, error) {
	if service != "Route 53" || region != constants.AWSChinaRoute53Region {
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	}

	return aws.Endpoint{
		URL:         "https://route53.amazonaws.com.cn",
		PartitionID: "aws-cn",
	}, nil
}
