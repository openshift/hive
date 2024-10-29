package metrics

import (
	"github.com/sirupsen/logrus"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

var (
	// empty interface for the ease of setting values for supportedMetrics
	empty interface{}
)

type clusterDeploymentLabelSelectorMetrics struct {
	// supportedMetrics lists all the metrics that hive currently logs, which support clusterDeploymentLabelSelector customization
	// It is a map with empty interface set as value, to keep it lightweight and with O(1) look up time.
	supportedMetrics map[string]interface{}
	// metricsWithLabelSelector will be the map of metrics that have clusterDeploymentLabelSelector customization added in metricsConfig
	metricsWithLabelSelector map[string]v1.LabelSelector
}

func newClusterDeploymentLabelSelectorMetrics() *clusterDeploymentLabelSelectorMetrics {
	return &clusterDeploymentLabelSelectorMetrics{
		supportedMetrics: map[string]interface{}{
			// counterOpts
			"hive_cluster_deployments_created_total":                   empty,
			"hive_cluster_deployments_installed_total":                 empty,
			"hive_cluster_deployments_deleted_total":                   empty,
			"hive_cluster_deployments_provision_failed_terminal_total": empty,
			// gaugeVecs
			"hive_cluster_deployments":                empty,
			"hive_cluster_deployments_installed":      empty,
			"hive_cluster_deployments_uninstalled":    empty,
			"hive_cluster_deployments_deprovisioning": empty,
			"hive_cluster_deployments_conditions":     empty,
			"hive_cluster_deployment_syncset_paused":  empty,
			// histogramVecs
			"hive_cluster_deployments_hibernation_transition_seconds":        empty,
			"hive_cluster_deployments_running_transition_seconds":            empty,
			"hive_cluster_deployments_stopping_seconds":                      empty,
			"hive_cluster_deployments_resuming_seconds":                      empty,
			"hive_cluster_deployments_waiting_for_cluster_operators_seconds": empty,
			"hive_cluster_deployment_install_job_duration_seconds":           empty,
			"hive_cluster_deployment_install_job_delay_seconds":              empty,
			"hive_cluster_deployment_imageset_job_delay_seconds":             empty,
			"hive_cluster_deployment_dns_delay_seconds":                      empty,
			"hive_cluster_deployment_uninstall_job_duration_seconds":         empty,
			// histogramOpts
			"hive_cluster_deployment_completed_install_restart": empty,
			"hive_cluster_deployment_install_failure_total":     empty,
			"hive_cluster_deployment_install_success_total":     empty,
			// custom metrics
			"hive_cluster_deployment_provision_underway_seconds":          empty,
			"hive_cluster_deployment_provision_underway_install_restarts": empty,
			"hive_cluster_deployment_deprovision_underway_seconds":        empty,
			"hive_clustersync_failing_seconds":                            empty,
		},
		metricsWithLabelSelector: make(map[string]v1.LabelSelector),
	}
}

// isMetricSupported should be used to validate if the metric name is valid and supported clusterDeploymentLabelSelector customization
func (ls *clusterDeploymentLabelSelectorMetrics) isMetricSupported(name string) bool {
	if _, ok := ls.supportedMetrics[name]; ok {
		return true
	}
	return false
}

// hasClusterDeploymentLabelSelector can be used to check if the metric has a related clusterDeploymentLabelSelector configured in HiveConfig
func (ls *clusterDeploymentLabelSelectorMetrics) hasClusterDeploymentLabelSelector(metricName string) bool {
	if _, ok := ls.metricsWithLabelSelector[metricName]; ok {
		return true
	}
	return false
}

// matchesLabelSelector can be used to determine the cluster deployment matches the label selector configured
func (ls *clusterDeploymentLabelSelectorMetrics) matchesLabelSelector(name string, cd *hivev1.ClusterDeployment, log logrus.FieldLogger) bool {
	if _, ok := ls.metricsWithLabelSelector[name]; !ok {
		return false
	}
	value := ls.metricsWithLabelSelector[name]
	selector, err := v1.LabelSelectorAsSelector(&value)
	if err != nil {
		log.WithError(err).WithField("metric", name).Errorf("cannot parse clusterDeploymentLabelSelector")
		return false
	}
	if selector.Matches(labels.Set(cd.Labels)) {
		return true
	}
	return false
}

// shouldLogMetric encapsulates the logic of ClusterDeploymentLabelSelector for the provided metric and the related clusterDeployment and decides if the
// metric must be logged
func (ls *clusterDeploymentLabelSelectorMetrics) shouldLogMetric(name string, cd *hivev1.ClusterDeployment, log logrus.FieldLogger) bool {
	// If there is no related clusterDeploymentLabelSelector, log the metric since there's no restriction on reporting it
	if !ls.hasClusterDeploymentLabelSelector(name) {
		return true
	}
	return ls.matchesLabelSelector(name, cd, log)
}
