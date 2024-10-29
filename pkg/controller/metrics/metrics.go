package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"os"
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
	"github.com/openshift/hive/apis/hive/v1/metricsconfig"
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
	}, []string{"cluster_type", "age_lt", "power_state"})
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

	// TODO: convert all metrics with both namespace and name as labels to namespaced_name (logged as $namespace/$name)

	// MetricClusterHibernationTransitionSeconds is a prometheus metric that tracks the number of seconds it takes for
	// clusters to transition to hibernated powerstate
	MetricClusterHibernationTransitionSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hive_cluster_deployments_hibernation_transition_seconds",
			Help:    "Distribution of the length of time for clusters to transition to the hibernated power state",
			Buckets: []float64{30, 60, 180, 300, 600},
		},
		[]string{"cluster_version", "platform", "cluster_pool_namespace", "cluster_pool_name"})
	// MetricClusterReadyTransitionSeconds is a prometheus metric that tracks the number of seconds it takes for hibernated
	// clusters to transition to running powerstate
	MetricClusterReadyTransitionSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hive_cluster_deployments_running_transition_seconds",
			Help:    "Distribution of the length of time for clusters to transition to running power state",
			Buckets: []float64{60, 90, 180, 300, 600, 1200},
		},
		[]string{"cluster_version", "platform", "cluster_pool_namespace", "cluster_pool_name"})
	// MetricStoppingClustersSeconds is a prometheus metric that tracks the transition time for currently stopping clusters
	MetricStoppingClustersSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hive_cluster_deployments_stopping_seconds",
			Help:    "Distribution of the length of transition time for clusters currently stopping",
			Buckets: []float64{30, 60, 180, 300, 600},
		},
		[]string{"cluster_deployment_namespace", "cluster_deployment", "platform", "cluster_version", "cluster_pool_namespace"})
	// MetricResumingClustersSeconds is a prometheus metric that tracks the transition time for currently resuming clusters
	MetricResumingClustersSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hive_cluster_deployments_resuming_seconds",
			Help:    "Distribution of the length of transition time for clusters currently resuming",
			Buckets: []float64{60, 90, 180, 300, 600, 1200},
		},
		[]string{"cluster_deployment_namespace", "cluster_deployment", "platform", "cluster_version", "cluster_pool_namespace"})
	// MetricWaitingForCOClustersSeconds is a prometheus metric that tracks the transition time for clusters currently in
	// waiting for cluster operators state
	MetricWaitingForCOClustersSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hive_cluster_deployments_waiting_for_cluster_operators_seconds",
			Help:    "Distribution of the length of transition time for clusters currently waiting for cluster operators",
			Buckets: []float64{180, 300, 600, 1800, 3600, 7200},
		},
		[]string{"cluster_deployment_namespace", "cluster_deployment", "platform", "cluster_version", "cluster_pool_namespace"})
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

	// mapMetricToDurationHistograms is a map of optional durationMetrics of type Histogram to their specific duration,
	// if mentioned
	mapMetricToDurationHistograms map[*prometheus.HistogramVec]time.Duration
	// mapMetricToDurationGauges is a map of optional durationMetrics of type Gauge to their specific duration, if
	// mentioned
	mapMetricToDurationGauges map[*prometheus.GaugeVec]time.Duration

	// Metrics reported by ClusterDeployment controller

	MetricInstallJobDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hive_cluster_deployment_install_job_duration_seconds",
			Help:    "Distribution of the runtime of completed install jobs.",
			Buckets: []float64{1800, 2400, 3000, 3600, 4500, 5400, 7200},
		},
		nil,
	)
	MetricInstallDelaySeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hive_cluster_deployment_install_job_delay_seconds",
			Help:    "Time between cluster deployment creation and creation of the job to install/provision the cluster.",
			Buckets: []float64{60, 120, 180, 240, 300, 600, 1200, 1800, 2700, 3600},
		},
		nil,
	)
	MetricImageSetDelaySeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hive_cluster_deployment_imageset_job_delay_seconds",
			Help:    "Time between cluster deployment creation and creation of the job which resolves the installer image to use for a ClusterImageSet.",
			Buckets: []float64{10, 30, 60, 300, 600, 1200, 1800},
		},
		nil,
	)
	MetricDNSDelaySeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hive_cluster_deployment_dns_delay_seconds",
			Help:    "Time between cluster deployment with spec.manageDNS creation and the DNSZone becoming ready.",
			Buckets: []float64{10, 30, 60, 300, 600, 1200, 1800},
		},
		nil,
	)
	// Metrics with additional label support. The dynamic labels will be set when we register these metrics after reading the metricsConfig.
	MetricCompletedInstallJobRestarts = *NewHistogramVecWithDynamicLabels(
		&prometheus.HistogramOpts{
			Name:    "hive_cluster_deployment_completed_install_restart",
			Help:    "Distribution of the number of restarts for all completed cluster installations.",
			Buckets: []float64{0, 2, 10, 20, 50},
		},
		nil,
		map[string]string{},
	)
	MetricClustersCreated = *NewCounterVecWithDynamicLabels(
		&prometheus.CounterOpts{
			Name: "hive_cluster_deployments_created_total",
			Help: "Counter incremented every time we observe a new cluster.",
		},
		nil,
		map[string]string{},
	)
	MetricClustersInstalled = *NewCounterVecWithDynamicLabels(
		&prometheus.CounterOpts{
			Name: "hive_cluster_deployments_installed_total",
			Help: "Counter incremented every time we observe a successful installation.",
		},
		nil,
		map[string]string{},
	)
	MetricClustersDeleted = *NewCounterVecWithDynamicLabels(
		&prometheus.CounterOpts{
			Name: "hive_cluster_deployments_deleted_total",
			Help: "Counter incremented every time we observe a deleted cluster.",
		},
		nil,
		map[string]string{},
	)
	MetricProvisionFailedTerminal = *NewCounterVecWithDynamicLabels(
		&prometheus.CounterOpts{
			Name: "hive_cluster_deployments_provision_failed_terminal_total",
			Help: "Counter incremented when a cluster provision has failed and won't be retried.",
		},
		[]string{"clusterpool_namespacedname", "failure_reason"},
		map[string]string{},
	)

	// Metrics reported by ClusterDeprovision controller

	MetricUninstallJobDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hive_cluster_deployment_uninstall_job_duration_seconds",
			Help:    "Distribution of the runtime of completed uninstall jobs.",
			Buckets: []float64{60, 300, 600, 1200, 1800, 2400, 3000, 3600},
		},
		nil,
	)

	// Some metrics reported by ClusterProvision controller, they support clusterDeploymentLabelSelector

	MetricInstallFailureSeconds = *NewHistogramVecWithDynamicLabels(
		&prometheus.HistogramOpts{
			Name:    "hive_cluster_deployment_install_failure_total",
			Help:    "Time taken before a cluster provision failed to install",
			Buckets: []float64{30, 120, 300, 600, 1800},
		},
		[]string{"platform", "region", "cluster_version", "workers", "install_attempt"},
		map[string]string{},
	)
	MetricInstallSuccessSeconds = *NewHistogramVecWithDynamicLabels(
		&prometheus.HistogramOpts{
			Name:    "hive_cluster_deployment_install_success_total",
			Help:    "Time taken before a cluster provision succeeded to install",
			Buckets: []float64{1800, 2400, 3000, 3600},
		},
		[]string{"platform", "region", "cluster_version", "workers", "install_attempt"},
		map[string]string{},
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
	metrics.Registry.MustRegister(metricClusterDeploymentSyncsetPaused)
	metrics.Registry.MustRegister(MetricInstallJobDuration)
	metrics.Registry.MustRegister(MetricInstallDelaySeconds)
	metrics.Registry.MustRegister(MetricImageSetDelaySeconds)
	metrics.Registry.MustRegister(MetricDNSDelaySeconds)
	metrics.Registry.MustRegister(MetricUninstallJobDuration)
}

// Add creates a new metrics Calculator and adds it to the Manager.
func Add(mgr manager.Manager) error {
	mc := &Calculator{
		Client:   mgr.GetClient(),
		Interval: 2 * time.Minute,
	}
	// TODO: Make these optional & configurable via HiveConfig.Spec.MetricsConfig
	metrics.Registry.MustRegister(newProvisioningUnderwaySecondsCollector(mgr.GetClient(), 1*time.Hour))
	metrics.Registry.MustRegister(newProvisioningUnderwayInstallRestartsCollector(mgr.GetClient(), 1))
	// TODO: Add deprovisioning underway metric to set of optional duration-based metrics
	metrics.Registry.MustRegister(newDeprovisioningUnderwaySecondsCollector(mgr.GetClient()))

	return mgr.Add(mc)
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
	// Get the metrics config from hiveConfig
	mConfig, err := ReadMetricsConfig()
	if err != nil {
		log.WithError(err).Error("error reading metrics config")
		return err
	}
	// Register optional metrics and update them in their corresponding maps, so controllers logging them can access
	// the information
	mc.registerOptionalMetrics(mConfig)

	// Run forever, sleep at the end:
	wait.UntilWithContext(ctx, func(ctx context.Context) {
		mcLog := log.WithField("controller", "metrics")
		recobsrv := NewReconcileObserver(ControllerName, mcLog)
		defer recobsrv.ObserveControllerReconcileTime()

		mcLog.Info("calculating metrics across all ClusterDeployments")
		// Load all ClusterDeployments so we can accumulate facts about them.
		clusterDeployments := &hivev1.ClusterDeploymentList{}
		err = mc.Client.List(ctx, clusterDeployments)
		if err != nil {
			log.WithError(err).Error("error listing cluster deployments")
		} else {
			// Reset metric on each pass to prevent reporting stale metrics when count drops to zero
			metricClusterDeploymentsTotal.Reset()
			accumulator, err := newClusterAccumulator(infinity, []string{"0h", "1h", "2h", "8h", "24h", "72h"})
			if err != nil {
				mcLog.WithError(err).Error("unable to calculate metrics")
				return
			}
			for _, cd := range clusterDeployments.Items {
				clusterType := GetLabelValue(&cd, hivev1.HiveClusterTypeLabel)
				accumulator.processCluster(&cd, mcLog)

				if paused, err := strconv.ParseBool(cd.Annotations[constants.SyncsetPauseAnnotation]); err == nil && paused && ShouldLogGaugeVec(metricClusterDeploymentSyncsetPaused, &cd, mcLog) {
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

				// Check if cluster is currently transitioning and set the appropriate metrics
				hibernatingCond := controllerutils.FindCondition(cd.Status.Conditions, hivev1.ClusterHibernatingCondition)
				readyCond := controllerutils.FindCondition(cd.Status.Conditions, hivev1.ClusterReadyCondition)
				// Ensure it is not too early in the cluster provisioning state and the conditions have been set at this point
				if readyCond != nil && hibernatingCond != nil {
					if readyCond.Reason == hivev1.ReadyReasonStoppingOrHibernating &&
						hibernatingCond.Reason != hivev1.HibernatingReasonHibernating {
						logHistogramDurationMetric(MetricStoppingClustersSeconds, &cd,
							time.Since(readyCond.LastTransitionTime.Time).Seconds())
					}
					if hibernatingCond.Reason == hivev1.HibernatingReasonResumingOrRunning &&
						readyCond.Reason != hivev1.ReadyReasonRunning {
						logHistogramDurationMetric(MetricResumingClustersSeconds, &cd,
							time.Since(hibernatingCond.LastTransitionTime.Time).Seconds())
					}
					// While logging for WaitingForClusterOperators, account for the hard coded pause while waiting
					// after nodes are ready, before we could query status of cluster operators
					if readyCond.Reason == hivev1.ReadyReasonWaitingForClusterOperators {
						logHistogramDurationMetric(MetricWaitingForCOClustersSeconds, &cd,
							(time.Since(hibernatingCond.LastTransitionTime.Time).Seconds())+
								constants.ClusterOperatorSettlePause.Seconds())
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
				accumulator.processCluster(&cd, mcLog)
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

		mc.calculateSyncSetMetrics(mcLog)
	}, mc.Interval)

	return nil
}

// registerOptionalMetrics registers the metrics, and stores their configs in the corresponding maps
func (mc *Calculator) registerOptionalMetrics(mConfig *metricsconfig.MetricsConfig) {
	mapMetricToDurationHistograms = make(map[*prometheus.HistogramVec]time.Duration)
	mapMetricToDurationGauges = make(map[*prometheus.GaugeVec]time.Duration)
	optionalLabels := GetOptionalClusterTypeLabels(mConfig)
	for _, metric := range mConfig.MetricsWithDuration {
		switch metric.Name {
		// Histograms
		case metricsconfig.CurrentStopping:
			metrics.Registry.MustRegister(MetricStoppingClustersSeconds)
			mapMetricToDurationHistograms[MetricStoppingClustersSeconds] = metric.Duration.Duration
		case metricsconfig.CurrentResuming:
			metrics.Registry.MustRegister(MetricResumingClustersSeconds)
			mapMetricToDurationHistograms[MetricResumingClustersSeconds] = metric.Duration.Duration
		case metricsconfig.CurrentWaitingForCO:
			metrics.Registry.MustRegister(MetricWaitingForCOClustersSeconds)
			mapMetricToDurationHistograms[MetricWaitingForCOClustersSeconds] = metric.Duration.Duration
		case metricsconfig.CumulativeHibernated:
			metrics.Registry.MustRegister(MetricClusterHibernationTransitionSeconds)
			mapMetricToDurationHistograms[MetricClusterHibernationTransitionSeconds] = metric.Duration.Duration
		case metricsconfig.CumulativeResumed:
			metrics.Registry.MustRegister(MetricClusterReadyTransitionSeconds)
			mapMetricToDurationHistograms[MetricClusterReadyTransitionSeconds] = metric.Duration.Duration
		// Gauges
		case metricsconfig.CurrentClusterSyncFailing:
			metrics.Registry.MustRegister(newClusterSyncFailingCollector(mc.Client, metric.Duration.Duration, optionalLabels))
		}
	}
	// Set dynamic labels for metrics with additional label support and register them
	MetricProvisionFailedTerminal.optionalLabels = optionalLabels
	MetricCompletedInstallJobRestarts.optionalLabels = optionalLabels
	MetricClustersCreated.optionalLabels = optionalLabels
	MetricClustersInstalled.optionalLabels = optionalLabels
	MetricClustersDeleted.optionalLabels = optionalLabels
	MetricInstallFailureSeconds.optionalLabels = optionalLabels
	MetricInstallSuccessSeconds.optionalLabels = optionalLabels

	MetricProvisionFailedTerminal.Register()
	MetricCompletedInstallJobRestarts.Register()
	MetricClustersCreated.Register()
	MetricClustersInstalled.Register()
	MetricClustersDeleted.Register()
	MetricInstallFailureSeconds.Register()
	MetricInstallSuccessSeconds.Register()
}

// ShouldLogHistogramDurationMetric decides whether the corresponding duration metric of type histogram should be logged.
// It first checks if that optional metric has been opted into for logging, and the duration has crossed the threshold set.
func ShouldLogHistogramDurationMetric(metricToLog *prometheus.HistogramVec, timeToLog float64) bool {
	duration, ok := mapMetricToDurationHistograms[metricToLog]
	return ok && duration.Seconds() <= timeToLog
}

func (mc *Calculator) calculateSyncSetMetrics(mcLog log.FieldLogger) {
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
		clusterType := GetLabelValue(&job, hivev1.HiveClusterTypeLabel)

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
// increment the appropriate metrics counter based on its type, installed state, length of time
// it has been uninstalled, and the conditions it has.
type clusterAccumulator struct {
	// ageFilter can optionally be specified to skip processing clusters older than this duration. If this is not desired,
	// specify "0h" to include all.
	ageFilter    string
	ageFilterDur time.Duration

	// total maps powerState to cluster type to counter.
	total map[string]map[string]int

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
		total:           map[string]map[string]int{},
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

func (ca *clusterAccumulator) ensureClusterTypeBuckets(clusterType string, powerState string) {
	// Make sure an entry exists for this cluster type in all relevant maps:

	_, ok := ca.total[powerState]
	if !ok {
		ca.total[powerState] = map[string]int{}
	}
	_, ok = ca.total[powerState][clusterType]
	if !ok {
		ca.total[powerState][clusterType] = 0
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

func (ca *clusterAccumulator) processCluster(cd *hivev1.ClusterDeployment, log log.FieldLogger) {
	if ca.ageFilter != infinity && time.Since(cd.CreationTimestamp.Time) > ca.ageFilterDur {
		return
	}

	clusterType := GetLabelValue(cd, hivev1.HiveClusterTypeLabel)
	powerState := GetPowerStateValue(cd.Status.PowerState)
	ca.ensureClusterTypeBuckets(clusterType, powerState)
	ca.clusterTypesSet[clusterType] = true

	if ShouldLogGaugeVec(metricClusterDeploymentsTotal, cd, log) {
		ca.total[powerState][clusterType]++
	}

	if cd.DeletionTimestamp != nil && ShouldLogGaugeVec(metricClusterDeploymentsDeprovisioningTotal, cd, log) {
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
		if ShouldLogGaugeVec(metricClusterDeploymentsInstalledTotal, cd, log) {
			ca.installed[clusterType]++
		}
	} else if ShouldLogGaugeVec(metricClusterDeploymentsUninstalledTotal, cd, log) {
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

	if ShouldLogGaugeVec(metricClusterDeploymentsWithConditionTotal, cd, log) {
		// Process conditions regardless if installed or not:
		for _, cond := range cd.Status.Conditions {
			if !controllerutils.IsConditionInDesiredState(cond) {
				ca.addConditionToMap(cond.Type, clusterType)
			}
		}
	}
}

func (ca *clusterAccumulator) setMetrics(total, installed, uninstalled, deprovisioning, conditions *prometheus.GaugeVec, mcLog log.FieldLogger) {

	for k, v := range ca.total {
		for clusterType := range ca.clusterTypesSet {
			total.WithLabelValues(clusterType, ca.ageFilter, k).Set(float64(v[clusterType]))
		}
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

// GetLabelValue returns the value of the label if set, otherwise a default value.
func GetLabelValue(obj metav1.Object, label string) string {
	if obj != nil {
		if typeStr := obj.GetLabels()[label]; typeStr != "" {
			return typeStr
		}
	}
	return constants.MetricLabelDefaultValue
}

// GetPowerStateValue returns the value of the power state if set, otherwise a default value.
func GetPowerStateValue(powerState hivev1.ClusterPowerState) string {
	if powerState != "" {
		return string(powerState)
	}
	return constants.MetricLabelDefaultValue
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

// ReadMetricsConfig reads the metrics config from the file pointed to by the MetricsConfigFileEnvVar
// environment variable.
func ReadMetricsConfig() (*metricsconfig.MetricsConfig, error) {
	path := os.Getenv(constants.MetricsConfigFileEnvVar)
	config := &metricsconfig.MetricsConfig{}
	if len(path) == 0 {
		return config, errors.New("metrics config environment variable not set")
	}

	fileBytes, err := os.ReadFile(path)
	if err != nil {
		return config, err
	}
	if err = json.Unmarshal(fileBytes, config); err != nil {
		return config, err
	}

	return config, nil
}

// logHistogramDurationMetric should be used to log duration metrics of Histogram type
// It accounts for cluster deployment name, namespace, platform, version and cluster pool namespace as labels
func logHistogramDurationMetric(metric *prometheus.HistogramVec, cd *hivev1.ClusterDeployment, time float64) {
	if !ShouldLogHistogramDurationMetric(metric, time) {
		return
	}
	poolNS := "<none>"
	if cd.Spec.ClusterPoolRef != nil {
		poolNS = cd.Spec.ClusterPoolRef.Namespace
	}
	metric.WithLabelValues(
		cd.Namespace,
		cd.Name,
		cd.Labels[hivev1.HiveClusterPlatformLabel],
		cd.Labels[constants.VersionLabel],
		poolNS).Observe(time)
}

var elapsedDurationBuckets = []time.Duration{2 * time.Minute, time.Minute, 30 * time.Second, 10 * time.Second, 5 * time.Second, time.Second, 0}
