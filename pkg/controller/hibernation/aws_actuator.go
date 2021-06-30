package hibernation

import (
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"

	awsclient "github.com/openshift/hive/pkg/awsclient"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

var (
	runningStates           = sets.NewString("running")
	stoppedStates           = sets.NewString("stopped")
	pendingStates           = sets.NewString("pending")
	stoppingStates          = sets.NewString("stopping", "shutting-down")
	runningOrPendingStates  = runningStates.Union(pendingStates)
	stoppedOrStoppingStates = stoppedStates.Union(stoppingStates)
	notRunningStates        = stoppedOrStoppingStates.Union(pendingStates)
	notStoppedStates        = runningOrPendingStates.Union(stoppingStates)
)

func init() {
	RegisterActuator(&awsActuator{awsClientFn: getAWSClient})
}

type awsActuator struct {
	// awsClientFn is the function to build an AWS client, here for testing
	awsClientFn func(*hivev1.ClusterDeployment, client.Client, log.FieldLogger) (awsclient.Client, error)
}

// CanHandle returns true if the actuator can handle a particular ClusterDeployment
func (a *awsActuator) CanHandle(cd *hivev1.ClusterDeployment) bool {
	return cd.Spec.Platform.AWS != nil
}

// StopMachines will stop machines belonging to the given ClusterDeployment
func (a *awsActuator) StopMachines(cd *hivev1.ClusterDeployment, c client.Client, logger log.FieldLogger) error {
	logger = logger.WithField("cloud", "aws")
	awsClient, err := a.awsClientFn(cd, c, logger)
	if err != nil {
		return err
	}
	instanceIDs, err := getClusterInstanceIDs(cd, awsClient, runningOrPendingStates, logger)
	if err != nil {
		return err
	}
	if len(instanceIDs) == 0 {
		logger.Warning("No instances were found to stop")
		return nil
	}
	logger.WithField("instanceIDs", instanceIDs).Info("Stopping cluster instances")
	_, err = awsClient.StopInstances(&ec2.StopInstancesInput{
		InstanceIds: instanceIDs,
	})
	if err != nil {
		logger.WithError(err).Error("failed to stop instances")
	}
	return err
}

// StartMachines will select machines belonging to the given ClusterDeployment
func (a *awsActuator) StartMachines(cd *hivev1.ClusterDeployment, c client.Client, logger log.FieldLogger) error {
	logger = logger.WithField("cloud", "aws")
	awsClient, err := a.awsClientFn(cd, c, logger)
	if err != nil {
		return err
	}
	instanceIDs, err := getClusterInstanceIDs(cd, awsClient, stoppedOrStoppingStates, logger)
	if err != nil {
		return err
	}
	if len(instanceIDs) == 0 {
		logger.Info("No instances were found to start")
		return nil
	}
	logger.WithField("instanceIDs", instanceIDs).Info("Starting cluster instances")
	_, err = awsClient.StartInstances(&ec2.StartInstancesInput{
		InstanceIds: instanceIDs,
	})
	if err != nil {
		logger.WithError(err).Error("failed to start instances")
	}
	return err
}

// MachinesRunning will return true if the machines associated with the given
// ClusterDeployment are in a running state. It also returns a list of machines that
// are not running.
func (a *awsActuator) MachinesRunning(cd *hivev1.ClusterDeployment, c client.Client, logger log.FieldLogger) (bool, []string, error) {
	logger = logger.WithField("cloud", "aws")
	logger.Infof("checking whether machines are running")
	awsClient, err := a.awsClientFn(cd, c, logger)
	if err != nil {
		return false, nil, err
	}
	instanceIDs, err := getClusterInstanceIDs(cd, awsClient, notRunningStates, logger)
	if err != nil {
		return false, nil, err
	}
	return len(instanceIDs) == 0, aws.StringValueSlice(instanceIDs), nil
}

// MachinesStopped will return true if the machines associated with the given
// ClusterDeployment are in a stopped state. It also returns a list of machines
// that have not stopped.
func (a *awsActuator) MachinesStopped(cd *hivev1.ClusterDeployment, c client.Client, logger log.FieldLogger) (bool, []string, error) {
	logger = logger.WithField("cloud", "aws")
	logger.Infof("checking whether machines are stopped")
	awsClient, err := a.awsClientFn(cd, c, logger)
	if err != nil {
		return false, nil, err
	}
	instanceIDs, err := getClusterInstanceIDs(cd, awsClient, notStoppedStates, logger)
	if err != nil {
		return false, nil, err
	}
	return len(instanceIDs) == 0, aws.StringValueSlice(instanceIDs), nil
}

func getAWSClient(cd *hivev1.ClusterDeployment, c client.Client, logger log.FieldLogger) (awsclient.Client, error) {
	options := awsclient.Options{
		Region: cd.Spec.Platform.AWS.Region,
		CredentialsSource: awsclient.CredentialsSource{
			Secret: &awsclient.SecretCredentialsSource{
				Namespace: cd.Namespace,
				Ref:       &cd.Spec.Platform.AWS.CredentialsSecretRef,
			},
			AssumeRole: &awsclient.AssumeRoleCredentialsSource{
				SecretRef: corev1.SecretReference{
					Name:      os.Getenv(constants.HiveAWSServiceProviderCredentialsSecretRefEnvVar),
					Namespace: controllerutils.GetHiveNamespace(),
				},
				Role: cd.Spec.Platform.AWS.CredentialsAssumeRole,
			},
		},
	}

	return awsclient.New(c, options)
}

func getClusterInstanceIDs(cd *hivev1.ClusterDeployment, c awsclient.Client, states sets.String, logger log.FieldLogger) ([]*string, error) {
	infraID := cd.Spec.ClusterMetadata.InfraID
	logger = logger.WithField("infraID", infraID)
	logger.Debug("listing cluster instances")
	out, err := c.DescribeInstances(&ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String(fmt.Sprintf("tag:kubernetes.io/cluster/%s", infraID)),
				Values: []*string{aws.String("owned")},
			},
		},
	})
	if err != nil {
		logger.WithError(err).Error("failed to list instances")
		return nil, err
	}
	result := []*string{}
	for _, r := range out.Reservations {
		for _, i := range r.Instances {
			if states.Has(aws.StringValue(i.State.Name)) {
				result = append(result, i.InstanceId)
			}
		}
	}
	logger.WithField("count", len(result)).WithField("states", states).Debug("result of listing instances")
	return result, nil
}
