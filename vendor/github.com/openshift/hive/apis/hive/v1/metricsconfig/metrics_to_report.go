package metricsconfig

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MetricsToReport represents metrics that have additional customizations
type MetricsToReport struct {
	// 	MetricNames is a list of metrics for which the following customizations must be added, if they support the customization
	// The name of the metric here must be valid, and it can only be present once in metricsToReport.
	MetricNames []string `json:"metricNames"`
	// ClusterDeploymentLabelSelector can be used to match cluster deployment label present, it can be used to filter the metrics reported.
	// It can only be used with metrics that have their clusterdeployment at hand when they are being reported.
	ClusterDeploymentLabelSelector metav1.LabelSelector `json:"clusterDeploymentLabelSelector"`
}
