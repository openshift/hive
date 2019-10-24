package clusterdeployment

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"sort"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"

	routev1 "github.com/openshift/api/route/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/tools/clientcmd"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	apihelpers "github.com/openshift/hive/pkg/apis/helpers"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/controller/images"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/imageset"
	"github.com/openshift/hive/pkg/install"
)

// controllerKind contains the schema.GroupVersionKind for this controller type.
var controllerKind = hivev1.SchemeGroupVersion.WithKind("ClusterDeployment")

const (
	controllerName     = "clusterDeployment"
	defaultRequeueTime = 10 * time.Second
	maxProvisions      = 3

	adminSSHKeySecretKey  = "ssh-publickey"
	adminKubeconfigKey    = "kubeconfig"
	rawAdminKubeconfigKey = "raw-kubeconfig"

	clusterImageSetNotFoundReason = "ClusterImageSetNotFound"
	clusterImageSetFoundReason    = "ClusterImageSetFound"

	dnsNotReadyReason  = "DNSNotReady"
	dnsReadyReason     = "DNSReady"
	dnsReadyAnnotation = "hive.openshift.io/dnsready"

	deleteAfterAnnotation    = "hive.openshift.io/delete-after" // contains a duration after which the cluster should be cleaned up.
	tryInstallOnceAnnotation = "hive.openshift.io/try-install-once"
)

var (
	metricCompletedInstallJobRestarts = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hive_cluster_deployment_completed_install_restart",
			Help:    "Distribution of the number of restarts for all completed cluster installations.",
			Buckets: []float64{0, 2, 10, 20, 50},
		},
		[]string{"cluster_type"},
	)
	metricInstallJobDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "hive_cluster_deployment_install_job_duration_seconds",
			Help:    "Distribution of the runtime of completed install jobs.",
			Buckets: []float64{60, 300, 600, 1200, 1800, 2400, 3000, 3600},
		},
	)
	metricInstallDelaySeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "hive_cluster_deployment_install_job_delay_seconds",
			Help:    "Time between cluster deployment creation and creation of the job to install/provision the cluster.",
			Buckets: []float64{30, 60, 120, 300, 600, 1200, 1800},
		},
	)
	metricImageSetDelaySeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "hive_cluster_deployment_imageset_job_delay_seconds",
			Help:    "Time between cluster deployment creation and creation of the job which resolves the installer image to use for a ClusterImageSet.",
			Buckets: []float64{10, 30, 60, 300, 600, 1200, 1800},
		},
	)
	metricClustersCreated = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hive_cluster_deployments_created_total",
		Help: "Counter incremented every time we observe a new cluster.",
	},
		[]string{"cluster_type"},
	)
	metricClustersInstalled = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hive_cluster_deployments_installed_total",
		Help: "Counter incremented every time we observe a successful installation.",
	},
		[]string{"cluster_type"},
	)
	metricClustersDeleted = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hive_cluster_deployments_deleted_total",
		Help: "Counter incremented every time we observe a deleted cluster.",
	},
		[]string{"cluster_type"},
	)
	metricDNSDelaySeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "hive_cluster_deployment_dns_delay_seconds",
			Help:    "Time between cluster deployment with spec.manageDNS creation and the DNSZone becoming ready.",
			Buckets: []float64{10, 30, 60, 300, 600, 1200, 1800},
		},
	)
)

func init() {
	metrics.Registry.MustRegister(metricInstallJobDuration)
	metrics.Registry.MustRegister(metricCompletedInstallJobRestarts)
	metrics.Registry.MustRegister(metricInstallDelaySeconds)
	metrics.Registry.MustRegister(metricImageSetDelaySeconds)
	metrics.Registry.MustRegister(metricClustersCreated)
	metrics.Registry.MustRegister(metricClustersInstalled)
	metrics.Registry.MustRegister(metricClustersDeleted)
	metrics.Registry.MustRegister(metricDNSDelaySeconds)
}

// Add creates a new ClusterDeployment controller and adds it to the manager with default RBAC.
func Add(mgr manager.Manager) error {
	return AddToManager(mgr, NewReconciler(mgr))
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager) reconcile.Reconciler {
	logger := log.WithField("controller", controllerName)
	return &ReconcileClusterDeployment{
		Client:                        controllerutils.NewClientWithMetricsOrDie(mgr, controllerName),
		scheme:                        mgr.GetScheme(),
		logger:                        logger,
		expectations:                  controllerutils.NewExpectations(logger),
		remoteClusterAPIClientBuilder: controllerutils.BuildClusterAPIClientFromKubeconfig,
	}
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler) error {
	cdReconciler, ok := r.(*ReconcileClusterDeployment)
	if !ok {
		return errors.New("reconciler supplied is not a ReconcileClusterDeployment")
	}

	c, err := controller.New("clusterdeployment-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: controllerutils.GetConcurrentReconciles()})
	if err != nil {
		log.WithField("controller", controllerName).WithError(err).Error("Error getting new cluster deployment")
		return err
	}

	// Watch for changes to ClusterDeployment
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		log.WithField("controller", controllerName).WithError(err).Error("Error watching cluster deployment")
		return err
	}

	// Watch for provisions
	if err := cdReconciler.watchClusterProvisions(c); err != nil {
		return err
	}

	// Watch for jobs created by a ClusterDeployment:
	err = c.Watch(&source.Kind{Type: &batchv1.Job{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &hivev1.ClusterDeployment{},
	})
	if err != nil {
		log.WithField("controller", controllerName).WithError(err).Error("Error watching cluster deployment job")
		return err
	}

	// Watch for pods created by an install job
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(selectorPodWatchHandler),
	})
	if err != nil {
		log.WithField("controller", controllerName).WithError(err).Error("Error watching cluster deployment pods")
		return err
	}

	// Watch for deprovision requests created by a ClusterDeployment
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeprovisionRequest{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &hivev1.ClusterDeployment{},
	})
	if err != nil {
		log.WithField("controller", controllerName).WithError(err).Error("Error watching deprovision request created by cluster deployment")
		return err
	}

	// Watch for dnszones created by a ClusterDeployment
	err = c.Watch(&source.Kind{Type: &hivev1.DNSZone{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &hivev1.ClusterDeployment{},
	})
	if err != nil {
		log.WithField("controller", controllerName).WithError(err).Error("Error watching cluster deployment dnszones")
		return err
	}

	// Watch for changes to SyncSetInstance
	err = c.Watch(&source.Kind{Type: &hivev1.SyncSetInstance{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &hivev1.ClusterDeployment{},
	})
	if err != nil {
		return fmt.Errorf("cannot start watch on syncset instance: %v", err)
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileClusterDeployment{}

// ReconcileClusterDeployment reconciles a ClusterDeployment object
type ReconcileClusterDeployment struct {
	client.Client
	scheme *runtime.Scheme
	logger log.FieldLogger

	// A TTLCache of clusterprovision creates each clusterdeployment expects to see
	expectations controllerutils.ExpectationsInterface

	// remoteClusterAPIClientBuilder is a function pointer to the function that builds a client for the
	// remote cluster's cluster-api
	remoteClusterAPIClientBuilder func(string, string) (client.Client, error)
}

// Reconcile reads that state of the cluster for a ClusterDeployment object and makes changes based on the state read
// and what is in the ClusterDeployment.Spec
//
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
//
func (r *ReconcileClusterDeployment) Reconcile(request reconcile.Request) (result reconcile.Result, returnErr error) {
	start := time.Now()
	cdLog := r.logger.WithFields(log.Fields{
		"controller":        controllerName,
		"clusterDeployment": request.Name,
		"namespace":         request.Namespace,
	})

	// For logging, we need to see when the reconciliation loop starts and ends.
	cdLog.Info("reconciling cluster deployment")
	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(controllerName).Observe(dur.Seconds())
		cdLog.WithField("elapsed", dur).WithField("result", result).Info("reconcile complete")
	}()

	// Fetch the ClusterDeployment instance
	cd := &hivev1.ClusterDeployment{}
	err := r.Get(context.TODO(), request.NamespacedName, cd)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			cdLog.Info("cluster deployment Not Found")
			r.expectations.DeleteExpectations(request.NamespacedName.String())
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		cdLog.WithError(err).Error("Error getting cluster deployment")
		return reconcile.Result{}, err
	}

	return r.reconcile(request, cd, cdLog)
}

func (r *ReconcileClusterDeployment) reconcile(request reconcile.Request, cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (result reconcile.Result, returnErr error) {
	origCD := cd
	cd = cd.DeepCopy()

	// TODO: We may want to remove this fix in future.
	// Handle pre-existing clusters with older status version structs that did not have the new
	// cluster version mandatory fields defined.
	// NOTE: removing this is causing the imageset job to fail. Please leave it in until
	// we can determine what needs to be fixed.
	controllerutils.FixupEmptyClusterVersionFields(&cd.Status.ClusterVersionStatus)
	if !reflect.DeepEqual(origCD.Status, cd.Status) {
		cdLog.Info("correcting empty cluster version fields")
		err := r.Status().Update(context.TODO(), cd)
		if err != nil {
			cdLog.WithError(err).Error("error updating cluster deployment status")
			return reconcile.Result{}, err
		}
		return reconcile.Result{
			Requeue:      true,
			RequeueAfter: defaultRequeueTime,
		}, nil
	}

	// Set platform label on the ClusterDeployment
	if platform := getClusterPlatform(cd); cd.Labels[hivev1.HiveClusterPlatformLabel] != platform {
		if cd.Labels == nil {
			cd.Labels = make(map[string]string)
		}
		if cd.Labels[hivev1.HiveClusterPlatformLabel] != "" {
			cdLog.Warnf("changing the value of %s from %s to %s", hivev1.HiveClusterPlatformLabel,
				cd.Labels[hivev1.HiveClusterPlatformLabel], platform)
		}
		cd.Labels[hivev1.HiveClusterPlatformLabel] = platform
		err := r.Update(context.TODO(), cd)
		if err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "failed to set cluster platform label")
		}
		return reconcile.Result{}, err
	}

	if cd.DeletionTimestamp != nil {
		if !controllerutils.HasFinalizer(cd, hivev1.FinalizerDeprovision) {
			clearUnderwaySecondsMetrics(cd)
			return reconcile.Result{}, nil
		}

		// Deprovision still underway, report metric for this cluster.
		hivemetrics.MetricClusterDeploymentDeprovisioningUnderwaySeconds.WithLabelValues(
			cd.Name,
			cd.Namespace,
			hivemetrics.GetClusterDeploymentType(cd)).Set(
			time.Since(cd.DeletionTimestamp.Time).Seconds())

		// If the cluster never made it to installed, make sure we clear the provisioning
		// underway metric.
		if !cd.Spec.Installed {
			hivemetrics.MetricClusterDeploymentProvisionUnderwaySeconds.WithLabelValues(
				cd.Name,
				cd.Namespace,
				hivemetrics.GetClusterDeploymentType(cd)).Set(0.0)
		}

		return r.syncDeletedClusterDeployment(cd, cdLog)
	}

	// Check for the delete-after annotation, and if the cluster has expired, delete it
	deleteAfter, ok := cd.Annotations[deleteAfterAnnotation]
	if ok {
		cdLog.Debugf("found delete after annotation: %s", deleteAfter)
		dur, err := time.ParseDuration(deleteAfter)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("error parsing %s as a duration: %v", deleteAfterAnnotation, err)
		}
		if !cd.CreationTimestamp.IsZero() {
			expiry := cd.CreationTimestamp.Add(dur)
			cdLog.Debugf("cluster expires at: %s", expiry)
			if time.Now().After(expiry) {
				cdLog.WithField("expiry", expiry).Info("cluster has expired, issuing delete")
				err := r.Delete(context.TODO(), cd)
				if err != nil {
					cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error deleting expired cluster")
				}
				return reconcile.Result{}, err
			}

			defer func() {
				requeueNow := result.Requeue && result.RequeueAfter <= 0
				if returnErr == nil && !requeueNow {
					// We have an expiry time but we're not expired yet. Set requeueAfter for just after expiry time
					// so that we requeue cluster for deletion once reconcile has completed
					requeueAfter := time.Until(expiry) + 60*time.Second
					if requeueAfter < result.RequeueAfter || result.RequeueAfter <= 0 {
						cdLog.Debugf("cluster will re-sync due to expiry time in: %v", requeueAfter)
						result.RequeueAfter = requeueAfter
					}
				}
			}()

		}
	}

	if !controllerutils.HasFinalizer(cd, hivev1.FinalizerDeprovision) {
		cdLog.Debugf("adding clusterdeployment finalizer")
		if err := r.addClusterDeploymentFinalizer(cd); err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error adding finalizer")
			return reconcile.Result{}, err
		}
		metricClustersCreated.WithLabelValues(hivemetrics.GetClusterDeploymentType(cd)).Inc()
		return reconcile.Result{}, nil
	}

	if cd.Spec.Installed {

		// update SyncSetFailedCondition status condition
		cdLog.Info("Check if any syncsetinstance Failed")
		updateCD, err := r.setSyncSetFailedCondition(cd, cdLog)
		if err != nil {
			cdLog.WithError(err).Error("Error updating SyncSetFailedCondition status condition")
			return reconcile.Result{}, err
		} else if updateCD {
			return reconcile.Result{}, nil
		}

		if cd.Status.Provision != nil {
			if cd.Status.InstalledTimestamp != nil {
				cdLog.Debug("cluster is already installed, no processing of provision needed")
				r.cleanupInstallLogPVC(cd, cdLog)
				return reconcile.Result{}, nil
			}
			return r.reconcileExistingProvision(cd, cdLog)
		}
		if !r.expectations.SatisfiedExpectations(request.String()) {
			cdLog.Debug("waiting for expectations to be satisfied")
			return reconcile.Result{}, nil
		}
		switch adopted, err := r.findAndAdoptRestoredClusterProvision(cd, cdLog); {
		case err != nil:
			return reconcile.Result{}, err
		case adopted:
			return reconcile.Result{}, nil
		case cd.Status.InfraID != "":
			// TODO: Remove the code for creating a dummy clusterprovision once
			// all legacy clusterdeployments have had clusterprovisions created
			// for them.
			cdLog.Info("cluster was created prior to clusterprovisions, creating dummy clusterprovision")
			return r.createClusterProvisionForLegacyCD(cd, cdLog)
		default:
			cdLog.Warn("cluster is installed but does not have an infra ID or a clusterprovision")
			return reconcile.Result{}, nil
		}
	}

	// Indicate that the cluster is still installing:
	hivemetrics.MetricClusterDeploymentProvisionUnderwaySeconds.WithLabelValues(
		cd.Name,
		cd.Namespace,
		hivemetrics.GetClusterDeploymentType(cd)).Set(
		time.Since(cd.CreationTimestamp.Time).Seconds())

	imageSet, err := r.getClusterImageSet(cd, cdLog)
	if err != nil {
		return reconcile.Result{}, err
	}

	releaseImage := r.getReleaseImage(cd, imageSet, cdLog)

	cdLog.Debug("loading pull secrets")
	pullSecret, err := r.mergePullSecrets(cd, cdLog)
	if err != nil {
		cdLog.WithError(err).Error("Error merging pull secrets")
		return reconcile.Result{}, err
	}

	// Update the pull secret object if required
	switch updated, err := r.updatePullSecretInfo(pullSecret, cd, cdLog); {
	case err != nil:
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "Error updating the merged pull secret")
		return reconcile.Result{}, err
	case updated:
		// The controller will not automatically requeue the cluster deployment
		// since the controller is not watching for secrets. So, requeue manually.
		return reconcile.Result{Requeue: true}, nil
	}

	if cd.Status.InstallerImage == nil || cd.Status.CLIImage == nil {
		return r.resolveInstallerImage(cd, imageSet, releaseImage, cdLog)
	}

	if !r.expectations.SatisfiedExpectations(request.String()) {
		cdLog.Debug("waiting for expectations to be satisfied")
		return reconcile.Result{}, nil
	}

	if cd.Status.Provision == nil {
		if cd.Status.InstallRestarts > 0 && cd.Annotations[tryInstallOnceAnnotation] == "true" {
			cdLog.Debug("not creating new provision since the deployment is set to try install only once")
			return reconcile.Result{}, nil
		}
		return r.startNewProvision(cd, releaseImage, cdLog)
	}

	return r.reconcileExistingProvision(cd, cdLog)
}

func (r *ReconcileClusterDeployment) startNewProvision(
	cd *hivev1.ClusterDeployment,
	releaseImage string,
	cdLog log.FieldLogger,
) (result reconcile.Result, returnedErr error) {
	existingProvisions, err := r.existingProvisions(cd, cdLog)
	if err != nil {
		return reconcile.Result{}, err
	}

	for _, provision := range existingProvisions {
		if provision.Spec.Stage != hivev1.ClusterProvisionStageFailed {
			return reconcile.Result{}, r.adoptProvision(cd, provision, cdLog)
		}
	}

	r.deleteStaleProvisions(existingProvisions, cdLog)

	if cd.Spec.ManageDNS {
		dnsZone, err := r.ensureManagedDNSZone(cd, cdLog)
		if err != nil {
			return reconcile.Result{}, err
		}
		if dnsZone == nil {
			return reconcile.Result{}, nil
		}

		updated, err := r.setDNSDelayMetric(cd, dnsZone, cdLog)
		if updated || err != nil {
			return reconcile.Result{}, err
		}
	}

	if err := controllerutils.SetupClusterInstallServiceAccount(r, cd.Namespace, cdLog); err != nil {
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error setting up service account and role")
		return reconcile.Result{}, err
	}

	provisionName := apihelpers.GetResourceName(cd.Name, fmt.Sprintf("%d-%s", cd.Status.InstallRestarts, utilrand.String(5)))

	labels := cd.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	labels[constants.ClusterDeploymentNameLabel] = cd.Name

	skipGatherLogs := os.Getenv(constants.SkipGatherLogsEnvVar) == "true"
	if !skipGatherLogs {
		if err := r.createPVC(cd, cdLog); err != nil {
			return reconcile.Result{}, err
		}
	}

	podSpec, err := install.InstallerPodSpec(
		cd,
		provisionName,
		releaseImage,
		controllerutils.ServiceAccountName,
		GetInstallLogsPVCName(cd),
		skipGatherLogs,
	)
	if err != nil {
		cdLog.WithError(err).Error("could not generate installer pod spec")
		return reconcile.Result{}, err
	}

	provision := &hivev1.ClusterProvision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      provisionName,
			Namespace: cd.Namespace,
			Labels:    labels,
		},
		Spec: hivev1.ClusterProvisionSpec{
			ClusterDeployment: corev1.LocalObjectReference{
				Name: cd.Name,
			},
			PodSpec: *podSpec,
			Attempt: cd.Status.InstallRestarts,
			Stage:   hivev1.ClusterProvisionStageInitializing,
		},
	}

	if v, ok := cd.Annotations[constants.InstallFailureTestAnnotation]; ok {
		if provision.Annotations == nil {
			provision.Annotations = map[string]string{}
		}
		provision.Annotations[constants.InstallFailureTestAnnotation] = v
	}

	// Copy over the cluster ID and infra ID from previous provision so that a failed install can be removed.
	if cd.Status.ClusterID != "" {
		provision.Spec.PrevClusterID = &cd.Status.ClusterID
	}
	if cd.Status.InfraID != "" {
		provision.Spec.PrevInfraID = &cd.Status.InfraID
	}
	if err := controllerutil.SetControllerReference(cd, provision, r.scheme); err != nil {
		cdLog.WithError(err).Error("could not set the owner ref on provision")
		return reconcile.Result{}, err
	}

	r.expectations.ExpectCreations(types.NamespacedName{Namespace: cd.Namespace, Name: cd.Name}.String(), 1)
	if err := r.Create(context.TODO(), provision); err != nil {
		cdLog.WithError(err).Error("could not create provision")
		r.expectations.CreationObserved(types.NamespacedName{Namespace: cd.Namespace, Name: cd.Name}.String())
		return reconcile.Result{}, err
	}

	cdLog.WithField("provision", provision.Name).Info("created new provision")

	if cd.Status.InstallRestarts == 0 {
		kickstartDuration := time.Since(cd.CreationTimestamp.Time)
		cdLog.WithField("elapsed", kickstartDuration.Seconds()).Info("calculated time to first provision seconds")
		metricInstallDelaySeconds.Observe(float64(kickstartDuration.Seconds()))
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileClusterDeployment) reconcileExistingProvision(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (result reconcile.Result, returnedErr error) {
	cdLog = cdLog.WithField("provision", cd.Status.Provision.Name)
	cdLog.Debug("reconciling existing provision")

	provision := &hivev1.ClusterProvision{}
	switch err := r.Get(context.TODO(), types.NamespacedName{Name: cd.Status.Provision.Name, Namespace: cd.Namespace}, provision); {
	case apierrors.IsNotFound(err):
		cdLog.Warn("linked provision not found")
		return r.clearOutCurrentProvision(cd, cdLog)
	case err != nil:
		cdLog.WithError(err).Error("could not get provision")
		return reconcile.Result{}, err
	}

	// Save the cluster ID and infra ID from the provision so that we can
	// clean up partial installs on the next provision attempt in case of failure.
	if (provision.Spec.ClusterID != nil && cd.Status.ClusterID != *provision.Spec.ClusterID) ||
		(provision.Spec.InfraID != nil && cd.Status.InfraID != *provision.Spec.InfraID) {
		cdLog.Infof("Saving infra ID %q for cluster", *provision.Spec.InfraID)
		if provision.Spec.ClusterID != nil {
			cd.Status.ClusterID = *provision.Spec.ClusterID
		}
		if provision.Spec.InfraID != nil {
			cd.Status.InfraID = *provision.Spec.InfraID
		}
		err := r.Status().Update(context.TODO(), cd)
		if err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error updating clusterdeployment status with infra ID")
		}
		return reconcile.Result{}, err
	}

	switch provision.Spec.Stage {
	case hivev1.ClusterProvisionStageInitializing:
		cdLog.Debug("still initializing provision")
		return reconcile.Result{}, nil
	case hivev1.ClusterProvisionStageProvisioning:
		cdLog.Debug("still provisioning")
		return reconcile.Result{}, nil
	case hivev1.ClusterProvisionStageFailed:
		return r.reconcileFailedProvision(cd, provision, cdLog)
	case hivev1.ClusterProvisionStageComplete:
		return r.reconcileCompletedProvision(cd, provision, cdLog)
	default:
		cdLog.WithField("stage", provision.Spec.Stage).Error("unknown provision stage")
		return reconcile.Result{}, errors.New("unknown provision stage")
	}
}

func (r *ReconcileClusterDeployment) reconcileFailedProvision(cd *hivev1.ClusterDeployment, provision *hivev1.ClusterProvision, cdLog log.FieldLogger) (reconcile.Result, error) {
	nextProvisionTime := time.Now()
	reason := "MissingCondition"

	failedCond := controllerutils.FindClusterProvisionCondition(provision.Status.Conditions, hivev1.ClusterProvisionFailedCondition)
	if failedCond != nil && failedCond.Status == corev1.ConditionTrue {
		nextProvisionTime = calculateNextProvisionTime(failedCond.LastTransitionTime.Time, cd.Status.InstallRestarts, cdLog)
		reason = failedCond.Reason
	} else {
		cdLog.Warnf("failed provision does not have a %s condition", hivev1.ClusterProvisionFailedCondition)
	}

	newConditions, condChange := controllerutils.SetClusterDeploymentConditionWithChangeCheck(
		cd.Status.Conditions,
		hivev1.ProvisionFailedCondition,
		corev1.ConditionTrue,
		reason,
		fmt.Sprintf("Provision %s failed. Next provision at %s.", provision.Name, nextProvisionTime.UTC().Format(time.RFC3339)),
		controllerutils.UpdateConditionIfReasonOrMessageChange,
	)
	cd.Status.Conditions = newConditions

	timeUntilNextProvision := time.Until(nextProvisionTime)
	if timeUntilNextProvision.Seconds() > 0 {
		cdLog.WithField("nextProvision", nextProvisionTime).Info("waiting to start a new provision after failure")
		if condChange {
			if err := r.statusUpdate(cd, cdLog); err != nil {
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{RequeueAfter: timeUntilNextProvision}, nil
	}

	cdLog.Info("clearing current failed provision to make way for a new provision")
	return r.clearOutCurrentProvision(cd, cdLog)
}

func (r *ReconcileClusterDeployment) reconcileCompletedProvision(cd *hivev1.ClusterDeployment, provision *hivev1.ClusterProvision, cdLog log.FieldLogger) (reconcile.Result, error) {
	cdLog.Info("provision completed successfully")

	if !cd.Spec.Installed {
		cd.Spec.Installed = true
		if err := r.Update(context.TODO(), cd); err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "failed to set the Installed flag")
			return reconcile.Result{}, err
		}

		// jobDuration calculates the time elapsed since the first clusterprovision was created
		startTime := cd.CreationTimestamp
		if firstProvision := r.getFirstProvision(cd, cdLog); firstProvision != nil {
			startTime = firstProvision.CreationTimestamp
		}
		jobDuration := time.Since(startTime.Time)
		cdLog.WithField("duration", jobDuration.Seconds()).Debug("install job completed")
		metricInstallJobDuration.Observe(float64(jobDuration.Seconds()))

		// Report a metric for the total number of install restarts:
		metricCompletedInstallJobRestarts.WithLabelValues(hivemetrics.GetClusterDeploymentType(cd)).
			Observe(float64(cd.Status.InstallRestarts))

		// Clear the install underway seconds metric. After this no-one should be reporting
		// this metric for this cluster.
		hivemetrics.MetricClusterDeploymentProvisionUnderwaySeconds.WithLabelValues(
			cd.Name,
			cd.Namespace,
			hivemetrics.GetClusterDeploymentType(cd)).Set(0.0)

		metricClustersInstalled.WithLabelValues(hivemetrics.GetClusterDeploymentType(cd)).Inc()
	}

	cd.Status.Installed = true
	now := metav1.Now()
	cd.Status.InstalledTimestamp = &now
	cd.Status.Conditions = controllerutils.SetClusterDeploymentCondition(
		cd.Status.Conditions,
		hivev1.ProvisionFailedCondition,
		corev1.ConditionFalse,
		"ProvisionSucceeded",
		fmt.Sprintf("Provision %s succeeded.", provision.Name),
		controllerutils.UpdateConditionNever,
	)

	if provision.Spec.AdminKubeconfigSecret != nil {
		cd.Status.AdminKubeconfigSecret = *provision.Spec.AdminKubeconfigSecret
	}
	if provision.Spec.AdminPasswordSecret != nil {
		cd.Status.AdminPasswordSecret = *provision.Spec.AdminPasswordSecret
	}

	adminKubeconfigSecret := &corev1.Secret{}
	if err := r.Get(context.Background(), types.NamespacedName{Namespace: cd.Namespace, Name: cd.Status.AdminKubeconfigSecret.Name}, adminKubeconfigSecret); err != nil {
		cdLog.WithError(err).Error("failed to get admin kubeconfig secret")
		return reconcile.Result{}, err
	}
	if err := r.fixupAdminKubeconfigSecret(adminKubeconfigSecret, cdLog); err != nil {
		cdLog.WithError(err).Error("failed to fix up admin kubeconfig secret")
		return reconcile.Result{}, err
	}
	if err := r.setAdminKubeconfigStatus(cd, adminKubeconfigSecret, cdLog); err != nil {
		cdLog.WithError(err).Error("failed to set admin kubeconfig status")
		return reconcile.Result{}, err
	}

	if err := r.Status().Update(context.TODO(), cd); err != nil {
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "could not set installed status")
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileClusterDeployment) clearOutCurrentProvision(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (reconcile.Result, error) {
	cd.Status.Provision = nil
	cd.Status.InstallRestarts = cd.Status.InstallRestarts + 1
	if err := r.Status().Update(context.TODO(), cd); err != nil {
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "could not clear out current provision")
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

// GetInstallLogsPVCName returns the expected name of the persistent volume claim for cluster install failure logs.
func GetInstallLogsPVCName(cd *hivev1.ClusterDeployment) string {
	return apihelpers.GetResourceName(cd.Name, "install-logs")
}

// createPVC will create the PVC for the install logs if it does not already exist.
func (r *ReconcileClusterDeployment) createPVC(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) error {

	pvcName := GetInstallLogsPVCName(cd)

	switch err := r.Get(context.TODO(), types.NamespacedName{Name: pvcName, Namespace: cd.Namespace}, &corev1.PersistentVolumeClaim{}); {
	case err == nil:
		cdLog.Debug("pvc already exists")
		return nil
	case !apierrors.IsNotFound(err):
		cdLog.WithError(err).Error("error getting persistent volume claim")
		return err
	}

	labels := map[string]string{
		constants.InstallJobLabel:            "true",
		constants.ClusterDeploymentNameLabel: cd.Name,
	}
	if cd.Labels != nil {
		typeStr, ok := cd.Labels[hivev1.HiveClusterTypeLabel]
		if ok {
			labels[hivev1.HiveClusterTypeLabel] = typeStr
		}
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: cd.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
		},
	}

	cdLog.WithField("pvc", pvc.Name).Info("creating persistent volume claim")
	if err := controllerutil.SetControllerReference(cd, pvc, r.scheme); err != nil {
		cdLog.WithError(err).Error("error setting controller reference on pvc")
		return err
	}
	err := r.Create(context.TODO(), pvc)
	if err != nil {
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error creating pvc")
	}
	return err
}

// getReleaseImage looks for a a release image in clusterdeployment or its corresponding imageset in the following order:
// 1 - specified in the cluster deployment spec.images.releaseImage
// 2 - referenced in the cluster deployment spec.imageSet
func (r *ReconcileClusterDeployment) getReleaseImage(cd *hivev1.ClusterDeployment, imageSet *hivev1.ClusterImageSet, cdLog log.FieldLogger) string {
	if cd.Spec.Images.ReleaseImage != "" {
		return cd.Spec.Images.ReleaseImage
	}
	if imageSet != nil && imageSet.Spec.ReleaseImage != nil {
		return *imageSet.Spec.ReleaseImage
	}
	return ""
}

func (r *ReconcileClusterDeployment) getClusterImageSet(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (*hivev1.ClusterImageSet, error) {
	if cd.Spec.ImageSet == nil || len(cd.Spec.ImageSet.Name) == 0 {
		return nil, nil
	}
	imageSet := &hivev1.ClusterImageSet{}
	if err := r.Get(context.TODO(), types.NamespacedName{Name: cd.Spec.ImageSet.Name}, imageSet); err != nil {
		if apierrors.IsNotFound(err) {
			cdLog.WithField("clusterimageset", cd.Spec.ImageSet.Name).Warning("clusterdeployment references non-existent clusterimageset")
			if err := r.setImageSetNotFoundCondition(cd, false, cdLog); err != nil {
				return nil, err
			}
		} else {
			cdLog.WithError(err).WithField("clusterimageset", cd.Spec.ImageSet.Name).Error("unexpected error retrieving clusterimageset")
		}
		return nil, err
	}
	return imageSet, nil
}

func (r *ReconcileClusterDeployment) statusUpdate(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) error {
	err := r.Status().Update(context.TODO(), cd)
	if err != nil {
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "cannot update clusterdeployment status")
	}
	return err
}

func (r *ReconcileClusterDeployment) resolveInstallerImage(cd *hivev1.ClusterDeployment, imageSet *hivev1.ClusterImageSet, releaseImage string, cdLog log.FieldLogger) (reconcile.Result, error) {
	if len(cd.Spec.Images.InstallerImage) > 0 {
		cdLog.WithField("image", cd.Spec.Images.InstallerImage).
			Debug("setting status.InstallerImage to the value in spec.images.installerImage")
		cd.Status.InstallerImage = &cd.Spec.Images.InstallerImage
		return reconcile.Result{}, r.statusUpdate(cd, cdLog)
	}
	if imageSet != nil && imageSet.Spec.InstallerImage != nil {
		cd.Status.InstallerImage = imageSet.Spec.InstallerImage
		cdLog.WithField("imageset", imageSet.Name).Debug("setting status.InstallerImage using imageSet.Spec.InstallerImage")
		return reconcile.Result{}, r.statusUpdate(cd, cdLog)
	}
	cliImage := images.GetCLIImage()
	job := imageset.GenerateImageSetJob(cd, releaseImage, controllerutils.ServiceAccountName, imageset.AlwaysPullImage(cliImage))
	if err := controllerutil.SetControllerReference(cd, job, r.scheme); err != nil {
		cdLog.WithError(err).Error("error setting controller reference on job")
		return reconcile.Result{}, err
	}

	jobName := types.NamespacedName{Name: job.Name, Namespace: job.Namespace}
	jobLog := cdLog.WithField("job", jobName)

	existingJob := &batchv1.Job{}
	err := r.Get(context.TODO(), jobName, existingJob)
	switch {
	// If the job exists but is in the process of getting deleted, requeue and wait for the delete
	// to complete.
	case err == nil && !job.DeletionTimestamp.IsZero():
		jobLog.Debug("imageset job is being deleted. Will recreate once deleted")
		return reconcile.Result{RequeueAfter: defaultRequeueTime}, err
	// If job exists and is finished, delete so we can recreate it
	case err == nil && controllerutils.IsFinished(existingJob):
		jobLog.WithField("successful", controllerutils.IsSuccessful(existingJob)).
			Warning("Finished job found, but all images not yet resolved. Deleting.")
		err := r.Delete(context.Background(), existingJob,
			client.PropagationPolicy(metav1.DeletePropagationForeground))
		if err != nil {
			jobLog.WithError(err).Log(controllerutils.LogLevel(err), "cannot delete imageset job")
		}
		return reconcile.Result{}, err
	case apierrors.IsNotFound(err):
		jobLog.WithField("releaseImage", releaseImage).Info("creating imageset job")
		err = controllerutils.SetupClusterInstallServiceAccount(r, cd.Namespace, cdLog)
		if err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error setting up service account and role")
			return reconcile.Result{}, err
		}

		err = r.Create(context.TODO(), job)
		if err != nil {
			jobLog.WithError(err).Log(controllerutils.LogLevel(err), "error creating job")
		} else {
			// kickstartDuration calculates the delay between creation of cd and start of imageset job
			kickstartDuration := time.Since(cd.CreationTimestamp.Time)
			cdLog.WithField("elapsed", kickstartDuration.Seconds()).Info("calculated time to imageset job seconds")
			metricImageSetDelaySeconds.Observe(float64(kickstartDuration.Seconds()))
		}
		return reconcile.Result{}, err
	case err != nil:
		jobLog.WithError(err).Error("cannot get job")
		return reconcile.Result{}, err
	default:
		jobLog.Debug("job exists and is in progress")
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileClusterDeployment) setDNSNotReadyCondition(cd *hivev1.ClusterDeployment, isReady bool, message string, cdLog log.FieldLogger) error {
	status := corev1.ConditionFalse
	reason := dnsReadyReason
	if !isReady {
		status = corev1.ConditionTrue
		reason = dnsNotReadyReason
	}
	conditions, changed := controllerutils.SetClusterDeploymentConditionWithChangeCheck(
		cd.Status.Conditions,
		hivev1.DNSNotReadyCondition,
		status,
		reason,
		message,
		controllerutils.UpdateConditionNever)
	if !changed {
		return nil
	}
	cd.Status.Conditions = conditions
	cdLog.Debugf("setting DNSNotReadyCondition to %v", status)
	return r.Status().Update(context.TODO(), cd)
}

func (r *ReconcileClusterDeployment) setImageSetNotFoundCondition(cd *hivev1.ClusterDeployment, isNotFound bool, cdLog log.FieldLogger) error {
	status := corev1.ConditionFalse
	reason := clusterImageSetFoundReason
	message := fmt.Sprintf("ClusterImageSet %s is available", cd.Spec.ImageSet.Name)
	if isNotFound {
		status = corev1.ConditionTrue
		reason = clusterImageSetNotFoundReason
		message = fmt.Sprintf("ClusterImageSet %s is not available", cd.Spec.ImageSet.Name)
	}
	conds, changed := controllerutils.SetClusterDeploymentConditionWithChangeCheck(
		cd.Status.Conditions,
		hivev1.ClusterImageSetNotFoundCondition,
		status,
		reason,
		message,
		controllerutils.UpdateConditionNever)
	if !changed {
		return nil
	}
	cdLog.Infof("setting ClusterImageSetNotFoundCondition to %v", status)
	cd.Status.Conditions = conds
	err := r.Status().Update(context.TODO(), cd)
	if err != nil {
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "cannot update status conditions")
	}
	return err
}

func (r *ReconcileClusterDeployment) fixupAdminKubeconfigSecret(secret *corev1.Secret, cdLog log.FieldLogger) error {
	originalSecret := secret.DeepCopy()

	rawData, hasRawData := secret.Data[rawAdminKubeconfigKey]
	if !hasRawData {
		secret.Data[rawAdminKubeconfigKey] = secret.Data[adminKubeconfigKey]
		rawData = secret.Data[adminKubeconfigKey]
	}

	var err error
	secret.Data[adminKubeconfigKey], err = controllerutils.FixupKubeconfig(rawData)
	if err != nil {
		cdLog.WithError(err).Errorf("cannot fixup kubeconfig to generate new one")
		return err
	}

	if reflect.DeepEqual(originalSecret.Data, secret.Data) {
		cdLog.Debug("secret data has not changed, no need to update")
		return nil
	}

	err = r.Update(context.TODO(), secret)
	if err != nil {
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error updated admin kubeconfig secret")
		return err
	}

	return nil
}

// setAdminKubeconfigStatus sets all cluster status fields that depend on the admin kubeconfig.
func (r *ReconcileClusterDeployment) setAdminKubeconfigStatus(cd *hivev1.ClusterDeployment, adminKubeconfigSecret *corev1.Secret, cdLog log.FieldLogger) error {
	if cd.Status.WebConsoleURL == "" || cd.Status.APIURL == "" {
		remoteClusterAPIClient, err := r.remoteClusterAPIClientBuilder(string(adminKubeconfigSecret.Data[adminKubeconfigKey]), controllerName)
		if err != nil {
			cdLog.WithError(err).Error("error building remote cluster-api client connection")
			return err
		}

		// Parse the admin kubeconfig for the server URL:
		config, err := clientcmd.Load(adminKubeconfigSecret.Data["kubeconfig"])
		if err != nil {
			return err
		}
		cluster, ok := config.Clusters[cd.Spec.ClusterName]
		if !ok {
			return fmt.Errorf("error parsing admin kubeconfig secret data")
		}

		// We should be able to assume only one cluster in here:
		server := cluster.Server
		cdLog.Debugf("found cluster API URL in kubeconfig: %s", server)
		cd.Status.APIURL = server
		routeObject := &routev1.Route{}
		err = remoteClusterAPIClient.Get(context.Background(),
			types.NamespacedName{Namespace: "openshift-console", Name: "console"}, routeObject)
		if err != nil {
			cdLog.WithError(err).Error("error fetching remote route object")
			return err
		}
		cdLog.Debugf("read remote route object: %s", routeObject)
		cd.Status.WebConsoleURL = "https://" + routeObject.Spec.Host
	}
	return nil
}

// ensureManagedDNSZoneDeleted is a safety check to ensure that the child managed DNSZone
// linked to the parent cluster deployment gets a deletionTimestamp when the parent is deleted.
// Normally we expect Kube garbage collection to do this for us, but in rare cases we've seen it
// not working as intended.
func (r *ReconcileClusterDeployment) ensureManagedDNSZoneDeleted(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (*reconcile.Result, error) {
	if !cd.Spec.ManageDNS {
		return nil, nil
	}
	dnsZone := &hivev1.DNSZone{}
	dnsZoneNamespacedName := types.NamespacedName{Namespace: cd.Namespace, Name: controllerutils.DNSZoneName(cd.Name)}
	err := r.Get(context.TODO(), dnsZoneNamespacedName, dnsZone)
	if err != nil && !apierrors.IsNotFound(err) {
		cdLog.WithError(err).Error("error looking up managed dnszone")
		return &reconcile.Result{}, err
	}
	if apierrors.IsNotFound(err) || !dnsZone.DeletionTimestamp.IsZero() {
		cdLog.Debug("dnszone has been deleted or is getting deleted")
		return nil, nil
	}
	err = r.Delete(context.TODO(), dnsZone,
		client.PropagationPolicy(metav1.DeletePropagationForeground))
	if err != nil {
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error deleting managed dnszone")
	}
	return &reconcile.Result{}, err
}

func (r *ReconcileClusterDeployment) syncDeletedClusterDeployment(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (reconcile.Result, error) {

	result, err := r.ensureManagedDNSZoneDeleted(cd, cdLog)
	if result != nil {
		return *result, err
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	// Wait for outstanding provision to be removed before creating deprovision request
	if cd.Status.Provision != nil {
		provision := &hivev1.ClusterProvision{}
		switch err := r.Get(context.TODO(), types.NamespacedName{Name: cd.Status.Provision.Name, Namespace: cd.Namespace}, provision); {
		case apierrors.IsNotFound(err):
			cdLog.Debug("linked provision removed")
		case err != nil:
			cdLog.WithError(err).Error("could not get provision")
			return reconcile.Result{}, err
		case provision.DeletionTimestamp == nil:
			if err := r.Delete(context.TODO(), provision); err != nil {
				cdLog.WithError(err).Log(controllerutils.LogLevel(err), "could not delete provision")
				return reconcile.Result{}, err
			}
			cdLog.Info("deleted outstanding provision")
			return reconcile.Result{RequeueAfter: defaultRequeueTime}, nil
		default:
			cdLog.Debug("still waiting for outstanding provision to be removed")
			return reconcile.Result{RequeueAfter: defaultRequeueTime}, nil
		}
	}

	// Skips creation of deprovision request if PreserveOnDelete is true and cluster is installed
	if cd.Spec.PreserveOnDelete {
		if cd.Spec.Installed {
			cdLog.Warn("skipping creation of deprovisioning request for installed cluster due to PreserveOnDelete=true")
			if controllerutils.HasFinalizer(cd, hivev1.FinalizerDeprovision) {
				err = r.removeClusterDeploymentFinalizer(cd)
				if err != nil {
					cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error removing finalizer")
				}
				return reconcile.Result{}, err
			}
			return reconcile.Result{}, nil
		}
		// Overriding PreserveOnDelete because we might have deleted the cluster deployment before it finished
		// installing, which can cause AWS resources to leak
		cdLog.Infof("PreserveOnDelete=true but creating deprovisioning request as cluster was never successfully provisioned")
	}

	if cd.Status.InfraID == "" {
		cdLog.Warn("skipping uninstall for cluster that never had clusterID set")
		err = r.removeClusterDeploymentFinalizer(cd)
		if err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error removing finalizer")
		}
		return reconcile.Result{}, err
	}

	// Generate a deprovision request
	request, err := generateDeprovisionRequest(cd)
	if err != nil {
		cdLog.WithError(err).Error("error generating deprovision request")
		return reconcile.Result{}, err
	}
	err = controllerutil.SetControllerReference(cd, request, r.scheme)
	if err != nil {
		cdLog.Errorf("error setting controller reference on deprovision request: %v", err)
		return reconcile.Result{}, err
	}

	// Check if deprovision request already exists:
	existingRequest := &hivev1.ClusterDeprovisionRequest{}
	switch err = r.Get(context.TODO(), types.NamespacedName{Name: cd.Name, Namespace: cd.Namespace}, existingRequest); {
	case apierrors.IsNotFound(err):
		cdLog.Info("creating deprovision request for cluster deployment")
		switch err = r.Create(context.TODO(), request); {
		case apierrors.IsAlreadyExists(err):
			cdLog.Info("deprovision request already exists")
			// requeue the clusterdeployment immediately to process the status of the deprovision request
			return reconcile.Result{Requeue: true}, nil
		case err != nil:
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error creating deprovision request")
			// Check if namespace is terminated, if so we can give up, remove the finalizer, and let
			// the cluster go away.
			ns := &corev1.Namespace{}
			err = r.Get(context.TODO(), types.NamespacedName{Name: cd.Namespace}, ns)
			if err != nil {
				cdLog.WithError(err).Error("error checking for deletionTimestamp on namespace")
				return reconcile.Result{}, err
			}
			if ns.DeletionTimestamp != nil {
				cdLog.Warn("detected a namespace deleted before deprovision request could be created, giving up on deprovision and removing finalizer")
				err = r.removeClusterDeploymentFinalizer(cd)
				if err != nil {
					cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error removing finalizer")
				}
			}
			return reconcile.Result{}, err
		default:
			return reconcile.Result{}, nil
		}
	case err != nil:
		cdLog.WithError(err).Error("error getting deprovision request")
		return reconcile.Result{}, err
	}

	// Deprovision request exists, check whether it has completed
	if existingRequest.Status.Completed {
		cdLog.Infof("deprovision request completed, removing finalizer")
		err = r.removeClusterDeploymentFinalizer(cd)
		if err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error removing finalizer")
		}
		return reconcile.Result{}, err
	}

	cdLog.Debug("deprovision request not yet completed")

	return reconcile.Result{}, nil
}

func (r *ReconcileClusterDeployment) addClusterDeploymentFinalizer(cd *hivev1.ClusterDeployment) error {
	cd = cd.DeepCopy()
	controllerutils.AddFinalizer(cd, hivev1.FinalizerDeprovision)
	return r.Update(context.TODO(), cd)
}

func (r *ReconcileClusterDeployment) removeClusterDeploymentFinalizer(cd *hivev1.ClusterDeployment) error {

	cd = cd.DeepCopy()
	controllerutils.DeleteFinalizer(cd, hivev1.FinalizerDeprovision)
	if err := r.Update(context.TODO(), cd); err != nil {
		return err
	}

	clearUnderwaySecondsMetrics(cd)

	// Increment the clusters deleted counter:
	metricClustersDeleted.WithLabelValues(hivemetrics.GetClusterDeploymentType(cd)).Inc()

	return nil
}

// setDNSDelayMetric will calculate the amount of time elapsed from clusterdeployment creation
// to when the dnszone became ready, and set a metric to report the delay.
// Will return a bool indicating whether the clusterdeployment has been modified, and whether any error was encountered.
func (r *ReconcileClusterDeployment) setDNSDelayMetric(cd *hivev1.ClusterDeployment, dnsZone *hivev1.DNSZone, cdLog log.FieldLogger) (bool, error) {
	modified := false
	initializeAnnotations(cd)

	if _, ok := cd.Annotations[dnsReadyAnnotation]; ok {
		// already have recorded the dnsdelay metric
		return modified, nil
	}

	readyTimestamp := dnsReadyTransitionTime(dnsZone)
	if readyTimestamp == nil {
		msg := "did not find timestamp for when dnszone became ready"
		cdLog.WithField("dnszone", dnsZone.Name).Error(msg)
		return modified, fmt.Errorf(msg)
	}

	dnsDelayDuration := readyTimestamp.Sub(cd.CreationTimestamp.Time)
	cdLog.WithField("duration", dnsDelayDuration.Seconds()).Info("DNS ready")
	cd.Annotations[dnsReadyAnnotation] = dnsDelayDuration.String()
	if err := r.Update(context.TODO(), cd); err != nil {
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "failed to save annotation marking DNS becoming ready")
		return modified, err
	}
	modified = true

	metricDNSDelaySeconds.Observe(float64(dnsDelayDuration.Seconds()))

	return modified, nil
}

func (r *ReconcileClusterDeployment) ensureManagedDNSZone(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (*hivev1.DNSZone, error) {
	// for now we only support AWS
	if cd.Spec.AWS == nil || cd.Spec.PlatformSecrets.AWS == nil {
		cdLog.Error("cluster deployment platform is not AWS, cannot manage DNS zone")
		if err := r.setDNSNotReadyCondition(cd, false, "Managed DNS is only supported on AWS", cdLog); err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "could not update DNSNotReadyCondition")
			return nil, err
		}
		return nil, errors.New("only AWS managed DNS is supported")
	}
	dnsZone := &hivev1.DNSZone{}
	dnsZoneNamespacedName := types.NamespacedName{Namespace: cd.Namespace, Name: controllerutils.DNSZoneName(cd.Name)}
	logger := cdLog.WithField("zone", dnsZoneNamespacedName.String())

	switch err := r.Get(context.TODO(), dnsZoneNamespacedName, dnsZone); {
	case apierrors.IsNotFound(err):
		logger.Info("creating new DNSZone for cluster deployment")
		return nil, r.createManagedDNSZone(cd, logger)
	case err != nil:
		logger.WithError(err).Error("failed to fetch DNS zone")
		return nil, err
	}

	if !metav1.IsControlledBy(dnsZone, cd) {
		cdLog.Error("DNS zone already exists but is not owned by cluster deployment")
		if err := r.setDNSNotReadyCondition(cd, false, "Existing DNS zone not owned by cluster deployment", cdLog); err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "could not update DNSNotReadyCondition")
			return nil, err
		}
		return nil, errors.New("Existing unowned DNS zone")
	}

	availableCondition := controllerutils.FindDNSZoneCondition(dnsZone.Status.Conditions, hivev1.ZoneAvailableDNSZoneCondition)
	if availableCondition == nil || availableCondition.Status != corev1.ConditionTrue {
		// The clusterdeployment will be queued when the owned DNSZone's status
		// is updated to available.
		cdLog.Debug("DNSZone is not yet available. Waiting for zone to become available.")
		if err := r.setDNSNotReadyCondition(cd, false, "DNS Zone not yet available", cdLog); err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "could not update DNSNotReadyCondition")
			return nil, err
		}
		return nil, nil
	}

	if err := r.setDNSNotReadyCondition(cd, true, "DNS Zone available", cdLog); err != nil {
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "could not update DNSNotReadyCondition")
		return nil, err
	}
	return dnsZone, nil
}

func (r *ReconcileClusterDeployment) createManagedDNSZone(cd *hivev1.ClusterDeployment, logger log.FieldLogger) error {
	dnsZone := &hivev1.DNSZone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      controllerutils.DNSZoneName(cd.Name),
			Namespace: cd.Namespace,
		},
		Spec: hivev1.DNSZoneSpec{
			Zone:               cd.Spec.BaseDomain,
			LinkToParentDomain: true,
			AWS: &hivev1.AWSDNSZoneSpec{
				AccountSecret: cd.Spec.PlatformSecrets.AWS.Credentials,
				Region:        cd.Spec.AWS.Region,
			},
		},
	}

	for k, v := range cd.Spec.AWS.UserTags {
		dnsZone.Spec.AWS.AdditionalTags = append(dnsZone.Spec.AWS.AdditionalTags, hivev1.AWSResourceTag{Key: k, Value: v})
	}

	if err := controllerutil.SetControllerReference(cd, dnsZone, r.scheme); err != nil {
		logger.WithError(err).Error("error setting controller reference on dnszone")
		return err
	}

	err := r.Create(context.TODO(), dnsZone)
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "cannot create DNS zone")
		return err
	}
	logger.Info("dns zone created")
	return nil
}

func selectorPodWatchHandler(a handler.MapObject) []reconcile.Request {
	retval := []reconcile.Request{}

	pod := a.Object.(*corev1.Pod)
	if pod == nil {
		// Wasn't a Pod, bail out. This should not happen.
		log.Errorf("Error converting MapObject.Object to Pod. Value: %+v", a.Object)
		return retval
	}
	if pod.Labels == nil {
		return retval
	}
	cdName, ok := pod.Labels[constants.ClusterDeploymentNameLabel]
	if !ok {
		return retval
	}
	retval = append(retval, reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      cdName,
		Namespace: pod.Namespace,
	}})
	return retval
}

// cleanupInstallLogPVC will immediately delete the PVC (should it exist) if the cluster was installed successfully, without retries.
// If there were retries, it will delete the PVC if it has been more than 7 days since the job was completed.
func (r *ReconcileClusterDeployment) cleanupInstallLogPVC(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) error {
	if !cd.Spec.Installed {
		return nil
	}

	pvc := &corev1.PersistentVolumeClaim{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: GetInstallLogsPVCName(cd), Namespace: cd.Namespace}, pvc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		cdLog.WithError(err).Error("error looking up install logs PVC")
		return err
	}

	pvcLog := cdLog.WithField("pvc", pvc.Name)

	switch {
	case cd.Status.InstallRestarts == 0:
		pvcLog.Info("deleting logs PersistentVolumeClaim for installed cluster with no restarts")
	case cd.Status.InstalledTimestamp == nil:
		pvcLog.Warn("deleting logs PersistentVolumeClaim for cluster with errors but no installed timestamp")
	// Otherwise, delete if more than 7 days have passed.
	case time.Since(cd.Status.InstalledTimestamp.Time) > (7 * 24 * time.Hour):
		pvcLog.Info("deleting logs PersistentVolumeClaim for cluster that was installed after restarts more than 7 days ago")
	default:
		cdLog.WithField("pvc", pvc.Name).Debug("preserving logs PersistentVolumeClaim for cluster with install restarts for 7 days")
		return nil
	}

	if err := r.Delete(context.TODO(), pvc); err != nil {
		pvcLog.WithError(err).Log(controllerutils.LogLevel(err), "error deleting install logs PVC")
		return err
	}
	return nil
}

func (r *ReconcileClusterDeployment) createClusterProvisionForLegacyCD(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (reconcile.Result, error) {
	labels := cd.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	labels[constants.ClusterDeploymentNameLabel] = cd.Name

	provision := &hivev1.ClusterProvision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      apihelpers.GetResourceName(cd.Name, fmt.Sprintf("0-%s", utilrand.String(5))),
			Namespace: cd.Namespace,
			Labels:    labels,
		},
		Spec: hivev1.ClusterProvisionSpec{
			ClusterDeployment: corev1.LocalObjectReference{
				Name: cd.Name,
			},
			Stage:                 hivev1.ClusterProvisionStageComplete,
			InfraID:               &cd.Status.InfraID,
			ClusterID:             &cd.Status.ClusterID,
			AdminKubeconfigSecret: &cd.Status.AdminKubeconfigSecret,
			AdminPasswordSecret:   &cd.Status.AdminPasswordSecret,
		},
	}

	logsConfigMap := &corev1.ConfigMap{}
	switch err := r.Get(context.TODO(), types.NamespacedName{Namespace: cd.Namespace, Name: fmt.Sprintf("%s-install-log", cd.Name)}, logsConfigMap); {
	case apierrors.IsNotFound(err):
		cdLog.Warn("legacy installed clusterdeployment does not have corresponding logs configmap")
	case err != nil:
		cdLog.WithError(err).Error("failed to get logs configmap for legacy clusterdeployment")
	default:
		if log, ok := logsConfigMap.Data["log"]; ok {
			provision.Spec.InstallLog = &log
		} else {
			cdLog.Warn("logs configmap for legacy clusterdeployment does not have a \"log\" data entry")
		}
	}

	metadataConfigMap := &corev1.ConfigMap{}
	switch err := r.Get(context.TODO(), types.NamespacedName{Namespace: cd.Namespace, Name: fmt.Sprintf("%s-metadata", cd.Name)}, metadataConfigMap); {
	case apierrors.IsNotFound(err):
		cdLog.Warn("legacy installed clusterdeployment does not have corresponding metadata configmap")
	case err != nil:
		cdLog.WithError(err).Error("failed to get metadata configmap for legacy clusterdeployment")
	default:
		if metadata, ok := metadataConfigMap.Data["metadata.json"]; ok {
			provision.Spec.Metadata = &runtime.RawExtension{
				Raw: []byte(metadata),
			}
		} else {
			cdLog.Warn("metadata configmap for legacy clusterdeployment does not have a \"metadata.json\" data entry")
		}
	}

	if err := controllerutil.SetControllerReference(cd, provision, r.scheme); err != nil {
		cdLog.WithError(err).Error("could not set the owner ref on provision")
		return reconcile.Result{}, err
	}

	r.expectations.ExpectCreations(types.NamespacedName{Namespace: cd.Namespace, Name: cd.Name}.String(), 1)
	if err := r.Create(context.TODO(), provision); err != nil {
		cdLog.WithError(err).Error("could not create provision")
		r.expectations.CreationObserved(types.NamespacedName{Namespace: cd.Namespace, Name: cd.Name}.String())
		return reconcile.Result{}, err
	}

	cdLog.WithField("provision", provision.Name).Info("dummy clusterprovision created")

	return reconcile.Result{}, nil
}

func (r *ReconcileClusterDeployment) findAndAdoptRestoredClusterProvision(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (bool, error) {
	existingProvisions, err := r.existingProvisions(cd, cdLog)
	if err != nil {
		return false, err
	}
	for _, provision := range existingProvisions {
		if provision.Spec.Stage == hivev1.ClusterProvisionStageComplete {
			return true, r.adoptProvision(cd, provision, cdLog)
		}
	}
	cdLog.Warn("cluster is already installed, but provision could not be found")
	return false, nil
}

func generateDeprovisionRequest(cd *hivev1.ClusterDeployment) (*hivev1.ClusterDeprovisionRequest, error) {
	req := &hivev1.ClusterDeprovisionRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cd.Name,
			Namespace: cd.Namespace,
		},
		Spec: hivev1.ClusterDeprovisionRequestSpec{
			InfraID:   cd.Status.InfraID,
			ClusterID: cd.Status.ClusterID,
		},
	}

	switch {
	case cd.Spec.Platform.AWS != nil:
		if cd.Spec.PlatformSecrets.AWS == nil {
			return nil, errors.New("missing AWS platform secrets")
		}
		req.Spec.Platform.AWS = &hivev1.AWSClusterDeprovisionRequest{
			Region:      cd.Spec.Platform.AWS.Region,
			Credentials: &cd.Spec.PlatformSecrets.AWS.Credentials,
		}
	case cd.Spec.Platform.Azure != nil:
		if cd.Spec.PlatformSecrets.Azure == nil {
			return nil, errors.New("missing Azure platform secrets")
		}
		req.Spec.Platform.Azure = &hivev1.AzureClusterDeprovisionRequest{
			Credentials: &cd.Spec.PlatformSecrets.Azure.Credentials,
		}
	case cd.Spec.Platform.GCP != nil:
		if cd.Spec.PlatformSecrets.GCP == nil {
			return nil, errors.New("missing GCP platform secrets")
		}
		req.Spec.Platform.GCP = &hivev1.GCPClusterDeprovisionRequest{
			Region:      cd.Spec.Platform.GCP.Region,
			ProjectID:   cd.Spec.Platform.GCP.ProjectID,
			Credentials: &cd.Spec.PlatformSecrets.GCP.Credentials,
		}
	default:
		return nil, errors.New("unsupported cloud provider for deprovision")
	}

	return req, nil
}

func generatePullSecretObj(pullSecret string, pullSecretName string, cd *hivev1.ClusterDeployment) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      pullSecretName,
			Namespace: cd.Namespace,
		},
		Type: corev1.SecretTypeDockerConfigJson,
		StringData: map[string]string{
			corev1.DockerConfigJsonKey: pullSecret,
		},
	}
}

func dnsReadyTransitionTime(dnsZone *hivev1.DNSZone) *time.Time {
	readyCondition := controllerutils.FindDNSZoneCondition(dnsZone.Status.Conditions, hivev1.ZoneAvailableDNSZoneCondition)

	if readyCondition != nil && readyCondition.Status == corev1.ConditionTrue {
		return &readyCondition.LastTransitionTime.Time
	}

	return nil
}

func clearUnderwaySecondsMetrics(cd *hivev1.ClusterDeployment) {
	// If we've successfully cleared the deprovision finalizer we know this is a good time to
	// reset the underway metric to 0, after which it will no longer be reported.
	hivemetrics.MetricClusterDeploymentDeprovisioningUnderwaySeconds.WithLabelValues(
		cd.Name,
		cd.Namespace,
		hivemetrics.GetClusterDeploymentType(cd)).Set(0.0)

	// Clear the install underway seconds metric if this cluster was still installing.
	if !cd.Spec.Installed {
		hivemetrics.MetricClusterDeploymentProvisionUnderwaySeconds.WithLabelValues(
			cd.Name,
			cd.Namespace,
			hivemetrics.GetClusterDeploymentType(cd)).Set(0.0)
	}
}

// initializeAnnotations() initializes the annotations if it is not already
func initializeAnnotations(cd *hivev1.ClusterDeployment) {
	if cd.Annotations == nil {
		cd.Annotations = map[string]string{}
	}
}

// mergePullSecrets merges the global pull secret JSON (if defined) with the cluster's pull secret JSON (if defined)
// An error will be returned if neither is defined
func (r *ReconcileClusterDeployment) mergePullSecrets(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (string, error) {
	var localPullSecret string
	var err error

	// For code readability let's call the pull secret in cluster deployment config as local pull secret
	if cd.Spec.PullSecret != nil {
		localPullSecret, err = controllerutils.LoadSecretData(r.Client, cd.Spec.PullSecret.Name, cd.Namespace, corev1.DockerConfigJsonKey)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return "", err
			}
		}
	}

	// Check if global pull secret from env as it comes from hive config
	globalPullSecretName := os.Getenv(constants.GlobalPullSecret)
	var globalPullSecret string
	if len(globalPullSecretName) != 0 {
		globalPullSecret, err = controllerutils.LoadSecretData(r.Client, globalPullSecretName, constants.HiveNamespace, corev1.DockerConfigJsonKey)
		if err != nil {
			return "", errors.Wrap(err, "global pull secret could not be retrieved")
		}
	}

	switch {
	case globalPullSecret != "" && localPullSecret != "":
		// Merge local pullSecret and globalPullSecret. If both pull secrets have same registry name
		// then the merged pull secret will have registry secret from local pull secret
		pullSecret, err := controllerutils.MergeJsons(globalPullSecret, localPullSecret, cdLog)
		if err != nil {
			errMsg := "unable to merge global pull secret with local pull secret"
			cdLog.WithError(err).Error(errMsg)
			return "", errors.Wrap(err, errMsg)
		}
		return pullSecret, nil
	case globalPullSecret != "":
		return globalPullSecret, nil
	case localPullSecret != "":
		return localPullSecret, nil
	default:
		errMsg := "clusterdeployment must specify pull secret since hiveconfig does not specify a global pull secret"
		cdLog.Error(errMsg)
		return "", errors.New(errMsg)
	}
}

// updatePullSecretInfo creates or updates the merged pull secret for the clusterdeployment.
// It returns true when the merged pull secret has been created or updated.
func (r *ReconcileClusterDeployment) updatePullSecretInfo(pullSecret string, cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (bool, error) {
	var err error
	pullSecretObjExists := true
	existingPullSecretObj := &corev1.Secret{}
	mergedSecretName := constants.GetMergedPullSecretName(cd)
	err = r.Get(context.TODO(), types.NamespacedName{Name: mergedSecretName, Namespace: cd.Namespace}, existingPullSecretObj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			cdLog.Info("Existing pull secret object not found")
			pullSecretObjExists = false
		} else {
			return false, errors.Wrap(err, "Error getting pull secret from cluster deployment")
		}
	}

	if pullSecretObjExists {
		existingPullSecret, ok := existingPullSecretObj.Data[corev1.DockerConfigJsonKey]
		if !ok {
			return false, fmt.Errorf("Pull secret %s did not contain key %s", mergedSecretName, corev1.DockerConfigJsonKey)
		}
		if string(existingPullSecret) == pullSecret {
			cdLog.Debug("Existing and the new merged pull secret are same")
			return false, nil
		}
		cdLog.Info("Existing merged pull secret hash did not match with latest merged pull secret")
		existingPullSecretObj.Data[corev1.DockerConfigJsonKey] = []byte(pullSecret)
		err = r.Update(context.TODO(), existingPullSecretObj)
		if err != nil {
			return false, errors.Wrap(err, "error updating merged pull secret object")
		}
		cdLog.WithField("secretName", mergedSecretName).Info("Updated the merged pull secret object successfully")
	} else {

		// create a new pull secret object
		newPullSecretObj := generatePullSecretObj(
			pullSecret,
			mergedSecretName,
			cd,
		)
		err = controllerutil.SetControllerReference(cd, newPullSecretObj, r.scheme)
		if err != nil {
			cdLog.Errorf("error setting controller reference on new merged pull secret: %v", err)
			return false, err
		}
		err = r.Create(context.TODO(), newPullSecretObj)
		if err != nil {
			return false, errors.Wrap(err, "error creating new pull secret object")
		}
		cdLog.WithField("secretName", mergedSecretName).Info("Created the merged pull secret object successfully")
	}
	return true, nil
}

func calculateNextProvisionTime(failureTime time.Time, retries int, cdLog log.FieldLogger) time.Time {
	// (2^currentRetries) * 60 seconds up to a max of 24 hours.
	const sleepCap = 24 * time.Hour
	const retryCap = 11 // log_2_(24*60)

	if retries >= retryCap {
		return failureTime.Add(sleepCap)
	}
	return failureTime.Add((1 << uint(retries)) * time.Minute)
}

func (r *ReconcileClusterDeployment) existingProvisions(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) ([]*hivev1.ClusterProvision, error) {
	provisionList := &hivev1.ClusterProvisionList{}
	if err := r.List(
		context.TODO(),
		provisionList,
		client.InNamespace(cd.Namespace),
		client.MatchingLabels(map[string]string{constants.ClusterDeploymentNameLabel: cd.Name}),
	); err != nil {
		cdLog.WithError(err).Warn("could not list provisions for clusterdeployment")
		return nil, err
	}
	provisions := make([]*hivev1.ClusterProvision, len(provisionList.Items))
	for i := range provisionList.Items {
		provisions[i] = &provisionList.Items[i]
	}
	return provisions, nil
}

func (r *ReconcileClusterDeployment) getFirstProvision(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) *hivev1.ClusterProvision {
	provisions, err := r.existingProvisions(cd, cdLog)
	if err != nil {
		return nil
	}
	for _, provision := range provisions {
		if provision.Spec.Attempt == 0 {
			return provision
		}
	}
	cdLog.Warn("could not find the first provision for clusterdeployment")
	return nil
}

func (r *ReconcileClusterDeployment) adoptProvision(cd *hivev1.ClusterDeployment, provision *hivev1.ClusterProvision, cdLog log.FieldLogger) error {
	pLog := cdLog.WithField("provision", provision.Name)
	cd.Status.Provision = &corev1.LocalObjectReference{Name: provision.Name}
	if err := r.Status().Update(context.TODO(), cd); err != nil {
		pLog.WithError(err).Log(controllerutils.LogLevel(err), "could not adopt provision")
		return err
	}
	pLog.Info("adopted provision")
	return nil
}

func (r *ReconcileClusterDeployment) deleteStaleProvisions(provs []*hivev1.ClusterProvision, cdLog log.FieldLogger) {
	// Cap the number of existing provisions. Always keep the earliest provision as
	// it is used to determine the total time that it took to install. Take off
	// one extra to make room for the new provision being started.
	amountToDelete := len(provs) - maxProvisions
	if amountToDelete <= 0 {
		return
	}
	cdLog.Infof("Deleting %d old provisions", amountToDelete)
	sort.Slice(provs, func(i, j int) bool { return provs[i].Spec.Attempt < provs[j].Spec.Attempt })
	for _, provision := range provs[1 : amountToDelete+1] {
		pLog := cdLog.WithField("provision", provision.Name)
		pLog.Info("Deleting old provision")
		if err := r.Delete(context.TODO(), provision); err != nil {
			pLog.WithError(err).Log(controllerutils.LogLevel(err), "failed to delete old provision")
		}
	}
}

// getAllSyncSetInstances returns all syncset instances for a specific cluster deployment
func (r *ReconcileClusterDeployment) getAllSyncSetInstances(cd *hivev1.ClusterDeployment) ([]*hivev1.SyncSetInstance, error) {

	list := &hivev1.SyncSetInstanceList{}
	err := r.List(context.TODO(), list, client.InNamespace(cd.Namespace))
	if err != nil {
		return nil, err
	}

	syncSetInstances := []*hivev1.SyncSetInstance{}
	for i, syncSetInstance := range list.Items {
		if syncSetInstance.Spec.ClusterDeployment.Name == cd.Name {
			syncSetInstances = append(syncSetInstances, &list.Items[i])
		}
	}
	return syncSetInstances, nil
}

// checkForFailedSyncSetInstance returns true if it finds failed syncset instance
func checkForFailedSyncSetInstance(syncSetInstances []*hivev1.SyncSetInstance) bool {

	for _, syncSetInstance := range syncSetInstances {
		if checkSyncSetConditionsForFailure(syncSetInstance.Status.Conditions) {
			return true
		}
		for _, r := range syncSetInstance.Status.Resources {
			if checkSyncSetConditionsForFailure(r.Conditions) {
				return true
			}
		}
		for _, p := range syncSetInstance.Status.Patches {
			if checkSyncSetConditionsForFailure(p.Conditions) {
				return true
			}
		}
		for _, s := range syncSetInstance.Status.SecretReferences {
			if checkSyncSetConditionsForFailure(s.Conditions) {
				return true
			}
		}
	}
	return false
}

// checkSyncSetConditionsForFailure returns true when the condition contains hivev1.ApplyFailureSyncCondition
// and condition status is equal to true
func checkSyncSetConditionsForFailure(conds []hivev1.SyncCondition) bool {
	for _, c := range conds {
		if c.Status != corev1.ConditionTrue {
			continue
		}
		switch c.Type {
		case hivev1.ApplyFailureSyncCondition, hivev1.DeletionFailedSyncCondition, hivev1.UnknownObjectSyncCondition:
			return true
		}
	}
	return false
}

// setSyncSetFailedCondition returns true when it sets or updates the hivev1.SyncSetFailedCondition
func (r *ReconcileClusterDeployment) setSyncSetFailedCondition(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (bool, error) {
	// get all syncset instances for this cluster deployment
	syncSetInstances, err := r.getAllSyncSetInstances(cd)
	if err != nil {
		cdLog.WithError(err).Error("Unable to list related syncset instances for cluster deployment")
		return false, err
	}

	isFailedCondition := checkForFailedSyncSetInstance(syncSetInstances)

	status := corev1.ConditionFalse
	reason := "SyncSetApplySuccess"
	message := "SyncSet apply is successful"
	if isFailedCondition {
		status = corev1.ConditionTrue
		reason = "SyncSetApplyFailure"
		message = "One of the SyncSetInstance apply has failed"
	}
	conds, changed := controllerutils.SetClusterDeploymentConditionWithChangeCheck(
		cd.Status.Conditions,
		hivev1.SyncSetFailedCondition,
		status,
		reason,
		message,
		controllerutils.UpdateConditionNever,
	)
	if !changed {
		return false, nil
	}
	cd.Status.Conditions = conds
	if err := r.Status().Update(context.TODO(), cd); err != nil {
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error updating syncset failed condition")
		return false, err
	}
	return true, nil
}

// getClusterPlatform returns the platform of a given ClusterDeployment
func getClusterPlatform(cd *hivev1.ClusterDeployment) string {
	switch {
	case cd.Spec.Platform.AWS != nil:
		return "aws"
	case cd.Spec.Platform.Azure != nil:
		return "azure"
	case cd.Spec.Platform.GCP != nil:
		return "gcp"
	}
	return "unknown"
}
