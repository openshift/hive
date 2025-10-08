package privatelink

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	configv1 "github.com/openshift/api/config/v1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	"github.com/openshift/hive/pkg/controller/privatelink/actuator"
	"github.com/openshift/hive/pkg/controller/privatelink/actuator/awsactuator"
	"github.com/openshift/hive/pkg/controller/privatelink/actuator/gcpactuator"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	ControllerName = hivev1.PrivateLinkControllerName
	finalizer      = "hive.openshift.io/private-link"

	lastCleanupAnnotationKey = "private-link-controller.hive.openshift.io/last-cleanup-for"
)

// PrivateLinkReconciler reconciles a PrivateLink for clusterdeployment object
type PrivateLinkReconciler struct {
	client.Client

	controllerconfig *hivev1.PrivateLinkConfig
}

// Add creates a new PrivateLink Controller and adds it to the Manager with default RBAC.
// The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	concurrentReconciles, clientRateLimiter, queueRateLimiter, err := controllerutils.GetControllerConfig(mgr.GetClient(), ControllerName)
	if err != nil {
		return errors.Wrap(err, "could not get controller configurations")
	}
	reconciler, err := NewReconciler(mgr, clientRateLimiter)
	if err != nil {
		return errors.Wrap(err, "could not create reconciler")
	}
	return AddToManager(mgr, reconciler, concurrentReconciles, queueRateLimiter)
}

// NewReconciler returns a new PrivateLinkReconciler
func NewReconciler(mgr manager.Manager, rateLimiter flowcontrol.RateLimiter) (*PrivateLinkReconciler, error) {
	reconciler := &PrivateLinkReconciler{
		Client: controllerutils.NewClientWithMetricsOrDie(mgr, ControllerName, &rateLimiter),
	}

	controllerConfig, err := ReadPrivateLinkControllerConfigFile()
	if err != nil {
		return reconciler, errors.Wrap(err, "could not load configuration")
	}

	reconciler.controllerconfig = controllerConfig

	return reconciler, nil
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r *PrivateLinkReconciler, concurrentReconciles int, rateLimiter workqueue.TypedRateLimiter[reconcile.Request]) error {
	logger := log.WithField("controller", ControllerName)
	// Create a new controller
	c, err := controller.New(string(ControllerName+"-controller"), mgr, controller.Options{
		Reconciler:              controllerutils.NewDelayingReconciler(r, logger),
		MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter:             rateLimiter,
	})
	if err != nil {
		return errors.Wrap(err, "error creating new controller")
	}

	// Watch for changes to ClusterDeployment
	if err = c.Watch(source.Kind(mgr.GetCache(), &hivev1.ClusterDeployment{}, controllerutils.NewTypedRateLimitedUpdateEventHandler(&handler.TypedEnqueueRequestForObject[*hivev1.ClusterDeployment]{}, controllerutils.IsClusterDeploymentErrorUpdateEvent))); err != nil {
		return errors.Wrap(err, "error watching cluster deployment")
	}

	// Watch for changes to ClusterProvision
	if err := c.Watch(source.Kind(mgr.GetCache(), &hivev1.ClusterProvision{}, handler.TypedEnqueueRequestForOwner[*hivev1.ClusterProvision](mgr.GetScheme(), mgr.GetRESTMapper(), &hivev1.ClusterDeployment{}, handler.OnlyControllerOwner()))); err != nil {
		return errors.Wrap(err, "error watching cluster provision")
	}

	// Watch for changes to ClusterDeprovision
	if err := c.Watch(source.Kind(mgr.GetCache(), &hivev1.ClusterDeprovision{}, handler.TypedEnqueueRequestForOwner[*hivev1.ClusterDeprovision](mgr.GetScheme(), mgr.GetRESTMapper(), &hivev1.ClusterDeployment{}, handler.OnlyControllerOwner()))); err != nil {
		return errors.Wrap(err, "error watching cluster deprovision")
	}

	return nil
}

func (r *PrivateLinkReconciler) Reconcile(ctx context.Context, request reconcile.Request) (result reconcile.Result, returnErr error) {
	logger := controllerutils.BuildControllerLogger(ControllerName, "clusterDeployment", request.NamespacedName)
	logger.Debug("reconciling cluster deployment")
	recobsrv := hivemetrics.NewReconcileObserver(ControllerName, logger)
	defer recobsrv.ObserveControllerReconcileTime()

	// Fetch the ClusterDeployment instance
	cd := &hivev1.ClusterDeployment{}
	err := r.Get(context.TODO(), request.NamespacedName, cd)
	if apierrors.IsNotFound(err) {
		logger.Debug("cluster deployment not found")
		return reconcile.Result{}, nil
	}
	if err != nil {
		// Error reading the object - requeue the request.
		return reconcile.Result{}, errors.Wrap(err, "error getting ClusterDeployment")
	}
	logger = controllerutils.AddLogFields(controllerutils.MetaObjectLogTagger{Object: cd}, logger)

	var privateLinkEnabled bool
	var spokePlatformType configv1.PlatformType
	switch {
	case cd.Spec.Platform.GCP != nil:
		spokePlatformType = configv1.GCPPlatformType
		if cd.Spec.Platform.GCP.PrivateServiceConnect != nil {
			privateLinkEnabled = cd.Spec.Platform.GCP.PrivateServiceConnect.Enabled
		}
	default:
		logger.Debug("controller cannot service the clusterdeployment, so skipping")
		return reconcile.Result{}, nil
	}

	// Either privatelink was never enabled, or it was disabled and cleanup has already happened.
	// We exit here to avoid creating actuators for clusters that are not using privatelink. The
	// actuators fail on creation and throw errors in the logs if they are unable to create the
	// necessary cloud clients, such as a hive config without privatelink configured.
	// We can't check this with cleanupRequired() yet because that method requires the actuators to
	// already exist. We also want to ensure we remove the finalizer, which happens later in the
	// process. We can use the finalizer itself to determine if we can safely return here. If there
	// are privatelink resources to be cleaned up, then the finalizer should exist.
	if !privateLinkEnabled && !controllerutils.HasFinalizer(cd, finalizer) {
		logger.Debug("cluster deployment does not have private link enabled, so skipping")
		return reconcile.Result{}, nil
	}

	privateLink := &PrivateLink{
		Client: r.Client,
		cd:     cd,
		logger: logger,
	}

	privateLink.linkActuator, err = CreateActuator(r.Client, spokePlatformType, actuator.ActuatorTypeLink, cd, r.controllerconfig, privateLinkEnabled, logger)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "could not get link actuator")
	}

	// TODO: Determine and use hub platform type
	// Ideally we would get the hub platform type from the cluster infrastructure. However,
	// the hive service account does not have access. For now, we hard-code aws platform.
	var hubPlatformType = configv1.AWSPlatformType

	privateLink.hubActuator, err = CreateActuator(r.Client, hubPlatformType, actuator.ActuatorTypeHub, nil, r.controllerconfig, privateLinkEnabled, logger)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "could not get hub actuator")
	}

	return privateLink.Reconcile(privateLinkEnabled)
}

// CreateActuator creates an actuator based on the cloud platform and actuator type.
func CreateActuator(
	client client.Client,
	platform configv1.PlatformType,
	actuatorType actuator.ActuatorType,
	cd *hivev1.ClusterDeployment,
	config *hivev1.PrivateLinkConfig,
	privateLinkEnabled bool,
	logger log.FieldLogger) (actuator.Actuator, error) {

	switch platform {
	case configv1.AWSPlatformType:
		var awsConfig *hivev1.AWSPrivateLinkConfig
		// This is the code for when we add AWS to hiveconfig's PrivateLink structure later.
		//if config != nil {
		//	awsConfig = config.AWS
		//}
		if actuatorType == actuator.ActuatorTypeHub {
			return awsactuator.NewAWSHubActuator(&client, awsConfig, privateLinkEnabled, nil, logger)
		}
	case configv1.GCPPlatformType:
		var gcpConfig *hivev1.GCPPrivateServiceConnectConfig
		if config != nil {
			gcpConfig = config.GCP
		}
		if actuatorType == actuator.ActuatorTypeLink {
			return gcpactuator.NewGCPLinkActuator(&client, gcpConfig, cd, privateLinkEnabled, nil, logger)
		}
	}
	return nil, fmt.Errorf("unable to create privatelink actuator, invalid actuator type: %s/%s", platform, actuatorType)
}

// ReadPrivateLinkControllerConfigFile reads the configuration from the env
// and unmarshals. If the env is set to a file but that file doesn't exist it returns
// a zero-value configuration.
func ReadPrivateLinkControllerConfigFile() (*hivev1.PrivateLinkConfig, error) {
	fPath := os.Getenv(constants.PrivateLinkControllerConfigFileEnvVar)
	if len(fPath) == 0 {
		return nil, nil
	}

	config := &hivev1.PrivateLinkConfig{}

	fileBytes, err := os.ReadFile(fPath)
	if os.IsNotExist(err) {
		return config, nil
	}
	if err != nil {
		return config, errors.Wrap(err, "failed to read the privatelink controller config file")
	}
	if err := json.Unmarshal(fileBytes, &config); err != nil {
		return config, err
	}

	return config, nil
}

func updateAnnotations(client client.Client, cd *hivev1.ClusterDeployment) error {
	var retryBackoff = wait.Backoff{
		Steps:    5,
		Duration: 1 * time.Second,
		Factor:   1.0,
		Jitter:   0.1,
	}
	return retry.RetryOnConflict(retryBackoff, func() error {
		curr := &hivev1.ClusterDeployment{}
		err := client.Get(context.TODO(), types.NamespacedName{Namespace: cd.Namespace, Name: cd.Name}, curr)
		if err != nil {
			return err
		}
		curr.Annotations = cd.Annotations
		return client.Update(context.TODO(), curr)
	})
}
