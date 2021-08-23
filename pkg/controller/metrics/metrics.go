package metrics

import (
	"context"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hiveintv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/imageset"
)

const (
	ControllerName = hivev1.MetricsControllerName
)

var (
	metricClusterDeploymentsTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_cluster_deployments",
		Help: "Total number of cluster deployments.",
	}, []string{"cluster_type", "age_lt"})
	metricClusterDeploymentsInstalledTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_cluster_deployments_installed",
		Help: "Total number of cluster deployments that are successfully installed.",
	}, []string{"cluster_type", "age_lt"})
	metricClusterDeploymentsUninstalledTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_cluster_deployments_uninstalled",
		Help: "Total number of cluster deployments that are not yet installed by type and bucket for length of time in this state.",
	},
		[]string{"cluster_type", "age_lt", "uninstalled_gt"},
	)
	metricClusterDeploymentsDeprovisioningTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_cluster_deployments_deprovisioning",
		Help: "Total number of cluster deployments in process of being deprovisioned.",
	}, []string{"cluster_type", "age_lt", "deprovisioning_gt"})
	metricClusterDeploymentsWithConditionTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_cluster_deployments_conditions",
		Help: "Total number of cluster deployments by type with conditions in their undesired state",
	}, []string{"cluster_type", "age_lt", "condition"})
	metricInstallJobsTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_install_jobs",
		Help: "Total number of install jobs running by cluster type and state.",
	}, []string{"cluster_type", "state"})
	metricUninstallJobsTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_uninstall_jobs",
		Help: "Total number of uninstall jobs running by cluster type and state.",
	}, []string{"cluster_type", "state"})
	metricImagesetJobsTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_imageset_jobs",
		Help: "Total number of imageset jobs running by cluster type and state.",
	}, []string{"cluster_type", "state"})
	metricSelectorSyncSetClustersTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_selectorsyncset_clusters_total",
		Help: "Total number of SyncSetInstances that match the label selector for each SelectorSyncSet.",
	}, []string{"name"})
	metricSelectorSyncSetClustersUnappliedTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_selectorsyncset_clusters_unapplied_total",
		Help: "Total number of SyncSetInstances that match the label selector for each SelectorSyncSet but have not successfully applied all resources/patches/secrets.",
	}, []string{"name"})
	metricSyncSetsTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "hive_syncsets_total",
		Help: "Total number of SyncSetInstances referencing non-selector SyncSets.",
	})
	metricSyncSetsUnappliedTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "hive_syncsets_unapplied_total",
		Help: "Total number of SyncSetsInstances referencing non-selector SyncSets that have not successfully applied all resources/patches/secrets.",
	})

	// MetricClusterDeploymentDeprovisioningUnderwaySeconds is a prometheus metric for the number of seconds
	// between when a still deprovisioning cluster was created and now.
	MetricClusterDeploymentDeprovisioningUnderwaySeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "hive_cluster_deployment_deprovision_underway_seconds",
			// Will clear once hive restarts.
			Help: "Length of time a cluster has been deprovisioning.",
		},
		[]string{"cluster_deployment", "namespace", "cluster_type"},
	)
	// metricClusterDeploymentPowerStateTransitionSeconds is a prometheus metric for the number of seconds it takes
	// for a cluster to transition to the desired power state. It is cleared once the desired state is reached
	metricClusterDeploymentPowerStateTransitionSeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "hive_cluster_deployment_powerstate_transition_seconds",
			Help: "Length of time for a cluster to transition to the desired power state",
		},
		[]string{"cluster_deployment", "desired_powerstate"},
	)
	// metricControllerReconcileTime tracks the length of time our reconcile loops take. controller-runtime
	// technically tracks this for us, but due to bugs currently also includes time in the queue, which leads to
	// extremely strange results. For now, track our own metric.
	metricControllerReconcileTime = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hive_controller_reconcile_seconds",
			Help:    "Distribution of the length of time each controllers reconcile loop takes.",
			Buckets: []float64{0.001, 0.01, 0.1, 1, 10, 30, 60, 120},
		},
		[]string{"controller", "outcome"},
	)
	// metricClusterDeploymentSyncsetPaused tracks ClusterDeployments which
	// are NOT being synchronized by Hive.  This is achieved by the presence
	// of a hive.openshift.io/syncset-pause="true" annotation.  The value is
	// a boolean with "1" indicating the annotation is present.
	metricClusterDeploymentSyncsetPaused = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "hive_cluster_deployment_syncset_paused",
			Help: "Whether Hive has paused syncing to the cluster",
		},
		[]string{"cluster_deployment", "namespace", "cluster_type"},
	)
)

// ReconcileOutcome is used in controller "reconcile complete" log entries, and the metricControllerReconcileTime
// above for controllers where we would like to monitor performance for different types of Reconcile outcomes. To help with
// prometheus cardinality this set of outcomes should be kept small and only used for coarse and very high value
// categories. Controllers must report an outcome but can report unspecified if this categorization is not needed.
type ReconcileOutcome string

const (
	ReconcileOutcomeUnspecified        ReconcileOutcome = "unspecified"
	ReconcileOutcomeNoOp               ReconcileOutcome = "no-op"
	ReconcileOutcomeSkippedSync        ReconcileOutcome = "skipped-sync"
	ReconcileOutcomeFullSync           ReconcileOutcome = "full-sync"
	ReconcileOutcomeClusterSyncCreated ReconcileOutcome = "clustersync-created"
)

func init() {
	metrics.Registry.MustRegister(metricClusterDeploymentsTotal)
	metrics.Registry.MustRegister(metricClusterDeploymentsInstalledTotal)
	metrics.Registry.MustRegister(metricClusterDeploymentsUninstalledTotal)
	metrics.Registry.MustRegister(metricClusterDeploymentsDeprovisioningTotal)
	metrics.Registry.MustRegister(metricClusterDeploymentsWithConditionTotal)
	metrics.Registry.MustRegister(metricInstallJobsTotal)
	metrics.Registry.MustRegister(metricUninstallJobsTotal)
	metrics.Registry.MustRegister(metricImagesetJobsTotal)
	metrics.Registry.MustRegister(metricSelectorSyncSetClustersTotal)
	metrics.Registry.MustRegister(metricSelectorSyncSetClustersUnappliedTotal)
	metrics.Registry.MustRegister(metricSyncSetsTotal)
	metrics.Registry.MustRegister(metricSyncSetsUnappliedTotal)
	metrics.Registry.MustRegister(metricControllerReconcileTime)

	metrics.Registry.MustRegister(MetricClusterDeploymentDeprovisioningUnderwaySeconds)
	metrics.Registry.MustRegister(metricClusterDeploymentPowerStateTransitionSeconds)
	metrics.Registry.MustRegister(metricClusterDeploymentSyncsetPaused)
}

// Add creates a new metrics Calculator and adds it to the Manager.
func Add(mgr manager.Manager) error {
	mc := &Calculator{
		Client:   mgr.GetClient(),
		Interval: 2 * time.Minute,
	}
	metrics.Registry.MustRegister(newProvisioningUnderwaySecondsCollector(mgr.GetClient(), 1*time.Hour))
	metrics.Registry.MustRegister(newProvisioningUnderwayInstallRestartsCollector(mgr.GetClient(), 1))
	err := mgr.Add(mc)
	if err != nil {
		return err
	}
	return nil
}

// Calculator runs in a goroutine and periodically calculates and publishes
// Prometheus metrics which will be exposed at our /metrics endpoint. Note that this is not
// a standard controller watching Kube resources, it runs periodically and then goes to sleep.
//
// This should be used for metrics which do not fit well into controller reconcile loops,
// things that are calculated globally rather than metrics related to specific reconciliations.
type Calculator struct {
	Client client.Client

	// Interval is the length of time we sleep between metrics calculations.
	Interval time.Duration
}

// Start begins the metrics calculation loop.
func (mc *Calculator) Start(ctx context.Context) error {
	log.Info("started metrics calculator goroutine")

	// Run forever, sleep at the end:
	wait.UntilWithContext(ctx, func(ctx context.Context) {
		mcLog := log.WithField("controller", "metrics")
		recobsrv := NewReconcileObserver(ControllerName, mcLog)
		defer recobsrv.ObserveControllerReconcileTime()

		mcLog.Info("calculating metrics across all ClusterDeployments")
		// Load all ClusterDeployments so we can accumulate facts about them.
		clusterDeployments := &hivev1.ClusterDeploymentList{}
		err := mc.Client.List(ctx, clusterDeployments)
		if err != nil {
			log.WithError(err).Error("error listing cluster deployments")
		} else {
			accumulator, err := newClusterAccumulator(infinity, []string{"0h", "1h", "2h", "8h", "24h", "72h"})
			if err != nil {
				mcLog.WithError(err).Error("unable to calculate metrics")
				return
			}
			for _, cd := range clusterDeployments.Items {
				clusterType := GetClusterDeploymentType(&cd)
				accumulator.processCluster(&cd)

				if cd.DeletionTimestamp != nil {

					// For deprovisioning clusters we report the seconds since
					// cluster was deleted. clusterdeployment_controller should delete this
					// when removing the finalizer.
					MetricClusterDeploymentDeprovisioningUnderwaySeconds.WithLabelValues(
						cd.Name,
						cd.Namespace,
						clusterType).Set(
						time.Since(cd.CreationTimestamp.Time).Seconds())
				}

				if paused, err := strconv.ParseBool(cd.Annotations[constants.SyncsetPauseAnnotation]); err == nil && paused {
					metricClusterDeploymentSyncsetPaused.WithLabelValues(
						cd.Name,
						cd.Namespace,
						clusterType).Set(1.0)
				} else {
					cleared := metricClusterDeploymentSyncsetPaused.Delete(map[string]string{
						"cluster_deployment": cd.Name,
						"namespace":          cd.Namespace,
						"cluster_type":       clusterType,
					})
					if cleared {
						mcLog.Infof("cleared metric: %v", metricClusterDeploymentSyncsetPaused)
					}
				}

				// For a cluster resuming from hibernated state or trying to hibernate, we report the seconds for that
				// transition time to the desired power state
				hibernatingCondition := controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions,
					hivev1.ClusterHibernatingCondition)
				if hibernatingCondition != nil {
					if hibernatingCondition.Reason == hivev1.ResumingHibernationReason ||
						hibernatingCondition.Reason == hivev1.StoppingHibernationReason {
						metricClusterDeploymentPowerStateTransitionSeconds.WithLabelValues(cd.Name,
							string(cd.Spec.PowerState)).Set(time.Since(
							hibernatingCondition.LastTransitionTime.Time).Seconds())
					} else {
						cleared := metricClusterDeploymentPowerStateTransitionSeconds.Delete(map[string]string{
							"cluster_deployment": cd.Name,
							"desired_powerstate": string(cd.Spec.PowerState),
						})
						if cleared {
							mcLog.Infof("cleared metric: %v", metricClusterDeploymentPowerStateTransitionSeconds)
						}
					}
				}
			}

			accumulator.setMetrics(metricClusterDeploymentsTotal,
				metricClusterDeploymentsInstalledTotal,
				metricClusterDeploymentsUninstalledTotal,
				metricClusterDeploymentsDeprovisioningTotal,
				metricClusterDeploymentsWithConditionTotal,
				mcLog)

			// Also add metrics only for clusters created in last 48h
			accumulator, err = newClusterAccumulator("48h", []string{"0h", "1h", "2h", "8h", "24h"})
			if err != nil {
				mcLog.WithError(err).Error("unable to calculate metrics")
				return
			}
			for _, cd := range clusterDeployments.Items {
				accumulator.processCluster(&cd)
			}

			accumulator.setMetrics(metricClusterDeploymentsTotal,
				metricClusterDeploymentsInstalledTotal,
				metricClusterDeploymentsUninstalledTotal,
				metricClusterDeploymentsDeprovisioningTotal,
				metricClusterDeploymentsWithConditionTotal,
				mcLog)
		}
		mcLog.Debug("calculating metrics across all install jobs")

		// install job metrics
		installJobs := &batchv1.JobList{}
		installJobLabelSelector := map[string]string{constants.InstallJobLabel: "true"}
		err = mc.Client.List(ctx, installJobs, client.MatchingLabels(installJobLabelSelector))
		if err != nil {
			log.WithError(err).Error("error listing install jobs")
		} else {
			runningTotal, succeededTotal, failedTotal := processJobs(installJobs.Items)
			for k, v := range runningTotal {
				metricInstallJobsTotal.WithLabelValues(k, stateRunning).Set(float64(v))
			}
			for k, v := range succeededTotal {
				metricInstallJobsTotal.WithLabelValues(k, stateSucceeded).Set(float64(v))
			}
			for k, v := range failedTotal {
				metricInstallJobsTotal.WithLabelValues(k, stateFailed).Set(float64(v))
			}
		}

		mcLog.Debug("calculating metrics across all uninstall jobs")
		// uninstall job metrics
		uninstallJobs := &batchv1.JobList{}
		uninstallJobLabelSelector := map[string]string{constants.UninstallJobLabel: "true"}
		err = mc.Client.List(ctx, uninstallJobs, client.MatchingLabels(uninstallJobLabelSelector))
		if err != nil {
			log.WithError(err).Error("error listing uninstall jobs")
		} else {
			runningTotal, succeededTotal, failedTotal := processJobs(uninstallJobs.Items)
			for k, v := range runningTotal {
				metricUninstallJobsTotal.WithLabelValues(k, stateRunning).Set(float64(v))
			}
			for k, v := range succeededTotal {
				metricUninstallJobsTotal.WithLabelValues(k, stateSucceeded).Set(float64(v))
			}
			for k, v := range failedTotal {
				metricUninstallJobsTotal.WithLabelValues(k, stateFailed).Set(float64(v))
			}
		}

		mcLog.Debug("calculating metrics across all imageset jobs")
		// imageset job metrics
		imagesetJobs := &batchv1.JobList{}
		imagesetJobLabelSelector := map[string]string{imageset.ImagesetJobLabel: "true"}
		err = mc.Client.List(ctx, imagesetJobs, client.MatchingLabels(imagesetJobLabelSelector))
		if err != nil {
			log.WithError(err).Error("error listing imageset jobs")
		} else {
			runningTotal, succeededTotal, failedTotal := processJobs(imagesetJobs.Items)
			for k, v := range runningTotal {
				metricImagesetJobsTotal.WithLabelValues(k, stateRunning).Set(float64(v))
			}
			for k, v := range succeededTotal {
				metricImagesetJobsTotal.WithLabelValues(k, stateSucceeded).Set(float64(v))
			}
			for k, v := range failedTotal {
				metricImagesetJobsTotal.WithLabelValues(k, stateFailed).Set(float64(v))
			}
		}

		mc.calculateSelectorSyncSetMetrics(mcLog)
	}, mc.Interval)

	return nil
}

func (mc *Calculator) calculateSelectorSyncSetMetrics(mcLog log.FieldLogger) {
	mcLog.Debug("calculating metrics across all ClusterSyncs")
	clusterSyncList := &hiveintv1alpha1.ClusterSyncList{}
	err := mc.Client.List(context.Background(), clusterSyncList)
	if err != nil {
		mcLog.WithError(err).Error("error listing all ClusterSyncs")
		return
	}

	sssInstancesTotal := map[string]int{}
	sssInstancesUnappliedTotal := map[string]int{}

	ssInstancesTotal := 0
	ssInstancesUnappliedTotal := 0
	for _, cs := range clusterSyncList.Items {
		for _, sss := range cs.Status.SelectorSyncSets {
			sssInstancesTotal[sss.Name]++
			if sss.Result != hiveintv1alpha1.SuccessSyncSetResult {
				sssInstancesUnappliedTotal[sss.Name]++
			}
		}
		for _, ss := range cs.Status.SyncSets {
			ssInstancesTotal++
			if ss.Result != hiveintv1alpha1.SuccessSyncSetResult {
				ssInstancesUnappliedTotal++
			}
		}
	}
	for k, v := range sssInstancesTotal {
		metricSelectorSyncSetClustersTotal.WithLabelValues(k).Set(float64(v))
		// If this selector sync set currently has no unapplied instances, ensure any past unapplied metric is cleared:
		if sssInstancesUnappliedTotal[k] == 0 {
			cleared := metricSelectorSyncSetClustersUnappliedTotal.Delete(map[string]string{
				"name": k,
			})
			if cleared {
				mcLog.Debugf("cleared selector syncset clusters unapplied metric for cluster: %s", k)
			}
		}
	}
	for k, v := range sssInstancesUnappliedTotal {
		metricSelectorSyncSetClustersUnappliedTotal.WithLabelValues(k).Set(float64(v))
	}
	metricSyncSetsTotal.Set(float64(ssInstancesTotal))
	metricSyncSetsUnappliedTotal.Set(float64(ssInstancesUnappliedTotal))
}

func processJobs(jobs []batchv1.Job) (runningTotal, succeededTotal, failedTotal map[string]int) {
	running := map[string]int{}
	failed := map[string]int{}
	succeeded := map[string]int{}
	for _, job := range jobs {
		clusterType := GetClusterDeploymentType(&job)

		// Sort the jobs:
		if job.Status.Failed > 0 {
			failed[clusterType]++
		} else if job.Status.Succeeded > 0 {
			succeeded[clusterType]++
		} else {
			running[clusterType]++
		}
	}
	return running, succeeded, failed
}

// clusterAccumulator is an object used to process cluster deployments and sort them so we can
// increment the appropriate metrics counter based on it's type, installed state, length of time
// it has been uninstalled, and the conditions it has.
type clusterAccumulator struct {
	// ageFilter can optionally be specified to skip processing clusters older than this duration. If this is not desired,
	// specify "0h" to include all.
	ageFilter    string
	ageFilterDur time.Duration

	// total maps cluster type to counter.
	total map[string]int

	// deprovisioning maps a "greater than" duration string (i.e. 8h) to
	// cluster type to counter. Specify 0h if you want a bucket for the smallest duration.
	deprovisioning map[string]map[string]int

	// installed maps cluster type to counter.
	installed map[string]int

	// uninstalled maps a "greater than" duration string (i.e. 8h) to
	// cluster type to counter. Specify 0h if you want a bucket for the smallest duration.
	uninstalled map[string]map[string]int

	// conditions maps conditions to cluster type to counter.
	conditions map[hivev1.ClusterDeploymentConditionType]map[string]int

	// clusterTypesSet will contain every cluster type we encounter during processing.
	// Used to zero out some values which may no longer exist when setting the final metrics.
	// Maps cluster type to a meaningless bool.
	clusterTypesSet map[string]bool
}

const (
	infinity       = "+Inf"
	stateRunning   = "running"
	stateSucceeded = "succeeded"
	stateFailed    = "failed"
)

// newClusterAccumulator initializes a new cluster accumulator.
// ageFilter can be used to exclude clusters older than a certain duration. Use "0h" to include all clusters.
// durationBuckets are used to sort uninstalled, or deleted clusters into buckets based on how long they have been in that state.
func newClusterAccumulator(ageFilter string, durationBuckets []string) (*clusterAccumulator, error) {
	ca := &clusterAccumulator{
		ageFilter:       ageFilter,
		total:           map[string]int{},
		installed:       map[string]int{},
		deprovisioning:  map[string]map[string]int{},
		uninstalled:     map[string]map[string]int{},
		conditions:      map[hivev1.ClusterDeploymentConditionType]map[string]int{},
		clusterTypesSet: map[string]bool{},
	}
	var err error
	if ageFilter != infinity {
		ca.ageFilterDur, err = time.ParseDuration(ageFilter)
		if err != nil {
			return nil, err
		}
	}

	for _, durStr := range durationBuckets {
		// Make sure all the strings parse as durations, we ignore errors below.
		_, err := time.ParseDuration(durStr)
		if err != nil {
			return nil, err
		}
		ca.uninstalled[durStr] = map[string]int{}
		ca.deprovisioning[durStr] = map[string]int{}
	}

	return ca, nil
}

func (ca *clusterAccumulator) ensureClusterTypeBuckets(clusterType string) {
	// Make sure an entry exists for this cluster type in all relevant maps:

	_, ok := ca.total[clusterType]
	if !ok {
		ca.total[clusterType] = 0
	}

	for k, v := range ca.deprovisioning {
		_, ok := v[clusterType]
		if !ok {
			ca.deprovisioning[k][clusterType] = 0
		}
	}

	_, ok = ca.installed[clusterType]
	if !ok {
		ca.installed[clusterType] = 0
	}

	for k, v := range ca.uninstalled {
		_, ok := v[clusterType]
		if !ok {
			ca.uninstalled[k][clusterType] = 0
		}
	}
	for k, v := range ca.conditions {
		_, ok := v[clusterType]
		if !ok {
			ca.conditions[k][clusterType] = 0
		}
	}
}

func (ca *clusterAccumulator) processCluster(cd *hivev1.ClusterDeployment) {
	if ca.ageFilter != infinity && time.Since(cd.CreationTimestamp.Time) > ca.ageFilterDur {
		return
	}

	clusterType := GetClusterDeploymentType(cd)
	ca.ensureClusterTypeBuckets(clusterType)
	ca.clusterTypesSet[clusterType] = true

	ca.total[clusterType]++

	if cd.DeletionTimestamp != nil {
		// Sort deleted clusters into buckets based on how long since
		// they were deleted. The larger the bucket the more serious the problem.
		deletedDur := time.Since(cd.DeletionTimestamp.Time)

		for k := range ca.deprovisioning {
			// We already error checked that these parse in constructor func:
			gtDurBucket, _ := time.ParseDuration(k)
			if deletedDur > gtDurBucket {
				ca.deprovisioning[k][clusterType]++
			}
		}
	}

	if cd.Spec.Installed {
		ca.installed[clusterType]++
	} else {
		// Sort uninstall clusters into buckets based on how long since
		// they were created. The larger the bucket the more serious the problem.
		uninstalledDur := time.Since(cd.CreationTimestamp.Time)

		for k := range ca.uninstalled {
			// We already error checked that these parse in constructor func:
			gtDurBucket, _ := time.ParseDuration(k)
			if uninstalledDur > gtDurBucket {
				ca.uninstalled[k][clusterType]++
			}
		}
	}

	// Process conditions regardless if installed or not:
	for _, cond := range cd.Status.Conditions {
		if !controllerutils.IsConditionInDesiredState(cond) {
			ca.addConditionToMap(cond.Type, clusterType)
		}
	}
}

func (ca *clusterAccumulator) setMetrics(total, installed, uninstalled, deprovisioning, conditions *prometheus.GaugeVec, mcLog log.FieldLogger) {

	for k, v := range ca.total {
		total.WithLabelValues(k, ca.ageFilter).Set(float64(v))
	}
	for k, v := range ca.installed {
		installed.WithLabelValues(k, ca.ageFilter).Set(float64(v))
	}
	for k, v := range ca.uninstalled {
		for clusterType := range ca.clusterTypesSet {
			if count, ok := v[clusterType]; ok {
				uninstalled.WithLabelValues(clusterType, ca.ageFilter, k).Set(float64(count))
			} else {
				// We need to potentially clear out old cluster types no longer showing in the list.
				// This will work so long as there is at least one cluster of that type still remaining
				// in hive somewhere.
				uninstalled.WithLabelValues(clusterType, ca.ageFilter, k).Set(float64(0))
			}
		}
	}
	for k, v := range ca.deprovisioning {
		for clusterType := range ca.clusterTypesSet {
			if count, ok := v[clusterType]; ok {
				deprovisioning.WithLabelValues(clusterType, ca.ageFilter, k).Set(float64(count))
			} else {
				// We need to potentially clear out old cluster types no longer showing in the list.
				// This will work so long as there is at least one cluster of that type still remaining
				// in hive somewhere.
				deprovisioning.WithLabelValues(clusterType, ca.ageFilter, k).Set(float64(0))
			}
		}
	}
	for k, v := range ca.conditions {
		for k1, v1 := range v {
			conditions.WithLabelValues(k1, ca.ageFilter, string(k)).Set(float64(v1))
		}
	}
}

// GetClusterDeploymentType returns the value of the hive.openshift.io/cluster-type label if set,
// otherwise a default value.
func GetClusterDeploymentType(obj metav1.Object) string {
	if typeStr, ok := obj.GetLabels()[hivev1.HiveClusterTypeLabel]; ok {
		return typeStr
	}
	return hivev1.DefaultClusterType
}

// ReconcileObserver is used to track, log, and report metrics for controller reconcile time and outcome. Each
// controller should instantiate one near the start of the reconcile loop, and defer a call to
// ObserveControllerReconcileTime.
type ReconcileObserver struct {
	startTime      time.Time
	controllerName hivev1.ControllerName
	logger         log.FieldLogger
	outcome        ReconcileOutcome
}

func NewReconcileObserver(controllerName hivev1.ControllerName, logger log.FieldLogger) *ReconcileObserver {
	return &ReconcileObserver{
		startTime:      time.Now(),
		controllerName: controllerName,
		logger:         logger,
		outcome:        ReconcileOutcomeUnspecified,
	}
}

func (ro *ReconcileObserver) ObserveControllerReconcileTime() {
	dur := time.Since(ro.startTime)
	metricControllerReconcileTime.WithLabelValues(string(ro.controllerName), string(ro.outcome)).Observe(dur.Seconds())
	fields := log.Fields{"elapsedMillis": dur.Milliseconds(), "outcome": ro.outcome}
	// Add a log field to categorize request duration into buckets. We can't easily query > in Kibana in
	// OpenShift deployments due to logging limitations, so these constant buckets give us a small number of
	// constant search strings we can use to identify slow reconcile loops.
	for _, bucket := range elapsedDurationBuckets {
		if dur >= bucket {
			fields["elapsedMillisGT"] = bucket.Milliseconds()
			break
		}
	}
	ro.logger.WithFields(fields).Info("reconcile complete")
}

func (ro *ReconcileObserver) SetOutcome(outcome ReconcileOutcome) {
	ro.outcome = outcome
}

func (ca *clusterAccumulator) addConditionToMap(cond hivev1.ClusterDeploymentConditionType, clusterType string) {
	if ca.conditions[cond] == nil {
		ca.conditions[cond] = map[string]int{}
	}
	ca.conditions[cond][clusterType]++
}

var elapsedDurationBuckets = []time.Duration{2 * time.Minute, time.Minute, 30 * time.Second, 10 * time.Second, 5 * time.Second, time.Second, 0}
