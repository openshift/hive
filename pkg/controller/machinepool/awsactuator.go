package machinepool

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	machineapi "github.com/openshift/api/machine/v1beta1"
	icaws "github.com/openshift/installer/pkg/asset/installconfig/aws"
	installaws "github.com/openshift/installer/pkg/asset/machines/aws"
	installertypesaws "github.com/openshift/installer/pkg/types/aws"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/awsclient"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

// AWSActuator encapsulates the pieces necessary to be able to generate
// a list of MachineSets to sync to the remote cluster.
type AWSActuator struct {
	client    client.Client
	awsClient awsclient.Client
	logger    log.FieldLogger
	region    string
	amiID     string
}

var (
	_ Actuator = &AWSActuator{}

	// noSuchSubnetRegex is a regex used to fetch condition message from error when subnets specified in the MachinePool are invalid
	noSuchSubnetRegex = regexp.MustCompile(`^InvalidSubnetID\.NotFound:\s+([^\t]+)\t`)

	versionsSupportingSpotInstances = semver.MustParseRange(">=4.5.0")
)

// NewAWSActuator is the constructor for building a AWSActuator
func NewAWSActuator(
	client client.Client,
	credentials awsclient.CredentialsSource,
	region string,
	pool *hivev1.MachinePool,
	masterMachine *machineapi.Machine,
	scheme *runtime.Scheme,
	logger log.FieldLogger,
) (*AWSActuator, error) {
	awsClient, err := awsclient.New(client, awsclient.Options{Region: region, CredentialsSource: credentials})
	if err != nil {
		logger.WithError(err).Warn("failed to create AWS client")
		return nil, err
	}
	amiID := pool.Annotations[hivev1.MachinePoolImageIDOverrideAnnotation]
	if amiID != "" {
		log.Infof("using AMI override from %s annotation: %s", hivev1.MachinePoolImageIDOverrideAnnotation, amiID)
	} else {
		refByTag, err := masterAMIRefByTag(masterMachine, scheme, logger)
		if err != nil {
			logger.WithError(err).Warn("unable to determine if master machine references AMI by tag")
			return nil, err
		}
		// We assume an AMI ID will be set within the master machine's AWSMachineProviderConfig when the master
		// machine doesn't reference an AMI by tag. When refByTag is true, amiID will remain an empty string and
		// will be provided to installer's machineset generation as an empty string.
		if !refByTag {
			amiID, err = getAWSAMIID(masterMachine, scheme, logger)
			if err != nil {
				logger.WithError(err).Warn("failed to get AMI ID")
				return nil, err
			}
		}
	}
	actuator := &AWSActuator{
		client:    client,
		awsClient: awsClient,
		logger:    logger,
		region:    region,
		amiID:     amiID,
	}
	return actuator, nil
}

// GenerateMachineSets satisfies the Actuator interface and will take a clusterDeployment and return a list of MachineSets
// to sync to the remote cluster.
func (a *AWSActuator) GenerateMachineSets(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool, logger log.FieldLogger) ([]*machineapi.MachineSet, bool, error) {
	if cd.Spec.ClusterMetadata == nil {
		return nil, false, errors.New("ClusterDeployment does not have cluster metadata")
	}
	if cd.Spec.Platform.AWS == nil {
		return nil, false, errors.New("ClusterDeployment is not for AWS")
	}
	if pool.Spec.Platform.AWS == nil {
		return nil, false, errors.New("MachinePool is not for AWS")
	}
	clusterVersion, err := getClusterVersion(cd)
	if err != nil {
		return nil, false, fmt.Errorf("unable to get cluster version: %v", err)
	}

	if isUsingUnsupportedSpotMarketOptions(pool, clusterVersion, logger) {
		logger.WithField("clusterVersion", clusterVersion).Debug("cluster does not support spot instances")
		conds, changed := controllerutils.SetMachinePoolConditionWithChangeCheck(
			pool.Status.Conditions,
			hivev1.UnsupportedConfigurationMachinePoolCondition,
			corev1.ConditionTrue,
			"UnsupportedSpotMarketOptions",
			"The version of the cluster does not support using spot instances",
			controllerutils.UpdateConditionIfReasonOrMessageChange,
		)
		if changed {
			pool.Status.Conditions = conds
			if err := a.client.Status().Update(context.Background(), pool); err != nil {
				return nil, false, errors.Wrap(err, "could not update MachinePool status")
			}
		}
		return nil, false, nil
	}
	statusChanged := false
	pool.Status.Conditions, statusChanged = controllerutils.SetMachinePoolConditionWithChangeCheck(
		pool.Status.Conditions,
		hivev1.UnsupportedConfigurationMachinePoolCondition,
		corev1.ConditionFalse,
		"ConfigurationSupported",
		"The configuration is supported",
		controllerutils.UpdateConditionIfReasonOrMessageChange,
	)

	computePool := baseMachinePool(pool)
	computePool.Platform.AWS = &installertypesaws.MachinePool{
		AMIID:        a.amiID,
		InstanceType: pool.Spec.Platform.AWS.InstanceType,
		EC2RootVolume: installertypesaws.EC2RootVolume{
			IOPS:      pool.Spec.Platform.AWS.EC2RootVolume.IOPS,
			Size:      pool.Spec.Platform.AWS.EC2RootVolume.Size,
			Type:      pool.Spec.Platform.AWS.EC2RootVolume.Type,
			KMSKeyARN: pool.Spec.Platform.AWS.EC2RootVolume.KMSKeyARN,
		},
		Zones: pool.Spec.Platform.AWS.Zones,
	}

	if pool.Spec.Platform.AWS.EC2Metadata != nil {
		computePool.Platform.AWS.EC2Metadata.Authentication = pool.Spec.Platform.AWS.EC2Metadata.Authentication
	}

	if len(computePool.Platform.AWS.Zones) == 0 {
		zones, err := a.fetchAvailabilityZones()
		if err != nil {
			return nil, false, errors.Wrap(err, "compute pool not providing list of zones and failed to fetch list of zones")
		}
		if len(zones) == 0 {
			return nil, false, fmt.Errorf("zero zones returned for region %s", cd.Spec.Platform.AWS.Region)
		}
		computePool.Platform.AWS.Zones = zones
	}

	// A map, keyed by availability zone, of subnets. By the time we pass this to MachineSets(), it
	// must either be empty or contain exactly one entry per AZ.
	subnets, err := a.getSubnetsByAvailabilityZone(pool)
	if err != nil {
		return nil, false, errors.Wrap(err, "processing subnets")
	}
	// userTags are settings available in the installconfig that we are choosing
	// to ignore for the timebeing. These empty settings should be updated to feed
	// from the machinepool / installconfig in the future.
	userTags := map[string]string{}

	installerMachineSets, err := installaws.MachineSets(
		cd.Spec.ClusterMetadata.InfraID,
		cd.Spec.Platform.AWS.Region,
		subnets,
		computePool,
		pool.Spec.Name,
		workerUserDataName,
		userTags,
	)
	if err != nil {
		if strings.Contains(err.Error(), "no subnet for zone") {
			conds, changed := controllerutils.SetMachinePoolConditionWithChangeCheck(
				pool.Status.Conditions,
				hivev1.InvalidSubnetsMachinePoolCondition,
				corev1.ConditionTrue,
				"NoSubnetForAvailabilityZone",
				err.Error(),
				controllerutils.UpdateConditionIfReasonOrMessageChange,
			)
			if statusChanged || changed {
				pool.Status.Conditions = conds
				if err := a.client.Status().Update(context.Background(), pool); err != nil {
					return nil, false, err
				}
			}
		}

		return nil, false, errors.Wrap(err, "failed to generate machinesets")
	}

	conds, changed := controllerutils.SetMachinePoolConditionWithChangeCheck(
		pool.Status.Conditions,
		hivev1.InvalidSubnetsMachinePoolCondition,
		corev1.ConditionFalse,
		"ValidSubnets",
		"Subnets are valid",
		controllerutils.UpdateConditionNever,
	)
	if statusChanged || changed {
		pool.Status.Conditions = conds
		if err := a.client.Status().Update(context.Background(), pool); err != nil {
			return nil, false, err
		}
	}

	var vpcID string
	if metav1.HasAnnotation(pool.ObjectMeta, constants.ExtraWorkerSecurityGroupAnnotation) {
		vpcID, err = GetVPCIDForMachinePool(a.awsClient, pool)
		if err != nil {
			return nil, false, err
		}
	}

	// Re-use existing AWS resources for generated MachineSets.
	for _, ms := range installerMachineSets {
		a.updateProviderConfig(ms, cd.Spec.ClusterMetadata.InfraID, pool, vpcID)
	}

	return installerMachineSets, true, nil
}

// Get the AMI ID from an existing master machine.
func getAWSAMIID(masterMachine *machineapi.Machine, scheme *runtime.Scheme, logger log.FieldLogger) (string, error) {
	providerSpec, err := decodeAWSMachineProviderSpec(masterMachine.Spec.ProviderSpec.Value, logger)
	if err != nil {
		logger.WithError(err).Warn("cannot decode AWSMachineProviderConfig from master machine")
		return "", errors.Wrap(err, "cannot decode AWSMachineProviderConfig from master machine")
	}
	if providerSpec.AMI.ID == nil {
		logger.Warn("master machine does not have AMI ID set")
		return "", errors.New("master machine does not have AMI ID set")
	}
	amiID := *providerSpec.AMI.ID
	logger.WithField("ami", amiID).Debug("resolved AMI to use for new machinesets")
	return amiID, nil
}

// Return true if the provided master machine references AMI by tag
func masterAMIRefByTag(masterMachine *machineapi.Machine, scheme *runtime.Scheme, logger log.FieldLogger) (bool, error) {
	providerSpec, err := decodeAWSMachineProviderSpec(masterMachine.Spec.ProviderSpec.Value, logger)
	if err != nil {
		return false, errors.Wrap(err, "cannot decode AWSMachineProviderConfig from master machine")
	}
	return len(providerSpec.AMI.Filters) > 0, nil
}

// fetchAvailabilityZones fetches availability zones for the AWS region
func (a *AWSActuator) fetchAvailabilityZones() ([]string, error) {
	zoneFilter := &ec2.Filter{
		Name:   aws.String("region-name"),
		Values: []*string{aws.String(a.region)},
	}
	req := &ec2.DescribeAvailabilityZonesInput{
		Filters: []*ec2.Filter{zoneFilter},
	}
	resp, err := a.awsClient.DescribeAvailabilityZones(req)
	if err != nil {
		return nil, err
	}
	zones := []string{}
	for _, zone := range resp.AvailabilityZones {
		zones = append(zones, *zone.ZoneName)
	}
	return zones, nil
}

func decodeAWSMachineProviderSpec(rawExtension *runtime.RawExtension, logger log.FieldLogger) (*machineapi.AWSMachineProviderConfig, error) {
	if rawExtension == nil {
		return &machineapi.AWSMachineProviderConfig{}, nil
	}

	spec := new(machineapi.AWSMachineProviderConfig)
	if err := json.Unmarshal(rawExtension.Raw, &spec); err != nil {
		return nil, fmt.Errorf("error unmarshalling providerSpec: %v", err)
	}

	return spec, nil
}

// updateProviderConfig modifies values in a MachineSet's AWSMachineProviderConfig.
// Currently we modify the AWSMachineProviderConfig IAMInstanceProfile, Subnet and SecurityGroups such that
// the values match the worker pool originally created by the installer.
func (a *AWSActuator) updateProviderConfig(machineSet *machineapi.MachineSet, infraID string, pool *hivev1.MachinePool, vpcID string) {
	providerConfig := machineSet.Spec.Template.Spec.ProviderSpec.Value.Object.(*machineapi.AWSMachineProviderConfig)

	// TODO: assumptions about pre-existing objects by name here is quite dangerous, it's already
	// broken on us once via renames in the installer. We need to start querying for what exists
	// here.
	providerConfig.IAMInstanceProfile = &machineapi.AWSResourceReference{ID: aws.String(fmt.Sprintf("%s-worker-profile", infraID))}
	// Update the subnet filter only if subnet id is absent
	if providerConfig.Subnet.ID == nil {
		providerConfig.Subnet = machineapi.AWSResourceReference{
			Filters: []machineapi.Filter{{
				Name:   "tag:Name",
				Values: []string{fmt.Sprintf("%s-private-%s", infraID, providerConfig.Placement.AvailabilityZone)},
			}},
		}
	}

	providerConfig.SecurityGroups = []machineapi.AWSResourceReference{{
		Filters: []machineapi.Filter{{
			Name:   "tag:Name",
			Values: []string{fmt.Sprintf("%s-worker-sg", infraID)},
		}},
	}}

	// Day 2: Hive MachinePools with an ExtraWorkerSecurityGroupAnnotation are configured with the additional
	// security group value specified in the annotation. For details, see HIVE-1802.
	//
	// NOTE: modifying the security group of an existing MachineSet will NOT result in updates to the
	// corresponding instances in AWS and will only be configured for newly created instances.
	if metav1.HasAnnotation(pool.ObjectMeta, constants.ExtraWorkerSecurityGroupAnnotation) && vpcID != "" {
		// Add the security group name obtained from the ExtraWorkerSecurityGroupAnnotation
		// annotation found on the MachinePool to the existing tag:Name filter in the worker
		// MachineSet manifest
		providerConfig.SecurityGroups[0].Filters[0].Values = append(providerConfig.SecurityGroups[0].Filters[0].Values, pool.Annotations[constants.ExtraWorkerSecurityGroupAnnotation])
		// Add a filter for the vpcID since the names of security groups may be the same within a
		// given AWS account. HIVE-1874
		providerConfig.SecurityGroups[0].Filters = append(providerConfig.SecurityGroups[0].Filters, machineapi.Filter{Name: "vpc-id", Values: []string{vpcID}})
	}

	if pool.Spec.Platform.AWS.SpotMarketOptions != nil {
		providerConfig.SpotMarketOptions = &machineapi.SpotMarketOptions{
			MaxPrice: pool.Spec.Platform.AWS.SpotMarketOptions.MaxPrice,
		}
	}

	machineSet.Spec.Template.Spec.ProviderSpec = machineapi.ProviderSpec{
		Value: &runtime.RawExtension{Object: providerConfig},
	}
}

// getSubnetsByAvailabilityZone maps availability zones to subnets. If the number of subnets in the pool
// spec matches the number of AZs, we will not check whether they are public or private. However, if there
// are two subnets per AZ, we will filter out the public ones, leaving only the private ones. If the pool
// specifies no subnets, an empty map is returned (this is a valid configuration, not an error).
func (a *AWSActuator) getSubnetsByAvailabilityZone(pool *hivev1.MachinePool) (icaws.Subnets, error) {
	// Preflight
	numZones := len(pool.Spec.Platform.AWS.Zones)
	numSubnets := len(pool.Spec.Platform.AWS.Subnets)
	switch numSubnets {
	case 0:
		// Zero subnets is legal.
		return icaws.Subnets{}, nil
	case numZones, 2 * numZones:
		// One per zone, or one public and one private per zone, is legal, and will be validated later.
		break
	default:
		// Any other number is bad.
		msg := fmt.Sprintf("Unexpected number of subnets (%d) versus availability zones (%d). "+
			"We expect zero, one per AZ, or one public and one private per AZ.", numSubnets, numZones)
		conds, changed := controllerutils.SetMachinePoolConditionWithChangeCheck(
			pool.Status.Conditions,
			hivev1.InvalidSubnetsMachinePoolCondition,
			corev1.ConditionTrue,
			"WrongNumberOfSubnets",
			msg,
			controllerutils.UpdateConditionIfReasonOrMessageChange,
		)
		if changed {
			pool.Status.Conditions = conds
			if err := a.client.Status().Update(context.Background(), pool); err != nil {
				return nil, err
			}
		}
		return nil, errors.New(msg)
	}

	idPointers := make([]*string, len(pool.Spec.Platform.AWS.Subnets))
	for i, id := range pool.Spec.Platform.AWS.Subnets {
		idPointers[i] = aws.String(id)
	}

	results, err := a.awsClient.DescribeSubnets(&ec2.DescribeSubnetsInput{SubnetIds: idPointers})
	if err != nil || len(results.Subnets) == 0 {
		if strings.Contains(err.Error(), "InvalidSubnet") {
			conditionMessage := err.Error()
			if submatches := noSuchSubnetRegex.FindStringSubmatch(err.Error()); submatches != nil {
				// formatting error message before adding it to condition when
				// sample error message: InvalidSubnetID.NotFound: The subnet ID 'subnet-1,subnet-2' does not exist\tstatus code: 400, request id: ea8b3bb7-de56-405f-9345-e5690a3ea8b2
				// message after formatting: The subnet ID 'subnet-1,subnet-2' does not exist
				conditionMessage = submatches[1]
			}
			conds, changed := controllerutils.SetMachinePoolConditionWithChangeCheck(
				pool.Status.Conditions,
				hivev1.InvalidSubnetsMachinePoolCondition,
				corev1.ConditionTrue,
				"SubnetsNotFound",
				conditionMessage,
				controllerutils.UpdateConditionIfReasonOrMessageChange,
			)
			if changed {
				pool.Status.Conditions = conds
				if err := a.client.Status().Update(context.Background(), pool); err != nil {
					return nil, err
				}
			}
		}
		return nil, err
	}

	var subnets []*ec2.Subnet
	if numSubnets == numZones {
		subnets = results.Subnets
	} else {
		subnets, err = a.filterPublicSubnets(results.Subnets, pool)
		if err != nil {
			return nil, err
		}
	}
	return a.validateSubnets(subnets, pool)
}

func (a *AWSActuator) filterPublicSubnets(subnets []*ec2.Subnet, pool *hivev1.MachinePool) ([]*ec2.Subnet, error) {

	vpc := *subnets[0].VpcId
	if vpc == "" {
		return nil, errors.Errorf("%s has no VPC", *subnets[0].SubnetId)
	}

	routeTables, err := a.awsClient.DescribeRouteTables(&ec2.DescribeRouteTablesInput{
		Filters: []*ec2.Filter{{
			Name:   aws.String("vpc-id"),
			Values: []*string{aws.String(vpc)},
		}},
	})
	if err != nil {
		return nil, errors.Wrap(err, "error describing route tables")
	}

	var privateSubnets, publicSubnets = []*ec2.Subnet{}, []*ec2.Subnet{}
	for _, subnet := range subnets {
		isPublic, err := isSubnetPublic(routeTables.RouteTables, subnet, a.logger)
		if err != nil {
			return nil, errors.Wrap(err, "error describing route tables")
		}
		if isPublic {
			publicSubnets = append(publicSubnets, subnet)
		} else {
			privateSubnets = append(privateSubnets, subnet)
		}
	}

	if len(publicSubnets) > 0 {
		// This enforces that we got one public subnet per AZ, even though we're not going to use them.
		_, err := a.validateSubnets(publicSubnets, pool)
		if err != nil {
			return nil, err
		}
	}

	return privateSubnets, nil
}

func isUsingUnsupportedSpotMarketOptions(pool *hivev1.MachinePool, clusterVersion string, logger log.FieldLogger) bool {
	if pool.Spec.Platform.AWS.SpotMarketOptions == nil {
		return false
	}
	parsedVersion, err := semver.ParseTolerant(clusterVersion)
	if err != nil {
		logger.WithError(err).WithField("clusterVersion", clusterVersion).Warn("could not parse the cluster version")
		return true
	}
	// Use only major, minor, and patch so that pre-release versions of 4.5.0 are within the >=4.5.0 range.
	parsedVersion = semver.Version{
		Major: parsedVersion.Major,
		Minor: parsedVersion.Minor,
		Patch: parsedVersion.Patch,
	}
	return !versionsSupportingSpotInstances(parsedVersion)
}

// tagNameSubnetPublicELB is the tag name used on a subnet to designate that
// it should be used for internet ELBs
const tagNameSubnetPublicELB = "kubernetes.io/role/elb"

// https://github.com/kubernetes/kubernetes/blob/9f036cd43d35a9c41d7ac4ca82398a6d0bef957b/staging/src/k8s.io/legacy-cloud-providers/aws/aws.go#L3376-L3419
func isSubnetPublic(rt []*ec2.RouteTable, subnet *ec2.Subnet, logger log.FieldLogger) (bool, error) {
	subnetID := aws.StringValue(subnet.SubnetId)
	var subnetTable *ec2.RouteTable
	for _, table := range rt {
		for _, assoc := range table.Associations {
			if aws.StringValue(assoc.SubnetId) == subnetID {
				subnetTable = table
				break
			}
		}
	}

	if subnetTable == nil {
		// If there is no explicit association, the subnet will be implicitly
		// associated with the VPC's main routing table.
		for _, table := range rt {
			for _, assoc := range table.Associations {
				if aws.BoolValue(assoc.Main) {
					logger.Debugf("Assuming implicit use of main routing table %s for %s",
						aws.StringValue(table.RouteTableId), subnetID)
					subnetTable = table
					break
				}
			}
		}
	}

	if subnetTable == nil {
		return false, fmt.Errorf("could not locate routing table for %s", subnetID)
	}

	for _, route := range subnetTable.Routes {
		// There is no direct way in the AWS API to determine if a subnet is public or private.
		// A public subnet is one which has an internet gateway route
		// we look for the gatewayId and make sure it has the prefix of igw to differentiate
		// from the default in-subnet route which is called "local"
		// or other virtual gateway (starting with vgv)
		// or vpc peering connections (starting with pcx).
		if strings.HasPrefix(aws.StringValue(route.GatewayId), "igw") {
			return true, nil
		}
	}

	// If we couldn't use the subnet table to figure out whether the subnet is public,
	// we let the users define whether this subnet should be used for internet-facing things
	// by looking for tagNameSubnetPublicELB tag.
	tagVal, subnetHasTag := findTag(subnet.Tags, tagNameSubnetPublicELB)
	if subnetHasTag && (tagVal == "" || tagVal == "1") {
		return true, nil
	}

	return false, nil
}

// Finds the value for a given tag.
func findTag(tags []*ec2.Tag, key string) (string, bool) {
	for _, tag := range tags {
		if aws.StringValue(tag.Key) == key {
			return aws.StringValue(tag.Value), true
		}
	}
	return "", false
}

func stringDereference(s *string) string {

	if s != nil {
		return *s
	}
	return ""
}

// validateSubnets ensures there's exactly one subnet per availability zone, and returns
// the mapping of subnets by availability zone
func (a *AWSActuator) validateSubnets(subnets []*ec2.Subnet, pool *hivev1.MachinePool) (icaws.Subnets, error) {
	conflictingSubnets := sets.NewString()
	subnetsByAvailabilityZone := make(icaws.Subnets, len(subnets))
	for _, subnet := range subnets {
		if oldSubnet, ok := subnetsByAvailabilityZone[*subnet.AvailabilityZone]; ok {
			conflictingSubnets.Insert(*subnet.SubnetId)
			conflictingSubnets.Insert(oldSubnet.ID)
			continue
		}
		az := stringDereference(subnet.AvailabilityZone)
		subnetsByAvailabilityZone[az] = icaws.Subnet{
			ID:   stringDereference(subnet.SubnetId),
			ARN:  stringDereference(subnet.SubnetArn),
			Zone: az,
			CIDR: stringDereference(subnet.CidrBlock),
			// TODO: populate local zone fields, Public, ZoneType, ZoneGroupName
		}
	}

	var msg, reason string
	switch {
	case len(conflictingSubnets) > 0:
		msg = fmt.Sprintf("more than one subnet found for some availability zones, conflicting subnets: %s",
			strings.Join(conflictingSubnets.List(), ", "))
		reason = "MoreThanOneSubnetForZone"
	case len(subnetsByAvailabilityZone) < len(pool.Spec.Platform.AWS.Zones):
		msg = fmt.Sprintf("not enough subnets (%d) found for the number of availability zones (%d)",
			len(subnetsByAvailabilityZone), len(pool.Spec.Platform.AWS.Zones))
		reason = "NotEnoughSubnetsForZones"
	default:
		// All good, we're done
		return subnetsByAvailabilityZone, nil
	}

	// If we get here, we've detected an error.
	conds, changed := controllerutils.SetMachinePoolConditionWithChangeCheck(
		pool.Status.Conditions,
		hivev1.InvalidSubnetsMachinePoolCondition,
		corev1.ConditionTrue,
		reason,
		msg,
		controllerutils.UpdateConditionIfReasonOrMessageChange,
	)
	if changed {
		pool.Status.Conditions = conds
		if err := a.client.Status().Update(context.Background(), pool); err != nil {
			return nil, err
		}
	}
	return nil, errors.New(msg)
}

// GetVPCIDForMachinePool retrieves the VPC ID of the first subnet configured in pool.Spec.Platform.AWS.Subnets
// using the provided AWS Client.
func GetVPCIDForMachinePool(awsClient awsclient.Client, pool *hivev1.MachinePool) (string, error) {
	if len(pool.Spec.Platform.AWS.Subnets) == 0 {
		return "", errors.New("MachinePool platform contains no subnets, cannot retrieve VPC ID")
	}
	subnetID := pool.Spec.Platform.AWS.Subnets[0]
	subnetsOutput, err := awsClient.DescribeSubnets(&ec2.DescribeSubnetsInput{
		SubnetIds: []*string{
			aws.String(subnetID),
		},
	})
	if err != nil {
		return "", err
	}
	// An error should be produced if no subnets are returned. This is here as a safety net.
	if len(subnetsOutput.Subnets) == 0 {
		return "", errors.Errorf("DescribeSubnets unexpectedly returned no results for subnet ID %s", subnetID)
	}
	vpcID := *subnetsOutput.Subnets[0].VpcId
	if vpcID == "" {
		return "", errors.Errorf("DescribeSubnets unexpectedly returned subnet %s without a VPC ID", subnetID)
	}
	return vpcID, nil
}
