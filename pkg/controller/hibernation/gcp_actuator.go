package hibernation

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	compute "google.golang.org/api/compute/v1"

	corev1 "k8s.io/api/core/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/gcpclient"
)

const (
	instanceFields = "items/*/instances(name,zone,status),nextPageToken"
)

var (
	gcpRunningStatuses           = sets.NewString("RUNNING")
	gcpStoppedStatuses           = sets.NewString("STOPPED", "TERMINATED")
	gcpPendingStatuses           = sets.NewString("PROVISIONING", "STAGING")
	gcpStoppingStatuses          = sets.NewString("STOPPING")
	gcpRunningOrPendingStatuses  = gcpRunningStatuses.Union(gcpPendingStatuses)
	gcpStoppedOrStoppingStatuses = gcpStoppedStatuses.Union(gcpStoppingStatuses)
	gcpNotRunningStatuses        = gcpStoppedOrStoppingStatuses.Union(gcpPendingStatuses)
	gcpNotStoppedStatuses        = gcpRunningOrPendingStatuses.Union(gcpStoppingStatuses)
)

func init() {
	RegisterActuator(&gcpActuator{getGCPClientFn: getGCPClient})
}

type gcpActuator struct {
	getGCPClientFn func(*hivev1.ClusterDeployment, client.Client, log.FieldLogger) (gcpclient.Client, error)
}

// CanHandle returns true if the actuator can handle a particular ClusterDeployment
func (a *gcpActuator) CanHandle(cd *hivev1.ClusterDeployment) bool {
	return cd.Spec.Platform.GCP != nil
}

// StopMachines will start machines belonging to the given ClusterDeployment
func (a *gcpActuator) StopMachines(cd *hivev1.ClusterDeployment, hiveClient client.Client, logger log.FieldLogger) error {
	logger = logger.WithField("cloud", "GCP")
	gcpClient, err := a.getGCPClientFn(cd, hiveClient, logger)
	if err != nil {
		return err
	}
	instances, err := gcpListComputeInstances(gcpClient, cd, gcpRunningOrPendingStatuses, logger)
	if err != nil {
		return err
	}
	opts := []gcpclient.InstancesStopCallOption{}
	if cd.Spec.Platform.GCP != nil && cd.Spec.Platform.GCP.DiscardLocalSsdOnHibernate != nil {
		opts = append(opts, func(stopCall *compute.InstancesStopCall) *compute.InstancesStopCall {
			return stopCall.DiscardLocalSsd(*cd.Spec.Platform.GCP.DiscardLocalSsdOnHibernate)
		})
	}
	var errs []error
	for _, instance := range instances {
		logger.WithField("instance", instance.Name).Info("Stopping instance")
		err = gcpClient.StopInstance(instance, opts...)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return utilerrors.NewAggregate(errs)
}

// StartMachines will select machines belonging to the given ClusterDeployment
func (a *gcpActuator) StartMachines(cd *hivev1.ClusterDeployment, hiveClient client.Client, logger log.FieldLogger) error {
	logger = logger.WithField("cloud", "GCP")
	gcpClient, err := a.getGCPClientFn(cd, hiveClient, logger)
	if err != nil {
		return err
	}
	instances, err := gcpListComputeInstances(gcpClient, cd, gcpStoppedOrStoppingStatuses, logger)
	if err != nil {
		return err
	}
	if len(instances) == 0 {
		logger.Info("No instances were found to start")
		return nil
	}
	var errs []error
	for _, instance := range instances {
		logger.WithField("instance", instance.Name).Info("Starting instance")
		err = gcpClient.StartInstance(instance)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return utilerrors.NewAggregate(errs)
}

// MachinesRunning will return true if the machines associated with the given
// ClusterDeployment are in a running state. It also returns a list of machines that
// are not running.
func (a *gcpActuator) MachinesRunning(cd *hivev1.ClusterDeployment, hiveClient client.Client, logger log.FieldLogger) (bool, []string, error) {
	logger = logger.WithField("cloud", "GCP")
	gcpClient, err := a.getGCPClientFn(cd, hiveClient, logger)
	if err != nil {
		return false, nil, err
	}
	instances, err := gcpListComputeInstances(gcpClient, cd, gcpNotRunningStatuses, logger)
	if err != nil {
		return false, nil, err
	}
	return len(instances) == 0, instanceNames(instances), nil
}

// MachinesStopped will return true if the machines associated with the given
// ClusterDeployment are in a stopped state. It also returns a list of machines
// that have not stopped.
func (a *gcpActuator) MachinesStopped(cd *hivev1.ClusterDeployment, hiveClient client.Client, logger log.FieldLogger) (bool, []string, error) {
	logger = logger.WithField("cloud", "GCP")
	gcpClient, err := a.getGCPClientFn(cd, hiveClient, logger)
	if err != nil {
		return false, nil, err
	}
	instances, err := gcpListComputeInstances(gcpClient, cd, gcpNotStoppedStatuses, logger)
	if err != nil {
		return false, nil, err
	}
	return len(instances) == 0, instanceNames(instances), nil
}

func getGCPClient(cd *hivev1.ClusterDeployment, c client.Client, logger log.FieldLogger) (gcpclient.Client, error) {
	if cd.Spec.Platform.GCP == nil {
		return nil, errors.New("GCP platform is not set in ClusterDeployment")
	}
	secret := &corev1.Secret{}
	err := c.Get(context.TODO(), client.ObjectKey{Name: cd.Spec.Platform.GCP.CredentialsSecretRef.Name, Namespace: cd.Namespace}, secret)
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "Failed to fetch GCP credentials secret")
		return nil, errors.Wrap(err, "failed to fetch GCP credentials secret")
	}
	return gcpclient.NewClientFromSecret(secret)
}

func instanceFilter(cd *hivev1.ClusterDeployment) string {
	return fmt.Sprintf("name eq \"%s-.*\"", cd.Spec.ClusterMetadata.InfraID)
}

func gcpListComputeInstances(gcpClient gcpclient.Client, cd *hivev1.ClusterDeployment, statuses sets.String, logger log.FieldLogger) ([]*compute.Instance, error) {
	var instances []*compute.Instance
	logger.Debug("listing client instances")
	err := gcpClient.ListComputeInstances(gcpclient.ListComputeInstancesOptions{
		Filter: instanceFilter(cd),
		Fields: instanceFields,
	}, func(list *compute.InstanceAggregatedList) error {
		for _, scopedList := range list.Items {
			for _, instance := range scopedList.Instances {
				if statuses.Has(instance.Status) {
					instances = append(instances, instance)
				}
			}
		}
		return nil
	})
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "Failed to fetch compute instances")
	} else {
		logger.WithField("count", len(instances)).WithField("statuses", statuses.List()).Debug("found instances")
	}
	return instances, err
}

func instanceNames(instances []*compute.Instance) []string {
	ret := make([]string, len(instances))
	for i, instance := range instances {
		ret[i] = instance.Name
	}
	return ret
}
