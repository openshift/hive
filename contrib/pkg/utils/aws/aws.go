package aws

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/contrib/pkg/utils"
	"github.com/openshift/hive/pkg/awsclient"
	"github.com/openshift/hive/pkg/constants"

	log "github.com/sirupsen/logrus"
	ini "gopkg.in/ini.v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// PrivateLinkHubAcctCredsName is the name of the AWS PrivateLink Hub account credentials Secret
	// created by the "hiveutil awsprivatelink enable" command
	PrivateLinkHubAcctCredsName = "awsprivatelink-hub-acct-creds"

	// PrivateLinkHubAcctCredsLabel is added to the AWS PrivateLink Hub account credentials Secret
	// created by the "hiveutil awsprivatelink enable" command and
	// referenced by HiveConfig.spec.awsPrivateLink.credentialsSecretRef.
	PrivateLinkHubAcctCredsLabel = "hive.openshift.io/awsprivatelink-hub-acct-credentials"
)

func GetAWSClientsByRegion(secret *corev1.Secret, regions sets.Set[string]) (map[string]awsclient.Client, error) {
	awsClientsByRegion := make(map[string]awsclient.Client)
	for region := range regions {
		awsClients, err := awsclient.NewClientFromSecret(secret, region)
		if err != nil {
			return awsClientsByRegion, err
		}
		awsClientsByRegion[region] = awsClients
	}

	return awsClientsByRegion, nil
}

func FindVpcInInventory(vpcId string, inventory []hivev1.AWSPrivateLinkInventory) (int, bool) {
	for i, endpointVpc := range inventory {
		if vpcId == endpointVpc.AWSPrivateLinkVPC.VPCID {
			return i, true
		}
	}

	return -1, false
}

// GetDefaultSGOfVpc gets the default SG of a VPC.
func GetDefaultSGOfVpc(awsClients awsclient.Client, vpcId string) (string, error) {
	describeSecurityGroupsOutput, err := awsClients.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []string{vpcId},
			},
			{
				Name:   aws.String("group-name"),
				Values: []string{"default"},
			},
		},
	})
	if err != nil {
		return "", err
	}
	if len(describeSecurityGroupsOutput.SecurityGroups) == 0 {
		return "", fmt.Errorf("default SG not found for VPC %v", vpcId)
	}

	return *describeSecurityGroupsOutput.SecurityGroups[0].GroupId, nil
}

// RevokeAllIngressFromCIDR removes an SG inbound rule which allows all ingress originating from a CIDR block.
func RevokeAllIngressFromCIDR(awsClients awsclient.Client, SG, cidr *string) (*ec2.RevokeSecurityGroupIngressOutput, error) {
	return awsClients.RevokeSecurityGroupIngress(&ec2.RevokeSecurityGroupIngressInput{
		GroupId: SG,
		IpPermissions: []ec2types.IpPermission{
			{
				IpRanges: []ec2types.IpRange{
					{
						CidrIp: cidr,
					},
				},
				IpProtocol: aws.String("-1"),
			},
		},
	})
}

// RevokeAllIngressFromSG removes an SG inbound rule which allows all ingress originating from another SG.
func RevokeAllIngressFromSG(awsClients awsclient.Client, SG, sourceSG *string) (*ec2.RevokeSecurityGroupIngressOutput, error) {
	return awsClients.RevokeSecurityGroupIngress(&ec2.RevokeSecurityGroupIngressInput{
		GroupId: SG,
		IpPermissions: []ec2types.IpPermission{
			{
				IpProtocol: aws.String("-1"),
				UserIdGroupPairs: []ec2types.UserIdGroupPair{
					{
						GroupId: sourceSG,
					},
				},
			},
		},
	})
}

// AuthorizeAllIngressFromCIDR adds an SG inbound rule which allows all ingress originating from a CIDR block.
func AuthorizeAllIngressFromCIDR(awsClients awsclient.Client, SG, cidr, description *string) (*ec2.AuthorizeSecurityGroupIngressOutput, error) {
	return awsClients.AuthorizeSecurityGroupIngress(&ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: SG,
		IpPermissions: []ec2types.IpPermission{
			{
				IpRanges: []ec2types.IpRange{
					{
						CidrIp:      cidr,
						Description: description,
					},
				},
				IpProtocol: aws.String("-1"),
			},
		},
	})
}

// AuthorizeAllIngressFromSG adds an SG inbound rule which allows all ingress originating from another SG.
func AuthorizeAllIngressFromSG(awsClients awsclient.Client, SG, sourceSG, description *string) (*ec2.AuthorizeSecurityGroupIngressOutput, error) {
	return awsClients.AuthorizeSecurityGroupIngress(&ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: SG,
		IpPermissions: []ec2types.IpPermission{
			{
				IpProtocol: aws.String("-1"),
				UserIdGroupPairs: []ec2types.UserIdGroupPair{
					{
						Description: description,
						GroupId:     sourceSG,
					},
				},
			},
		},
	})
}

// GetInfraIdFromVpcId gets the infraID of an OCP cluster using the ID of the VPC it resides.
func GetInfraIdFromVpcId(awsClients awsclient.Client, vpcId string) (string, error) {
	// When we specify the resource IDs explicitly instead of using filtering,
	// AWS functions will return a non-nil error if nothing is found.
	describeVpcsOutput, err := awsClients.DescribeVpcs(&ec2.DescribeVpcsInput{
		VpcIds: []string{vpcId},
	})
	if err != nil {
		return "", err
	}

	targetPrefix := "kubernetes.io/cluster/"
	for _, tag := range describeVpcsOutput.Vpcs[0].Tags {
		if strings.HasPrefix(*tag.Key, targetPrefix) {
			return strings.Replace(*tag.Key, targetPrefix, "", 1), nil
		}
	}
	return "", fmt.Errorf("no tag with prefix %v found on VPC %v", targetPrefix, vpcId)
}

// GetWorkerSGFromVpcId gets the worker SG ID of an OCP cluster using the ID of the VPC it resides.
func GetWorkerSGFromVpcId(awsClients awsclient.Client, vpcId string) (string, error) {
	infraID, err := GetInfraIdFromVpcId(awsClients, vpcId)
	if err != nil {
		return "", err
	}

	describeSecurityGroupsOutput, err := awsClients.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String("tag:Name"),
				Values: []string{infraID + "-worker-sg", infraID + "-node"},
			},
		},
	})
	if err != nil {
		return "", err
	}
	if len(describeSecurityGroupsOutput.SecurityGroups) == 0 {
		return "", fmt.Errorf("worker SG not found for VPC %v", vpcId)
	}

	return *describeSecurityGroupsOutput.SecurityGroups[0].GroupId, err
}

// GetCIDRFromVpcId gets the CIDR block of a VPC using the ID of it.
func GetCIDRFromVpcId(awsClients awsclient.Client, vpcId string) (string, error) {
	// When we specify the resource IDs explicitly instead of using filtering,
	// AWS functions will return a non-nil error if nothing is found.
	describeVpcOutput, err := awsClients.DescribeVpcs(&ec2.DescribeVpcsInput{
		VpcIds: []string{vpcId},
	})
	if err != nil {
		return "", err
	}

	return *describeVpcOutput.Vpcs[0].CidrBlock, nil
}

// GetAWSCreds reads AWS credentials either from either the specified credentials file,
// the standard environment variables, or a default credentials file. (~/.aws/credentials)
// The defaultCredsFile will only be used if credsFile is empty and the environment variables
// are not set.
func GetAWSCreds(credsFile, defaultCredsFile string) (string, string, error) {
	credsFilePath := defaultCredsFile
	switch {
	case credsFile != "":
		credsFilePath = credsFile
	default:
		secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
		accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
		if len(secretAccessKey) > 0 && len(accessKeyID) > 0 {
			return accessKeyID, secretAccessKey, nil
		}
	}
	credFile, err := ini.Load(credsFilePath)
	if err != nil {
		log.Error("Cannot load AWS credentials")
		return "", "", err
	}
	defaultSection, err := credFile.GetSection("default")
	if err != nil {
		log.Error("Cannot get default section from AWS credentials file")
		return "", "", err
	}
	accessKeyIDValue := defaultSection.Key("aws_access_key_id")
	secretAccessKeyValue := defaultSection.Key("aws_secret_access_key")
	if accessKeyIDValue == nil || secretAccessKeyValue == nil {
		log.Error("AWS credentials file missing keys in default section")
	}
	return accessKeyIDValue.String(), secretAccessKeyValue.String(), nil
}

var awsConfigForbidCredentialProcess utils.ProjectToDirFileFilter = func(key string, contents []byte) (basename string, newContents []byte, err error) {
	// First, only process aws_config
	bn, newContents, err := utils.ProjectOnlyTheseKeys(constants.AWSConfigSecretKey)(key, contents)
	// If that passed, scrub for credential_process
	if err == nil && bn != "" && awsclient.ContainsCredentialProcess(newContents) {
		return "", nil, errors.New("credential_process is insecure and thus forbidden")
	}
	return bn, newContents, err
}

// ConfigureCreds loads a secret designated by the environment variables CLUSTERDEPLOYMENT_NAMESPACE
// and CREDS_SECRET_NAME and configures AWS credential environment variables and config files
// accordingly.
func ConfigureCreds(c client.Client) {
	credsSecret := utils.LoadSecretOrDie(c, "CREDS_SECRET_NAME")
	if credsSecret == nil {
		return
	}
	// Should we bounce if any of the following already exist?
	if id := string(credsSecret.Data[constants.AWSAccessKeyIDSecretKey]); id != "" {
		os.Setenv("AWS_ACCESS_KEY_ID", id)
	}
	if secret := string(credsSecret.Data[constants.AWSSecretAccessKeySecretKey]); secret != "" {
		os.Setenv("AWS_SECRET_ACCESS_KEY", secret)
	}
	if config := credsSecret.Data[constants.AWSConfigSecretKey]; len(config) != 0 {
		// Lay this down as a file, but forbid credential_process
		utils.ProjectToDir(credsSecret, constants.AWSCredsMount, awsConfigForbidCredentialProcess)
		os.Setenv("AWS_CONFIG_FILE", filepath.Join(constants.AWSCredsMount, constants.AWSConfigSecretKey))
	}
	// This would normally allow credential_process in the config file, but we checked for that above.
	os.Setenv("AWS_SDK_LOAD_CONFIG", "true")
	// Install cluster proxy trusted CA bundle
	utils.InstallCerts(constants.TrustedCABundleDir)
}
