package endpointvpc

import (
	"context"
	"fmt"
	"reflect"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/contrib/pkg/awsprivatelink/common"
	awsutils "github.com/openshift/hive/contrib/pkg/utils/aws"
	"github.com/openshift/hive/pkg/awsclient"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"errors"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
)

type endpointVPCAddOptions struct {
	hiveConfig        hivev1.HiveConfig
	associatedVpcs    []hivev1.AWSAssociatedVPC
	endpointVpcId     string
	endpointVpcRegion string
	endpointSubnetIds []string

	endpointVpcClients awsclient.Client
	awsClientsByRegion map[string]awsclient.Client
}

func NewEndpointVPCAddCommand() *cobra.Command {
	opt := &endpointVPCAddOptions{}

	cmd := &cobra.Command{
		Use:   "add vpc-id",
		Short: "Networking setup between an endpoint VPC and each associated VPC",
		Long: `Networking setup between an endpoint VPC and each associated VPC 
specified in HiveConfig.spec.awsPrivateLink.associatedVPCs: 
1) Establish VPC peering connection between the endpoint VPC and the associated VPC
2) Add a route (to the peered VPC) in relevant route tables of the associated VPC and the endpoint VPC 
3) Update relevant security groups of the associated VPC and the endpoint VPC to allow traffic between them
4) Add the endpoint VPC to HiveConfig.spec.awsPrivateLink.endpointVPCInventory`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := opt.Complete(cmd, args); err != nil {
				return
			}
			if err := opt.Validate(cmd, args); err != nil {
				return
			}
			if err := opt.Run(cmd, args); err != nil {
				return
			}
		},
	}

	regionFlag := "region"
	subnetIdsFlag := "subnet-ids"

	flags := cmd.Flags()
	flags.StringVar(&opt.endpointVpcRegion, regionFlag, "", "AWS Region of the endpoint VPC to add")
	flags.StringSliceVar(&opt.endpointSubnetIds, subnetIdsFlag, []string{}, "IDs of the endpoint subnets to use (as a comma-separated string)")

	_ = cmd.MarkFlagRequired(regionFlag)
	_ = cmd.MarkFlagRequired(subnetIdsFlag)
	return cmd
}

func (o *endpointVPCAddOptions) Complete(cmd *cobra.Command, args []string) error {
	o.endpointVpcId = args[0]

	// Get HiveConfig
	if err := common.DynamicClient.Get(context.Background(), types.NamespacedName{Name: "hive"}, &o.hiveConfig); err != nil {
		log.WithError(err).Fatal("Failed to get HiveConfig/hive")
	}
	if o.hiveConfig.Spec.AWSPrivateLink == nil {
		log.Fatal(`AWS PrivateLink is not enabled in HiveConfig. Please call "hiveutil awsprivatelink enable" first.`)
	}
	o.associatedVpcs = o.hiveConfig.Spec.AWSPrivateLink.AssociatedVPCs
	if len(o.associatedVpcs) == 0 {
		log.Warn("HiveConfig/hive does not specify any associated VPC. " +
			"The endpoint VPC passed in as argument will still be added to HiveConfig, " +
			"and yet no cloud resources will be set up for networking.")
	}

	// Get AWS clients by region
	regions := sets.New(o.endpointVpcRegion)
	for _, associatedVpc := range o.associatedVpcs {
		regions.Insert(associatedVpc.AWSPrivateLinkVPC.Region)
	}
	// Use the passed-in credsSecret if possible
	awsClientsByRegion, err := awsutils.GetAWSClientsByRegion(common.CredsSecret, regions)
	if err != nil {
		log.WithError(err).Fatal("Failed to get AWS clients")
	}
	o.awsClientsByRegion = awsClientsByRegion
	// A shortcut to AWS clients of the endpoint VPC
	o.endpointVpcClients = o.awsClientsByRegion[o.endpointVpcRegion]

	return nil
}

func (o *endpointVPCAddOptions) Validate(cmd *cobra.Command, args []string) error {
	// Check if the endpoint VPC exists
	if _, err := o.endpointVpcClients.DescribeVpcs(&ec2.DescribeVpcsInput{
		VpcIds: []string{o.endpointVpcId},
	}); err != nil {
		log.WithError(err).Fatal("Failed to describe endpoint VPC")
	}

	// Check if the endpoint subnets belong to the endpoint VPC
	err := o.endpointVpcClients.DescribeSubnetsPages(
		&ec2.DescribeSubnetsInput{
			SubnetIds: o.endpointSubnetIds,
		},
		func(page *ec2.DescribeSubnetsOutput, lastPage bool) bool {
			for _, subnet := range page.Subnets {
				if aws.ToString(subnet.VpcId) != o.endpointVpcId {
					log.Fatalf("Subnet %v does not belong to the endpoint VPC", aws.ToString(subnet.SubnetId))
				}
			}
			return !lastPage
		},
	)
	if err != nil {
		log.WithError(err).Fatalf("Failed to describe the endpoint subnets")
	}

	return nil
}

func (o *endpointVPCAddOptions) Run(cmd *cobra.Command, args []string) error {
	// Get default SG of the endpoint VPC
	endpointVPCDefaultSG, err := awsutils.GetDefaultSGOfVpc(o.endpointVpcClients, aws.String(o.endpointVpcId))
	if err != nil {
		log.WithError(err).Fatal("Failed to get default SG of the endpoint VPC")
	}
	log.Debugf("Found default SG %v of the endpoint VPC", endpointVPCDefaultSG)

	// Networking setup between the endpoint VPC and each associated VPC
	for _, associatedVpc := range o.associatedVpcs {
		associatedVpcRegion := associatedVpc.AWSPrivateLinkVPC.Region
		associatedVpcClients := o.awsClientsByRegion[associatedVpcRegion]
		associatedVpcId := associatedVpc.AWSPrivateLinkVPC.VPCID
		log.Infof("Setting up networking between associated VPC %v and endpoint VPC %v", associatedVpcId, o.endpointVpcId)

		// Setup peering connection
		acceptVpcPeeringConnectionOutput, err := setupVpcPeeringConnection(
			associatedVpcClients,
			o.endpointVpcClients,
			aws.String(associatedVpcId),
			aws.String(o.endpointVpcId),
			aws.String(o.endpointVpcRegion),
		)
		if err != nil {
			log.WithError(err).Fatal("Failed to setup VPC peering connection")
		}
		vpcPeeringConnectionId := acceptVpcPeeringConnectionOutput.VpcPeeringConnection.VpcPeeringConnectionId
		associatedVpcCIDR := acceptVpcPeeringConnectionOutput.VpcPeeringConnection.RequesterVpcInfo.CidrBlock
		endpointVpcCIDR := acceptVpcPeeringConnectionOutput.VpcPeeringConnection.AccepterVpcInfo.CidrBlock
		log.Debugf("Found associated VPC CIDR = %v, endpoint VPC CIDR = %v", aws.ToString(associatedVpcCIDR), aws.ToString(endpointVpcCIDR))

		// Update route tables
		log.Info("Adding route to private route tables of the associated VPC")
		if err = addRouteToRouteTables(
			associatedVpcClients,
			aws.String(associatedVpcId),
			endpointVpcCIDR,
			vpcPeeringConnectionId,
			&ec2types.Filter{Name: aws.String("tag:Name"), Values: []string{"*private*"}},
		); err != nil {
			log.WithError(err).Fatal("Failed to add route to private route tables of the associated VPC")
		}

		log.Info("Adding route to route tables of the endpoint subnets")
		if err = addRouteToRouteTables(
			o.endpointVpcClients,
			aws.String(o.endpointVpcId),
			associatedVpcCIDR,
			vpcPeeringConnectionId,
			&ec2types.Filter{Name: aws.String("association.subnet-id"), Values: o.endpointSubnetIds},
		); err != nil {
			log.WithError(err).Fatal("Failed to add route to route tables of the endpoint subnets")
		}

		// Update SGs
		associatedVpcWorkerSG, err := awsutils.GetWorkerSGFromVpcId(
			associatedVpcClients,
			aws.String(associatedVpcId),
		)
		if err != nil {
			log.WithError(err).Fatal("Failed to get worker SG of the associated VPC")
		}
		log.Debugf("Found worker SG %v of the associated Hive cluster", associatedVpcWorkerSG)

		// Different treatment for cross-region vs same-region peering
		switch associatedVpcRegion {
		// Associated VPC & endpoint VPC in same region => allow ingress from SG of the peer
		case o.endpointVpcRegion:
			log.Info("Authorizing traffic from the associated VPC's worker SG to the endpoint VPC's default SG")
			if _, err = awsutils.AuthorizeAllIngressFromSG(
				o.endpointVpcClients,
				aws.String(endpointVPCDefaultSG),
				aws.String(associatedVpcWorkerSG),
				aws.String(fmt.Sprintf("Access from worker SG of associated VPC %s", associatedVpcId)),
			); err != nil {
				// Proceed if ingress already authorized, fail otherwise
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) && apiErr.ErrorCode() == "InvalidPermission.Duplicate" {
					log.Warnf("Traffic from the associated VPC's worker SG to the endpoint VPC's default SG is already authorized")
				} else {
					log.WithError(err).Fatal("Failed to authorize traffic from the associated VPC's worker SG to the endpoint VPC's default SG")
				}
			}

			log.Info("Authorizing traffic from the endpoint VPC's default SG to the associated VPC's worker SG")
			if _, err = awsutils.AuthorizeAllIngressFromSG(
				associatedVpcClients,
				aws.String(associatedVpcWorkerSG),
				aws.String(endpointVPCDefaultSG),
				aws.String(fmt.Sprintf("Access from default SG of endpoint VPC %s", o.endpointVpcId)),
			); err != nil {
				// Proceed if ingress already authorized, fail otherwise
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) && apiErr.ErrorCode() == "InvalidPermission.Duplicate" {
					log.Warnf("Traffic from the endpoint VPC's default SG to the associated VPC's worker SG is already authorized")
				} else {
					log.WithError(err).Fatal("Failed to authorize traffic from the endpoint VPC's default SG to the associated VPC's worker SG")
				}
			}

		// Associated VPC & endpoint VPC in different regions => allow ingress from CIDR of the peer
		default:
			log.Info("Authorizing traffic from the associated VPC's CIDR block to the endpoint VPC's default SG")
			if _, err = awsutils.AuthorizeAllIngressFromCIDR(
				o.endpointVpcClients,
				aws.String(endpointVPCDefaultSG),
				associatedVpcCIDR,
				aws.String(fmt.Sprintf("Access from CIDR block of associated VPC %s", associatedVpcId)),
			); err != nil {
				// Proceed if ingress already authorized, fail otherwise
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) && apiErr.ErrorCode() == "InvalidPermission.Duplicate" {
					log.Warnf("Traffic from the associated VPC's CIDR block to the endpoint VPC's default SG is already authorized")
				} else {
					log.WithError(err).Fatal("Failed to authorize traffic from the associated VPC's CIDR block to the endpoint VPC's default SG")
				}
			}

			log.Info("Authorizing traffic from the endpoint VPC's CIDR block to the associated VPC's worker SG")
			if _, err = awsutils.AuthorizeAllIngressFromCIDR(
				associatedVpcClients,
				aws.String(associatedVpcWorkerSG),
				endpointVpcCIDR,
				aws.String(fmt.Sprintf("Access from CIDR block of endpoint VPC %s", o.endpointVpcId)),
			); err != nil {
				// Proceed if ingress already authorized, fail otherwise
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) && apiErr.ErrorCode() == "InvalidPermission.Duplicate" {
					log.Warnf("Traffic from the endpoint VPC's CIDR block to the associated VPC's worker SG is already authorized")
				} else {
					log.WithError(err).Fatal("Failed to authorize traffic from the endpoint VPC's CIDR block to the associated VPC's worker SG")
				}
			}
		}
	}

	// Update HiveConfig
	o.addEndpointVpcToHiveConfig()

	return nil
}

func (o *endpointVPCAddOptions) addEndpointVpcToHiveConfig() {
	log.Infof("Adding endpoint VPC %v to HiveConfig", o.endpointVpcId)

	// Get AZ of each endpoint subnet
	var endpointSubnets []hivev1.AWSPrivateLinkSubnet
	if err := o.endpointVpcClients.DescribeSubnetsPages(
		&ec2.DescribeSubnetsInput{
			SubnetIds: o.endpointSubnetIds,
		},
		func(page *ec2.DescribeSubnetsOutput, lastPage bool) bool {
			for _, subnet := range page.Subnets {
				endpointSubnet := hivev1.AWSPrivateLinkSubnet{
					SubnetID:         aws.ToString(subnet.SubnetId),
					AvailabilityZone: aws.ToString(subnet.AvailabilityZone),
				}
				endpointSubnets = append(endpointSubnets, endpointSubnet)
			}
			return !lastPage
		},
	); err != nil {
		log.WithError(err).Fatal("Failed to describe endpoint subnets")
	}
	// Sort endpoint subnets by AZ first and then by subnet ID
	sort.Slice(endpointSubnets, func(i, j int) bool {
		return endpointSubnets[i].AvailabilityZone+endpointSubnets[i].SubnetID <
			endpointSubnets[j].AvailabilityZone+endpointSubnets[j].SubnetID
	})

	// Make sure the endpoint VPC is specified in HiveConfig.
	// Append/update HiveConfig.spec.awsPrivateLink.endpointVpcInventory depending on the situation.
	endpointVpcToAdd := hivev1.AWSPrivateLinkInventory{
		AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{
			VPCID:  o.endpointVpcId,
			Region: o.endpointVpcRegion,
		},
		Subnets: endpointSubnets,
	}
	if idx, ok := awsutils.FindVpcInInventory(o.endpointVpcId, o.hiveConfig.Spec.AWSPrivateLink.EndpointVPCInventory); ok {
		if reflect.DeepEqual(o.hiveConfig.Spec.AWSPrivateLink.EndpointVPCInventory[idx], endpointVpcToAdd) {
			log.Warn("Endpoint VPC found in HiveConfig. HiveConfig unchanged.")
			return
		}
		log.Warn("Endpoint VPC found in HiveConfig but needs update.")
		o.hiveConfig.Spec.AWSPrivateLink.EndpointVPCInventory[idx] = endpointVpcToAdd
	} else {
		o.hiveConfig.Spec.AWSPrivateLink.EndpointVPCInventory = append(
			o.hiveConfig.Spec.AWSPrivateLink.EndpointVPCInventory,
			endpointVpcToAdd,
		)
		log.Debugf("Endpoint VPC added to HiveConfig")
	}

	if err := common.DynamicClient.Update(context.Background(), &o.hiveConfig); err != nil {
		log.WithError(err).Fatal("Failed to update HiveConfig/hive")
	}
}

func addRouteToRouteTables(
	vpcClients awsclient.Client,
	vpcId, peerCIDR, VpcPeeringConnectionId *string,
	additionalFiltersForRouteTables ...*ec2types.Filter,
) error {
	filters := []ec2types.Filter{
		{
			Name:   aws.String("vpc-id"),
			Values: []string{aws.ToString(vpcId)},
		},
	}
	
	for _, filter := range additionalFiltersForRouteTables {
		filters = append(filters, *filter)
	}

	return vpcClients.DescribeRouteTablesPages(
		&ec2.DescribeRouteTablesInput{
			Filters: filters,
		},
		func(page *ec2.DescribeRouteTablesOutput, lastPage bool) bool {
			for _, routeTable := range page.RouteTables {
				_, err := vpcClients.CreateRoute(&ec2.CreateRouteInput{
					RouteTableId:           routeTable.RouteTableId,
					DestinationCidrBlock:   peerCIDR,
					VpcPeeringConnectionId: VpcPeeringConnectionId,
				})
				if err != nil {
					// Proceed if route already exists, fail otherwise
					var apiErr smithy.APIError
					if errors.As(err, &apiErr) && apiErr.ErrorCode() == "RouteAlreadyExists" {
						log.Warnf("Route already exists in route table %v", aws.ToString(routeTable.RouteTableId))
					} else {
						log.WithError(err).Fatalf("Failed to create route for route table %v", aws.ToString(routeTable.RouteTableId))
					}
				} else {
					log.Debugf("Route added to route table %v", aws.ToString(routeTable.RouteTableId))
				}
			}

			return !lastPage
		},
	)
}

// Does not error out even if the peering connection is already established between the two VPCs.
func setupVpcPeeringConnection(
	associatedVpcClients, endpointVpcClients awsclient.Client,
	associatedVpcId, endpointVpcId, endpointVpcRegion *string,
) (*ec2.AcceptVpcPeeringConnectionOutput, error) {
	log.Info("Setting up VPC peering connection between the associated VPC and the endpoint VPC")

	createVpcPeeringConnectionOutput, err := associatedVpcClients.CreateVpcPeeringConnection(&ec2.CreateVpcPeeringConnectionInput{
		VpcId:      associatedVpcId,
		PeerVpcId:  endpointVpcId,
		PeerRegion: endpointVpcRegion,
	})
	if err != nil {
		return nil, err
	}
	log.Debugf("VPC peering connection %v requested", aws.ToString(createVpcPeeringConnectionOutput.VpcPeeringConnection.VpcPeeringConnectionId))

	err = endpointVpcClients.WaitUntilVpcPeeringConnectionExists(&ec2.DescribeVpcPeeringConnectionsInput{
		VpcPeeringConnectionIds: []string{aws.ToString(createVpcPeeringConnectionOutput.VpcPeeringConnection.VpcPeeringConnectionId)},
	})
	if err != nil {
		return nil, err
	}
	log.Debugf("VPC peering connection %v exists", aws.ToString(createVpcPeeringConnectionOutput.VpcPeeringConnection.VpcPeeringConnectionId))

	acceptVpcPeeringConnectionOutput, err := endpointVpcClients.AcceptVpcPeeringConnection(&ec2.AcceptVpcPeeringConnectionInput{
		VpcPeeringConnectionId: createVpcPeeringConnectionOutput.VpcPeeringConnection.VpcPeeringConnectionId,
	})
	if err != nil {
		return nil, err
	}
	log.Debugf("VPC peering connection %v accepted", aws.ToString(acceptVpcPeeringConnectionOutput.VpcPeeringConnection.VpcPeeringConnectionId))

	return acceptVpcPeeringConnectionOutput, nil
}
