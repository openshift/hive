package remotemachineset

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	awsprovider "sigs.k8s.io/cluster-api-provider-aws/pkg/apis/awsproviderconfig/v1beta1"

	machineapi "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	installaws "github.com/openshift/installer/pkg/asset/machines/aws"
	installertypes "github.com/openshift/installer/pkg/types"
	installertypesaws "github.com/openshift/installer/pkg/types/aws"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/awsclient"
)

// AWSActuator encapsulates the pieces necessary to be able to generate
// a list of MachineSets to sync to the remote cluster.
type AWSActuator struct {
	client awsclient.Client
	logger log.FieldLogger
	region string
	amiID  string
}

var _ Actuator = &AWSActuator{}

// NewAWSActuator is the constructor for building a AWSActuator
func NewAWSActuator(awsCreds *corev1.Secret, region string, remoteMachineSets []machineapi.MachineSet, scheme *runtime.Scheme, logger log.FieldLogger) (*AWSActuator, error) {
	awsClient, err := awsclient.NewClientFromSecret(awsCreds, region)
	if err != nil {
		logger.WithError(err).Warn("failed to create AWS client")
		return nil, err
	}
	amiID, err := getAWSAMIID(remoteMachineSets, scheme, logger)
	if err != nil {
		logger.WithError(err).Warn("failed to get AMI ID")
		return nil, err
	}
	actuator := &AWSActuator{
		client: awsClient,
		logger: logger,
		region: region,
		amiID:  amiID,
	}
	return actuator, nil
}

// GenerateMachineSets satisfies the Actuator interface and will take a clusterDeployment and return a list of MachineSets
// to sync to the remote cluster.
func (a *AWSActuator) GenerateMachineSets(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool, logger log.FieldLogger) ([]*machineapi.MachineSet, error) {
	if cd.Spec.ClusterMetadata == nil {
		return nil, errors.New("ClusterDeployment does not have cluster metadata")
	}
	if cd.Spec.Platform.AWS == nil {
		return nil, errors.New("ClusterDeployment is not for AWS")
	}
	if pool.Spec.Platform.AWS == nil {
		return nil, errors.New("MachinePool is not for AWS")
	}

	ic := &installertypes.InstallConfig{
		Platform: installertypes.Platform{
			AWS: &installertypesaws.Platform{
				Region: cd.Spec.Platform.AWS.Region,
			},
		},
	}

	computePool := baseMachinePool(pool)
	computePool.Platform.AWS = &installertypesaws.MachinePool{
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
			return nil, errors.Wrap(err, "compute pool not providing list of zones and failed to fetch list of zones")
		}
		if len(zones) == 0 {
			return nil, fmt.Errorf("zero zones returned for region %s", cd.Spec.Platform.AWS.Region)
		}
		computePool.Platform.AWS.Zones = zones
	}

	installerMachineSets, err := installaws.MachineSets(cd.Spec.ClusterMetadata.InfraID, ic, computePool, a.amiID, pool.Spec.Name, "worker-user-data")
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate machinesets")
	}

	// Re-use existing AWS resources for generated MachineSets.
	for _, ms := range installerMachineSets {
		a.updateProviderConfig(ms, cd.Spec.ClusterMetadata.InfraID)
	}

	return installerMachineSets, nil
}

// Scan the pre-existing machinesets to find an AMI ID we can use if we need to create
// new machinesets.
// TODO: this will need work at some point in the future, ideally the AMI should come from
// release image someday, hopefully we can hold off until that is the case, and look it up when
// we extract installer image refs.
func getAWSAMIID(remoteMachineSets []machineapi.MachineSet, scheme *runtime.Scheme, logger log.FieldLogger) (string, error) {
	var amiID string
	for _, ms := range remoteMachineSets {
		awsProviderSpec, err := decodeAWSMachineProviderSpec(ms.Spec.Template.Spec.ProviderSpec.Value, scheme)
		if err != nil {
			logger.WithError(err).Warn("error decoding AWSMachineProviderConfig, skipping MachineSet for AMI check")
			continue
		}
		if awsProviderSpec.AMI.ID == nil {
			// Really weird, but keep looking...
			continue
		}
		amiID = *awsProviderSpec.AMI.ID
		logger.
			WithField("fromRemoteMachineSet", ms.Name).
			WithField("ami", amiID).
			Debug("resolved AMI to use for new machinesets")
		return amiID, nil
	}
	return "", errors.New("unable to locate AMI to use from pre-existing machine set")
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
	resp, err := a.client.DescribeAvailabilityZones(req)
	if err != nil {
		return nil, err
	}
	zones := []string{}
	for _, zone := range resp.AvailabilityZones {
		zones = append(zones, *zone.ZoneName)
	}
	return zones, nil
}

func decodeAWSMachineProviderSpec(rawExt *runtime.RawExtension, scheme *runtime.Scheme) (*awsprovider.AWSMachineProviderConfig, error) {
	codecFactory := serializer.NewCodecFactory(scheme)
	decoder := codecFactory.UniversalDecoder(awsprovider.SchemeGroupVersion)
	if rawExt == nil {
		return nil, fmt.Errorf("MachineSet has no ProviderSpec")
	}
	obj, gvk, err := decoder.Decode([]byte(rawExt.Raw), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("could not decode AWS ProviderConfig: %v", err)
	}
	spec, ok := obj.(*awsprovider.AWSMachineProviderConfig)
	if !ok {
		return nil, fmt.Errorf("Unexpected object: %#v", gvk)
	}
	return spec, nil
}

// updateProviderConfig modifies values in a MachineSet's AWSMachineProviderConfig.
// Currently we modify the AWSMachineProviderConfig IAMInstanceProfile, Subnet and SecurityGroups such that
// the values match the worker pool originally created by the installer.
func (a *AWSActuator) updateProviderConfig(machineSet *machineapi.MachineSet, infraID string) {
	providerConfig := machineSet.Spec.Template.Spec.ProviderSpec.Value.Object.(*awsprovider.AWSMachineProviderConfig)

	// TODO: assumptions about pre-existing objects by name here is quite dangerous, it's already
	// broken on us once via renames in the installer. We need to start querying for what exists
	// here.
	providerConfig.IAMInstanceProfile = &awsprovider.AWSResourceReference{ID: aws.String(fmt.Sprintf("%s-worker-profile", infraID))}
	providerConfig.Subnet = awsprovider.AWSResourceReference{
		Filters: []awsprovider.Filter{{
			Name:   "tag:Name",
			Values: []string{fmt.Sprintf("%s-private-%s", infraID, providerConfig.Placement.AvailabilityZone)},
		}},
	}
	providerConfig.SecurityGroups = []awsprovider.AWSResourceReference{{
		Filters: []awsprovider.Filter{{
			Name:   "tag:Name",
			Values: []string{fmt.Sprintf("%s-worker-sg", infraID)},
		}},
	}}
	machineSet.Spec.Template.Spec.ProviderSpec = machineapi.ProviderSpec{
		Value: &runtime.RawExtension{Object: providerConfig},
	}
}
