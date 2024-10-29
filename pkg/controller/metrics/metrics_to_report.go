package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/apis/hive/v1/metricsconfig"
)

var (
	// Maintain a list of all the metrics with their names for validation and observing purposes
	// We have to maintain the name of the metrics separately as a map because of the way metrics are currently declared, their names are not available in the code
	counterVecs = map[*prometheus.CounterOpts]string{
		MetricClustersCreated.CounterOpts:         "hive_cluster_deployments_created_total",
		MetricClustersInstalled.CounterOpts:       "hive_cluster_deployments_installed_total",
		MetricClustersDeleted.CounterOpts:         "hive_cluster_deployments_deleted_total",
		MetricProvisionFailedTerminal.CounterOpts: "hive_cluster_deployments_provision_failed_terminal_total",
	}
	gaugeVecs = map[*prometheus.GaugeVec]string{
		metricClusterDeploymentsTotal:               "hive_cluster_deployments",
		metricClusterDeploymentsInstalledTotal:      "hive_cluster_deployments_installed",
		metricClusterDeploymentsUninstalledTotal:    "hive_cluster_deployments_uninstalled",
		metricClusterDeploymentsDeprovisioningTotal: "hive_cluster_deployments_deprovisioning",
		metricClusterDeploymentsWithConditionTotal:  "hive_cluster_deployments_conditions",
		metricClusterDeploymentSyncsetPaused:        "hive_cluster_deployment_syncset_paused",
	}
	histogramVecs = map[*prometheus.HistogramVec]string{
		MetricClusterHibernationTransitionSeconds: "hive_cluster_deployments_hibernation_transition_seconds",
		MetricClusterReadyTransitionSeconds:       "hive_cluster_deployments_running_transition_seconds",
		MetricStoppingClustersSeconds:             "hive_cluster_deployments_stopping_seconds",
		MetricResumingClustersSeconds:             "hive_cluster_deployments_resuming_seconds",
		MetricWaitingForCOClustersSeconds:         "hive_cluster_deployments_waiting_for_cluster_operators_seconds",
		MetricInstallJobDuration:                  "hive_cluster_deployment_install_job_duration_seconds",
		MetricInstallDelaySeconds:                 "hive_cluster_deployment_install_job_delay_seconds",
		MetricImageSetDelaySeconds:                "hive_cluster_deployment_imageset_job_delay_seconds",
		MetricDNSDelaySeconds:                     "hive_cluster_deployment_dns_delay_seconds",
		MetricUninstallJobDuration:                "hive_cluster_deployment_uninstall_job_duration_seconds",
	}
	histogramOpts = map[*prometheus.HistogramOpts]string{
		MetricCompletedInstallJobRestarts.HistogramOpts: "hive_cluster_deployment_completed_install_restart",
		MetricInstallFailureSeconds.HistogramOpts:       "hive_cluster_deployment_install_failure_total",
		MetricInstallSuccessSeconds.HistogramOpts:       "hive_cluster_deployment_install_success_total",
	}
	customMetrics = map[*prometheus.Desc]string{
		metricClusterDeploymentProvisionUnderwaySecondsDesc:   "hive_cluster_deployment_provision_underway_seconds",
		provisioningUnderwayInstallRestartsCollectorDesc:      "hive_cluster_deployment_provision_underway_install_restarts",
		metricClusterDeploymentDeprovisionUnderwaySecondsDesc: "hive_cluster_deployment_deprovision_underway_seconds",
		metricClusterSyncFailingSeconds:                       "hive_clustersync_failing_seconds",
	}

	ls clusterDeploymentLabelSelectorMetrics
)

// GetClusterDeploymentLabelSelectors reads the MetricsToReport from the metricsConfig section of HiveConfig and updates metricsWithLabelSelector map
// Todo: Adapt this function to read all customizations as a part of https://issues.redhat.com/browse/HIVE-2618
func GetClusterDeploymentLabelSelectors(log logrus.FieldLogger, mConfig *metricsconfig.MetricsConfig) error {
	var err error
	ls = *newClusterDeploymentLabelSelectorMetrics()
	for _, entries := range mConfig.MetricsToReport {
		for _, name := range entries.MetricNames {
			if _, ok := ls.metricsWithLabelSelector[name]; ok {
				log.WithError(err).Errorf("Duplicate entries in MetricsConfig.MetricsToReport for %s", name)
				return err
			}
			// metric name must be valid and must support clusterDeploymentLabelSelector customization
			if ls.isMetricSupported(name) == false {
				log.WithError(err).Errorf("Metric %s either not valid or does not support the feature", name)
				return err
			}
			ls.metricsWithLabelSelector[name] = entries.ClusterDeploymentLabelSelector
		}
	}
	return err
}

func ShouldLogCounterOpts(c *prometheus.CounterOpts, cd *hivev1.ClusterDeployment, log logrus.FieldLogger) bool {
	if name, ok := counterVecs[c]; ok {
		ls.shouldLogMetric(name, cd, log)
	}
	return true
}

func ShouldLogGaugeVec(gv *prometheus.GaugeVec, cd *hivev1.ClusterDeployment, log logrus.FieldLogger) bool {
	if name, ok := gaugeVecs[gv]; ok {
		ls.shouldLogMetric(name, cd, log)
	}
	return true
}

func ShouldLogHistogramVec(hv *prometheus.HistogramVec, cd *hivev1.ClusterDeployment, log logrus.FieldLogger) bool {
	if name, ok := histogramVecs[hv]; ok {
		ls.shouldLogMetric(name, cd, log)
	}
	return true
}

func ShouldLogHistogramOpts(h *prometheus.HistogramOpts, cd *hivev1.ClusterDeployment, log logrus.FieldLogger) bool {
	if name, ok := histogramOpts[h]; ok {
		ls.shouldLogMetric(name, cd, log)
	}
	return true
}

func ShouldLogCustomMetric(d *prometheus.Desc, cd *hivev1.ClusterDeployment, log logrus.FieldLogger) bool {
	if name, ok := customMetrics[d]; ok {
		ls.shouldLogMetric(name, cd, log)
	}
	return true
}
