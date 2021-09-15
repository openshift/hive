package clusterpool

import (
	"github.com/prometheus/client_golang/prometheus"

	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// metricStaleClusterDeploymentsDeleted tracks the total number of CDs we delete because they
	// became "stale". That is, the ClusterPool was modified in a substantive way such that these
	// CDs no longer match its spec. Note that this only counts stale CDs we've *deleted* -- there
	// may be other stale CDs we haven't gotten around to deleting yet.
	metricStaleClusterDeploymentsDeleted = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hive_clusterpool_stale_clusterdeployments_deleted",
		Help: "The number of ClusterDeployments deleted because they no longer match the spec of their ClusterPool.",
	}, []string{"clusterpool_namespace", "clusterpool_name"})
)

func init() {
	metrics.Registry.MustRegister(metricStaleClusterDeploymentsDeleted)
}
