package clusterdeployment

import (
	"reflect"
	"sync"

	installtypes "github.com/openshift/installer/pkg/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hivev1client "github.com/openshift/hive/pkg/client/clientset-generated/clientset/typed/hive/v1"
	hiveapi "github.com/openshift/hive/pkg/hive/apis/hive"
	"github.com/openshift/hive/pkg/hive/apiserver/registry/util"
)

type proxyWatcher struct {
	hiveClient         hivev1client.HiveV1Interface
	coreClient         corev1client.CoreV1Interface
	cdWatcher          watch.Interface
	secretWatcher      watch.Interface
	machinePoolWatcher watch.Interface
	result             chan watch.Event
	stopCh             chan struct{}
	mutex              sync.Mutex
	stopped            bool
	logger             log.FieldLogger
}

func newProxyWatcher(hiveClient hivev1client.HiveV1Interface, coreClient corev1client.CoreV1Interface, cdWatcher watch.Interface, secretWatcher watch.Interface, machinePoolWatcher watch.Interface, logger log.FieldLogger) *proxyWatcher {
	pw := &proxyWatcher{
		hiveClient:         hiveClient,
		coreClient:         coreClient,
		cdWatcher:          cdWatcher,
		secretWatcher:      secretWatcher,
		machinePoolWatcher: machinePoolWatcher,
		result:             make(chan watch.Event),
		stopCh:             make(chan struct{}),
		stopped:            false,
		logger:             logger,
	}
	go pw.receive()
	return pw
}

func (pw *proxyWatcher) Stop() {
	pw.mutex.Lock()
	defer pw.mutex.Unlock()
	if !pw.stopped {
		pw.stopped = true
		close(pw.stopCh)
	}
	pw.cdWatcher.Stop()
	pw.secretWatcher.Stop()
	pw.machinePoolWatcher.Stop()
}

func (pw *proxyWatcher) ResultChan() <-chan watch.Event {
	return pw.result
}

func (pw *proxyWatcher) stopping() bool {
	pw.mutex.Lock()
	defer pw.mutex.Unlock()
	return pw.stopped
}

func (pw *proxyWatcher) receive() {
	defer close(pw.result)
	defer pw.Stop()

	cdSource := pw.cdWatcher.ResultChan()
	secretSource := pw.secretWatcher.ResultChan()
	machinePoolSource := pw.machinePoolWatcher.ResultChan()

	for {
		var sourceEvent watch.Event
		var ok bool
		select {
		case <-pw.stopCh:
			pw.logger.Debug("received on stop channel")
			ok = false
		case sourceEvent, ok = <-cdSource:
			pw.logger.Debug("received on clusterdeployment channel")
		case sourceEvent, ok = <-secretSource:
			pw.logger.Debug("received on secret channel")
		case sourceEvent, ok = <-machinePoolSource:
			pw.logger.Debug("received on machinepool channel")
		}
		if !ok {
			pw.logger.Debug("shutting down receive")
			break
		}

		if pw.stopping() {
			break
		}

		if err := pw.processEvent(sourceEvent); err != nil {
			status := apierrors.NewInternalError(err).Status()
			pw.result <- watch.Event{
				Type:   watch.Error,
				Object: &status,
			}
		}
	}
}

func (pw *proxyWatcher) processEvent(sourceEvent watch.Event) error {
	if sourceEvent.Type == watch.Error {
		pw.result <- sourceEvent
		return nil
	}

	eventType := watch.Modified
	var v1ClusterDeployment *hivev1.ClusterDeployment
	var installConfig *installtypes.InstallConfig
	var installConfigResourceVersion string
	var err error

	switch t := sourceEvent.Object.(type) {
	case *hivev1.ClusterDeployment:
		eventType = sourceEvent.Type
		v1ClusterDeployment, installConfig, installConfigResourceVersion, err = pw.objectsForClusterDeploymentEvent(t)
	case *corev1.Secret:
		v1ClusterDeployment, installConfig, installConfigResourceVersion, err = pw.objectsForSecretEvent(t)
	case *hivev1.MachinePool:
		v1ClusterDeployment, installConfig, installConfigResourceVersion, err = pw.objectsForMachinePoolEvent(t)
	default:
		pw.logger.WithField("type", reflect.TypeOf(t)).Error("unknown event object type")
		return errors.Errorf("unknown event object type: %T", t)
	}

	if err != nil {
		return err
	}

	if v1ClusterDeployment == nil {
		// object in source event is not associated with any cluster deployment
		return nil
	}

	machinePools, err := getMachinePools(v1ClusterDeployment.Name, pw.hiveClient.MachinePools(v1ClusterDeployment.Namespace))
	if err != nil {
		pw.logger.WithField("namespace", v1ClusterDeployment.Namespace).WithField("name", v1ClusterDeployment.Name).WithError(err).Error("could not get machine pools")
		return errors.Wrap(err, "could not get machine pools")
	}

	v1alpha1ClusterDeploymnet := &hiveapi.ClusterDeployment{}
	if err := util.ClusterDeploymentFromHiveV1(v1ClusterDeployment, installConfig, machinePools, installConfigResourceVersion, v1alpha1ClusterDeploymnet); err != nil {
		pw.logger.WithField("namespace", v1ClusterDeployment.Namespace).WithField("name", v1ClusterDeployment.Name).WithError(err).Error("could not convert clusterdeployment from v1")
		return errors.Wrap(err, "could not convert clusterdeployment from v1")
	}

	pw.result <- watch.Event{
		Type:   eventType,
		Object: v1alpha1ClusterDeploymnet,
	}

	return nil
}

func (pw *proxyWatcher) objectsForClusterDeploymentEvent(in *hivev1.ClusterDeployment) (
	cd *hivev1.ClusterDeployment,
	installConfig *installtypes.InstallConfig,
	installConfigResourceVersion string,
	returnErr error,
) {
	cd = in
	var installConfigSecret *corev1.Secret
	installConfig, installConfigSecret = getInstallConfig(cd, pw.coreClient.Secrets(cd.Namespace))
	if installConfigSecret != nil {
		installConfigResourceVersion = installConfigSecret.ResourceVersion
	}
	return
}

func (pw *proxyWatcher) objectsForSecretEvent(secret *corev1.Secret) (
	cd *hivev1.ClusterDeployment,
	installConfig *installtypes.InstallConfig,
	installConfigResourceVersion string,
	returnErr error,
) {
	installConfig = getInstallConfigFromSecret(secret)
	if installConfig == nil {
		pw.logger.WithField("namespace", secret.Namespace).WithField("name", secret.Name).Debug("secret is not an installconfig for a clusterdeployment")
		return
	}
	installConfigResourceVersion = secret.ResourceVersion
	clusterDeployments, err := pw.hiveClient.ClusterDeployments(secret.Namespace).List(metav1.ListOptions{})
	if err != nil {
		pw.logger.WithField("namespace", secret.Namespace).WithField("name", secret.Name).WithError(err).Error("could not get clusterdeployment for installconfig")
		returnErr = errors.Wrap(err, "could not get clusterdeployment for installconfig")
		return
	}
	for i, clusterDeployment := range clusterDeployments.Items {
		if prov := clusterDeployment.Spec.Provisioning; prov != nil {
			if prov.InstallConfigSecretRef.Name == secret.Name {
				cd = &clusterDeployments.Items[i]
				return
			}
		}
	}
	pw.logger.WithField("namespace", secret.Namespace).WithField("name", secret.Name).Debug("no clusterdeployment found referencing installconfig secret")
	return
}

func (pw *proxyWatcher) objectsForMachinePoolEvent(machinePool *hivev1.MachinePool) (
	cd *hivev1.ClusterDeployment,
	installConfig *installtypes.InstallConfig,
	installConfigResourceVersion string,
	returnErr error,
) {
	cd, err := pw.hiveClient.ClusterDeployments(machinePool.Namespace).Get(machinePool.Spec.ClusterDeploymentRef.Name, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		pw.logger.WithField("namespace", machinePool.Namespace).WithField("name", machinePool.Name).Debug("no clusterdeployment found for machinepool")
		return
	case err != nil:
		pw.logger.WithField("namespace", machinePool.Namespace).WithField("name", machinePool.Name).WithError(err).Error("could not get clusterdeployment for machinepool")
		returnErr = errors.Wrap(err, "could not get clusterdeployment for machinepool")
		return
	}
	installConfig, installConfigSecret := getInstallConfig(cd, pw.coreClient.Secrets(cd.Namespace))
	if installConfigSecret != nil {
		installConfigResourceVersion = installConfigSecret.ResourceVersion
	}
	return
}
