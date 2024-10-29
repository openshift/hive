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

	metricInstallErrors.Register()
	metricClusterProvisionsTotal.Register()
}
