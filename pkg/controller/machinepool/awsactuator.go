package machinepool

import (
	"context"
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
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/sets"
	awsprovider "sigs.k8s.io/cluster-api-provider-aws/pkg/apis"
	awsproviderv1beta1 "sigs.k8s.io/cluster-api-provider-aws/pkg/apis/awsprovider/v1beta1"
	capiv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/controller-runtime/pkg/client"

	installaws "github.com/openshift/installer/pkg/asset/machines/aws"
	installertypesaws "github.com/openshift/installer/pkg/types/aws"
	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/awsclient"
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

	// reg is a regex used to fetch condition message from error when subnets specified in the MachinePool are invalid
	reg = regexp.MustCompile(`^InvalidSubnetID\.NotFound:\s+([^\t]+)\t`)

	versionsSupportingSpotInstances = semver.MustParseRange(">=4.5.0")
)

func addAWSProviderToScheme(scheme *runtime.Scheme) error {
	return awsprovider.AddToScheme(scheme)
}

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
		amiID, err = getAWSAMIID(masterMachine, scheme, logger)
		if err != nil {
			logger.WithError(err).Warn("failed to get AMI ID")
			return nil, err
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

// GenerateCAPIMachineSets takes a clusterDeployment and returns CAPI MachineSets
func (a *AWSActuator) GenerateCAPIMachineSets(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool, logger log.FieldLogger) ([]*capiv1.MachineSet, bool, error) {
	computePool := baseMachinePool(pool)
	computePool.Platform.AWS = &installertypesaws.MachinePool{
		AMIID:        a.amiID,
		InstanceType: pool.Spec.Platform.AWS.InstanceType,
		EC2RootVolume: installertypesaws.EC2RootVolume{
			IOPS: pool.Spec.Platform.AWS.EC2RootVolume.IOPS,
			Size: pool.Spec.Platform.AWS.EC2RootVolume.Size,
			Type: pool.Spec.Platform.AWS.EC2RootVolume.Type,
		},
		Zones: pool.Spec.Platform.AWS.Zones,
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

	subnets := map[string]string{}
	// Fetching private subnets from the machinepool and then mapping availability zones to subnets
	if len(pool.Spec.Platform.AWS.Subnets) > 0 {
		subnetsByAvailabilityZone, err := a.getPrivateSubnetsByAvailabilityZone(pool)
		if err != nil {
			return nil, false, errors.Wrap(err, "describing subnets")
		}
		subnets = subnetsByAvailabilityZone
	}

	total := int64(0)
	if pool.Spec.Replicas != nil {
		total = *pool.Spec.Replicas
	}
	numOfAZs := int64(len(computePool.Platform.AWS.Zones))

	dataSecretName := fmt.Sprintf("%s-user-data", cd.Name)
	machineSets := make([]*capiv1.MachineSet, len(computePool.Platform.AWS.Zones))
	for i, zone := range computePool.Platform.AWS.Zones {
		_, ok := subnets[zone]
		if len(subnets) > 0 && !ok {
			return machineSets, false, errors.Errorf("no subnet for zone %s", zone)
		}

		replicas := int32(total / numOfAZs)
		if int64(i) < total%numOfAZs {
			replicas++
		}

		name := fmt.Sprintf("%s-%s-%s", cd.Spec.ClusterMetadata.InfraID, computePool.Name, zone)
		machineSet := &capiv1.MachineSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   cd.Spec.MachineManagement.TargetNamespace,
				Annotations: map[string]string{},
				Labels:      map[string]string{},
			},
			TypeMeta: metav1.TypeMeta{},
			Spec: capiv1.MachineSetSpec{
				ClusterName: cd.Name,
				Replicas:    &replicas,
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						name: name,
					},
				},
				Template: capiv1.MachineTemplateSpec{
					ObjectMeta: capiv1.ObjectMeta{
						Labels: map[string]string{
							name:                    name,
							capiv1.ClusterLabelName: cd.Spec.ClusterMetadata.InfraID,
						},
					},
					Spec: capiv1.MachineSpec{
						Bootstrap: capiv1.Bootstrap{
							DataSecretName: &dataSecretName,
						},
						ClusterName: cd.Name,
						InfrastructureRef: corev1.ObjectReference{
							Namespace:  cd.Spec.MachineManagement.TargetNamespace,
							Name:       name,
							APIVersion: "infrastructure.cluster.x-k8s.io/v1alpha3",
							Kind:       "AWSMachineTemplate",
						},
					},
				},
			},
		}
		machineSets[i] = machineSet
	}
	logger.Infof("generated %v machinesets", len(machineSets))
	return machineSets, true, nil
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
		return nil, false, fmt.Errorf("Unable to get cluster version: %v", err)
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

	subnets := map[string]string{}
	// Fetching private subnets from the machinepool and then mapping availability zones to subnets
	if len(pool.Spec.Platform.AWS.Subnets) > 0 {
		subnetsByAvailabilityZone, err := a.getPrivateSubnetsByAvailabilityZone(pool)
		if err != nil {
			return nil, false, errors.Wrap(err, "describing subnets")
		}
		subnets = subnetsByAvailabilityZone
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

	// Re-use existing AWS resources for generated MachineSets.
	for _, ms := range installerMachineSets {
		a.updateProviderConfig(ms, cd.Spec.ClusterMetadata.InfraID, pool)
	}

	return installerMachineSets, true, nil
}

// Get the AMI ID from an existing master machine.
func getAWSAMIID(masterMachine *machineapi.Machine, scheme *runtime.Scheme, logger log.FieldLogger) (string, error) {
	providerSpec, err := decodeAWSMachineProviderSpec(masterMachine.Spec.ProviderSpec.Value, scheme)
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

func decodeAWSMachineProviderSpec(rawExt *runtime.RawExtension, scheme *runtime.Scheme) (*awsproviderv1beta1.AWSMachineProviderConfig, error) {
	codecFactory := serializer.NewCodecFactory(scheme)
	decoder := codecFactory.UniversalDecoder(awsproviderv1beta1.SchemeGroupVersion)
	if rawExt == nil {
		return nil, fmt.Errorf("MachineSet has no ProviderSpec")
	}
	obj, gvk, err := decoder.Decode([]byte(rawExt.Raw), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("could not decode AWS ProviderConfig: %v", err)
	}
	spec, ok := obj.(*awsproviderv1beta1.AWSMachineProviderConfig)
	if !ok {
		return nil, fmt.Errorf("Unexpected object: %#v", gvk)
	}
	return spec, nil
}

// updateProviderConfig modifies values in a MachineSet's AWSMachineProviderConfig.
// Currently we modify the AWSMachineProviderConfig IAMInstanceProfile, Subnet and SecurityGroups such that
// the values match the worker pool originally created by the installer.
func (a *AWSActuator) updateProviderConfig(machineSet *machineapi.MachineSet, infraID string, pool *hivev1.MachinePool) {
	providerConfig := machineSet.Spec.Template.Spec.ProviderSpec.Value.Object.(*awsproviderv1beta1.AWSMachineProviderConfig)

	// TODO: assumptions about pre-existing objects by name here is quite dangerous, it's already
	// broken on us once via renames in the installer. We need to start querying for what exists
	// here.
	providerConfig.IAMInstanceProfile = &awsproviderv1beta1.AWSResourceReference{ID: aws.String(fmt.Sprintf("%s-worker-profile", infraID))}
	// Update the subnet filter only if subnet id is absent
	if providerConfig.Subnet.ID == nil {
		providerConfig.Subnet = awsproviderv1beta1.AWSResourceReference{
			Filters: []awsproviderv1beta1.Filter{{
				Name:   "tag:Name",
				Values: []string{fmt.Sprintf("%s-private-%s", infraID, providerConfig.Placement.AvailabilityZone)},
			}},
		}
	}

	providerConfig.SecurityGroups = []awsproviderv1beta1.AWSResourceReference{{
		Filters: []awsproviderv1beta1.Filter{{
			Name:   "tag:Name",
			Values: []string{fmt.Sprintf("%s-worker-sg", infraID)},
		}},
	}}
	if pool.Spec.Platform.AWS.SpotMarketOptions != nil {
		providerConfig.SpotMarketOptions = &awsproviderv1beta1.SpotMarketOptions{
			MaxPrice: pool.Spec.Platform.AWS.SpotMarketOptions.MaxPrice,
		}
	}

	machineSet.Spec.Template.Spec.ProviderSpec = machineapi.ProviderSpec{
		Value: &runtime.RawExtension{Object: providerConfig},
	}

}

// getPrivateSubnetsByAvailabilityZones maps availability zones to private subnet
func (a *AWSActuator) getPrivateSubnetsByAvailabilityZone(pool *hivev1.MachinePool) (map[string]string, error) {
	idPointers := make([]*string, len(pool.Spec.Platform.AWS.Subnets))
	for i, id := range pool.Spec.Platform.AWS.Subnets {
		idPointers[i] = aws.String(id)
	}

	results, err := a.awsClient.DescribeSubnets(&ec2.DescribeSubnetsInput{SubnetIds: idPointers})
	if err != nil || len(results.Subnets) == 0 {
		if strings.Contains(err.Error(), "InvalidSubnet") {
			conditionMessage := err.Error()
			if submatches := reg.FindStringSubmatch(err.Error()); submatches != nil {
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

	vpc := *results.Subnets[0].VpcId
	if vpc == "" {
		return nil, errors.Errorf("%s has no VPC", *results.Subnets[0].SubnetId)
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

	var privateSubnets, publicSubnets = map[string]ec2.Subnet{}, map[string]ec2.Subnet{}
	for _, subnet := range results.Subnets {
		isPublic, err := isSubnetPublic(routeTables.RouteTables, *subnet.SubnetId, a.logger)
		if err != nil {
			return nil, errors.Wrap(err, "error describing route tables")
		}
		if isPublic {
			publicSubnets[*subnet.SubnetId] = *subnet
		} else {
			privateSubnets[*subnet.SubnetId] = *subnet
		}
	}

	if len(publicSubnets) > 0 {
		_, err := a.validateSubnets(publicSubnets, pool)
		if err != nil {
			return nil, err
		}
	}

	subnetsByAvailabilityZone, err := a.validateSubnets(privateSubnets, pool)
	if err != nil {
		return nil, err
	}

	if len(publicSubnets) > 0 && len(publicSubnets) < len(privateSubnets) {
		conds, changed := controllerutils.SetMachinePoolConditionWithChangeCheck(
			pool.Status.Conditions,
			hivev1.InvalidSubnetsMachinePoolCondition,
			corev1.ConditionTrue,
			"InsufficientPublicSubnets",
			fmt.Sprintf("Public subnet does not exist for each zone with a private subnet"),
			controllerutils.UpdateConditionIfReasonOrMessageChange,
		)
		if changed {
			pool.Status.Conditions = conds
			if err := a.client.Status().Update(context.Background(), pool); err != nil {
				return nil, err
			}
			return nil, errors.Errorf("insufficient public subnets for availability zones and private subnets")
		}
	}

	return subnetsByAvailabilityZone, nil
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

// https://github.com/kubernetes/kubernetes/blob/9f036cd43d35a9c41d7ac4ca82398a6d0bef957b/staging/src/k8s.io/legacy-cloud-providers/aws/aws.go#L3376-L3419
func isSubnetPublic(rt []*ec2.RouteTable, subnetID string, logger log.FieldLogger) (bool, error) {
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

	return false, nil
}

// validateSubnets ensures there's only one public or private subnet per availability zone, and returns
// the mapping of subnets by availability zone
func (a *AWSActuator) validateSubnets(subnets map[string]ec2.Subnet, pool *hivev1.MachinePool) (map[string]string, error) {
	conflictingSubnets := sets.NewString()
	subnetsByAvailabilityZone := make(map[string]string, len(subnets))
	for _, subnet := range subnets {
		if subnetID, ok := subnetsByAvailabilityZone[*subnet.AvailabilityZone]; ok {
			conflictingSubnets.Insert(*subnet.SubnetId)
			conflictingSubnets.Insert(subnetID)
			continue
		}
		subnetsByAvailabilityZone[*subnet.AvailabilityZone] = *subnet.SubnetId
	}

	if len(conflictingSubnets) > 0 {
		conds, changed := controllerutils.SetMachinePoolConditionWithChangeCheck(
			pool.Status.Conditions,
			hivev1.InvalidSubnetsMachinePoolCondition,
			corev1.ConditionTrue,
			"MoreThanOneSubnetForZone",
			fmt.Sprintf("more than one subnet found for some availability zones, conflicting subnets: %s", strings.Join(conflictingSubnets.List(), ", ")),
			controllerutils.UpdateConditionIfReasonOrMessageChange,
		)
		if changed {
			pool.Status.Conditions = conds
			if err := a.client.Status().Update(context.Background(), pool); err != nil {
				return nil, err
			}
		}

		return nil, errors.Errorf("more than one subnet found for some availability zones, conflicting subnets: %s", strings.Join(conflictingSubnets.List(), ", "))
	}
	return subnetsByAvailabilityZone, nil
}
