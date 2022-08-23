package hibernation

import (
	"context"
	"fmt"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/ecs"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/alibabaclient"
)

var (
	// States described in Alibaba Cloud API docs
	// https://www.alibabacloud.com/help/en/elastic-compute-service/latest/instance-lifecycle
	alibabaRunningStates           = sets.NewString("Running")
	alibabaStoppedStates           = sets.NewString("Stopped")
	alibabaPendingStates           = sets.NewString("Pending")
	alibabaStoppingStates          = sets.NewString("Stopping")
	alibabaRunningOrPendingStates  = alibabaRunningStates.Union(alibabaPendingStates)
	alibabaStoppedOrStoppingStates = alibabaStoppedStates.Union(alibabaStoppingStates)
	alibabaNotRunningStates        = alibabaStoppedOrStoppingStates.Union(alibabaPendingStates)
	alibabaNotStoppedStates        = alibabaRunningOrPendingStates.Union(alibabaStoppingStates)
)

func init() {
	RegisterActuator(&alibabaCloudActuator{alibabaCloudClientFn: getAlibabaCloudClient})
}

type alibabaCloudActuator struct {
	// alibabaCloudClientFn is the function to build an Alibaba Cloud client, here for testing
	alibabaCloudClientFn func(*hivev1.ClusterDeployment, client.Client, log.FieldLogger) (alibabaclient.API, error)
}

// CanHandle returns true if the actuator can handle a particular ClusterDeployment
func (a *alibabaCloudActuator) CanHandle(cd *hivev1.ClusterDeployment) bool {
	return cd.Spec.Platform.AlibabaCloud != nil
}

// StopMachines will stop machines belonging to the given ClusterDeployment
func (a *alibabaCloudActuator) StopMachines(cd *hivev1.ClusterDeployment, dpClient, cpClient client.Client, logger log.FieldLogger) error {
	logger = logger.WithField("cloud", "alibabacloud")
	alibabaCloudClient, err := a.alibabaCloudClientFn(cd, dpClient, logger)
	if err != nil {
		return err
	}

	instances, err := getAlibabaCloudClusterInstances(cd, alibabaCloudClient, alibabaRunningOrPendingStates, logger)
	if err != nil {
		return err
	}
	if len(instances) == 0 {
		logger.Info("No instances were found to stop")
		return nil
	}

	instanceIDs := getAlibabaInstanceIDs(instances)
	stopInstancesRequest := ecs.CreateStopInstancesRequest()
	stopInstancesRequest.InstanceId = &instanceIDs

	// We stop ECS instances in economical mode so that they are charged at lesser rate
	// when stopped. In this stopping mode, the computing resources (vCPUs and memory)
	// and public IP address of the instance are automatically released.
	stopInstancesRequest.StoppedMode = "StopCharging"
	_, err = alibabaCloudClient.StopInstances(stopInstancesRequest)
	if err != nil {
		logger.WithError(err).Error("failed to stop Alibaba Cloud instances")
		return err
	}
	return nil
}

// StartMachines will start machines belonging to the given ClusterDeployment
func (a *alibabaCloudActuator) StartMachines(cd *hivev1.ClusterDeployment, dpClient, cpClient client.Client, logger log.FieldLogger) error {
	logger = logger.WithField("cloud", "alibabacloud")
	alibabaCloudClient, err := a.alibabaCloudClientFn(cd, dpClient, logger)
	if err != nil {
		return err
	}

	instances, err := getAlibabaCloudClusterInstances(cd, alibabaCloudClient, alibabaStoppedOrStoppingStates, logger)
	if err != nil {
		return err
	}
	if len(instances) == 0 {
		logger.Info("No instances were found to stop")
		return nil
	}

	instanceIDs := getAlibabaInstanceIDs(instances)
	startInstancesRequest := ecs.CreateStartInstancesRequest()
	startInstancesRequest.InstanceId = &instanceIDs
	_, err = alibabaCloudClient.StartInstances(startInstancesRequest)
	if err != nil {
		logger.WithError(err).Error("failed to start Alibaba Cloud instances")
		return err
	}
	return nil
}

// getAlibabaInstanceIDs fetches IDs of the ECS Instances
func getAlibabaInstanceIDs(instances []ecs.Instance) []string {
	result := make([]string, len(instances))
	for idx, i := range instances {
		result[idx] = i.InstanceId
	}
	return result
}

// MachinesRunning will return true if the machines associated with the given
// ClusterDeployment are in a running state. It also returns a list of machines that
// are not running.
func (a *alibabaCloudActuator) MachinesRunning(cd *hivev1.ClusterDeployment, dpClient, cpClient client.Client, logger log.FieldLogger) (bool, []string, error) {
	logger = logger.WithField("cloud", "alibabacloud")
	logger.Infof("checking whether machines are running")
	alibabaCloudClient, err := a.alibabaCloudClientFn(cd, dpClient, logger)
	if err != nil {
		return false, nil, err
	}
	instances, err := getAlibabaCloudClusterInstances(cd, alibabaCloudClient, alibabaNotRunningStates, logger)
	if err != nil {
		return false, nil, err
	}
	return len(instances) == 0, getAlibabaInstanceIDs(instances), nil
}

// MachinesStopped will return true if the machines associated with the given
// ClusterDeployment are in a stopped state. It also returns a list of machines
// that have not stopped.
func (a *alibabaCloudActuator) MachinesStopped(cd *hivev1.ClusterDeployment, dpClient, cpClient client.Client, logger log.FieldLogger) (bool, []string, error) {
	logger = logger.WithField("cloud", "alibabacloud")
	logger.Infof("checking whether machines are stopped")
	alibabaCloudClient, err := a.alibabaCloudClientFn(cd, dpClient, logger)
	if err != nil {
		return false, nil, err
	}
	instances, err := getAlibabaCloudClusterInstances(cd, alibabaCloudClient, alibabaNotStoppedStates, logger)
	if err != nil {
		return false, nil, err
	}
	return len(instances) == 0, getAlibabaInstanceIDs(instances), nil
}

// getAlibabaCloudClient builds Alibaba Cloud client from given ClusterDeployment
// The client is getting the secret from the data plane.
func getAlibabaCloudClient(cd *hivev1.ClusterDeployment, c client.Client, logger log.FieldLogger) (alibabaclient.API, error) {
	secret := &corev1.Secret{}
	err := c.Get(context.TODO(), client.ObjectKey{Name: cd.Spec.Platform.AlibabaCloud.CredentialsSecretRef.Name, Namespace: cd.Namespace}, secret)
	if err != nil {
		logger.WithError(err).Error("failed to fetch Alibaba Cloud credentials secret")
		return nil, errors.Wrap(err, "failed to fetch Alibaba Cloud credentials secret")
	}

	return alibabaclient.NewClientFromSecret(secret, cd.Spec.Platform.AlibabaCloud.Region)
}

// getAlibabaCloudClusterInstances fetches ECS instances for given ClusterDeployment filtered by given cluster states
func getAlibabaCloudClusterInstances(cd *hivev1.ClusterDeployment, c alibabaclient.API, states sets.String, logger log.FieldLogger) ([]ecs.Instance, error) {
	infraID := cd.Spec.ClusterMetadata.InfraID
	logger = logger.WithField("infraID", infraID)
	logger.Debug("listing cluster instances")

	describeInstancesRequest := ecs.CreateDescribeInstancesRequest()
	describeInstancesTags := []ecs.DescribeInstancesTag{
		{
			Key:   fmt.Sprintf("kubernetes.io/cluster/%s", infraID),
			Value: "owned",
		},
	}
	describeInstancesRequest.Tag = &describeInstancesTags
	describeInstancesResponse, err := c.DescribeInstances(describeInstancesRequest)
	if err != nil {
		logger.WithError(err).Error("failed to list instances")
		return nil, err
	}
	var result []ecs.Instance
	for idx, i := range describeInstancesResponse.Instances.Instance {
		if states.Has(i.Status) {
			result = append(result, describeInstancesResponse.Instances.Instance[idx])
		}
	}
	logger.WithField("count", len(result)).WithField("states", states).Debug("result of listing instances")
	return result, nil
}
