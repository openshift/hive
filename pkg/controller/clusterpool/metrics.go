package clusterpool

import (
	"github.com/prometheus/client_golang/prometheus"

	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	metricClusterDeploymentsAssignable = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_clusterpool_clusterdeployments_assignable",
		Help: "The number of ClusterDeployments ready to be claimed (installed and running). Contributes to Size and MaxSize.",
	}, []string{"clusterpool_namespace", "clusterpool_name"})
	metricClusterDeploymentsClaimed = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_clusterpool_clusterdeployments_claimed",
		Help: "The number of claimed ClusterDeployments, including those being deleted. Contributes to MaxSize.",
	}, []string{"clusterpool_namespace", "clusterpool_name"})
	metricClusterDeploymentsDeleting = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_clusterpool_clusterdeployments_deleting",
		Help: "The number of ClusterDeployments marked for or actively deprovisioning and deleting. Contributes to MaxConcurrent.",
	}, []string{"clusterpool_namespace", "clusterpool_name"})
	metricClusterDeploymentsInstalling = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_clusterpool_clusterdeployments_installing",
		Help: "The number of ClusterDeployments being added to the pool, in the process of installing. Contributes to Size, MaxSize, and MaxConcurrent.",
	}, []string{"clusterpool_namespace", "clusterpool_name"})
	metricClusterDeploymentsUnclaimed = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_clusterpool_clusterdeployments_unclaimed",
		Help: "The number of unclaimed ClusterDeployments, including installing, standby, and assignable. Should tend toward the pool Size, unless constrained by MaxConcurrent or exceeded due to excess claims.",
	}, []string{"clusterpool_namespace", "clusterpool_name"})
	metricClusterDeploymentsStandby = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_clusterpool_clusterdeployments_standby",
		Help: "The number of ClusterDeployments that are installed but not running. Should tend toward pool Size minus RunningCount minus the number of pending ClusterClaims.",
	}, []string{"clusterpool_namespace", "clusterpool_name"})
	metricClusterDeploymentsStale = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_clusterpool_clusterdeployments_stale",
		Help: "The number of ClusterDeployments which no longer match the spec of their ClusterPool. Should tend toward zero as such clusters are gradually replaced.",
	}, []string{"clusterpool_namespace", "clusterpool_name"})
	metricClusterDeploymentsBroken = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_clusterpool_clusterdeployments_broken",
		Help: "The number of ClusterDeployments we have deemed unrecoverable and unusable. Should tend toward zero as such clusters are gradually replaced.",
	}, []string{"clusterpool_namespace", "clusterpool_name"})
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
	metrics.Registry.MustRegister(metricClusterDeploymentsAssignable)
	metrics.Registry.MustRegister(metricClusterDeploymentsClaimed)
	metrics.Registry.MustRegister(metricClusterDeploymentsDeleting)
	metrics.Registry.MustRegister(metricClusterDeploymentsInstalling)
	metrics.Registry.MustRegister(metricClusterDeploymentsUnclaimed)
	metrics.Registry.MustRegister(metricClusterDeploymentsStandby)
	metrics.Registry.MustRegister(metricClusterDeploymentsStale)
	metrics.Registry.MustRegister(metricClusterDeploymentsBroken)
	metrics.Registry.MustRegister(metricStaleClusterDeploymentsDeleted)
	metrics.Registry.MustRegister(metricClaimDelaySeconds)
}
