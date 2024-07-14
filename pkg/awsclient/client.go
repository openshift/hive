package awsclient

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/service/elb/elbiface"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/route53/route53iface"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/aws/aws-sdk-go/service/sts/stsiface"

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
	DescribeLoadBalancers(*elbv2.DescribeLoadBalancersInput) (*elbv2.DescribeLoadBalancersOutput, error)

	// S3 Manager
	Upload(*s3manager.UploadInput) (*s3manager.UploadOutput, error)

	// Custom
	GetS3API() s3iface.S3API

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
	ec2Client     ec2iface.EC2API
	elbClient     elbiface.ELBAPI
	elbv2Client   elbv2iface.ELBV2API
	route53Client route53iface.Route53API
	s3Client      s3iface.S3API
	s3Uploader    *s3manager.Uploader
	stsClient     stsiface.STSAPI
	tagClient     *resourcegroupstaggingapi.ResourceGroupsTaggingAPI
}

func (c *awsClient) DescribeAvailabilityZones(input *ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeAvailabilityZones").Inc()
	return c.ec2Client.DescribeAvailabilityZones(input)
}

func (c *awsClient) DescribeSubnets(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeSubnets").Inc()
	return c.ec2Client.DescribeSubnets(input)
}

func (c *awsClient) DescribeSubnetsPages(input *ec2.DescribeSubnetsInput, fn func(*ec2.DescribeSubnetsOutput, bool) bool) error {
	metricAWSAPICalls.WithLabelValues("DescribeSubnetsPages").Inc()
	return c.ec2Client.DescribeSubnetsPages(input, fn)
}

func (c *awsClient) DescribeRouteTables(input *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeRouteTables").Inc()
	return c.ec2Client.DescribeRouteTables(input)
}

func (c *awsClient) DescribeRouteTablesPages(input *ec2.DescribeRouteTablesInput, fn func(*ec2.DescribeRouteTablesOutput, bool) bool) error {
	metricAWSAPICalls.WithLabelValues("DescribeRouteTablesPages").Inc()
	return c.ec2Client.DescribeRouteTablesPages(input, fn)
}

func (c *awsClient) CreateRoute(input *ec2.CreateRouteInput) (*ec2.CreateRouteOutput, error) {
	metricAWSAPICalls.WithLabelValues("CreateRoute").Inc()
	return c.ec2Client.CreateRoute(input)
}

func (c *awsClient) DeleteRoute(input *ec2.DeleteRouteInput) (*ec2.DeleteRouteOutput, error) {
	metricAWSAPICalls.WithLabelValues("DeleteRoute").Inc()
	return c.ec2Client.DeleteRoute(input)
}

func (c *awsClient) DescribeInstances(input *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeInstances").Inc()
	return c.ec2Client.DescribeInstances(input)
}

func (c *awsClient) StopInstances(input *ec2.StopInstancesInput) (*ec2.StopInstancesOutput, error) {
	metricAWSAPICalls.WithLabelValues("StopInstances").Inc()
	return c.ec2Client.StopInstances(input)
}

func (c *awsClient) TerminateInstances(input *ec2.TerminateInstancesInput) (*ec2.TerminateInstancesOutput, error) {
	metricAWSAPICalls.WithLabelValues("TerminateInstances").Inc()
	return c.ec2Client.TerminateInstances(input)
}

func (c *awsClient) StartInstances(input *ec2.StartInstancesInput) (*ec2.StartInstancesOutput, error) {
	metricAWSAPICalls.WithLabelValues("StartInstances").Inc()
	return c.ec2Client.StartInstances(input)
}

func (c *awsClient) DescribeSecurityGroups(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeSecurityGroups").Inc()
	return c.ec2Client.DescribeSecurityGroups(input)
}

func (c *awsClient) AuthorizeSecurityGroupIngress(input *ec2.AuthorizeSecurityGroupIngressInput) (*ec2.AuthorizeSecurityGroupIngressOutput, error) {
	metricAWSAPICalls.WithLabelValues("AuthorizeSecurityGroupIngress").Inc()
	return c.ec2Client.AuthorizeSecurityGroupIngress(input)
}

func (c *awsClient) RevokeSecurityGroupIngress(input *ec2.RevokeSecurityGroupIngressInput) (*ec2.RevokeSecurityGroupIngressOutput, error) {
	metricAWSAPICalls.WithLabelValues("RevokeSecurityGroupIngress").Inc()
	return c.ec2Client.RevokeSecurityGroupIngress(input)
}

func (c *awsClient) CreateVpcPeeringConnection(input *ec2.CreateVpcPeeringConnectionInput) (*ec2.CreateVpcPeeringConnectionOutput, error) {
	metricAWSAPICalls.WithLabelValues("CreateVpcPeeringConnection").Inc()
	return c.ec2Client.CreateVpcPeeringConnection(input)
}

func (c *awsClient) DescribeVpcPeeringConnections(input *ec2.DescribeVpcPeeringConnectionsInput) (*ec2.DescribeVpcPeeringConnectionsOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeVpcPeeringConnections").Inc()
	return c.ec2Client.DescribeVpcPeeringConnections(input)
}

func (c *awsClient) AcceptVpcPeeringConnection(input *ec2.AcceptVpcPeeringConnectionInput) (*ec2.AcceptVpcPeeringConnectionOutput, error) {
	metricAWSAPICalls.WithLabelValues("AcceptVpcPeeringConnection").Inc()
	return c.ec2Client.AcceptVpcPeeringConnection(input)
}

func (c *awsClient) DeleteVpcPeeringConnection(input *ec2.DeleteVpcPeeringConnectionInput) (*ec2.DeleteVpcPeeringConnectionOutput, error) {
	metricAWSAPICalls.WithLabelValues("DeleteVpcPeeringConnection").Inc()
	return c.ec2Client.DeleteVpcPeeringConnection(input)
}

func (c *awsClient) WaitUntilVpcPeeringConnectionExists(input *ec2.DescribeVpcPeeringConnectionsInput) error {
	metricAWSAPICalls.WithLabelValues("WaitUntilVpcPeeringConnectionExists").Inc()
	return c.ec2Client.WaitUntilVpcPeeringConnectionExists(input)
}

func (c *awsClient) WaitUntilVpcPeeringConnectionDeleted(input *ec2.DescribeVpcPeeringConnectionsInput) error {
	metricAWSAPICalls.WithLabelValues("WaitUntilVpcPeeringConnectionDeleted").Inc()
	return c.ec2Client.WaitUntilVpcPeeringConnectionDeleted(input)
}

func (c *awsClient) CreateVpcEndpointServiceConfiguration(input *ec2.CreateVpcEndpointServiceConfigurationInput) (*ec2.CreateVpcEndpointServiceConfigurationOutput, error) {
	metricAWSAPICalls.WithLabelValues("CreateVpcEndpointServiceConfiguration").Inc()
	return c.ec2Client.CreateVpcEndpointServiceConfiguration(input)
}

func (c *awsClient) DescribeVpcEndpointServiceConfigurations(input *ec2.DescribeVpcEndpointServiceConfigurationsInput) (*ec2.DescribeVpcEndpointServiceConfigurationsOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeVpcEndpointServiceConfigurations").Inc()
	return c.ec2Client.DescribeVpcEndpointServiceConfigurations(input)
}

func (c *awsClient) ModifyVpcEndpointServiceConfiguration(input *ec2.ModifyVpcEndpointServiceConfigurationInput) (*ec2.ModifyVpcEndpointServiceConfigurationOutput, error) {
	metricAWSAPICalls.WithLabelValues("ModifyVpcEndpointServiceConfiguration").Inc()
	return c.ec2Client.ModifyVpcEndpointServiceConfiguration(input)
}

func (c *awsClient) DeleteVpcEndpointServiceConfigurations(input *ec2.DeleteVpcEndpointServiceConfigurationsInput) (*ec2.DeleteVpcEndpointServiceConfigurationsOutput, error) {
	metricAWSAPICalls.WithLabelValues("DeleteVpcEndpointServiceConfigurations").Inc()
	return c.ec2Client.DeleteVpcEndpointServiceConfigurations(input)
}

func (c *awsClient) DescribeVpcEndpointServicePermissions(input *ec2.DescribeVpcEndpointServicePermissionsInput) (*ec2.DescribeVpcEndpointServicePermissionsOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeVpcEndpointServicePermissions").Inc()
	return c.ec2Client.DescribeVpcEndpointServicePermissions(input)
}

func (c *awsClient) ModifyVpcEndpointServicePermissions(input *ec2.ModifyVpcEndpointServicePermissionsInput) (*ec2.ModifyVpcEndpointServicePermissionsOutput, error) {
	metricAWSAPICalls.WithLabelValues("ModifyVpcEndpointServicePermissions").Inc()
	return c.ec2Client.ModifyVpcEndpointServicePermissions(input)
}

func (c *awsClient) DescribeVpcEndpointServices(input *ec2.DescribeVpcEndpointServicesInput) (*ec2.DescribeVpcEndpointServicesOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeVpcEndpointServices").Inc()
	return c.ec2Client.DescribeVpcEndpointServices(input)
}

func (c *awsClient) DescribeVpcEndpoints(input *ec2.DescribeVpcEndpointsInput) (*ec2.DescribeVpcEndpointsOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeVpcEndpoints").Inc()
	return c.ec2Client.DescribeVpcEndpoints(input)
}

func (c *awsClient) DescribeVpcEndpointsPages(input *ec2.DescribeVpcEndpointsInput, fn func(*ec2.DescribeVpcEndpointsOutput, bool) bool) error {
	metricAWSAPICalls.WithLabelValues("DescribeVpcEndpointsPages").Inc()
	return c.ec2Client.DescribeVpcEndpointsPages(input, fn)
}

func (c *awsClient) DescribeNetworkInterfaces(input *ec2.DescribeNetworkInterfacesInput) (*ec2.DescribeNetworkInterfacesOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeNetworkInterfaces").Inc()
	return c.ec2Client.DescribeNetworkInterfaces(input)
}

func (c *awsClient) CreateVpcEndpoint(input *ec2.CreateVpcEndpointInput) (*ec2.CreateVpcEndpointOutput, error) {
	metricAWSAPICalls.WithLabelValues("CreateVpcEndpoint").Inc()
	return c.ec2Client.CreateVpcEndpoint(input)
}

func (c *awsClient) DeleteVpcEndpoints(input *ec2.DeleteVpcEndpointsInput) (*ec2.DeleteVpcEndpointsOutput, error) {
	metricAWSAPICalls.WithLabelValues("DeleteVpcEndpoints").Inc()
	return c.ec2Client.DeleteVpcEndpoints(input)
}

func (c *awsClient) DescribeVpcs(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeVpcs").Inc()
	return c.ec2Client.DescribeVpcs(input)
}

func (c *awsClient) DescribeLoadBalancers(input *elbv2.DescribeLoadBalancersInput) (*elbv2.DescribeLoadBalancersOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeLoadBalancers").Inc()
	return c.elbv2Client.DescribeLoadBalancers(input)
}

func (c *awsClient) GetS3API() s3iface.S3API {
	return c.s3Client
}

func (c *awsClient) Upload(input *s3manager.UploadInput) (*s3manager.UploadOutput, error) {
	return c.s3Uploader.Upload(input)
}

func (c *awsClient) ListHostedZonesByName(input *route53.ListHostedZonesByNameInput) (*route53.ListHostedZonesByNameOutput, error) {
	metricAWSAPICalls.WithLabelValues("ListHostedZonesByName").Inc()
	return c.route53Client.ListHostedZonesByName(input)
}

func (c *awsClient) ListHostedZonesByVPC(input *route53.ListHostedZonesByVPCInput) (*route53.ListHostedZonesByVPCOutput, error) {
	metricAWSAPICalls.WithLabelValues("ListHostedZonesByVPC").Inc()
	return c.route53Client.ListHostedZonesByVPC(input)
}

func (c *awsClient) CreateHostedZone(input *route53.CreateHostedZoneInput) (*route53.CreateHostedZoneOutput, error) {
	metricAWSAPICalls.WithLabelValues("CreateHostedZone").Inc()
	return c.route53Client.CreateHostedZone(input)
}

func (c *awsClient) GetHostedZone(input *route53.GetHostedZoneInput) (*route53.GetHostedZoneOutput, error) {
	metricAWSAPICalls.WithLabelValues("GetHostedZone").Inc()
	return c.route53Client.GetHostedZone(input)
}

func (c *awsClient) ListTagsForResource(input *route53.ListTagsForResourceInput) (*route53.ListTagsForResourceOutput, error) {
	metricAWSAPICalls.WithLabelValues("ListTagsForResource").Inc()
	return c.route53Client.ListTagsForResource(input)
}

func (c *awsClient) ChangeTagsForResource(input *route53.ChangeTagsForResourceInput) (*route53.ChangeTagsForResourceOutput, error) {
	metricAWSAPICalls.WithLabelValues("ChangeTagsForResource").Inc()
	return c.route53Client.ChangeTagsForResource(input)
}

func (c *awsClient) DeleteHostedZone(input *route53.DeleteHostedZoneInput) (*route53.DeleteHostedZoneOutput, error) {
	metricAWSAPICalls.WithLabelValues("DeleteHostedZone").Inc()
	return c.route53Client.DeleteHostedZone(input)
}

func (c *awsClient) GetResourcesPages(input *resourcegroupstaggingapi.GetResourcesInput, fn func(*resourcegroupstaggingapi.GetResourcesOutput, bool) bool) error {
	metricAWSAPICalls.WithLabelValues("GetResourcesPages").Inc()
	return c.tagClient.GetResourcesPages(input, fn)
}

func (c *awsClient) ListResourceRecordSets(input *route53.ListResourceRecordSetsInput) (*route53.ListResourceRecordSetsOutput, error) {
	metricAWSAPICalls.WithLabelValues("ListResourceRecordSets").Inc()
	return c.route53Client.ListResourceRecordSets(input)
}

func (c *awsClient) ChangeResourceRecordSets(input *route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error) {
	metricAWSAPICalls.WithLabelValues("ChangeResourceRecordSets").Inc()
	return c.route53Client.ChangeResourceRecordSets(input)
}

func (c *awsClient) AssociateVPCWithHostedZone(input *route53.AssociateVPCWithHostedZoneInput) (*route53.AssociateVPCWithHostedZoneOutput, error) {
	metricAWSAPICalls.WithLabelValues("AssociateVPCWithHostedZone").Inc()
	return c.route53Client.AssociateVPCWithHostedZone(input)
}

func (c *awsClient) CreateVPCAssociationAuthorization(input *route53.CreateVPCAssociationAuthorizationInput) (*route53.CreateVPCAssociationAuthorizationOutput, error) {
	metricAWSAPICalls.WithLabelValues("CreateVPCAssociationAuthorization").Inc()
	return c.route53Client.CreateVPCAssociationAuthorization(input)
}

func (c *awsClient) DeleteVPCAssociationAuthorization(input *route53.DeleteVPCAssociationAuthorizationInput) (*route53.DeleteVPCAssociationAuthorizationOutput, error) {
	metricAWSAPICalls.WithLabelValues("DeleteVPCAssociationAuthorization").Inc()
	return c.route53Client.DeleteVPCAssociationAuthorization(input)
}

func (c *awsClient) DisassociateVPCFromHostedZone(input *route53.DisassociateVPCFromHostedZoneInput) (*route53.DisassociateVPCFromHostedZoneOutput, error) {
	metricAWSAPICalls.WithLabelValues("DisassociateVPCFromHostedZone").Inc()
	return c.route53Client.DisassociateVPCFromHostedZone(input)
}

func (c *awsClient) GetCallerIdentity(input *sts.GetCallerIdentityInput) (*sts.GetCallerIdentityOutput, error) {
	metricAWSAPICalls.WithLabelValues("GetCallerIdentity").Inc()
	return c.stsClient.GetCallerIdentity(input)
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

	sess, err := NewSessionFromSecret(secret, region)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create AWS session")
	}

	duration := stscreds.DefaultDuration
	sess.Config.Credentials = stscreds.NewCredentials(sess, role.RoleARN, func(p *stscreds.AssumeRoleProvider) {
		p.Duration = duration
		if role.ExternalID != "" {
			p.ExternalID = &role.ExternalID
		}
	})

	return newClientFromSession(sess)
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
	s, err := NewSessionFromSecret(secret, region)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create AWS session")
	}
	return newClientFromSession(s)
}

func newClientFromSession(s *session.Session, cfgs ...*aws.Config) (Client, error) {
	return &awsClient{
		ec2Client:     ec2.New(s, cfgs...),
		elbClient:     elb.New(s, cfgs...),
		elbv2Client:   elbv2.New(s, cfgs...),
		s3Client:      s3.New(s, cfgs...),
		s3Uploader:    s3manager.NewUploader(s),
		route53Client: route53.New(s, cfgs...),
		stsClient:     sts.New(s, cfgs...),
		tagClient:     resourcegroupstaggingapi.New(s, cfgs...),
	}, nil
}

// NewSessionFromSecret creates a new AWS session using the configuration in the secret. If the secret
// was nil, it initializes a new session using configuration of the envionment.
func NewSessionFromSecret(secret *corev1.Secret, region string) (*session.Session, error) {
	options := session.Options{
		Config: aws.Config{
			Region:           aws.String(region),
			EndpointResolver: endpoints.ResolverFunc(awsChinaEndpointResolver),
		},
		SharedConfigState: session.SharedConfigEnable,
	}

	// Special case to not use a secret to gather credentials.
	if secret != nil {
		config, err := awsCLIConfigFromSecret(secret)
		if err != nil {
			return nil, err
		}
		f, err := os.CreateTemp("", "hive-aws-config")
		if err != nil {
			return nil, err
		}
		defer f.Close()
		if _, err := f.Write(config); err != nil {
			return nil, err
		}
		defer os.Remove(f.Name())

		options.SharedConfigFiles = []string{f.Name()}
		options.Profile = "default"
	}

	// Otherwise default to relying on the environment where the actuator is running:
	s, err := session.NewSessionWithOptions(options)
	if err != nil {
		return nil, err
	}

	s.Handlers.Build.PushBackNamed(request.NamedHandler{
		Name: "openshift.io/hive",
		Fn:   request.MakeAddToUserAgentHandler("openshift.io hive", "v1"),
	})

	return s, nil
}

var credentialProcessRE = regexp.MustCompile(`\bcredential_process\b`)

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
		if k == "credential_process" {
			return nil, errors.New("credential_process is insecure and thus forbidden")
		}
		fmt.Fprintf(buf, "%s = %s\n", k, v)
	}
	return buf.Bytes(), nil
}

func awsChinaEndpointResolver(service, region string, optFns ...func(*endpoints.Options)) (endpoints.ResolvedEndpoint, error) {
	if service != route53.EndpointsID || region != constants.AWSChinaRoute53Region {
		return endpoints.DefaultResolver().EndpointFor(service, region, optFns...)
	}

	return endpoints.ResolvedEndpoint{
		URL:         "https://route53.amazonaws.com.cn",
		PartitionID: "aws-cn",
	}, nil
}
