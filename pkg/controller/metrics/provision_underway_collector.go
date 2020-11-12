package metrics

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

var (
	// ClusterDeployment conditions that could indicate provisioning problems with a cluster
	// First condition appearing True will be used in the metric labels.
	provisioningDelayCondition = [...]hivev1.ClusterDeploymentConditionType{
		hivev1.DNSNotReadyCondition,
		hivev1.InstallLaunchErrorCondition,
		hivev1.ProvisionFailedCondition,
	}
)

// provisioning underway metrics collected through a custom prometheus collector
type provisioningUnderwayCollector struct {
	client client.Client

	// metricClusterDeploymentProvisionUnderwaySeconds is a prometheus metric for the number of seconds
	// between when a still provisioning cluster was created and now.
	metricClusterDeploymentProvisionUnderwaySeconds *prometheus.Desc
}

// collects the metrics for provisioningUnderwayCollector
func (cc provisioningUnderwayCollector) Collect(ch chan<- prometheus.Metric) {
	ccLog := log.WithField("controller", "metrics")
	ccLog.Info("calculating provisioning underway metrics across all ClusterDeployments")

	// Load all ClusterDeployments so we can accumulate facts about them.
	clusterDeployments := &hivev1.ClusterDeploymentList{}
	err := cc.client.List(context.Background(), clusterDeployments)
	if err != nil {
		log.WithError(err).Error("error listing cluster deployments")
		return
	}
	for _, cd := range clusterDeployments.Items {
		if cd.DeletionTimestamp != nil {
			continue
		}
		if cd.Spec.Installed {
			continue
		}

		// Add install failure details for stuck provision
		condition, reason := "Unknown", "Unknown"
		for _, delayCondition := range provisioningDelayCondition {
			if cdCondition := controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions,
				delayCondition); cdCondition != nil {
				condition = string(delayCondition)
				if cdCondition.Status == corev1.ConditionTrue && cdCondition.Reason != "" {
					reason = cdCondition.Reason
				}
				break
			}
		}

		// For installing clusters we report the seconds since the cluster was created.
		ch <- prometheus.MustNewConstMetric(
			cc.metricClusterDeploymentProvisionUnderwaySeconds,
			prometheus.GaugeValue,
			time.Since(cd.CreationTimestamp.Time).Seconds(),
			cd.Name,
			cd.Namespace,
			GetClusterDeploymentType(&cd),
			condition,
			reason,
		)

	}

}

func (cc provisioningUnderwayCollector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(cc, ch)
}

func newProvisioningUnderwayCollector(client client.Client) prometheus.Collector {
	return provisioningUnderwayCollector{
		client: client,
		metricClusterDeploymentProvisionUnderwaySeconds: prometheus.NewDesc(
			"hive_cluster_deployment_provision_underway_seconds",
			"Length of time a cluster has been provisioning.",
			[]string{"cluster_deployment", "namespace", "cluster_type", "condition", "reason"},
			nil,
		),
	}
}
