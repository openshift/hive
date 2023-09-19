package awsprivatelink

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/sts"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	"github.com/openshift/hive/pkg/awsclient"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	ControllerName = hivev1.AWSPrivateLinkControllerName
	finalizer      = "hive.openshift.io/aws-private-link"

	lastCleanupAnnotationKey = "aws-private-link-controller.hive.openshift.io/last-cleanup-for"

	defaultRequeueLater = 1 * time.Minute
)

// clusterDeploymentAWSPrivateLinkConditions are the cluster deployment conditions controlled by
// AWS private link controller
var clusterDeploymentAWSPrivateLinkConditions = []hivev1.ClusterDeploymentConditionType{
	hivev1.AWSPrivateLinkFailedClusterDeploymentCondition,
	hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
}

// Add creates a new AWSPrivateLink Controller and adds it to the Manager with default RBAC.
// The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	logger := log.WithField("controller", ControllerName)
	concurrentReconciles, clientRateLimiter, queueRateLimiter, err := controllerutils.GetControllerConfig(mgr.GetClient(), ControllerName)
	if err != nil {
		logger.WithError(err).Error("could not get controller configurations")
		return err
	}
	reconciler, err := NewReconciler(mgr, clientRateLimiter)
	if err != nil {
		logger.WithError(err).Error("could not create reconciler")
		return err
	}
	return AddToManager(mgr, reconciler, concurrentReconciles, queueRateLimiter)
}

// NewReconciler returns a new ReconcileClusterClaim
func NewReconciler(mgr manager.Manager, rateLimiter flowcontrol.RateLimiter) (*ReconcileAWSPrivateLink, error) {
	logger := log.WithField("controller", ControllerName)
	reconciler := &ReconcileAWSPrivateLink{
		Client: controllerutils.NewClientWithMetricsOrDie(mgr, ControllerName, &rateLimiter),
	}

	config, err := ReadAWSPrivateLinkControllerConfigFile()
	if err != nil {
		logger.WithError(err).Error("could not get load configuration")
		return reconciler, err
	}
	reconciler.controllerconfig = config
	reconciler.awsClientFn = awsclient.New
	return reconciler, nil
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r *ReconcileAWSPrivateLink, concurrentReconciles int, rateLimiter workqueue.RateLimiter) error {
	// Create a new controller
	c, err := controller.New("awsprivatelink-controller", mgr, controller.Options{
		Reconciler:              controllerutils.NewDelayingReconciler(r, log.WithField("controller", ControllerName)),
		MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter:             rateLimiter,
	})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterDeployment
	err = c.Watch(source.Kind(mgr.GetCache(), &hivev1.ClusterDeployment{}),
		controllerutils.NewRateLimitedUpdateEventHandler(&handler.EnqueueRequestForObject{}, controllerutils.IsClusterDeploymentErrorUpdateEvent))
	if err != nil {
		log.WithField("controller", ControllerName).WithError(err).Error("Error watching cluster deployment")
		return err
	}

	// Watch for changes to ClusterProvision
	if err := c.Watch(source.Kind(mgr.GetCache(), &hivev1.ClusterProvision{}),
		handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &hivev1.ClusterDeployment{}, handler.OnlyControllerOwner())); err != nil {
		log.WithField("controller", ControllerName).WithError(err).Error("Error watching cluster provision")
		return err
	}

	// Watch for changes to ClusterDeprovision
	if err := c.Watch(source.Kind(mgr.GetCache(), &hivev1.ClusterDeprovision{}),
		handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &hivev1.ClusterDeployment{}, handler.OnlyControllerOwner())); err != nil {
		log.WithField("controller", ControllerName).WithError(err).Error("Error watching cluster deprovision")
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileAWSPrivateLink{}

// ReconcileAWSPrivateLink reconciles a PrivateLink for clusterdeployment object
type ReconcileAWSPrivateLink struct {
	client.Client

	controllerconfig *hivev1.AWSPrivateLinkConfig

	// testing purpose
	awsClientFn awsClientFn
}

type awsClientFn func(client.Client, awsclient.Options) (awsclient.Client, error)

// Reconcile reconciles PrivateLink for ClusterDeployment.
func (r *ReconcileAWSPrivateLink) Reconcile(ctx context.Context, request reconcile.Request) (result reconcile.Result, returnErr error) {
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
		logger.WithError(err).Error("error getting ClusterDeployment")
		return reconcile.Result{}, err
	}
	logger = controllerutils.AddLogFields(controllerutils.MetaObjectLogTagger{Object: cd}, logger)

	if paused, err := strconv.ParseBool(cd.Annotations[constants.ReconcilePauseAnnotation]); err == nil && paused {
		logger.Info("skipping reconcile due to ClusterDeployment pause annotation")
		return reconcile.Result{}, nil
	}

	// Initialize cluster deployment conditions if not present
	newConditions, changed := controllerutils.InitializeClusterDeploymentConditions(cd.Status.Conditions, clusterDeploymentAWSPrivateLinkConditions)
	if changed {
		cd.Status.Conditions = newConditions
		logger.Info("initializing AWS private link controller conditions")
		if err := r.Status().Update(context.TODO(), cd); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to update cluster deployment status")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	if cd.Spec.Platform.AWS == nil ||
		cd.Spec.Platform.AWS.PrivateLink == nil {
		logger.Debug("controller cannot service the clusterdeployment, so skipping")
		return reconcile.Result{}, nil
	}
	if !cd.Spec.Platform.AWS.PrivateLink.Enabled {
		if cleanupRequired(cd) {
			// private link was disabled for this cluster so cleanup is required.
			return r.cleanupClusterDeployment(cd, cd.Spec.ClusterMetadata, logger)
		}

		logger.Debug("cluster deployment does not have private link enabled, so skipping")
		return reconcile.Result{}, nil
	}

	if cd.DeletionTimestamp != nil {
		return r.cleanupClusterDeployment(cd, cd.Spec.ClusterMetadata, logger)
	}

	// Add finalizer if not already present
	if !controllerutils.HasFinalizer(cd, finalizer) {
		logger.Debug("adding finalizer to ClusterDeployment")
		controllerutils.AddFinalizer(cd, finalizer)
		if err := r.Update(context.Background(), cd); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "error adding finalizer to ClusterDeployment")
			return reconcile.Result{}, err
		}
	}

	supportedRegion := false
	for _, item := range r.controllerconfig.EndpointVPCInventory {
		if strings.EqualFold(item.Region, cd.Spec.Platform.AWS.Region) {
			supportedRegion = true
			break
		}
	}
	if !supportedRegion {
		err := errors.Errorf("cluster deployment region %q is not supported as there is no inventory to create necessary resources",
			cd.Spec.Platform.AWS.Region)
		logger.WithError(err).Error("cluster deployment region is not supported, so skipping")

		if err := r.setErrCondition(cd, "UnsupportedRegion", err, logger); err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	// See if we need to sync. This is what rate limits our cloud API usage, but allows for immediate syncing
	// on changes and deletes.
	shouldSync, syncAfter := shouldSync(cd)
	if !shouldSync {
		logger.WithFields(log.Fields{
			"syncAfter": syncAfter,
		}).Debug("Sync not needed")

		return reconcile.Result{RequeueAfter: syncAfter}, nil
	}

	if cd.Spec.Installed {
		logger.Debug("reconciling already installed cluster deployment")
		return r.reconcilePrivateLink(cd, cd.Spec.ClusterMetadata, logger)
	}

	if cd.Status.ProvisionRef == nil {
		logger.Debug("waiting for cluster deployment provision to start, will retry soon.")
		return reconcile.Result{}, nil
	}

	cpLog := logger.WithField("provision", cd.Status.ProvisionRef.Name)
	cp := &hivev1.ClusterProvision{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: cd.Status.ProvisionRef.Name, Namespace: cd.Namespace}, cp)
	if apierrors.IsNotFound(err) {
		cpLog.Warn("linked cluster provision not found")
		return reconcile.Result{}, err
	}
	if err != nil {
		cpLog.WithError(err).Error("could not get provision")
		return reconcile.Result{}, err
	}

	if cp.Spec.PrevInfraID != nil && *cp.Spec.PrevInfraID != "" && cleanupRequired(cd) {
		lastCleanup := cd.Annotations[lastCleanupAnnotationKey]
		if lastCleanup != *cp.Spec.PrevInfraID {
			logger.WithField("prevInfraID", *cp.Spec.PrevInfraID).
				Info("cleaning up PrivateLink resources from previous attempt")

			if err := r.cleanupPreviousProvisionAttempt(cd, cp, logger); err != nil {
				logger.WithError(err).Error("error cleaning up PrivateLink resources for ClusterDeployment")

				if err := r.setErrCondition(cd, "CleanupForProvisionReattemptFailed", err, logger); err != nil {
					logger.WithError(err).Error("failed to update condition on cluster deployment")
					return reconcile.Result{}, err
				}
				return reconcile.Result{}, err
			}

			if err := r.setReadyCondition(cd, corev1.ConditionFalse,
				"PreviousAttemptCleanupComplete",
				"successfully cleaned up resources from previous provision attempt so that next attempt can start",
				logger); err != nil {
				logger.WithError(err).Error("failed to update condition on cluster deployment")
				return reconcile.Result{}, err
			}

			return reconcile.Result{Requeue: true}, nil
		}
	}

	if cp.Spec.InfraID == nil ||
		(cp.Spec.InfraID != nil && *cp.Spec.InfraID == "") ||
		(cp.Spec.AdminKubeconfigSecretRef == nil) ||
		(cp.Spec.AdminKubeconfigSecretRef != nil && cp.Spec.AdminKubeconfigSecretRef.Name == "") {
		logger.Debug("waiting for cluster deployment provision to provide ClusterMetadata, will retry soon.")
		return reconcile.Result{}, nil
	}

	return r.reconcilePrivateLink(cd, &hivev1.ClusterMetadata{InfraID: *cp.Spec.InfraID, AdminKubeconfigSecretRef: *cp.Spec.AdminKubeconfigSecretRef}, logger)
}

// shouldSync returns if we should sync the desired ClusterDeployment. If it returns false, it also returns
// the duration after which we should try to check if sync is required.
func shouldSync(desired *hivev1.ClusterDeployment) (bool, time.Duration) {
	window := 2 * time.Hour
	if desired.DeletionTimestamp != nil && !controllerutils.HasFinalizer(desired, finalizer) {
		return false, 0 // No finalizer means our cleanup has been completed. There's nothing left to do.
	}

	if desired.DeletionTimestamp != nil {
		return true, 0 // We're in a deleting state, sync now.
	}

	failedCondition := controllerutils.FindCondition(desired.Status.Conditions, hivev1.AWSPrivateLinkFailedClusterDeploymentCondition)
	if failedCondition != nil && failedCondition.Status == corev1.ConditionTrue {
		return true, 0 // we have failed to reconcile and therefore should continue to retry for quick recovery
	}

	readyCondition := controllerutils.FindCondition(desired.Status.Conditions, hivev1.AWSPrivateLinkReadyClusterDeploymentCondition)
	if readyCondition == nil || readyCondition.Status != corev1.ConditionTrue {
		return true, 0 // we have not reached Ready level
	}
	delta := time.Since(readyCondition.LastProbeTime.Time)

	if !desired.Spec.Installed {
		// as cluster is installing, but the private link has been setup once, we wait
		// for a shorter duration before reconciling again.
		window = 10 * time.Minute
	}

	if delta >= window {
		// We haven't sync'd in over resync duration time, sync now.
		return true, 0
	}

	syncAfter := (window - delta).Round(time.Minute)
	if syncAfter == 0 {
		// if it is less than a minute, sync after a minute
		syncAfter = time.Minute
	}

	if desired.Spec.Platform.AWS.PrivateLink != nil {
		statusAdditionalAllowedPrincipals := sets.NewString()
		if desired.Status.Platform != nil &&
			desired.Status.Platform.AWS != nil &&
			desired.Status.Platform.AWS.PrivateLink != nil &&
			desired.Status.Platform.AWS.PrivateLink.VPCEndpointService.AdditionalAllowedPrincipals != nil {
			statusAdditionalAllowedPrincipals = sets.NewString(*desired.Status.Platform.AWS.PrivateLink.VPCEndpointService.AdditionalAllowedPrincipals...)
		}
		specAdditionalAllowedPrincipals := sets.NewString()
		if desired.Spec.Platform.AWS.PrivateLink.AdditionalAllowedPrincipals != nil {
			specAdditionalAllowedPrincipals = sets.NewString(*desired.Spec.Platform.AWS.PrivateLink.AdditionalAllowedPrincipals...)
		}
		if !specAdditionalAllowedPrincipals.Equal(statusAdditionalAllowedPrincipals) {
			return true, 0 // there is a diff between configured additionalAllowedPrincipals within spec and additionalallowedPrincipals within status, sync now.
		}
	}

	// We didn't meet any of the criteria above, so we should not sync.
	return false, syncAfter
}

func (r *ReconcileAWSPrivateLink) setErrCondition(cd *hivev1.ClusterDeployment,
	reason string, err error,
	logger log.FieldLogger) error {
	curr := &hivev1.ClusterDeployment{}
	errGet := r.Get(context.TODO(), types.NamespacedName{Namespace: cd.Namespace, Name: cd.Name}, curr)
	if errGet != nil {
		return errGet
	}
	message := filterErrorMessage(err)
	conditions, failedChanged := controllerutils.SetClusterDeploymentConditionWithChangeCheck(
		curr.Status.Conditions,
		hivev1.AWSPrivateLinkFailedClusterDeploymentCondition,
		corev1.ConditionTrue,
		reason,
		message,
		controllerutils.UpdateConditionIfReasonOrMessageChange)
	conditions, readyChanged := controllerutils.SetClusterDeploymentConditionWithChangeCheck(
		conditions,
		hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
		corev1.ConditionFalse,
		reason,
		message,
		controllerutils.UpdateConditionIfReasonOrMessageChange)
	if !readyChanged && !failedChanged {
		return nil
	}
	curr.Status.Conditions = conditions
	logger.Debug("setting AWSPrivateLinkFailedClusterDeploymentCondition to true")
	return r.Status().Update(context.TODO(), curr)
}

func (r *ReconcileAWSPrivateLink) setReadyCondition(cd *hivev1.ClusterDeployment,
	completed corev1.ConditionStatus,
	reason string, message string,
	logger log.FieldLogger) error {

	curr := &hivev1.ClusterDeployment{}
	errGet := r.Get(context.TODO(), types.NamespacedName{Namespace: cd.Namespace, Name: cd.Name}, curr)
	if errGet != nil {
		return errGet
	}

	conditions := curr.Status.Conditions

	var failedChanged bool
	if completed == corev1.ConditionTrue {
		conditions, failedChanged = controllerutils.SetClusterDeploymentConditionWithChangeCheck(
			conditions,
			hivev1.AWSPrivateLinkFailedClusterDeploymentCondition,
			corev1.ConditionFalse,
			reason,
			message,
			controllerutils.UpdateConditionIfReasonOrMessageChange)
	}

	var readyChanged bool
	ready := controllerutils.FindCondition(conditions, hivev1.AWSPrivateLinkReadyClusterDeploymentCondition)
	if ready == nil || ready.Status != corev1.ConditionTrue {
		// we want to allow Ready condition to reach Ready level
		conditions, readyChanged = controllerutils.SetClusterDeploymentConditionWithChangeCheck(
			conditions,
			hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
			completed,
			reason,
			message,
			controllerutils.UpdateConditionIfReasonOrMessageChange)
	} else {
		if completed == corev1.ConditionTrue {
			// allow reinforcing Ready level to track the last Ready probe.
			// we have a higher level control of when to sync an already Ready cluster
			conditions, readyChanged = controllerutils.SetClusterDeploymentConditionWithChangeCheck(
				conditions,
				hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
				corev1.ConditionTrue,
				reason,
				message,
				controllerutils.UpdateConditionAlways)
		}
	}
	if !readyChanged && !failedChanged {
		return nil
	}
	curr.Status.Conditions = conditions
	logger.Debugf("setting AWSPrivateLinkReadyClusterDeploymentCondition to %s", completed)
	return r.Status().Update(context.TODO(), curr)
}

func (r *ReconcileAWSPrivateLink) reconcilePrivateLink(cd *hivev1.ClusterDeployment, clusterMetadata *hivev1.ClusterMetadata, logger log.FieldLogger) (reconcile.Result, error) {
	logger.Debug("reconciling PrivateLink resources")
	awsClient, err := newAWSClient(r, cd)
	if err != nil {
		logger.WithError(err).Error("error creating AWS client for the cluster")
		return reconcile.Result{}, err
	}

	// discover the NLB for the cluster.
	nlbARN, err := discoverNLBForCluster(awsClient.user, clusterMetadata.InfraID, logger)
	if err != nil {
		if awsErrCodeEquals(err, "LoadBalancerNotFound") {
			logger.WithField("infraID", clusterMetadata.InfraID).Debug("NLB is not yet created for the cluster, will retry later")

			if err := r.setReadyCondition(cd, corev1.ConditionFalse,
				"DiscoveringNLBNotYetFound",
				"discovering NLB for the cluster, but it does not exist yet",
				logger); err != nil {
				logger.WithError(err).Error("failed to update condition on cluster deployment")
				return reconcile.Result{}, err
			}
			return reconcile.Result{RequeueAfter: defaultRequeueLater}, nil
		}

		logger.WithField("infraID", clusterMetadata.InfraID).WithError(err).Error("error discovering NLB for the cluster")

		if err := r.setErrCondition(cd, "DiscoveringNLBFailed", errors.New(controllerutils.ErrorScrub(err)), logger); err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, err
	}

	// reconcile the VPC Endpoint Service
	serviceModified, vpcEndpointService, err := r.reconcileVPCEndpointService(awsClient, cd, clusterMetadata, nlbARN, logger)
	if err != nil {
		logger.WithError(err).Error("failed to reconcile the VPC Endpoint Service")

		if err := r.setErrCondition(cd, "VPCEndpointServiceReconcileFailed", errors.New(controllerutils.ErrorScrub(err)), logger); err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, errors.Wrap(err, "failed to reconcile the VPC Endpoint Service")
	}
	if serviceModified {
		if err := r.setReadyCondition(cd, corev1.ConditionFalse,
			"ReconciledVPCEndpointService",
			"reconciled the VPC Endpoint Service for the cluster",
			logger); err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
			return reconcile.Result{}, err
		}
	}

	// Create the VPC endpoint with the chosen VPC.
	endpointModified, vpcEndpoint, err := r.reconcileVPCEndpoint(awsClient, cd, clusterMetadata, vpcEndpointService, logger)
	if err != nil {
		logger.WithError(err).Error("failed to reconcile the VPC Endpoint")
		reason := "VPCEndpointReconcileFailed"
		if errors.Is(err, errNoSupportedAZsInInventory) {
			reason = "NoSupportedAZsInInventory"
		}
		if err := r.setErrCondition(cd, reason, err, logger); err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, errors.Wrap(err, "failed to reconcile the VPC Endpoint")
	}

	if endpointModified {
		if err := r.setReadyCondition(cd, corev1.ConditionFalse,
			"ReconciledVPCEndpoint",
			"reconciled the VPC Endpoint for the cluster",
			logger); err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
			return reconcile.Result{}, err
		}
	}

	// Figure out the API address for cluster.
	apiDomain, err := initialURL(r.Client,
		client.ObjectKey{Namespace: cd.Namespace, Name: clusterMetadata.AdminKubeconfigSecretRef.Name})
	if err != nil {
		logger.WithError(err).Error("could not get API URL from kubeconfig")

		if err := r.setErrCondition(cd, "CouldNotCalculateAPIDomain", err, logger); err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, err
	}

	// Create the Private Hosted Zone for the VPC Endpoint.
	hzModified, hostedZoneID, err := r.reconcileHostedZone(awsClient, cd, clusterMetadata, vpcEndpoint, apiDomain, logger)
	if err != nil {
		logger.WithError(err).Error("could not reconcile the Hosted Zone")

		if err := r.setErrCondition(cd, "PrivateHostedZoneReconcileFailed", err, logger); err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, err
	}

	if hzModified {
		if err := r.setReadyCondition(cd, corev1.ConditionFalse,
			"ReconciledPrivateHostedZone",
			"reconciled the Private Hosted Zone for the VPC Endpoint of the cluster",
			logger); err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
			return reconcile.Result{}, err
		}
	}

	// Associate the VPCs to the hosted zone.
	associationsModified, err := r.reconcileHostedZoneAssociations(awsClient, cd, hostedZoneID, vpcEndpoint, logger)
	if err != nil {
		logger.WithError(err).Error("could not reconcile the associations of the Hosted Zone")

		if err := r.setErrCondition(cd, "AssociatingVPCsToHostedZoneFailed", err, logger); err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, err
	}

	if associationsModified {
		if err := r.setReadyCondition(cd, corev1.ConditionFalse,
			"ReconciledAssociationsToVPCs",
			"reconciled the associations of all the required VPCs to the Private Hosted Zone for the VPC Endpoint",
			logger); err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
			return reconcile.Result{}, err
		}
	}

	if err := r.setReadyCondition(cd, corev1.ConditionTrue,
		"PrivateLinkAccessReady",
		"private link access is ready for use",
		logger); err != nil {
		logger.WithError(err).Error("failed to update condition on cluster deployment")
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// discoverNLBForCluster uses the AWS client to find the NLB for cluster's internal APIserver.
// The NLB created by the installer is named as {infraID}-int
func discoverNLBForCluster(client awsclient.Client, infraID string, logger log.FieldLogger) (string, error) {
	nlbName := infraID + "-int"
	nlbs, err := client.DescribeLoadBalancers(&elbv2.DescribeLoadBalancersInput{
		Names: aws.StringSlice([]string{nlbName}),
	})
	if err != nil {
		return "", errors.Wrap(err, "failed to describe load balancer for the cluster")
	}
	nlbARN := aws.StringValue(nlbs.LoadBalancers[0].LoadBalancerArn)
	nlbLog := logger.WithField("nlbARN", nlbARN)

	if err := waitForState(elbv2.LoadBalancerStateEnumActive, 1*time.Minute, func() (string, error) {
		resp, err := client.DescribeLoadBalancers(&elbv2.DescribeLoadBalancersInput{
			LoadBalancerArns: aws.StringSlice([]string{nlbARN}),
		})
		if err != nil {
			return "", errors.Wrap(err, "failed to find the NLB")
		}
		return *resp.LoadBalancers[0].State.Code, nil
	}, nlbLog); err != nil {
		nlbLog.WithError(err).Error("NLB did not become Available in time.")
		return "", err
	}
	return nlbARN, nil
}

func waitForState(state string, timeout time.Duration, currentState func() (string, error), logger log.FieldLogger) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return wait.PollImmediateUntil(1*time.Minute, func() (done bool, err error) {
		curr, err := currentState()
		if err != nil {
			logger.WithError(err).Error("failed to get the current state")
			return false, nil
		}
		if curr != state {
			logger.Debugf("Desired state %q is not yet achieved, currently %q", state, curr)
			return false, nil
		}
		return true, nil
	}, ctx.Done())
}

// reconcileVPCEndpointService ensure that a VPC endpoint service is created for cluster using nlbARN.
// It continously makes sure that only the HUB user/role is allowed to create endpoints to the service, and
// also makes sure that acceptance is not required when a VPC endpoint is created for the service.
// The function also continously makes sure that the NLB used by the service is always the one computed
// by the controller for the cluster.
func (r *ReconcileAWSPrivateLink) reconcileVPCEndpointService(awsClient *awsClient,
	cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata,
	nlbARN string,
	logger log.FieldLogger) (bool, *ec2.ServiceConfiguration, error) {
	modified := false

	serviceModified, serviceConfig, err := r.ensureVPCEndpointService(awsClient.user, cd, metadata, nlbARN, logger)
	if err != nil {
		logger.WithError(err).Error("error making sure VPC Endpoint Service exists for the cluster")
		return modified, nil, err
	}
	modified = serviceModified

	serviceLog := logger.WithField("serviceID", *serviceConfig.ServiceId)

	oldNLBs := sets.NewString(aws.StringValueSlice(serviceConfig.NetworkLoadBalancerArns)...)
	desiredNLBs := sets.NewString(nlbARN)
	if aws.BoolValue(serviceConfig.AcceptanceRequired) ||
		!desiredNLBs.Equal(oldNLBs) {
		modified = true
		modification := &ec2.ModifyVpcEndpointServiceConfigurationInput{
			AcceptanceRequired: aws.Bool(false),
			ServiceId:          serviceConfig.ServiceId,
		}

		if added := desiredNLBs.Difference(oldNLBs).List(); len(added) > 0 {
			modification.AddNetworkLoadBalancerArns = aws.StringSlice(added)
		}
		if removed := oldNLBs.Difference(desiredNLBs).List(); len(removed) > 0 {
			modification.RemoveNetworkLoadBalancerArns = aws.StringSlice(removed)
		}

		_, err := awsClient.user.ModifyVpcEndpointServiceConfiguration(modification)
		if err != nil {
			serviceLog.WithError(err).Error("error updating VPC Endpoint Service configuration to match the desired state")
			return modified, nil, err
		}
	}

	stsResp, err := awsClient.hub.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		serviceLog.WithError(err).Error("error getting the identity of the user that will create the VPC Endpoint")
		return modified, nil, err
	}

	permResp, err := awsClient.user.DescribeVpcEndpointServicePermissions(&ec2.DescribeVpcEndpointServicePermissionsInput{
		ServiceId: serviceConfig.ServiceId,
	})
	if err != nil {
		serviceLog.WithError(err).Error("error getting VPC Endpoint Service permissions")
		return modified, nil, err
	}

	oldPerms := sets.NewString()
	for _, allowed := range permResp.AllowedPrincipals {
		oldPerms.Insert(aws.StringValue(allowed.Principal))
	}
	// desiredPerms is the set of Allowed Principals that will be configured for the cluster's VPC Endpoint Service.
	// desiredPerms only contains the IAM entity used by Hive (defaultARN) by default but may contain additional Allowed Principal
	// ARNs as configured within cd.Spec.Platform.AWS.PrivateLink.AdditionalAllowedPrincipals.
	defaultARN := aws.StringValue(stsResp.Arn)
	desiredPerms := sets.NewString(defaultARN)
	if allowedPrincipals := cd.Spec.Platform.AWS.PrivateLink.AdditionalAllowedPrincipals; allowedPrincipals != nil {
		desiredPerms.Insert(*allowedPrincipals...)
	}

	if !desiredPerms.Equal(oldPerms) {
		modified = true
		input := &ec2.ModifyVpcEndpointServicePermissionsInput{
			ServiceId: serviceConfig.ServiceId,
		}
		if added := desiredPerms.Difference(oldPerms); len(added) > 0 {
			input.AddAllowedPrincipals = aws.StringSlice(added.List())
		}
		if removed := oldPerms.Difference(desiredPerms); len(removed) > 0 {
			input.RemoveAllowedPrincipals = aws.StringSlice(removed.List())
		}
		serviceLog.WithField("addAllowed", aws.StringValueSlice(input.AddAllowedPrincipals)).
			WithField("removeAllowed", aws.StringValueSlice(input.RemoveAllowedPrincipals)).
			Infof("updating VPC Endpoint Service permission to match the desired state")
		_, err := awsClient.user.ModifyVpcEndpointServicePermissions(input)
		if err != nil {
			serviceLog.WithField("addAllowed", aws.StringValueSlice(input.AddAllowedPrincipals)).
				WithField("removeAllowed", aws.StringValueSlice(input.RemoveAllowedPrincipals)).
				WithError(err).Error("error updating VPC Endpoint Service permission to match the desired state")
			return modified, nil, err
		}
	}

	// Update status with modified additionalAllowedPrincipals
	// Permissions were modified on the vpc endpoint service or were not modified (because they were
	// correct) and status is empty so status must be updated.
	// Checking for empty status avoids a rare hotloop that could occur when the vpc endpoint is deleted externally
	// and recreated by hive which wipes out the status in ensureVPCEndpointService(). If the allowed principals on
	// the vpc endpoint service were correct, no modification would occur but status would remain empty resulting in
	// shouldSync() always returning true.
	specHasAdditionalAllowedPrincipals := cd.Spec.Platform.AWS.PrivateLink.AdditionalAllowedPrincipals != nil
	additionalAllowedPrincipalsStatusEmpty := cd.Status.Platform.AWS.PrivateLink.VPCEndpointService.AdditionalAllowedPrincipals == nil
	defaultAllowedPrincipalStatusEmpty := cd.Status.Platform.AWS.PrivateLink.VPCEndpointService.DefaultAllowedPrincipal == nil
	if modified || (specHasAdditionalAllowedPrincipals && additionalAllowedPrincipalsStatusEmpty) || defaultAllowedPrincipalStatusEmpty {
		initPrivateLinkStatus(cd)
		// Remove the defaultARN from the list of AdditionalAllowedPrincipals to be recorded in
		// cd.Status.Platform.AWS.PrivateLink.VPCEndpointService.AdditionalAllowedPrincipals.
		// The defaultARN will be stored in a separate status field
		// cd.Status.Platform.AWS.PrivateLink.VPCEndpointService.DefaultAllowedPrincipal
		desiredPerms.Delete(defaultARN)
		desiredPermsSlice := desiredPerms.List() // sorted by sets.List()
		if len(desiredPermsSlice) == 0 {
			cd.Status.Platform.AWS.PrivateLink.VPCEndpointService.AdditionalAllowedPrincipals = nil
		} else {
			cd.Status.Platform.AWS.PrivateLink.VPCEndpointService.AdditionalAllowedPrincipals = &desiredPermsSlice
		}
		cd.Status.Platform.AWS.PrivateLink.VPCEndpointService.DefaultAllowedPrincipal = &defaultARN
		if err := r.updatePrivateLinkStatus(cd, logger); err != nil {
			logger.WithError(err).Error("error updating clusterdeployment status with vpcEndpointService additionalAllowedPrincipals")
			return modified, nil, err
		}
	}

	return modified, serviceConfig, nil
}

func (r *ReconcileAWSPrivateLink) ensureVPCEndpointService(awsClient awsclient.Client, cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata, clusterNLB string, logger log.FieldLogger) (bool, *ec2.ServiceConfiguration, error) {
	modified := false
	tag := ec2FilterForCluster(metadata)
	serviceLog := logger.WithField("tag:key", aws.StringValue(tag.Name)).WithField("tag:value", aws.StringValueSlice(tag.Values))

	var serviceConfig *ec2.ServiceConfiguration
	resp, err := awsClient.DescribeVpcEndpointServiceConfigurations(&ec2.DescribeVpcEndpointServiceConfigurationsInput{
		Filters: []*ec2.Filter{tag},
	})
	if err != nil {
		serviceLog.WithError(err).Error("failed to get VPC Endpoint Service for cluster")
		return modified, nil, err
	}
	if len(resp.ServiceConfigurations) == 0 {
		modified = true
		serviceConfig, err = createVPCEndpointService(awsClient, cd, metadata, clusterNLB, logger)
		if err != nil {
			logger.WithError(err).Error("failed to create VPC Endpoint Service for cluster")
			return modified, nil, errors.Wrap(err, "failed to create VPC Endpoint Service for cluster")
		}
	} else {
		serviceConfig = resp.ServiceConfigurations[0]
	}

	initPrivateLinkStatus(cd)
	cd.Status.Platform.AWS.PrivateLink.VPCEndpointService = hivev1aws.VPCEndpointService{
		ID:   *serviceConfig.ServiceId,
		Name: *serviceConfig.ServiceName,
	}
	if err := r.updatePrivateLinkStatus(cd, logger); err != nil {
		logger.WithError(err).Error("error updating clusterdeployment status with vpcEndpointService")
		return modified, nil, err
	}

	return modified, serviceConfig, nil
}

func createVPCEndpointService(awsClient awsclient.Client, cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata, clusterNLB string, logger log.FieldLogger) (*ec2.ServiceConfiguration, error) {
	resp, err := awsClient.CreateVpcEndpointServiceConfiguration(&ec2.CreateVpcEndpointServiceConfigurationInput{
		AcceptanceRequired:      aws.Bool(false),
		NetworkLoadBalancerArns: aws.StringSlice([]string{clusterNLB}),
		TagSpecifications:       []*ec2.TagSpecification{ec2TagSpecification(metadata, "vpc-endpoint-service")},
	})
	if err != nil {
		logger.WithError(err).Error("failed to create endpoint service for cluster")
		return nil, err
	}

	serviceLog := logger.WithField("serviceID", *resp.ServiceConfiguration.ServiceId)

	if err := waitForState(ec2.ServiceStateAvailable, 1*time.Minute, func() (string, error) {
		resp, err := awsClient.DescribeVpcEndpointServiceConfigurations(&ec2.DescribeVpcEndpointServiceConfigurationsInput{
			ServiceIds: aws.StringSlice([]string{*resp.ServiceConfiguration.ServiceId}),
		})
		if err != nil {
			return "", errors.Wrap(err, "failed to find the VPC endpoint service")
		}
		return *resp.ServiceConfigurations[0].ServiceState, nil
	}, serviceLog); err != nil {
		serviceLog.WithError(err).Error("VPC Endpoint Service did not become Available in time.")
		return nil, err
	}

	return resp.ServiceConfiguration, nil
}

// reconcileVPCEndpoint ensures that a VPC endpoint is created for the VPC endpoint service in the
// HUB account.
// It chooses a VPC from the list of VPCs given to the controller using criteria like
//   - VPC that is in the same region as the VPC endpoint service
//   - VPC that has at least one subnet in the AZs supported by the VPC endpoint service
//   - VPC that has the fewest existing VPC endpoints ("spread" strategy)
//
// It currently doesn't manage any properties of the VPC endpoint once it is created.
func (r *ReconcileAWSPrivateLink) reconcileVPCEndpoint(awsClient *awsClient,
	cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata,
	vpcEndpointService *ec2.ServiceConfiguration,
	logger log.FieldLogger) (bool, *ec2.VpcEndpoint, error) {
	modified := false
	tag := ec2FilterForCluster(metadata)
	endpointLog := logger.WithField("tag:key", aws.StringValue(tag.Name)).WithField("tag:value", aws.StringValueSlice(tag.Values))

	var vpcEndpoint *ec2.VpcEndpoint
	resp, err := awsClient.hub.DescribeVpcEndpoints(&ec2.DescribeVpcEndpointsInput{
		Filters: []*ec2.Filter{tag},
	})
	if err != nil {
		endpointLog.WithError(err).Error("error getting VPC Endpoint")
		return modified, nil, err
	}
	if len(resp.VpcEndpoints) == 0 {
		modified = true
		vpcEndpoint, err = r.createVPCEndpoint(awsClient.hub, cd, metadata, vpcEndpointService, logger)
		if err != nil {
			logger.WithError(err).Error("error creating VPC Endpoint for service")
			return modified, nil, err
		}
	} else {
		vpcEndpoint = resp.VpcEndpoints[0]
	}

	initPrivateLinkStatus(cd)
	cd.Status.Platform.AWS.PrivateLink.VPCEndpointID = *vpcEndpoint.VpcEndpointId
	if err := r.updatePrivateLinkStatus(cd, logger); err != nil {
		logger.WithError(err).Error("error updating clusterdeployment status with vpcEndpointID")
		return modified, nil, err
	}

	return modified, vpcEndpoint, nil
}

func (r *ReconcileAWSPrivateLink) createVPCEndpoint(awsClient awsclient.Client,
	cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata,
	vpcEndpointService *ec2.ServiceConfiguration,
	logger log.FieldLogger) (*ec2.VpcEndpoint, error) {
	chosen, err := r.chooseVPCForVPCEndpoint(awsClient, cd, *vpcEndpointService.ServiceName, logger)
	if err != nil {
		logger.WithError(err).Error("failed to choose VPC for the VPC Endpoint from the inventory")
		return nil, err
	}

	subnetIDs := make([]string, 0, len(chosen.Subnets))
	for _, subnet := range chosen.Subnets {
		subnetIDs = append(subnetIDs, subnet.SubnetID)
	}
	resp, err := awsClient.CreateVpcEndpoint(&ec2.CreateVpcEndpointInput{
		PrivateDnsEnabled: aws.Bool(false),
		ServiceName:       vpcEndpointService.ServiceName,
		SubnetIds:         aws.StringSlice(subnetIDs),
		TagSpecifications: []*ec2.TagSpecification{ec2TagSpecification(metadata, "vpc-endpoint")},
		VpcEndpointType:   aws.String(ec2.VpcEndpointTypeInterface),
		VpcId:             aws.String(chosen.VPCID),
	})
	if err != nil {
		logger.WithError(err).Error("error creating VPC Endpoint")
		return nil, err
	}
	endpointLog := logger.WithField("endpointID", *resp.VpcEndpoint.VpcEndpointId)

	if err := waitForState("available", 1*time.Minute, func() (string, error) {
		resp, err := awsClient.DescribeVpcEndpoints(&ec2.DescribeVpcEndpointsInput{
			VpcEndpointIds: aws.StringSlice([]string{*resp.VpcEndpoint.VpcEndpointId}),
		})
		if err != nil {
			return "", errors.Wrap(err, "failed to get VPC endpoint")
		}
		return *resp.VpcEndpoints[0].State, nil
	}, endpointLog); err != nil {
		endpointLog.WithError(err).Error("VPC Endpoint did not become Available in time")
		return nil, err
	}

	return resp.VpcEndpoint, nil
}

// reconcileHostedZone ensures that a Private Hosted Zone apiDomain exists for the VPC
// where VPC endpoint was created. It also make sure the DNS zone has an ALIAS record pointing
// to the regional DNS name of the VPC endpoint.
func (r *ReconcileAWSPrivateLink) reconcileHostedZone(awsClient *awsClient,
	cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata,
	vpcEndpoint *ec2.VpcEndpoint, apiDomain string,
	logger log.FieldLogger) (bool, string, error) {
	modified, hostedZoneID, err := r.ensureHostedZone(awsClient.hub, cd, vpcEndpoint, apiDomain, logger)
	if err != nil {
		logger.WithError(err).Error("error ensuring Hosted Zone was created")
		return modified, "", err
	}

	hzLog := logger.WithField("hostedZoneID", hostedZoneID)

	rSet, err := r.recordSet(awsClient.hub, apiDomain, vpcEndpoint)
	if err != nil {
		hzLog.WithField("vpcEndpoint", aws.StringValue(vpcEndpoint.VpcEndpointId)).
			WithError(err).Error("error generating DNS records")
		return modified, "", err
	}

	_, err = awsClient.hub.ChangeResourceRecordSets(&route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(hostedZoneID),
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{{
				Action:            aws.String(route53.ChangeActionUpsert),
				ResourceRecordSet: rSet,
			}},
		},
	})
	if err != nil {
		hzLog.WithError(err).Error("error adding record to Hosted Zone for VPC Endpoint")
		return modified, "", err
	}
	return modified, hostedZoneID, nil
}

func (r *ReconcileAWSPrivateLink) recordSet(awsClient awsclient.Client, apiDomain string, vpcEndpoint *ec2.VpcEndpoint) (*route53.ResourceRecordSet, error) {
	rSet := &route53.ResourceRecordSet{
		Name: aws.String(apiDomain),
	}
	switch r.controllerconfig.DNSRecordType {
	case hivev1.ARecordAWSPrivateLinkDNSRecordType:
		rSet.Type = aws.String("A")
		rSet.TTL = aws.Int64(10)

		// get the ips from the elastic networking interfaces attached to the VPC endpoint
		enis := vpcEndpoint.NetworkInterfaceIds
		if len(enis) == 0 {
			return nil, errors.New("No network interfaces attached to the vpc endpoint")
		}
		res, err := awsClient.DescribeNetworkInterfaces(&ec2.DescribeNetworkInterfacesInput{NetworkInterfaceIds: vpcEndpoint.NetworkInterfaceIds})
		if err != nil {
			return nil, errors.Wrap(err, "failed to list network interfaces attached to the vpc endpoint")
		}
		if len(res.NetworkInterfaces) == 0 {
			return nil, errors.New("No network interfaces attached to the vpc endpoint")
		}

		var ips []string
		for _, eni := range res.NetworkInterfaces {
			ips = append(ips, aws.StringValue(eni.PrivateIpAddress))
		}
		sort.Strings(ips)

		for _, ip := range ips {
			rSet.ResourceRecords = append(rSet.ResourceRecords, &route53.ResourceRecord{
				Value: aws.String(ip),
			})
		}

	default: // Alias is the default case.
		rSet.Type = aws.String("A")
		rSet.AliasTarget = &route53.AliasTarget{
			DNSName:              vpcEndpoint.DnsEntries[0].DnsName,
			HostedZoneId:         vpcEndpoint.DnsEntries[0].HostedZoneId,
			EvaluateTargetHealth: aws.Bool(false),
		}
	}
	return rSet, nil
}

func (r *ReconcileAWSPrivateLink) ensureHostedZone(awsClient awsclient.Client,
	cd *hivev1.ClusterDeployment,
	endpoint *ec2.VpcEndpoint, apiDomain string,
	logger log.FieldLogger) (bool, string, error) {
	modified := false
	hzID, err := findHostedZone(awsClient, *endpoint.VpcId, cd.Spec.Platform.AWS.Region, apiDomain, logger)
	if err != nil && errors.Is(err, errNoHostedZoneFoundForVPC) {
		modified = true
		hzID, err = r.createHostedZone(awsClient, cd, endpoint, apiDomain, logger)
		if err != nil {
			return modified, "", err
		}
	}
	if err != nil {
		logger.WithError(err).Error("failed to get Hosted Zone")
		return modified, "", err
	}

	initPrivateLinkStatus(cd)
	cd.Status.Platform.AWS.PrivateLink.HostedZoneID = hzID
	if err := r.updatePrivateLinkStatus(cd, logger); err != nil {
		logger.WithError(err).Error("failed to update the hosted zone ID for cluster deployment")
		return modified, "", err
	}

	return modified, hzID, nil
}

var errNoHostedZoneFoundForVPC = errors.New("no hosted zone found")

// findHostedZone finds a Private Hosted Zone for apiDomain that is associated with the given
// VPC.
// If no such hosted zone exists, it return an errNoHostedZoneFoundForVPC error.
func findHostedZone(awsClient awsclient.Client, vpcID, vpcRegion, apiDomain string, logger log.FieldLogger) (string, error) {
	input := &route53.ListHostedZonesByVPCInput{
		VPCId:     aws.String(vpcID),
		VPCRegion: aws.String(vpcRegion),

		MaxItems: aws.String("100"),
	}
	var nextToken *string
	for {
		input.NextToken = nextToken
		resp, err := awsClient.ListHostedZonesByVPC(input)
		if err != nil {
			return "", err
		}
		for _, summary := range resp.HostedZoneSummaries {
			if strings.EqualFold(apiDomain, strings.TrimSuffix(aws.StringValue(summary.Name), ".")) {
				return *summary.HostedZoneId, nil
			}
		}
		if resp.NextToken == nil {
			break
		}
		nextToken = resp.NextToken
	}
	return "", errNoHostedZoneFoundForVPC
}

func (r *ReconcileAWSPrivateLink) createHostedZone(awsClient awsclient.Client,
	cd *hivev1.ClusterDeployment,
	endpoint *ec2.VpcEndpoint, apiDomain string,
	logger log.FieldLogger) (string, error) {
	hzLog := logger.WithField("vpcID", *endpoint.VpcId).WithField("apiDomain", apiDomain)
	resp, err := awsClient.CreateHostedZone(&route53.CreateHostedZoneInput{
		CallerReference: aws.String(time.Now().String()),
		Name:            aws.String(apiDomain),
		HostedZoneConfig: &route53.HostedZoneConfig{
			PrivateZone: aws.Bool(true),
		},
		VPC: &route53.VPC{
			VPCId:     endpoint.VpcId,
			VPCRegion: aws.String(cd.Spec.Platform.AWS.Region),
		},
	})
	if err != nil {
		hzLog.WithError(err).Error("could not create Private Hosted Zone")
		return "", err
	}

	return *resp.HostedZone.Id, nil
}

// reconcileHostedZoneAssociations ensures that the all the VPCs in the associatedVPCs list from
// the controller config are associated to the PHZ hostedZoneID.
func (r *ReconcileAWSPrivateLink) reconcileHostedZoneAssociations(awsClient *awsClient,
	cd *hivev1.ClusterDeployment,
	hostedZoneID string, vpcEndpoint *ec2.VpcEndpoint,
	logger log.FieldLogger) (bool, error) {
	hzLog := logger.WithField("hostedZoneID", hostedZoneID)
	modified := false
	vpcInfo := r.controllerconfig.DeepCopy().AssociatedVPCs
	vpcIdx := map[string]int{}
	for i, v := range vpcInfo {
		vpcIdx[v.VPCID] = i
	}

	zoneResp, err := awsClient.hub.GetHostedZone(&route53.GetHostedZoneInput{
		Id: aws.String(hostedZoneID),
	})
	if err != nil {
		hzLog.WithError(err).Error("failed to get the Hosted Zone")
		return modified, err
	}

	oldVPCs := sets.NewString()
	for _, vpc := range zoneResp.VPCs {
		id := aws.StringValue(vpc.VPCId)
		oldVPCs.Insert(id)
		if _, ok := vpcIdx[id]; !ok { // make sure we have info for all VPCs for later use
			vpcInfo = append(vpcInfo, hivev1.AWSAssociatedVPC{
				AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{
					VPCID:  id,
					Region: aws.StringValue(vpc.VPCRegion),
				},
			})
			vpcIdx[id] = len(vpcInfo) - 1
		}
	}
	desiredVPCs := sets.NewString(*vpcEndpoint.VpcId)
	for _, vpc := range r.controllerconfig.AssociatedVPCs {
		desiredVPCs.Insert(vpc.VPCID)
	}

	added := desiredVPCs.Difference(oldVPCs).List()
	removed := oldVPCs.Difference(desiredVPCs).List()
	if len(added) > 0 || len(removed) > 0 {
		modified = true
		hzLog.WithFields(log.Fields{
			"associate":    added,
			"disassociate": removed,
		}).Debug("updating the VPCs attached to the Hosted Zone")
	}

	for _, vpc := range added {
		vpcLog := hzLog.WithField("vpc", vpc)
		info := vpcInfo[vpcIdx[vpc]]

		awsAssociationClient := awsClient.hub
		if info.CredentialsSecretRef != nil {
			// since this VPC is in different account we need to authorize before continuing
			_, err := awsClient.hub.CreateVPCAssociationAuthorization(&route53.CreateVPCAssociationAuthorizationInput{
				HostedZoneId: aws.String(hostedZoneID),
				VPC: &route53.VPC{
					VPCId:     aws.String(vpc),
					VPCRegion: aws.String(info.Region),
				},
			})
			if err != nil {
				vpcLog.WithError(err).Error("failed to create authorization for association of the Hosted Zone to the VPC")
				return modified, err
			}

			awsAssociationClient, err = r.awsClientFn(r.Client, awsclient.Options{
				Region: info.Region,
				CredentialsSource: awsclient.CredentialsSource{
					Secret: &awsclient.SecretCredentialsSource{
						Namespace: controllerutils.GetHiveNamespace(),
						Ref:       info.CredentialsSecretRef,
					},
				},
			})
			if err != nil {
				vpcLog.WithError(err).Error("failed to create AWS client for association of the Hosted Zone to the VPC")
				return modified, err
			}
		}

		_, err = awsAssociationClient.AssociateVPCWithHostedZone(&route53.AssociateVPCWithHostedZoneInput{
			HostedZoneId: aws.String(hostedZoneID),
			VPC: &route53.VPC{
				VPCId:     aws.String(vpc),
				VPCRegion: aws.String(info.Region),
			},
		})
		if err != nil {
			hzLog.WithField("vpc", vpc).WithError(err).Error("failed to associate the Hosted Zone to the VPC")
			return modified, err
		}

		if info.CredentialsSecretRef != nil {
			// since we created an authorization and association is complete, we should remove the object
			// as recommended by AWS best practices.
			_, err := awsClient.hub.DeleteVPCAssociationAuthorization(&route53.DeleteVPCAssociationAuthorizationInput{
				HostedZoneId: aws.String(hostedZoneID),
				VPC: &route53.VPC{
					VPCId:     aws.String(vpc),
					VPCRegion: aws.String(info.Region),
				},
			})
			if err != nil {
				vpcLog.WithError(err).Error("failed to delete authorization for association of the Hosted Zone to the VPC")
				return modified, err
			}
		}
	}
	for _, vpc := range removed {
		vpcLog := hzLog.WithField("vpc", vpc)
		info := vpcInfo[vpcIdx[vpc]]
		_, err = awsClient.hub.DisassociateVPCFromHostedZone(&route53.DisassociateVPCFromHostedZoneInput{
			HostedZoneId: aws.String(hostedZoneID),
			VPC: &route53.VPC{
				VPCId:     aws.String(vpc),
				VPCRegion: aws.String(info.Region),
			},
		})
		if err != nil {
			vpcLog.WithError(err).Error("failed to disassociate the Hosted Zone to the VPC")
			return modified, err
		}
	}

	return modified, nil
}

// ec2FilterForCluster is the filter that is used to find the resources tied to the cluster.
func ec2FilterForCluster(metadata *hivev1.ClusterMetadata) *ec2.Filter {
	return &ec2.Filter{
		Name:   aws.String("tag:hive.openshift.io/private-link-access-for"),
		Values: aws.StringSlice([]string{metadata.InfraID}),
	}
}

// ec2TagSpecification is the list of tags that should be added to the resources
// created for the cluster.
func ec2TagSpecification(metadata *hivev1.ClusterMetadata, resource string) *ec2.TagSpecification {
	return &ec2.TagSpecification{
		ResourceType: aws.String(resource),
		Tags: []*ec2.Tag{{
			Key:   aws.String("hive.openshift.io/private-link-access-for"),
			Value: aws.String(metadata.InfraID),
		}, {
			Key:   aws.String("Name"),
			Value: aws.String(metadata.InfraID + "-" + resource),
		}},
	}
}

func filterErrorMessage(err error) string {
	skipRequestIDRE := regexp.MustCompile(`(request id|Request ID): ([-0-9a-f]+)`)
	return skipRequestIDRE.ReplaceAllString(err.Error(), "${1}: XXXX")
}

type awsClient struct {
	hub  awsclient.Client
	user awsclient.Client
}

func newAWSClient(r *ReconcileAWSPrivateLink, cd *hivev1.ClusterDeployment) (*awsClient, error) {
	uClient, err := r.awsClientFn(r.Client, awsclient.Options{
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
	})
	if err != nil {
		return nil, err
	}
	hClient, err := r.awsClientFn(r.Client, awsclient.Options{
		Region: cd.Spec.Platform.AWS.Region,
		CredentialsSource: awsclient.CredentialsSource{
			Secret: &awsclient.SecretCredentialsSource{
				Namespace: controllerutils.GetHiveNamespace(),
				Ref:       &r.controllerconfig.CredentialsSecretRef,
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return &awsClient{hub: hClient, user: uClient}, nil
}

// initialURL returns the initial API URL for the ClusterProvision.
func initialURL(c client.Client, key client.ObjectKey) (string, error) {
	kubeconfigSecret := &corev1.Secret{}
	if err := c.Get(
		context.Background(),
		key,
		kubeconfigSecret,
	); err != nil {
		return "", err
	}
	cfg, err := restConfigFromSecret(kubeconfigSecret)
	if err != nil {
		return "", errors.Wrap(err, "failed to load the kubeconfig")
	}

	u, err := url.Parse(cfg.Host)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(u.Hostname(), "."), nil
}

func restConfigFromSecret(kubeconfigSecret *corev1.Secret) (*rest.Config, error) {
	kubeconfigData := kubeconfigSecret.Data[constants.RawKubeconfigSecretKey]
	if len(kubeconfigData) == 0 {
		kubeconfigData = kubeconfigSecret.Data[constants.KubeconfigSecretKey]
	}
	if len(kubeconfigData) == 0 {
		return nil, errors.New("kubeconfig secret does not contain necessary data")
	}
	config, err := clientcmd.Load(kubeconfigData)
	if err != nil {
		return nil, err
	}
	kubeConfig := clientcmd.NewDefaultClientConfig(*config, &clientcmd.ConfigOverrides{})
	return kubeConfig.ClientConfig()
}

// awsErrCodeEquals returns true if the error matches all these conditions:
//   - err is of type awserr.Error
//   - Error.Code() equals code
func awsErrCodeEquals(err error, code string) bool {
	if err == nil {
		return false
	}
	var awsErr awserr.Error
	if errors.As(err, &awsErr) {
		return awsErr.Code() == code
	}
	return false
}

// ReadAWSPrivateLinkControllerConfigFile reads the configuration from the env
// and unmarshals. If the env is set to a file but that file doesn't exist it returns
// a zero-value configuration.
func ReadAWSPrivateLinkControllerConfigFile() (*hivev1.AWSPrivateLinkConfig, error) {
	fPath := os.Getenv(constants.AWSPrivateLinkControllerConfigFileEnvVar)
	if len(fPath) == 0 {
		return nil, nil
	}

	config := &hivev1.AWSPrivateLinkConfig{}

	fileBytes, err := os.ReadFile(fPath)
	if os.IsNotExist(err) {
		return config, nil
	}
	if err != nil {
		return config, errors.Wrap(err, "failed to read the aws privatelink controller config file")
	}
	if err := json.Unmarshal(fileBytes, &config); err != nil {
		return config, err
	}

	return config, nil
}

var retryBackoff = wait.Backoff{
	Steps:    5,
	Duration: 1 * time.Second,
	Factor:   1.0,
	Jitter:   0.1,
}

func (r *ReconcileAWSPrivateLink) updatePrivateLinkStatus(cd *hivev1.ClusterDeployment, logger log.FieldLogger) error {
	return retry.RetryOnConflict(retryBackoff, func() error {
		curr := &hivev1.ClusterDeployment{}
		err := r.Client.Get(context.TODO(), types.NamespacedName{Namespace: cd.Namespace, Name: cd.Name}, curr)
		if err != nil {
			return err
		}

		initPrivateLinkStatus(curr)
		curr.Status.Platform.AWS.PrivateLink = cd.Status.Platform.AWS.PrivateLink
		return r.Client.Status().Update(context.TODO(), curr)
	})
}

func initPrivateLinkStatus(cd *hivev1.ClusterDeployment) {
	if cd.Status.Platform == nil {
		cd.Status.Platform = &hivev1.PlatformStatus{}
	}
	if cd.Status.Platform.AWS == nil {
		cd.Status.Platform.AWS = &hivev1aws.PlatformStatus{}
	}
	if cd.Status.Platform.AWS.PrivateLink == nil {
		cd.Status.Platform.AWS.PrivateLink = &hivev1aws.PrivateLinkAccessStatus{}
	}
}

func updateAnnotations(client client.Client, cd *hivev1.ClusterDeployment) error {
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
