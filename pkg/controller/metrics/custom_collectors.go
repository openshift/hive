package metrics

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hiveintv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
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

	// minDuration, when non-zero, is the minimum duration after which clusters provisioning
	// will start becoming part of this metric. When set to zero, all clusters provisioning
	// will be included in the metric.
	minDuration time.Duration

	// metricClusterDeploymentProvisionUnderwaySeconds is a prometheus metric for the number of seconds
	// between when a still provisioning cluster was created and now.
	metricClusterDeploymentProvisionUnderwaySeconds *prometheus.Desc
}

// Collect collects the metrics for provisioningUnderwayCollector
func (cc provisioningUnderwayCollector) Collect(ch chan<- prometheus.Metric) {
	ccLog := log.WithField("controller", "metrics")
	ccLog.Info("calculating provisioning underway metrics across all ClusterDeployments")

	// Load all ClusterDeployments so we can accumulate facts about them.
	clusterDeployments := &hivev1.ClusterDeploymentList{}
	err := cc.client.List(context.Background(), clusterDeployments)
	if err != nil {
		ccLog.WithError(err).Error("error listing cluster deployments")
		return
	}
	for _, cd := range clusterDeployments.Items {
		if cd.DeletionTimestamp != nil {
			continue
		}
		if cd.Spec.Installed {
			continue
		}
		if !ShouldLogCustomMetric(metricClusterDeploymentProvisionUnderwaySecondsDesc, &cd, ccLog) {
			continue
		}

		platform := cd.Labels[hivev1.HiveClusterPlatformLabel]
		imageSet := "none"
		if cd.Spec.Provisioning != nil && cd.Spec.Provisioning.ImageSetRef != nil {
			imageSet = cd.Spec.Provisioning.ImageSetRef.Name
		}

		elapsedDuration := time.Since(cd.CreationTimestamp.Time)
		if cc.minDuration.Seconds() > 0 && elapsedDuration < cc.minDuration {
			continue // skip reporting the metric for clusterdeployment until the elapsed time is at least minDuration
		}

		// Add install failure details for stuck provision
		condition, reason, skip := getConditionAndReason(cd.Status.Conditions)
		if skip {
			continue
		}
		// For installing clusters we report the seconds since the cluster was created.
		ch <- prometheus.MustNewConstMetric(
			cc.metricClusterDeploymentProvisionUnderwaySeconds,
			prometheus.GaugeValue,
			elapsedDuration.Seconds(),
			cd.Name,
			cd.Namespace,
			GetLabelValue(&cd, hivev1.HiveClusterTypeLabel),
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

// Collect collects the metrics for provisioningUnderwayInstallRestartsCollector
func (cc provisioningUnderwayInstallRestartsCollector) Collect(ch chan<- prometheus.Metric) {
	ccLog := log.WithField("controller", "metrics")
	ccLog.Info("calculating provisioning underway install restarts metrics across all ClusterDeployments")

	// Load all ClusterDeployments so we can accumulate facts about them.
	clusterDeployments := &hivev1.ClusterDeploymentList{}
	err := cc.client.List(context.Background(), clusterDeployments)
	if err != nil {
		ccLog.WithError(err).Error("error listing cluster deployments")
		return
	}
	for _, cd := range clusterDeployments.Items {
		if cd.DeletionTimestamp != nil {
			continue
		}
		if cd.Spec.Installed {
			continue
		}
		if !ShouldLogCustomMetric(provisioningUnderwayInstallRestartsCollectorDesc, &cd, ccLog) {
			continue
		}

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

		// Add install failure details for stuck provision
		condition, reason, skip := getConditionAndReason(cd.Status.Conditions)
		if skip {
			continue
		}

		// For installing clusters we report the seconds since the cluster was created.
		ch <- prometheus.MustNewConstMetric(
			cc.metricClusterDeploymentProvisionUnderwayInstallRestarts,
			prometheus.GaugeValue,
			float64(restarts),
			cd.Name,
			cd.Namespace,
			GetLabelValue(&cd, hivev1.HiveClusterTypeLabel),
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

// getConditionAndReason fetches a condition and reason if some condition is found in undesired state.
// All other conditions apart from provisionDelayConditions are tagged as unknown for metric reporting purposes
// Reporting of the metric is to be skipped when all conditions are in their desired state
func getConditionAndReason(conditions []hivev1.ClusterDeploymentCondition) (condition, reason string, skip bool) {
	condition, reason = "Unknown", "Unknown"
	for _, delayCondition := range provisioningDelayCondition {
		if cdCondition := controllerutils.FindCondition(conditions,
			delayCondition); cdCondition != nil {
			if cdCondition.Status != corev1.ConditionUnknown &&
				!controllerutils.IsConditionInDesiredState(*cdCondition) {
				condition = string(delayCondition)
				if cdCondition.Reason != "" {
					reason = cdCondition.Reason
				}
				break
			}
		}
	}
	// skip reporting if all conditions are in their desired state
	if condition == "Unknown" && len(conditions) > 0 {
		if controllerutils.AreAllConditionsInDesiredState(conditions) {
			skip = true
		}
	}
	return condition, reason, skip
}

// deprovisioning underway metric collected through a custom prometheus collector
type deprovisioningUnderwayCollector struct {
	client client.Client

	// TODO: Make metric optional and allow for a minDuration to be specified by user.

	// metricClusterDeploymentDeprovisionUnderwaySeconds is a prometheus metric for the number of seconds
	// between when a deprovisioning cluster DeletionTimestamp was set and now.
	metricClusterDeploymentDeprovisionUnderwaySeconds *prometheus.Desc
}

// Collect collects the metrics for deprovisioningUnderwayCollector
func (cc deprovisioningUnderwayCollector) Collect(ch chan<- prometheus.Metric) {
	ccLog := log.WithField("controller", "metrics")
	ccLog.Info("calculating deprovisioning underway metrics across all ClusterDeployments")

	// Load all ClusterDeployments so we can accumulate facts about them.
	clusterDeployments := &hivev1.ClusterDeploymentList{}
	err := cc.client.List(context.Background(), clusterDeployments)
	if err != nil {
		ccLog.WithError(err).Error("error listing cluster deployments")
		return
	}
	for _, cd := range clusterDeployments.Items {
		if cd.DeletionTimestamp == nil {
			continue
		}
		if !ShouldLogCustomMetric(metricClusterDeploymentDeprovisionUnderwaySecondsDesc, &cd, ccLog) {
			continue
		}

		elapsedDuration := time.Since(cd.DeletionTimestamp.Time)

		// For installing clusters we report the seconds since the cluster was created.
		ch <- prometheus.MustNewConstMetric(
			cc.metricClusterDeploymentDeprovisionUnderwaySeconds,
			prometheus.GaugeValue,
			elapsedDuration.Seconds(),
			cd.Name,
			cd.Namespace,
			GetLabelValue(&cd, hivev1.HiveClusterTypeLabel),
		)
	}
}

func (cc deprovisioningUnderwayCollector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(cc, ch)
}

var (
	metricClusterDeploymentDeprovisionUnderwaySecondsDesc = prometheus.NewDesc(
		"hive_cluster_deployment_deprovision_underway_seconds",
		"Length of time a cluster has been deprovisioning.",
		[]string{"cluster_deployment", "namespace", "cluster_type"},
		nil,
	)
)

func newDeprovisioningUnderwaySecondsCollector(client client.Client) prometheus.Collector {
	return deprovisioningUnderwayCollector{
		client: client,
		metricClusterDeploymentDeprovisionUnderwaySeconds: metricClusterDeploymentDeprovisionUnderwaySecondsDesc,
	}
}

// clustersync failing metric collected through a custom prometheus collector
type clusterSyncFailingCollector struct {
	client client.Client

	// minDuration, when non-zero, is the minimum duration after which clustersync failure
	// will start becoming part of this metric. When set to zero, all clustersync failures
	// will be included in the metric.
	minDuration time.Duration

	// metricClusterSyncFailingSeconds is a prometheus metric for the number of seconds
	// between the start of a failing clustersync and now.
	metricClusterSyncFailingSeconds *prometheus.Desc

	// dynamicLabels is a collection of fixed (mandatory) and optional labels for the clusterSyncFailing metric.
	// It also has labelList which stores the order of labels as defined in Desc. MustNewConstMetric doesn't
	// accept a map of labels and values, so maintaining the order is needed while observing the metric.
	dynamicLabels dynamicLabelsForCustomCollectors
}

// Collect collects the metrics for custerSyncFailingCollector
func (cc clusterSyncFailingCollector) Collect(ch chan<- prometheus.Metric) {
	ccLog := log.WithField("controller", "metrics")
	ccLog.Info("calculating cluster sync failing seconds metrics")

	clusterSyncList := &hiveintv1alpha1.ClusterSyncList{}
	err := cc.client.List(context.Background(), clusterSyncList)
	if err != nil {
		ccLog.WithError(err).Error("error listing all ClusterSyncs")
		return
	}

	clusterDeployments := &hivev1.ClusterDeploymentList{}
	err = cc.client.List(context.Background(), clusterDeployments)
	if err != nil {
		ccLog.WithError(err).Error("error listing cluster deployments")
		return
	}

	for _, cs := range clusterSyncList.Items {
		// Failing cluster syncs
		cond := controllerutils.FindCondition(cs.Status.Conditions, hiveintv1alpha1.ClusterSyncFailed)
		if cond != nil && cond.Status == corev1.ConditionTrue {
			var cdRef hivev1.ClusterDeployment
			for _, cd := range clusterDeployments.Items {
				if cd.Namespace == cs.Namespace && cd.Name == cs.Name {
					cdRef = cd
					break
				}
			}
			if ShouldLogCustomMetric(metricClusterSyncFailingSeconds, &cdRef, ccLog) {
				fixedLabels := make(map[string]string, len(cc.dynamicLabels.fixedLabels))
				fixedLabels["namespaced_name"] = cs.Namespace + "/" + cs.Name
				if !reflect.ValueOf(cdRef).IsZero() {
					if unreachableCondition := controllerutils.FindCondition(cdRef.Status.Conditions, hivev1.UnreachableCondition); unreachableCondition != nil {
						fixedLabels["unreachable"] = string(unreachableCondition.Status)
					}
				}
				labelValues := cc.dynamicLabels.buildLabelSlice(fixedLabels, &cdRef)
				seconds := time.Since(cond.LastTransitionTime.Time).Seconds()
				// check if duration crosses the threshold
				if cc.minDuration.Seconds() <= seconds {
					ch <- prometheus.MustNewConstMetric(
						cc.metricClusterSyncFailingSeconds,
						prometheus.GaugeValue,
						seconds,
						labelValues...,
					)
				}
			}
		}
	}
}

func (cc clusterSyncFailingCollector) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(cc, ch)
}

var metricClusterSyncFailingSeconds *prometheus.Desc

func newClusterSyncFailingCollector(client client.Client, minimum time.Duration, optionalLabels map[string]string) prometheus.Collector {
	metricName := "hive_clustersync_failing_seconds"
	baseLabels := dynamicLabels{
		// namespaced_name would be logged as $namespace/$name
		// unreachable reports the status of unreachable condition of the corresponding cluster deployment
		fixedLabels:    []string{"namespaced_name", "unreachable"},
		optionalLabels: optionalLabels,
	}
	if repeatedLabels := baseLabels.getRepeatedLabels(); repeatedLabels != nil {
		panic(fmt.Sprintf("Label(s) %v in HiveConfig.Spec.AdditionalClusterDeploymentLabels conflict with fixed label(s) for the metric %s. Please rename your label.", repeatedLabels, metricName))
	}
	labels := dynamicLabelsForCustomCollectors{
		dynamicLabels: baseLabels,
		labelList:     baseLabels.getLabelList(),
	}
	metricClusterSyncFailingSeconds = prometheus.NewDesc(
		metricName,
		"Length of time a clustersync has been failing",
		labels.labelList,
		nil,
	)
	return clusterSyncFailingCollector{
		client:                          client,
		metricClusterSyncFailingSeconds: metricClusterSyncFailingSeconds,
		minDuration:                     minimum,
		dynamicLabels:                   labels,
	}
}
