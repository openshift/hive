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
	// metricClaimDelaySeconds tracks how long it takes for a claim to be assigned, labeled by
	// cluster pool.
	metricClaimDelaySeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "hive_clusterclaim_assignment_delay_seconds",
		Help: "Time between ClusterClaim creation and fulfillment by a ClusterDeployment",
		// USUALLY:
		// - <10m indicates claims of *running* clusters. Bias toward the high end of that range
		//   may indicate that we can optimize the controller logic.
		// - ~10m is CDs waking up from hibernation. Lots of hits here indicate a pool using a
		//   runningCount that is too low (or zero).
		// - ~40m is CDs being provisioned on demand to satisfy claims. Probably indicates pool
		//   Size is too low.
		// - <1h either means CDs are needing multiple install attempts, or MaxSize is too low
		//   so we're having to wait to create CDs to fulfill claims.
		Buckets: []float64{1, 30, 120, 600, 1800, 3000, 7200},
	}, []string{"clusterpool_namespace", "clusterpool_name"})
)

func init() {
	metrics.Registry.MustRegister(metricStaleClusterDeploymentsDeleted)
	metrics.Registry.MustRegister(metricClaimDelaySeconds)
}
