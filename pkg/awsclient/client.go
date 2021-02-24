package awsclient

import (
	"context"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ec2/ec2iface"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/service/elb/elbiface"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/elbv2/elbv2iface"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/iam/iamiface"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/route53/route53iface"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/aws/aws-sdk-go/service/sts/stsiface"

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
	DescribeImages(*ec2.DescribeImagesInput) (*ec2.DescribeImagesOutput, error)
	DescribeVpcs(*ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error)
	DescribeSubnets(*ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error)
	DescribeRouteTables(*ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error)
	DescribeSecurityGroups(*ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error)
	RunInstances(*ec2.RunInstancesInput) (*ec2.Reservation, error)
	DescribeInstances(*ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error)
	TerminateInstances(*ec2.TerminateInstancesInput) (*ec2.TerminateInstancesOutput, error)
	StopInstances(*ec2.StopInstancesInput) (*ec2.StopInstancesOutput, error)
	StartInstances(*ec2.StartInstancesInput) (*ec2.StartInstancesOutput, error)
	CreateVpcEndpointServiceConfiguration(*ec2.CreateVpcEndpointServiceConfigurationInput) (*ec2.CreateVpcEndpointServiceConfigurationOutput, error)
	DescribeVpcEndpointServiceConfigurations(*ec2.DescribeVpcEndpointServiceConfigurationsInput) (*ec2.DescribeVpcEndpointServiceConfigurationsOutput, error)
	ModifyVpcEndpointServiceConfiguration(*ec2.ModifyVpcEndpointServiceConfigurationInput) (*ec2.ModifyVpcEndpointServiceConfigurationOutput, error)
	DeleteVpcEndpointServiceConfigurations(*ec2.DeleteVpcEndpointServiceConfigurationsInput) (*ec2.DeleteVpcEndpointServiceConfigurationsOutput, error)
	DescribeVpcEndpointServicePermissions(*ec2.DescribeVpcEndpointServicePermissionsInput) (*ec2.DescribeVpcEndpointServicePermissionsOutput, error)
	ModifyVpcEndpointServicePermissions(*ec2.ModifyVpcEndpointServicePermissionsInput) (*ec2.ModifyVpcEndpointServicePermissionsOutput, error)
	DescribeVpcEndpointServices(*ec2.DescribeVpcEndpointServicesInput) (*ec2.DescribeVpcEndpointServicesOutput, error)
	DescribeVpcEndpoints(*ec2.DescribeVpcEndpointsInput) (*ec2.DescribeVpcEndpointsOutput, error)
	CreateVpcEndpoint(*ec2.CreateVpcEndpointInput) (*ec2.CreateVpcEndpointOutput, error)
	DeleteVpcEndpoints(*ec2.DeleteVpcEndpointsInput) (*ec2.DeleteVpcEndpointsOutput, error)

	// ELB
	RegisterInstancesWithLoadBalancer(*elb.RegisterInstancesWithLoadBalancerInput) (*elb.RegisterInstancesWithLoadBalancerOutput, error)

	// ELBV2
	DescribeLoadBalancers(*elbv2.DescribeLoadBalancersInput) (*elbv2.DescribeLoadBalancersOutput, error)

	// IAM
	CreateAccessKey(*iam.CreateAccessKeyInput) (*iam.CreateAccessKeyOutput, error)
	CreateUser(*iam.CreateUserInput) (*iam.CreateUserOutput, error)
	DeleteAccessKey(*iam.DeleteAccessKeyInput) (*iam.DeleteAccessKeyOutput, error)
	DeleteUser(*iam.DeleteUserInput) (*iam.DeleteUserOutput, error)
	DeleteUserPolicy(*iam.DeleteUserPolicyInput) (*iam.DeleteUserPolicyOutput, error)
	GetUser(*iam.GetUserInput) (*iam.GetUserOutput, error)
	ListAccessKeys(*iam.ListAccessKeysInput) (*iam.ListAccessKeysOutput, error)
	ListUserPolicies(*iam.ListUserPoliciesInput) (*iam.ListUserPoliciesOutput, error)
	PutUserPolicy(*iam.PutUserPolicyInput) (*iam.PutUserPolicyOutput, error)

	// S3
	CreateBucket(*s3.CreateBucketInput) (*s3.CreateBucketOutput, error)
	DeleteBucket(*s3.DeleteBucketInput) (*s3.DeleteBucketOutput, error)
	ListBuckets(*s3.ListBucketsInput) (*s3.ListBucketsOutput, error)
	PutObject(input *s3.PutObjectInput) (*s3.PutObjectOutput, error)

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
	ListHostedZones(input *route53.ListHostedZonesInput) (*route53.ListHostedZonesOutput, error)
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
	iamClient     iamiface.IAMAPI
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

func (c *awsClient) DescribeImages(input *ec2.DescribeImagesInput) (*ec2.DescribeImagesOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeImages").Inc()
	return c.ec2Client.DescribeImages(input)
}

func (c *awsClient) DescribeVpcs(input *ec2.DescribeVpcsInput) (*ec2.DescribeVpcsOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeVpcs").Inc()
	return c.ec2Client.DescribeVpcs(input)
}

func (c *awsClient) DescribeSubnets(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeSubnets").Inc()
	return c.ec2Client.DescribeSubnets(input)
}

func (c *awsClient) DescribeRouteTables(input *ec2.DescribeRouteTablesInput) (*ec2.DescribeRouteTablesOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeRouteTables").Inc()
	return c.ec2Client.DescribeRouteTables(input)
}

func (c *awsClient) DescribeSecurityGroups(input *ec2.DescribeSecurityGroupsInput) (*ec2.DescribeSecurityGroupsOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeSecurityGroups").Inc()
	return c.ec2Client.DescribeSecurityGroups(input)
}

func (c *awsClient) RunInstances(input *ec2.RunInstancesInput) (*ec2.Reservation, error) {
	metricAWSAPICalls.WithLabelValues("RunInstances").Inc()
	return c.ec2Client.RunInstances(input)
}

func (c *awsClient) DescribeInstances(input *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeInstances").Inc()
	return c.ec2Client.DescribeInstances(input)
}

func (c *awsClient) TerminateInstances(input *ec2.TerminateInstancesInput) (*ec2.TerminateInstancesOutput, error) {
	metricAWSAPICalls.WithLabelValues("TerminateInstances").Inc()
	return c.ec2Client.TerminateInstances(input)
}

func (c *awsClient) StopInstances(input *ec2.StopInstancesInput) (*ec2.StopInstancesOutput, error) {
	metricAWSAPICalls.WithLabelValues("StopInstances").Inc()
	return c.ec2Client.StopInstances(input)
}

func (c *awsClient) StartInstances(input *ec2.StartInstancesInput) (*ec2.StartInstancesOutput, error) {
	metricAWSAPICalls.WithLabelValues("StartInstances").Inc()
	return c.ec2Client.StartInstances(input)
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

func (c *awsClient) CreateVpcEndpoint(input *ec2.CreateVpcEndpointInput) (*ec2.CreateVpcEndpointOutput, error) {
	metricAWSAPICalls.WithLabelValues("CreateVpcEndpoint").Inc()
	return c.ec2Client.CreateVpcEndpoint(input)
}

func (c *awsClient) DeleteVpcEndpoints(input *ec2.DeleteVpcEndpointsInput) (*ec2.DeleteVpcEndpointsOutput, error) {
	metricAWSAPICalls.WithLabelValues("DeleteVpcEndpoints").Inc()
	return c.ec2Client.DeleteVpcEndpoints(input)
}

func (c *awsClient) RegisterInstancesWithLoadBalancer(input *elb.RegisterInstancesWithLoadBalancerInput) (*elb.RegisterInstancesWithLoadBalancerOutput, error) {
	metricAWSAPICalls.WithLabelValues("RegisterInstancesWithLoadBalancer").Inc()
	return c.elbClient.RegisterInstancesWithLoadBalancer(input)
}

func (c *awsClient) DescribeLoadBalancers(input *elbv2.DescribeLoadBalancersInput) (*elbv2.DescribeLoadBalancersOutput, error) {
	metricAWSAPICalls.WithLabelValues("DescribeLoadBalancers").Inc()
	return c.elbv2Client.DescribeLoadBalancers(input)
}

func (c *awsClient) CreateAccessKey(input *iam.CreateAccessKeyInput) (*iam.CreateAccessKeyOutput, error) {
	metricAWSAPICalls.WithLabelValues("CreateAccessKey").Inc()
	return c.iamClient.CreateAccessKey(input)
}

func (c *awsClient) CreateUser(input *iam.CreateUserInput) (*iam.CreateUserOutput, error) {
	metricAWSAPICalls.WithLabelValues("CreateUser").Inc()
	return c.iamClient.CreateUser(input)
}

func (c *awsClient) DeleteAccessKey(input *iam.DeleteAccessKeyInput) (*iam.DeleteAccessKeyOutput, error) {
	metricAWSAPICalls.WithLabelValues("DeleteAccessKey").Inc()
	return c.iamClient.DeleteAccessKey(input)
}

func (c *awsClient) DeleteUser(input *iam.DeleteUserInput) (*iam.DeleteUserOutput, error) {
	metricAWSAPICalls.WithLabelValues("DeleteUser").Inc()
	return c.iamClient.DeleteUser(input)
}

func (c *awsClient) DeleteUserPolicy(input *iam.DeleteUserPolicyInput) (*iam.DeleteUserPolicyOutput, error) {
	metricAWSAPICalls.WithLabelValues("DeleteUserPolicy").Inc()
	return c.iamClient.DeleteUserPolicy(input)
}
func (c *awsClient) GetUser(input *iam.GetUserInput) (*iam.GetUserOutput, error) {
	metricAWSAPICalls.WithLabelValues("GetUser").Inc()
	return c.iamClient.GetUser(input)
}

func (c *awsClient) ListAccessKeys(input *iam.ListAccessKeysInput) (*iam.ListAccessKeysOutput, error) {
	metricAWSAPICalls.WithLabelValues("ListAccessKeys").Inc()
	return c.iamClient.ListAccessKeys(input)
}

func (c *awsClient) ListUserPolicies(input *iam.ListUserPoliciesInput) (*iam.ListUserPoliciesOutput, error) {
	metricAWSAPICalls.WithLabelValues("ListUserPolicies").Inc()
	return c.iamClient.ListUserPolicies(input)
}

func (c *awsClient) PutUserPolicy(input *iam.PutUserPolicyInput) (*iam.PutUserPolicyOutput, error) {
	metricAWSAPICalls.WithLabelValues("PutUserPolicy").Inc()
	return c.iamClient.PutUserPolicy(input)
}

func (c *awsClient) CreateBucket(input *s3.CreateBucketInput) (*s3.CreateBucketOutput, error) {
	metricAWSAPICalls.WithLabelValues("CreateBucket").Inc()
	return c.s3Client.CreateBucket(input)
}

func (c *awsClient) DeleteBucket(input *s3.DeleteBucketInput) (*s3.DeleteBucketOutput, error) {
	metricAWSAPICalls.WithLabelValues("DeleteBucket").Inc()
	return c.s3Client.DeleteBucket(input)
}

func (c *awsClient) ListBuckets(input *s3.ListBucketsInput) (*s3.ListBucketsOutput, error) {
	metricAWSAPICalls.WithLabelValues("ListBuckets").Inc()
	return c.s3Client.ListBuckets(input)
}

func (c *awsClient) PutObject(input *s3.PutObjectInput) (*s3.PutObjectOutput, error) {
	metricAWSAPICalls.WithLabelValues("PutObject").Inc()
	return c.s3Client.PutObject(input)
}

func (c *awsClient) GetS3API() s3iface.S3API {
	return c.s3Client
}

func (c *awsClient) Upload(input *s3manager.UploadInput) (*s3manager.UploadOutput, error) {
	return c.s3Uploader.Upload(input)
}

func (c *awsClient) ListHostedZones(input *route53.ListHostedZonesInput) (*route53.ListHostedZonesOutput, error) {
	metricAWSAPICalls.WithLabelValues("ListHostedZones").Inc()
	return c.route53Client.ListHostedZones(input)
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
	awsConfig := &aws.Config{
		Region:           aws.String(region),
		EndpointResolver: endpoints.ResolverFunc(awsChinaEndpointResolver),
	}

	// Special case to not use a secret to gather credentials.
	if secret != nil {
		accessKeyID, ok := secret.Data[constants.AWSAccessKeyIDSecretKey]
		if !ok {
			return nil, fmt.Errorf("AWS credentials secret %v did not contain key %v",
				secret.Name, constants.AWSAccessKeyIDSecretKey)
		}
		secretAccessKey, ok := secret.Data[constants.AWSSecretAccessKeySecretKey]
		if !ok {
			return nil, fmt.Errorf("AWS credentials secret %v did not contain key %v",
				secret.Name, constants.AWSSecretAccessKeySecretKey)

		}

		awsConfig.Credentials = credentials.NewStaticCredentials(
			string(accessKeyID), string(secretAccessKey), "")
	}

	// Otherwise default to relying on the IAM role of the masters where the actuator is running:
	s, err := session.NewSession(awsConfig)
	if err != nil {
		return nil, err
	}

	s.Handlers.Build.PushBackNamed(request.NamedHandler{
		Name: "openshift.io/hive",
		Fn:   request.MakeAddToUserAgentHandler("openshift.io hive", "v1"),
	})

	return &awsClient{
		ec2Client:     ec2.New(s),
		elbClient:     elb.New(s),
		elbv2Client:   elbv2.New(s),
		iamClient:     iam.New(s),
		s3Client:      s3.New(s),
		s3Uploader:    s3manager.NewUploader(s),
		route53Client: route53.New(s),
		stsClient:     sts.New(s),
		tagClient:     resourcegroupstaggingapi.New(s),
	}, nil
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
