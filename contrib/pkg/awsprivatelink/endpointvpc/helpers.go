package endpointvpc

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/awsclient"
)

func getAWSClientsByRegion(secret *corev1.Secret, regions sets.Set[string]) (map[string]awsclient.Client, error) {
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

func findVpcInInventory(vpcId string, inventory []hivev1.AWSPrivateLinkInventory) (int, bool) {
	for i, endpointVpc := range inventory {
		if vpcId == endpointVpc.AWSPrivateLinkVPC.VPCID {
			return i, true
		}
	}

	return -1, false
}

// getDefaultSGOfVpc gets the default SG of a VPC.
func getDefaultSGOfVpc(awsClients awsclient.Client, vpcId string) (string, error) {
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

// revokeAllIngressFromCIDR removes an SG inbound rule which allows all ingress originating from a CIDR block.
func revokeAllIngressFromCIDR(awsClients awsclient.Client, SG, cidr *string) (*ec2.RevokeSecurityGroupIngressOutput, error) {
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

// revokeAllIngressFromSG removes an SG inbound rule which allows all ingress originating from another SG.
func revokeAllIngressFromSG(awsClients awsclient.Client, SG, sourceSG *string) (*ec2.RevokeSecurityGroupIngressOutput, error) {
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

// authorizeAllIngressFromCIDR adds an SG inbound rule which allows all ingress originating from a CIDR block.
func authorizeAllIngressFromCIDR(awsClients awsclient.Client, SG, cidr, description *string) (*ec2.AuthorizeSecurityGroupIngressOutput, error) {
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

// authorizeAllIngressFromSG adds an SG inbound rule which allows all ingress originating from another SG.
func authorizeAllIngressFromSG(awsClients awsclient.Client, SG, sourceSG, description *string) (*ec2.AuthorizeSecurityGroupIngressOutput, error) {
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

// getInfraIdFromVpcId gets the infraID of an OCP cluster using the ID of the VPC it resides.
func getInfraIdFromVpcId(awsClients awsclient.Client, vpcId string) (string, error) {
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
		if k := aws.ToString(tag.Key); strings.HasPrefix(k, targetPrefix) {
			return strings.Replace(k, targetPrefix, "", 1), nil
		}
	}
	return "", fmt.Errorf("no tag with prefix %v found on VPC %v", targetPrefix, vpcId)
}

// getWorkerSGFromVpcId gets the worker SG ID of an OCP cluster using the ID of the VPC it resides.
func getWorkerSGFromVpcId(awsClients awsclient.Client, vpcId string) (string, error) {
	infraID, err := getInfraIdFromVpcId(awsClients, vpcId)
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

	return aws.ToString(describeSecurityGroupsOutput.SecurityGroups[0].GroupId), err
}

// getCIDRFromVpcId gets the CIDR block of a VPC using the ID of it.
func getCIDRFromVpcId(awsClients awsclient.Client, vpcId string) (string, error) {
	// When we specify the resource IDs explicitly instead of using filtering,
	// AWS functions will return a non-nil error if nothing is found.
	describeVpcOutput, err := awsClients.DescribeVpcs(&ec2.DescribeVpcsInput{
		VpcIds: []string{vpcId},
	})
	if err != nil {
		return "", err
	}

	return aws.ToString(describeVpcOutput.Vpcs[0].CidrBlock), nil
}
