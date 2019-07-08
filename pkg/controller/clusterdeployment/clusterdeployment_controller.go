package clusterdeployment

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"time"

	"github.com/pkg/errors"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"

	routev1 "github.com/openshift/api/route/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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

const (
	controllerName     = "clusterDeployment"
	serviceAccountName = "cluster-installer" // service account that can run the installer and upload artifacts to the cluster's namespace.
	defaultRequeueTime = 10 * time.Second

	adminSSHKeySecretKey  = "ssh-publickey"
	adminKubeconfigKey    = "kubeconfig"
	rawAdminKubeconfigKey = "raw-kubeconfig"

	clusterImageSetNotFoundReason = "ClusterImageSetNotFound"
	clusterImageSetFoundReason    = "ClusterImageSetFound"

	dnsNotReadyReason  = "DNSNotReady"
	dnsReadyReason     = "DNSReady"
	dnsReadyAnnotation = "hive.openshift.io/dnsready"

	clusterDeploymentGenerationAnnotation = "hive.openshift.io/cluster-deployment-generation"
	jobHashAnnotation                     = "hive.openshift.io/jobhash"
	firstTimeInstallAnnotation            = "hive.openshift.io/first-time-install"
	deleteAfterAnnotation                 = "hive.openshift.io/delete-after" // contains a duration after which the cluster should be cleaned up.

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

	// regex to find/replace wildcard ingress entries
	// case-insensitive leading literal '*' followed by a literal '.'
	wildcardDomain = regexp.MustCompile(`(?i)^\*\.`)
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
	return &ReconcileClusterDeployment{
		Client:                        controllerutils.NewClientWithMetricsOrDie(mgr, controllerName),
		scheme:                        mgr.GetScheme(),
		logger:                        log.WithField("controller", controllerName),
		remoteClusterAPIClientBuilder: controllerutils.BuildClusterAPIClientFromKubeconfig,
	}
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler) error {
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

	return nil
}

var _ reconcile.Reconciler = &ReconcileClusterDeployment{}

// ReconcileClusterDeployment reconciles a ClusterDeployment object
type ReconcileClusterDeployment struct {
	client.Client
	scheme *runtime.Scheme
	logger log.FieldLogger

	// remoteClusterAPIClientBuilder is a function pointer to the function that builds a client for the
	// remote cluster's cluster-api
	remoteClusterAPIClientBuilder func(string, string) (client.Client, error)
}

// Reconcile reads that state of the cluster for a ClusterDeployment object and makes changes based on the state read
// and what is in the ClusterDeployment.Spec
//
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
//
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=serviceaccounts;secrets;configmaps;events;persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods;namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hive.openshift.io,resources=clusterdeployments;clusterdeployments/status;clusterdeployments/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hive.openshift.io,resources=clusterimagesets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hive.openshift.io,resources=clusterimagesets/status,verbs=get;update;patch
func (r *ReconcileClusterDeployment) Reconcile(request reconcile.Request) (reconcile.Result, error) {
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
		cdLog.WithField("elapsed", dur).Info("reconcile complete")
	}()

	// Fetch the ClusterDeployment instance
	cd := &hivev1.ClusterDeployment{}
	err := r.Get(context.TODO(), request.NamespacedName, cd)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			cdLog.Info("cluster deployment Not Found")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		cdLog.WithError(err).Error("Error getting cluster deployment")
		return reconcile.Result{}, err
	}

	return r.reconcile(request, cd, cdLog)
}

func (r *ReconcileClusterDeployment) reconcile(request reconcile.Request, cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (reconcile.Result, error) {
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

	// We previously allowed clusterdeployment.spec.ingress[] entries to have ingress domains with a leading '*'.
	// Migrate the clusterdeployment to the new format if we find a wildcard ingress domain.
	// TODO: we can one day remove this once all clusterdeployment are known to have non-wildcard data
	if migrateWildcardIngress(cd) {
		cdLog.Info("migrating wildcard ingress entries")
		err := r.Update(context.TODO(), cd)
		if err != nil {
			cdLog.WithError(err).Error("failed to update cluster deployment")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	// TODO: remove this once clusterdeployments have been migrated. We are no longer storing syncset status
	// on clusterdeployments, remove it.
	if len(cd.Status.SyncSetStatus) > 0 || len(cd.Status.SelectorSyncSetStatus) > 0 {
		cd.Status.SyncSetStatus = nil
		cd.Status.SelectorSyncSetStatus = nil
		err := r.Status().Update(context.TODO(), cd)
		if err != nil {
			cdLog.WithError(err).Error("failed to migrate cluster deployment status")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	imageSet, modified, err := r.getClusterImageSet(cd, cdLog)
	if modified || err != nil {
		return reconcile.Result{}, err
	}

	hiveImage := r.getHiveImage(cd, imageSet, cdLog)
	releaseImage := r.getReleaseImage(cd, imageSet, cdLog)

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
		if !cd.Status.Installed {
			hivemetrics.MetricClusterDeploymentProvisionUnderwaySeconds.WithLabelValues(
				cd.Name,
				cd.Namespace,
				hivemetrics.GetClusterDeploymentType(cd)).Set(0.0)
		}

		return r.syncDeletedClusterDeployment(cd, hiveImage, cdLog)
	}

	// requeueAfter will be used to determine if cluster should be requeued after
	// reconcile has completed
	var requeueAfter time.Duration
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
					cdLog.WithError(err).Error("error deleting expired cluster")
				}
				return reconcile.Result{}, err
			}

			// We have an expiry time but we're not expired yet. Set requeueAfter for just after expiry time
			// so that we requeue cluster for deletion once reconcile has completed
			requeueAfter = expiry.Sub(time.Now()) + 60*time.Second
		}
	}

	if !controllerutils.HasFinalizer(cd, hivev1.FinalizerDeprovision) {
		cdLog.Debugf("adding clusterdeployment finalizer")
		if err := r.addClusterDeploymentFinalizer(cd); err != nil {
			cdLog.WithError(err).Error("error adding finalizer")
			return reconcile.Result{}, err
		}
		metricClustersCreated.WithLabelValues(hivemetrics.GetClusterDeploymentType(cd)).Inc()
		return reconcile.Result{}, nil
	}

	cdLog.Debug("loading SSH key secret")
	if cd.Spec.SSHKey == nil {
		cdLog.Error("cluster has no ssh key set, unable to launch install")
		return reconcile.Result{}, fmt.Errorf("cluster has no ssh key set, unable to launch install")
	}
	sshKey, err := controllerutils.LoadSecretData(r.Client, cd.Spec.SSHKey.Name,
		cd.Namespace, adminSSHKeySecretKey)
	if err != nil {
		cdLog.WithError(err).Error("unable to load ssh key from secret")
		return reconcile.Result{}, err
	}

	cdLog.Debug("loading pull secrets")
	pullSecret, err := r.mergePullSecrets(cd, cdLog)
	if err != nil {
		cdLog.WithError(err).Error("Error merging pull secrets")
		return reconcile.Result{}, err
	}

	// Update the pull secret object if required
	modifiedCD, err := r.updatePullSecretInfo(pullSecret, cd, cdLog)
	if err != nil || modifiedCD {
		if err != nil {
			cdLog.WithError(err).Error("Error updating the merged pull secret")
		}
		return reconcile.Result{}, err
	}

	if cd.Status.InstallerImage == nil {
		return r.resolveInstallerImage(cd, imageSet, releaseImage, hiveImage, cdLog)
	}

	if cd.Spec.ManageDNS {
		managedDNSZoneAvailable, dnsZone, err := r.ensureManagedDNSZone(cd, cdLog)
		if err != nil {
			return reconcile.Result{}, err
		}

		modified, err := r.setDNSNotReadyCondition(cd, managedDNSZoneAvailable, cdLog)
		if modified || err != nil {
			return reconcile.Result{}, err
		}

		if !managedDNSZoneAvailable {
			// The clusterdeployment will be queued when the owned DNSZone's status
			// is updated to available.
			cdLog.Debug("DNSZone is not yet available. Waiting for zone to become available.")
			return reconcile.Result{}, nil
		}
		updated, err := r.setDNSDelayMetric(cd, dnsZone, cdLog)
		if updated || err != nil {
			return reconcile.Result{}, err
		}
	}

	// firstInstalledObserve is the flag that is used for reporting the provision job duration metric
	firstInstalledObserve := false
	containerRestarts := 0
	// Check if an install job already exists:
	existingJob := &batchv1.Job{}
	installJobName := install.GetInstallJobName(cd)
	err = r.Get(context.TODO(), types.NamespacedName{Name: installJobName, Namespace: cd.Namespace}, existingJob)
	if err != nil {
		if apierrors.IsNotFound(err) {
			cdLog.Debug("no install job exists")
			existingJob = nil
		} else {
			cdLog.WithError(err).Error("error looking for install job")
			return reconcile.Result{}, err
		}
	} else {
		if !existingJob.DeletionTimestamp.IsZero() {
			cdLog.WithError(err).Error("install job is being deleted, requeueing to wait for deletion")
			return reconcile.Result{RequeueAfter: defaultRequeueTime}, nil
		}
		// setting the flag so that we can report the metric after cd is installed
		if existingJob.Status.Succeeded > 0 && !cd.Status.Installed {
			firstInstalledObserve = true
		}
	}

	if cd.Status.Installed {
		cdLog.Debug("cluster is already installed, no processing of install job needed")
		r.cleanupInstallLogPVC(cd, existingJob, cdLog)
	} else {
		// Indicate that the cluster is still installing:
		hivemetrics.MetricClusterDeploymentProvisionUnderwaySeconds.WithLabelValues(
			cd.Name,
			cd.Namespace,
			hivemetrics.GetClusterDeploymentType(cd)).Set(
			time.Since(cd.CreationTimestamp.Time).Seconds())

		job, pvc, err := install.GenerateInstallerJob(
			cd,
			hiveImage,
			releaseImage,
			serviceAccountName,
			sshKey)
		if err != nil {
			cdLog.WithError(err).Error("error generating install job")
			return reconcile.Result{}, err
		}

		jobHash, err := controllerutils.CalculateJobSpecHash(job)
		if err != nil {
			cdLog.WithError(err).Error("failed to calculate hash for generated install job")
			return reconcile.Result{}, err
		}
		if job.Annotations == nil {
			job.Annotations = map[string]string{}
		}
		job.Annotations[jobHashAnnotation] = jobHash

		if err = controllerutil.SetControllerReference(cd, job, r.scheme); err != nil {
			cdLog.WithError(err).Error("error setting controller reference on job")
			return reconcile.Result{}, err
		}

		cdLog = cdLog.WithField("job", job.Name)

		// pvc will be nil if log gathering is disabled:
		if pvc != nil {
			if err := r.reconcileLogsPVC(cd, pvc, cdLog); err != nil {
				return reconcile.Result{}, err
			}
		}

		if existingJob == nil {
			cdLog.Infof("creating install job")
			_, err = controllerutils.SetupClusterInstallServiceAccount(r, cd.Namespace, cdLog)
			if err != nil {
				cdLog.WithError(err).Error("error setting up service account and role")
				return reconcile.Result{}, err
			}

			err = r.Create(context.TODO(), job)
			if err != nil {
				cdLog.Errorf("error creating job: %v", err)
				return reconcile.Result{}, err
			}
			if _, ok := cd.Annotations[firstTimeInstallAnnotation]; !ok {
				initializeAnnotations(cd)

				// Add the annotation for first time install
				cd.Annotations[firstTimeInstallAnnotation] = "true"
				if err := r.Client.Update(context.TODO(), cd); err != nil {
					cdLog.WithError(err).Error("failed to save annotation for firstTimeInstall")
				}
				kickstartDuration := time.Since(cd.CreationTimestamp.Time)
				cdLog.WithField("elapsed", kickstartDuration.Seconds()).Info("calculated time to install job seconds")
				metricInstallDelaySeconds.Observe(float64(kickstartDuration.Seconds()))
			}
		} else {
			cdLog.Debug("provision job exists")
			containerRestarts, err = r.calcInstallPodRestarts(cd, cdLog)
			if err != nil {
				// Metrics calculation should not shut down reconciliation, logging and moving on.
				cdLog.WithError(err).Warn("error listing pods, unable to calculate pod restarts but continuing")
			} else {
				if containerRestarts > 0 {
					cdLog.WithFields(log.Fields{
						"restarts": containerRestarts,
					}).Warn("install pod has restarted")
				}

				// Store the restart count on the cluster deployment status.
				cd.Status.InstallRestarts = containerRestarts
			}

			if existingJob.Annotations != nil {
				didGenerationChange, err := r.updateOutdatedConfigurations(cd.Generation, existingJob, cdLog)
				if didGenerationChange || err != nil {
					return reconcile.Result{}, err
				}
			}

			jobDeleted, err := r.deleteJobOnHashChange(existingJob, job, cdLog)
			if jobDeleted || err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	err = r.updateClusterDeploymentStatus(cd, origCD, existingJob, cdLog)
	if err != nil {
		cdLog.WithError(err).Errorf("error updating cluster deployment status")
		return reconcile.Result{}, err
	}

	// firstInstalledObserve will be true if this is the first time we've noticed the install job completed.
	// If true, we know we can report the metrics associated with a completed job.
	if firstInstalledObserve {
		// jobDuration calculates the time elapsed since the install job started
		jobDuration := existingJob.Status.CompletionTime.Time.Sub(existingJob.Status.StartTime.Time)
		cdLog.WithField("duration", jobDuration.Seconds()).Debug("install job completed")
		metricInstallJobDuration.Observe(float64(jobDuration.Seconds()))

		// Report a metric for the total number of container restarts:
		metricCompletedInstallJobRestarts.WithLabelValues(hivemetrics.GetClusterDeploymentType(cd)).
			Observe(float64(containerRestarts))

		// Clear the install underway seconds metric. After this no-one should be reporting
		// this metric for this cluster.
		hivemetrics.MetricClusterDeploymentProvisionUnderwaySeconds.WithLabelValues(
			cd.Name,
			cd.Namespace,
			hivemetrics.GetClusterDeploymentType(cd)).Set(0.0)

		metricClustersInstalled.WithLabelValues(hivemetrics.GetClusterDeploymentType(cd)).Inc()
	}

	// Check for requeueAfter duration
	if requeueAfter != 0 {
		cdLog.Debugf("cluster will re-sync due to expiry time in: %v", requeueAfter)
		return reconcile.Result{RequeueAfter: requeueAfter}, nil
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileClusterDeployment) reconcileLogsPVC(cd *hivev1.ClusterDeployment, pvc *corev1.PersistentVolumeClaim, cdLog log.FieldLogger) error {
	if err := controllerutil.SetControllerReference(cd, pvc, r.scheme); err != nil {
		cdLog.WithError(err).Error("error setting controller reference on pvc")
		return err
	}

	existingPVC := &corev1.PersistentVolumeClaim{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: pvc.Name, Namespace: pvc.Namespace}, existingPVC)
	if apierrors.IsNotFound(err) {
		cdLog.WithField("pvc", pvc.Name).Info("creating persistent volume claim")
		err = r.Create(context.TODO(), pvc)
		if err != nil {
			cdLog.WithError(err).Error("error creating pvc")
			return err
		}
	} else if err != nil {
		cdLog.WithError(err).Error("error getting persistent volume claim")
		return err
	}

	return nil
}

// getHiveImage looks for a Hive image to use in clusterdeployment jobs in the following order:
// 1 - specified in the cluster deployment spec.images.hiveImage
// 2 - referenced in the cluster deployment spec.imageSet
// 3 - specified via environment variable to the hive controller
// 4 - fallback default hardcoded image reference
func (r *ReconcileClusterDeployment) getHiveImage(cd *hivev1.ClusterDeployment, imageSet *hivev1.ClusterImageSet, cdLog log.FieldLogger) string {
	if cd.Spec.Images.HiveImage != "" {
		return cd.Spec.Images.HiveImage
	}
	if imageSet != nil && imageSet.Spec.HiveImage != nil {
		return *imageSet.Spec.HiveImage
	}
	return images.GetHiveImage(cdLog)
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

func (r *ReconcileClusterDeployment) getClusterImageSet(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (*hivev1.ClusterImageSet, bool, error) {
	if cd.Spec.ImageSet == nil || len(cd.Spec.ImageSet.Name) == 0 {
		return nil, false, nil
	}
	imageSet := &hivev1.ClusterImageSet{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: cd.Spec.ImageSet.Name}, imageSet)
	switch {
	case apierrors.IsNotFound(err):
		cdLog.WithField("clusterimageset", cd.Spec.ImageSet.Name).Warning("clusterdeployment references non-existent clusterimageset")
		modified, err := r.setImageSetNotFoundCondition(cd, false, cdLog)
		return nil, modified, err
	case err != nil:
		cdLog.WithError(err).WithField("clusterimageset", cd.Spec.ImageSet.Name).Error("unexpected error retrieving clusterimageset")
		return nil, false, err
	default:
		return imageSet, false, nil
	}
}

func (r *ReconcileClusterDeployment) statusUpdate(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) error {
	err := r.Status().Update(context.TODO(), cd)
	if err != nil {
		cdLog.WithError(err).Error("cannot update clusterdeployment status")
	}
	return err
}

func (r *ReconcileClusterDeployment) resolveInstallerImage(cd *hivev1.ClusterDeployment, imageSet *hivev1.ClusterImageSet, releaseImage, hiveImage string, cdLog log.FieldLogger) (reconcile.Result, error) {
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
	cliImage := images.GetCLIImage(cdLog)
	job := imageset.GenerateImageSetJob(cd, releaseImage, serviceAccountName, imageset.AlwaysPullImage(cliImage), imageset.AlwaysPullImage(hiveImage))
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
			Warning("Finished job found, but installer image is not yet resolved. Deleting.")
		err := r.Delete(context.Background(), existingJob,
			client.PropagationPolicy(metav1.DeletePropagationForeground))
		if err != nil {
			jobLog.WithError(err).Error("cannot delete imageset job")
		}
		return reconcile.Result{}, err
	case apierrors.IsNotFound(err):
		jobLog.WithField("releaseImage", releaseImage).Info("creating imageset job")
		_, err = controllerutils.SetupClusterInstallServiceAccount(r, cd.Namespace, cdLog)
		if err != nil {
			cdLog.WithError(err).Error("error setting up service account and role")
			return reconcile.Result{}, err
		}

		err = r.Create(context.TODO(), job)
		if err != nil {
			jobLog.WithError(err).Error("error creating job")
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

func (r *ReconcileClusterDeployment) setDNSNotReadyCondition(cd *hivev1.ClusterDeployment, isReady bool, cdLog log.FieldLogger) (modified bool, err error) {
	original := cd.DeepCopy()
	status := corev1.ConditionFalse
	reason := dnsReadyReason
	message := "DNS Zone available"
	if !isReady {
		status = corev1.ConditionTrue
		reason = dnsNotReadyReason
		message = "DNS Zone not yet available"
	}
	cd.Status.Conditions = controllerutils.SetClusterDeploymentCondition(
		cd.Status.Conditions,
		hivev1.DNSNotReadyCondition,
		status,
		reason,
		message,
		controllerutils.UpdateConditionNever)
	if !reflect.DeepEqual(original.Status.Conditions, cd.Status.Conditions) {
		cdLog.Debugf("setting DNSNotReadyCondition to %v", status)
		err := r.Status().Update(context.TODO(), cd)
		if err != nil {
			cdLog.WithError(err).Error("cannot update status conditions")
		}
		return true, err
	}
	return false, nil
}

func (r *ReconcileClusterDeployment) setImageSetNotFoundCondition(cd *hivev1.ClusterDeployment, isNotFound bool, cdLog log.FieldLogger) (modified bool, err error) {
	original := cd.DeepCopy()
	status := corev1.ConditionFalse
	reason := clusterImageSetFoundReason
	message := fmt.Sprintf("ClusterImageSet %s is available", cd.Spec.ImageSet.Name)
	if isNotFound {
		status = corev1.ConditionTrue
		reason = clusterImageSetNotFoundReason
		message = fmt.Sprintf("ClusterImageSet %s is not available", cd.Spec.ImageSet.Name)
	}
	cd.Status.Conditions = controllerutils.SetClusterDeploymentCondition(
		cd.Status.Conditions,
		hivev1.ClusterImageSetNotFoundCondition,
		status,
		reason,
		message,
		controllerutils.UpdateConditionNever)
	if !reflect.DeepEqual(original.Status.Conditions, cd.Status.Conditions) {
		cdLog.Info("setting ClusterImageSetNotFoundCondition to %v", status)
		err := r.Status().Update(context.TODO(), cd)
		if err != nil {
			cdLog.WithError(err).Error("cannot update status conditions")
		}
		return true, err
	}
	return false, nil
}

// Deletes the job if it exists and its generation does not match the cluster deployment's
// genetation. Updates the config map if it is outdated too
func (r *ReconcileClusterDeployment) updateOutdatedConfigurations(cdGeneration int64, existingJob *batchv1.Job, cdLog log.FieldLogger) (bool, error) {
	var err error
	var didGenerationChange bool
	if jobGeneration, ok := existingJob.Annotations[clusterDeploymentGenerationAnnotation]; ok {
		convertedJobGeneration, _ := strconv.ParseInt(jobGeneration, 10, 64)
		if convertedJobGeneration < cdGeneration {
			didGenerationChange = true
			cdLog.Info("deleting outdated install job due to cluster deployment generation change")
			err = r.Delete(context.TODO(), existingJob, client.PropagationPolicy(metav1.DeletePropagationForeground))
			if err != nil {
				cdLog.WithError(err).Errorf("error deleting outdated install job")
				return didGenerationChange, err
			}
		}
	}
	return didGenerationChange, err
}

func (r *ReconcileClusterDeployment) updateClusterDeploymentStatus(cd *hivev1.ClusterDeployment, origCD *hivev1.ClusterDeployment, job *batchv1.Job, cdLog log.FieldLogger) error {
	cdLog.Debug("updating cluster deployment status")
	if job != nil && job.Name != "" && job.Namespace != "" {
		// Job exists, check it's status:
		cd.Status.Installed = controllerutils.IsSuccessful(job)
	}

	// The install manager sets this secret name, but we don't consider it a critical failure and
	// will attempt to heal it here, as the value is predictable.
	if cd.Status.Installed && cd.Status.AdminKubeconfigSecret.Name == "" {
		cd.Status.AdminKubeconfigSecret = corev1.LocalObjectReference{Name: apihelpers.GetResourceName(cd.Name, "admin-kubeconfig")}
	}

	if cd.Status.AdminKubeconfigSecret.Name != "" {
		adminKubeconfigSecret := &corev1.Secret{}
		err := r.Get(context.Background(), types.NamespacedName{Namespace: cd.Namespace, Name: cd.Status.AdminKubeconfigSecret.Name}, adminKubeconfigSecret)
		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Warn("admin kubeconfig does not yet exist")
			} else {
				return err
			}
		} else {
			err = r.fixupAdminKubeconfigSecret(adminKubeconfigSecret, cdLog)
			if err != nil {
				return err
			}
			err = r.setAdminKubeconfigStatus(cd, adminKubeconfigSecret, cdLog)
			if err != nil {
				return err
			}
		}
	}

	// Update cluster deployment status if changed:
	if !reflect.DeepEqual(cd.Status, origCD.Status) {
		cdLog.Infof("status has changed, updating cluster deployment")
		cdLog.Debugf("orig: %v", origCD)
		cdLog.Debugf("new : %v", cd.Status)
		err := r.Status().Update(context.TODO(), cd)
		if err != nil {
			cdLog.Errorf("error updating cluster deployment: %v", err)
			return err
		}
	} else {
		cdLog.Debug("cluster deployment status unchanged")
	}
	return nil
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
		cdLog.WithError(err).Error("error updated admin kubeconfig secret")
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
	dnsZoneNamespacedName := types.NamespacedName{Namespace: cd.Namespace, Name: dnsZoneName(cd.Name)}
	err := r.Get(context.TODO(), dnsZoneNamespacedName, dnsZone)
	if err != nil && !apierrors.IsNotFound(err) {
		cdLog.WithError(err).Error("error looking up managed dnszone")
		return &reconcile.Result{}, err
	}
	if apierrors.IsNotFound(err) || !dnsZone.DeletionTimestamp.IsZero() {
		cdLog.Debug("dnszone has been deleted or is getting deleted")
		return nil, nil
	}
	cdLog.Warn("managed dnszone did not get a deletionTimestamp when parent cluster deployment was deleted, deleting manually")
	err = r.Delete(context.TODO(), dnsZone,
		client.PropagationPolicy(metav1.DeletePropagationForeground))
	if err != nil {
		cdLog.WithError(err).Error("error deleting managed dnszone")
	}
	return &reconcile.Result{}, err
}

func (r *ReconcileClusterDeployment) syncDeletedClusterDeployment(cd *hivev1.ClusterDeployment, hiveImage string, cdLog log.FieldLogger) (reconcile.Result, error) {

	result, err := r.ensureManagedDNSZoneDeleted(cd, cdLog)
	if result != nil {
		return *result, err
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	// Delete the install job in case it's still running:
	installJob := &batchv1.Job{}
	err = r.Get(context.Background(),
		types.NamespacedName{
			Name:      install.GetInstallJobName(cd),
			Namespace: cd.Namespace,
		},
		installJob)
	if err != nil && apierrors.IsNotFound(err) {
		cdLog.Debug("install job no longer exists, nothing to cleanup")
	} else if err != nil {
		cdLog.WithError(err).Errorf("error getting existing install job for deleted cluster deployment")
		return reconcile.Result{}, err
	} else if !installJob.DeletionTimestamp.IsZero() {
		cdLog.WithField("finalizers", installJob.Finalizers).Info("install job is being deleted, requeueing to wait for deletion")
		return reconcile.Result{RequeueAfter: defaultRequeueTime}, nil
	} else {
		err = r.Delete(context.Background(), installJob,
			client.PropagationPolicy(metav1.DeletePropagationForeground))
		if err != nil {
			cdLog.WithError(err).Errorf("error deleting existing install job for deleted cluster deployment")
			return reconcile.Result{}, err
		}
		cdLog.WithField("jobName", installJob.Name).Info("install job deleted")
		return reconcile.Result{}, nil
	}

	// Skips creation of deprovision request if PreserveOnDelete is true and cluster is installed
	if cd.Spec.PreserveOnDelete {
		if cd.Status.Installed {
			cdLog.Warn("skipping creation of deprovisioning request for installed cluster due to PreserveOnDelete=true")
			if controllerutils.HasFinalizer(cd, hivev1.FinalizerDeprovision) {
				err = r.removeClusterDeploymentFinalizer(cd)
				if err != nil {
					cdLog.WithError(err).Error("error removing finalizer")
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
			cdLog.WithError(err).Error("error removing finalizer")
		}
		return reconcile.Result{}, err
	}

	// Generate a deprovision request
	request := generateDeprovisionRequest(cd)
	err = controllerutil.SetControllerReference(cd, request, r.scheme)
	if err != nil {
		cdLog.Errorf("error setting controller reference on deprovision request: %v", err)
		return reconcile.Result{}, err
	}

	// Check if deprovision request already exists:
	existingRequest := &hivev1.ClusterDeprovisionRequest{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: cd.Name, Namespace: cd.Namespace}, existingRequest)
	if err != nil && apierrors.IsNotFound(err) {
		cdLog.Infof("creating deprovision request for cluster deployment")
		err = r.Create(context.TODO(), request)
		if err != nil {
			cdLog.WithError(err).Errorf("error creating deprovision request")
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
					cdLog.WithError(err).Error("error removing finalizer")
				}
			}
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	} else if err != nil {
		cdLog.WithError(err).Errorf("error getting deprovision request")
		return reconcile.Result{}, err
	}

	// Deprovision request exists, check whether it has completed
	if existingRequest.Status.Completed {
		cdLog.Infof("deprovision request completed, removing finalizer")
		err = r.removeClusterDeploymentFinalizer(cd)
		if err != nil {
			cdLog.WithError(err).Error("error removing finalizer")
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
	err := r.Update(context.TODO(), cd)

	if err == nil {
		clearUnderwaySecondsMetrics(cd)

		// Increment the clusters deleted counter:
		metricClustersDeleted.WithLabelValues(hivemetrics.GetClusterDeploymentType(cd)).Inc()
	}

	return err
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
	if err := r.Client.Update(context.TODO(), cd); err != nil {
		cdLog.WithError(err).Error("failed to save annotation marking DNS becoming ready")
		return modified, err
	}
	modified = true

	metricDNSDelaySeconds.Observe(float64(dnsDelayDuration.Seconds()))

	return modified, nil
}

func (r *ReconcileClusterDeployment) ensureManagedDNSZone(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (bool, *hivev1.DNSZone, error) {
	// for now we only support AWS
	if cd.Spec.AWS == nil || cd.Spec.PlatformSecrets.AWS == nil {
		cdLog.Error("cluster deployment platform is not AWS, cannot manage DNS zone")
		return false, nil, fmt.Errorf("only AWS managed DNS is supported")
	}
	dnsZone := &hivev1.DNSZone{}
	dnsZoneNamespacedName := types.NamespacedName{Namespace: cd.Namespace, Name: dnsZoneName(cd.Name)}
	logger := cdLog.WithField("zone", dnsZoneNamespacedName.String())

	err := r.Get(context.TODO(), dnsZoneNamespacedName, dnsZone)
	if err == nil {
		availableCondition := controllerutils.FindDNSZoneCondition(dnsZone.Status.Conditions, hivev1.ZoneAvailableDNSZoneCondition)
		return availableCondition != nil && availableCondition.Status == corev1.ConditionTrue, dnsZone, nil
	}
	if apierrors.IsNotFound(err) {
		logger.Info("creating new DNSZone for cluster deployment")
		return false, nil, r.createManagedDNSZone(cd, logger)
	}
	logger.WithError(err).Error("failed to fetch DNS zone")
	return false, nil, err
}

func (r *ReconcileClusterDeployment) createManagedDNSZone(cd *hivev1.ClusterDeployment, logger log.FieldLogger) error {
	dnsZone := &hivev1.DNSZone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dnsZoneName(cd.Name),
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
		logger.WithError(err).Error("cannot create DNS zone")
		return err
	}
	logger.Info("dns zone created")
	return nil
}

func dnsZoneName(cdName string) string {
	return apihelpers.GetResourceName(cdName, "zone")
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
	cdName, ok := pod.Labels[install.ClusterDeploymentNameLabel]
	if !ok {
		return retval
	}
	retval = append(retval, reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      cdName,
		Namespace: pod.Namespace,
	}})
	return retval
}

func (r *ReconcileClusterDeployment) calcInstallPodRestarts(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (int, error) {
	installerPodLabels := map[string]string{install.ClusterDeploymentNameLabel: cd.Name, install.InstallJobLabel: "true"}
	pods := &corev1.PodList{}
	err := r.Client.List(context.Background(), pods, client.InNamespace(cd.Namespace), client.MatchingLabels(installerPodLabels))
	if err != nil {
		return 0, err
	}

	if len(pods.Items) > 1 {
		log.Warnf("found %d install pods for cluster", len(pods.Items))
	}

	// Calculate restarts across all containers in the pod:
	containerRestarts := 0
	for _, pod := range pods.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			containerRestarts += int(cs.RestartCount)
		}
	}
	return containerRestarts, nil
}

func (r *ReconcileClusterDeployment) deleteJobOnHashChange(existingJob, generatedJob *batchv1.Job, cdLog log.FieldLogger) (bool, error) {
	newJobNeeded := false
	if _, ok := existingJob.Annotations[jobHashAnnotation]; !ok {
		// this job predates tracking the job hash, so assume we need a new job
		newJobNeeded = true
	}

	if existingJob.Annotations[jobHashAnnotation] != generatedJob.Annotations[jobHashAnnotation] {
		// delete the job so we get a fresh one with the new job spec
		newJobNeeded = true
	}

	if newJobNeeded {
		// delete the existing job
		cdLog.Info("deleting existing install job due to updated/missing hash detected")
		err := r.Delete(context.TODO(), existingJob, client.PropagationPolicy(metav1.DeletePropagationForeground))
		if err != nil {
			cdLog.WithError(err).Errorf("error deleting outdated install job")
			return newJobNeeded, err
		}
	}

	return newJobNeeded, nil
}

// cleanupInstallLogPVC will immediately delete the PVC (should it exist) if the cluster was installed successfully, without retries.
// If there were retries, it will delete the PVC if it has been more than 7 days since the job was completed.
func (r *ReconcileClusterDeployment) cleanupInstallLogPVC(cd *hivev1.ClusterDeployment, existingJob *batchv1.Job, cdLog log.FieldLogger) error {
	if !cd.Status.Installed {
		return nil
	}

	if existingJob == nil {
		return nil
	}

	pvc := &corev1.PersistentVolumeClaim{}
	// PVC re-uses the name of the install job as they are related, and we know it should be unique.
	err := r.Get(context.TODO(), types.NamespacedName{Name: existingJob.Name, Namespace: existingJob.Namespace}, pvc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		cdLog.WithError(err).Error("error looking up install logs PVC")
		return err
	}

	if cd.Status.InstallRestarts == 0 {
		cdLog.WithField("pvc", pvc.Name).Info("deleting logs PersistentVolumeClaim for installed cluster with no restarts")
		if err := r.Delete(context.TODO(), pvc); err != nil {
			cdLog.WithError(err).Error("error deleting install logs PVC")
			return err
		}
		return nil
	}

	// Otherwise, delete if more than 7 days have passed.
	if time.Since(existingJob.Status.CompletionTime.Time) > (7 * 24 * time.Hour) {
		cdLog.WithField("pvc", pvc.Name).Info("deleting logs PersistentVolumeClaim for cluster that was installed after restarts more than 7 days ago")
		if err := r.Delete(context.TODO(), pvc); err != nil {
			cdLog.WithError(err).Error("error deleting install logs PVC")
			return err
		}
		return nil
	}

	cdLog.WithField("pvc", pvc.Name).Debug("preserving logs PersistentVolumeClaim for cluster with install restarts for 7 days")
	return nil

}

func generateDeprovisionRequest(cd *hivev1.ClusterDeployment) *hivev1.ClusterDeprovisionRequest {
	req := &hivev1.ClusterDeprovisionRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cd.Name,
			Namespace: cd.Namespace,
		},
		Spec: hivev1.ClusterDeprovisionRequestSpec{
			InfraID:   cd.Status.InfraID,
			ClusterID: cd.Status.ClusterID,
			Platform: hivev1.ClusterDeprovisionRequestPlatform{
				AWS: &hivev1.AWSClusterDeprovisionRequest{},
			},
		},
	}

	if cd.Spec.Platform.AWS != nil {
		req.Spec.Platform.AWS.Region = cd.Spec.Platform.AWS.Region
	}

	if cd.Spec.PlatformSecrets.AWS != nil {
		req.Spec.Platform.AWS.Credentials = &cd.Spec.PlatformSecrets.AWS.Credentials
	}

	return req
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

func migrateWildcardIngress(cd *hivev1.ClusterDeployment) bool {
	migrated := false
	for i, ingress := range cd.Spec.Ingress {
		newIngress := wildcardDomain.ReplaceAllString(ingress.Domain, "")
		if newIngress != ingress.Domain {
			cd.Spec.Ingress[i].Domain = newIngress
			migrated = true
		}
	}
	return migrated
}

func dnsReadyTransitionTime(dnsZone *hivev1.DNSZone) *time.Time {
	readyCondition := controllerutils.FindDNSZoneCondition(dnsZone.Status.Conditions, hivev1.ZoneAvailableDNSZoneCondition)

	if readyCondition != nil && readyCondition.Status == corev1.ConditionTrue {
		return &readyCondition.LastTransitionTime.Time
	}

	return nil
}

func strPtr(s string) *string {
	return &s
}

func clearUnderwaySecondsMetrics(cd *hivev1.ClusterDeployment) {
	// If we've successfully cleared the deprovision finalizer we know this is a good time to
	// reset the underway metric to 0, after which it will no longer be reported.
	hivemetrics.MetricClusterDeploymentDeprovisioningUnderwaySeconds.WithLabelValues(
		cd.Name,
		cd.Namespace,
		hivemetrics.GetClusterDeploymentType(cd)).Set(0.0)

	// Clear the install underway seconds metric if this cluster was still installing.
	if !cd.Status.Installed {
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
	globalPullSecret := os.Getenv("GLOBAL_PULL_SECRET")

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

// updatePullSecretInfo adds pull secret information in cluster deployment and cluster deployment status.
// It returns true when cluster deployment status has been updated.
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
			return false, errors.New(fmt.Sprintf("Pull secret %s did not contain key %s", mergedSecretName, corev1.DockerConfigJsonKey))
		}
		if controllerutils.GetHashOfPullSecret(string(existingPullSecret)) == controllerutils.GetHashOfPullSecret(pullSecret) {
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
