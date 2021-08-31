package metrics

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"

	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

var (
	// ClusterDeployment conditions that could indicate provisioning problems with a cluster
	// First condition appearing True will be used in the metric labels.
	provisioningDelayCondition = [...]hivev1.ClusterDeploymentConditionType{
		hivev1.RequirementsMetCondition,
		hivev1.DNSNotReadyCondition,
		hivev1.InstallLaunchErrorCondition,
		hivev1.ProvisionFailedCondition,
		hivev1.AuthenticationFailureClusterDeploymentCondition,
		hivev1.InstallImagesNotResolvedCondition,
	}
)

// provisioning underway metrics collected through a custom prometheus collector
type provisioningUnderwayCollector struct {
	client client.Client

	// minDuration, when non-zero, is the minimum duration afer which clusters provisioning
	// will start becomming part of this metric. When set to zero, all clusters provisioning
	// will be included in the metric.
	minDuration time.Duration

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
		condition, reason := getKnownConditions(cd.Status.Conditions)

		platform := cd.Labels[hivev1.HiveClusterPlatformLabel]
		imageSet := "none"
		if cd.Spec.Provisioning != nil && cd.Spec.Provisioning.ImageSetRef != nil {
			imageSet = cd.Spec.Provisioning.ImageSetRef.Name
		}

		elapsedDuration := time.Since(cd.CreationTimestamp.Time)
		if cc.minDuration.Seconds() > 0 && elapsedDuration < cc.minDuration {
			continue // skip reporting the metric for clusterdeployment until the elapsed time is at least minDuration
		}

		// For installing clusters we report the seconds since the cluster was created.
		ch <- prometheus.MustNewConstMetric(
			cc.metricClusterDeploymentProvisionUnderwaySeconds,
			prometheus.GaugeValue,
			elapsedDuration.Seconds(),
			cd.Name,
			cd.Namespace,
			GetClusterDeploymentType(&cd),
			condition,
			reason,
			platform,
			imageSet,
		)

	}

}

func (cc provisioningUnderwayCollector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(cc, ch)
}

var (
	metricClusterDeploymentProvisionUnderwaySecondsDesc = prometheus.NewDesc(
		"hive_cluster_deployment_provision_underway_seconds",
		"Length of time a cluster has been provisioning.",
		[]string{"cluster_deployment", "namespace", "cluster_type", "condition", "reason", "platform", "image_set"},
		nil,
	)
)

func newProvisioningUnderwaySecondsCollector(client client.Client, minimum time.Duration) prometheus.Collector {
	return provisioningUnderwayCollector{
		client: client,
		metricClusterDeploymentProvisionUnderwaySeconds: metricClusterDeploymentProvisionUnderwaySecondsDesc,
		minDuration: minimum,
	}
}

// provisioning underway install restarts metrics collected through a custom prometheus collector
type provisioningUnderwayInstallRestartsCollector struct {
	client client.Client

	// minRestarts, when non-zero, is the minimum restarts after which clusters provisioning
	// will start becoming part of the metric. When set to zero, all clusters provisioning
	// will be included in the metric.
	minRestarts int

	// metricClusterDeploymentProvisionUnderwayInstallRestarts is a prometheus metric for the number of install
	// restarts for a still provisioning cluster.
	metricClusterDeploymentProvisionUnderwayInstallRestarts *prometheus.Desc
}

// collects the metrics for provisioningUnderwayInstallRestartsCollector
func (cc provisioningUnderwayInstallRestartsCollector) Collect(ch chan<- prometheus.Metric) {
	ccLog := log.WithField("controller", "metrics")
	ccLog.Info("calculating provisioning underway install restarts metrics across all ClusterDeployments")

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
		condition, reason := getKnownConditions(cd.Status.Conditions)

		platform := cd.Labels[hivev1.HiveClusterPlatformLabel]
		imageSet := "none"
		if cd.Spec.Provisioning != nil && cd.Spec.Provisioning.ImageSetRef != nil {
			imageSet = cd.Spec.Provisioning.ImageSetRef.Name
		}

		restarts := cd.Status.InstallRestarts
		if restarts == 0 {
			continue // skip reporting the metric for clusterdeployment that hasn't restarted at all
		}
		if cc.minRestarts > 0 && restarts < cc.minRestarts {
			continue // skip reporting the metric for clusterdeployment until the InstallRestarts is at least minRestarts
		}

		// For installing clusters we report the seconds since the cluster was created.
		ch <- prometheus.MustNewConstMetric(
			cc.metricClusterDeploymentProvisionUnderwayInstallRestarts,
			prometheus.GaugeValue,
			float64(restarts),
			cd.Name,
			cd.Namespace,
			GetClusterDeploymentType(&cd),
			condition,
			reason,
			platform,
			imageSet,
		)

	}

}

func (cc provisioningUnderwayInstallRestartsCollector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(cc, ch)
}

var (
	provisioningUnderwayInstallRestartsCollectorDesc = prometheus.NewDesc(
		"hive_cluster_deployment_provision_underway_install_restarts",
		"Number install restarts for a cluster that has been provisioning.",
		[]string{"cluster_deployment", "namespace", "cluster_type", "condition", "reason", "platform", "image_set"},
		nil,
	)
)

func newProvisioningUnderwayInstallRestartsCollector(client client.Client, minimum int) prometheus.Collector {
	return provisioningUnderwayInstallRestartsCollector{
		client: client,
		metricClusterDeploymentProvisionUnderwayInstallRestarts: provisioningUnderwayInstallRestartsCollectorDesc,
		minRestarts: minimum,
	}
}

func getKnownConditions(conditions []hivev1.ClusterDeploymentCondition) (condition, reason string) {
	condition, reason = "Unknown", "Unknown"
	for _, delayCondition := range provisioningDelayCondition {
		if cdCondition := controllerutils.FindClusterDeploymentCondition(conditions,
			delayCondition); cdCondition != nil {
			if !controllerutils.IsConditionInDesiredState(*cdCondition) {
				condition = string(delayCondition)
				if cdCondition.Reason != "" {
					reason = cdCondition.Reason
				}
				break
			}
		}
	}
	return condition, reason
}
