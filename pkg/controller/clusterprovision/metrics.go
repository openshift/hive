package clusterprovision

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// Declare the metrics which allow optional labels to be added.
	// They are defined later once the hive config has been read.
	metricClusterProvisionsTotal *prometheus.CounterVec
	metricInstallErrors          *prometheus.CounterVec

	metricInstallFailureSeconds *prometheus.HistogramVec
	metricInstallSuccessSeconds *prometheus.HistogramVec
)

func registerMetrics() {

	metricClusterProvisionsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hive_cluster_provision_results_total",
		Help: "Counter incremented every time we observe a completed cluster provision.",
	},
		append([]string{"cluster_type", "result"}, optionalClusterTypeLabels...),
	)
	metricInstallErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hive_install_errors",
		Help: "Counter incremented every time we observe certain errors strings in install logs.",
	},
		append([]string{"cluster_type", "reason"}, optionalClusterTypeLabels...),
	)
	metricInstallFailureSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "hive_cluster_deployment_install_failure_total",
		Help:    "Time taken before a cluster provision failed to install",
		Buckets: []float64{30, 120, 300, 600, 1800},
	},
		append([]string{"cluster_type", "platform", "region", "cluster_version", "workers", "install_attempt"}, optionalClusterTypeLabels...),
	)

	metricInstallSuccessSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "hive_cluster_deployment_install_success_total",
		Help:    "Time taken before a cluster provision succeeded to install",
		Buckets: []float64{1800, 2400, 3000, 3600},
	},
		append([]string{"cluster_type", "platform", "region", "cluster_version", "workers", "install_attempt"}, optionalClusterTypeLabels...),
	)

	metrics.Registry.MustRegister(metricInstallErrors)
	metrics.Registry.MustRegister(metricClusterProvisionsTotal)
	metrics.Registry.MustRegister(metricInstallFailureSeconds)
	metrics.Registry.MustRegister(metricInstallSuccessSeconds)
}
