package clusterprovision

import (
	"github.com/prometheus/client_golang/prometheus"

	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	metricInstallFailureSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "hive_cluster_deployment_install_failure_total",
		Help:    "Time taken before a cluster provision failed to install",
		Buckets: []float64{30, 120, 300, 600, 1800},
	},
		[]string{"cluster_type", "platform", "region", "cluster_version", "workers", "install_attempt"},
	)

	metricInstallSuccessSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "hive_cluster_deployment_install_success_total",
		Help:    "Time taken before a cluster provision succeeded to install",
		Buckets: []float64{1800, 2400, 3000, 3600},
	},
		[]string{"cluster_type", "platform", "region", "cluster_version", "workers", "install_attempt"},
	)
)

func init() {
	metrics.Registry.MustRegister(metricInstallFailureSeconds)
	metrics.Registry.MustRegister(metricInstallSuccessSeconds)
}
