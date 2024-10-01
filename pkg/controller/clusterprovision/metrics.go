package clusterprovision

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/openshift/hive/apis/hive/v1/metricsconfig"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
)

var (
	// Declare the metrics which allow optional labels to be added.
	// They are defined later once the hive config has been read.
	metricClusterProvisionsTotal hivemetrics.CounterVecWithDynamicLabels
	metricInstallErrors          hivemetrics.CounterVecWithDynamicLabels

	metricInstallFailureSeconds hivemetrics.HistogramVecWithDynamicLabels
	metricInstallSuccessSeconds hivemetrics.HistogramVecWithDynamicLabels
)

func registerMetrics(mConfig *metricsconfig.MetricsConfig) {
	mapClusterTypeLabelToValue := hivemetrics.GetOptionalClusterTypeLabels(mConfig)

	metricClusterProvisionsTotal = *hivemetrics.NewCounterVecWithDynamicLabels(
		&prometheus.CounterOpts{
			Name: "hive_cluster_provision_results_total",
			Help: "Counter incremented every time we observe a completed cluster provision.",
		},
		[]string{"result"},
		mapClusterTypeLabelToValue,
	)
	metricInstallErrors = *hivemetrics.NewCounterVecWithDynamicLabels(
		&prometheus.CounterOpts{
			Name: "hive_install_errors",
			Help: "Counter incremented every time we observe certain errors strings in install logs.",
		},
		[]string{"reason"},
		mapClusterTypeLabelToValue,
	)

	metricInstallFailureSeconds = *hivemetrics.NewHistogramVecWithDynamicLabels(
		&prometheus.HistogramOpts{
			Name:    "hive_cluster_deployment_install_failure_total",
			Help:    "Time taken before a cluster provision failed to install",
			Buckets: []float64{30, 120, 300, 600, 1800},
		},
		[]string{"platform", "region", "cluster_version", "workers", "install_attempt"},
		mapClusterTypeLabelToValue,
	)
	metricInstallSuccessSeconds = *hivemetrics.NewHistogramVecWithDynamicLabels(
		&prometheus.HistogramOpts{
			Name:    "hive_cluster_deployment_install_success_total",
			Help:    "Time taken before a cluster provision succeeded to install",
			Buckets: []float64{1800, 2400, 3000, 3600},
		},
		[]string{"platform", "region", "cluster_version", "workers", "install_attempt"},
		mapClusterTypeLabelToValue,
	)

	metricInstallErrors.Register()
	metricClusterProvisionsTotal.Register()
	metricInstallFailureSeconds.Register()
	metricInstallSuccessSeconds.Register()
}
