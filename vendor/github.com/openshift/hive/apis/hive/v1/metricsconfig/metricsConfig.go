package metricsconfig

type MetricsConfig struct {
	// Optional metrics and their configurations
	// +optional
	MetricsWithDuration []MetricsWithDuration `json:"metricsWithDuration"`
	// MetricsWithClusterTypeLabels is a map of cluster type to the label applied on the cluster deployment.
	// This map is meant to be used when it would be helpful to label certain metrics with more cluster type labels.
	// If the provided label is present on a cluster deployment, all metrics that are logged per cluster type will
	// attempt to label with the provided cluster type as well.
	// +optional
	MetricsWithClusterTypeLabels map[string]string `json:"metricsWithClusterTypeLabels"`
}
