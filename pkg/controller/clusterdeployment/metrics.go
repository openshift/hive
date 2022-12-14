package clusterdeployment

import (
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"

	"sigs.k8s.io/controller-runtime/pkg/metrics"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

var (
	metricInstallJobDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "hive_cluster_deployment_install_job_duration_seconds",
			Help:    "Distribution of the runtime of completed install jobs.",
			Buckets: []float64{1800, 2400, 3000, 3600, 4500, 5400, 7200},
		},
	)
	metricInstallDelaySeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "hive_cluster_deployment_install_job_delay_seconds",
			Help:    "Time between cluster deployment creation and creation of the job to install/provision the cluster.",
			Buckets: []float64{60, 120, 180, 240, 300, 600, 1200, 1800, 2700, 3600},
		},
	)
	metricImageSetDelaySeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "hive_cluster_deployment_imageset_job_delay_seconds",
			Help:    "Time between cluster deployment creation and creation of the job which resolves the installer image to use for a ClusterImageSet.",
			Buckets: []float64{10, 30, 60, 300, 600, 1200, 1800},
		},
	)
	metricDNSDelaySeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "hive_cluster_deployment_dns_delay_seconds",
			Help:    "Time between cluster deployment with spec.manageDNS creation and the DNSZone becoming ready.",
			Buckets: []float64{10, 30, 60, 300, 600, 1200, 1800},
		},
	)

	// Declare the metrics which allow optional labels to be added.
	// They are defined later once the hive config has been read.
	metricCompletedInstallJobRestarts *prometheus.HistogramVec

	metricClustersCreated         *prometheus.CounterVec
	metricClustersInstalled       *prometheus.CounterVec
	metricClustersDeleted         *prometheus.CounterVec
	metricProvisionFailedTerminal *prometheus.CounterVec
)

func incProvisionFailedTerminal(cd *hivev1.ClusterDeployment, log log.FieldLogger) {
	poolNSName := ""
	if poolRef := cd.Spec.ClusterPoolRef; poolRef != nil {
		poolNSName = poolRef.Namespace + "/" + poolRef.PoolName
	}
	stoppedReason := "unknown"
	stoppedCondition := controllerutils.FindCondition(cd.Status.Conditions, hivev1.ProvisionStoppedCondition)
	if stoppedCondition != nil {
		stoppedReason = stoppedCondition.Reason
	}
	hivemetrics.LogCounterMetricWithOptionalLabels(metricProvisionFailedTerminal, cd, map[string]string{
		"clusterpool_namespacedname": poolNSName,
		"cluster_type":               hivemetrics.GetClusterDeploymentType(cd, hivev1.HiveClusterTypeLabel),
		"failure_reason":             stoppedReason,
	}, mapClusterTypeLabelToValue, log)
}

func registerMetrics() {

	metricCompletedInstallJobRestarts = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hive_cluster_deployment_completed_install_restart",
			Help:    "Distribution of the number of restarts for all completed cluster installations.",
			Buckets: []float64{0, 2, 10, 20, 50},
		},
		append([]string{"cluster_type"}, optionalClusterTypeLabels...),
	)
	metricClustersCreated = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hive_cluster_deployments_created_total",
		Help: "Counter incremented every time we observe a new cluster.",
	},
		append([]string{"cluster_type"}, optionalClusterTypeLabels...),
	)
	metricClustersInstalled = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hive_cluster_deployments_installed_total",
		Help: "Counter incremented every time we observe a successful installation.",
	},
		append([]string{"cluster_type"}, optionalClusterTypeLabels...),
	)
	metricClustersDeleted = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hive_cluster_deployments_deleted_total",
		Help: "Counter incremented every time we observe a deleted cluster.",
	},
		append([]string{"cluster_type"}, optionalClusterTypeLabels...),
	)
	metricProvisionFailedTerminal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hive_cluster_deployments_provision_failed_terminal_total",
		Help: "Counter incremented when a cluster provision has failed and won't be retried.",
	},
		append([]string{"clusterpool_namespacedname", "cluster_type", "failure_reason"}, optionalClusterTypeLabels...),
	)

	metrics.Registry.MustRegister(metricInstallJobDuration)
	metrics.Registry.MustRegister(metricCompletedInstallJobRestarts)
	metrics.Registry.MustRegister(metricInstallDelaySeconds)
	metrics.Registry.MustRegister(metricImageSetDelaySeconds)
	metrics.Registry.MustRegister(metricClustersCreated)
	metrics.Registry.MustRegister(metricClustersInstalled)
	metrics.Registry.MustRegister(metricClustersDeleted)
	metrics.Registry.MustRegister(metricDNSDelaySeconds)
	metrics.Registry.MustRegister(metricProvisionFailedTerminal)
}
