package metricsconfig

type MetricsConfig struct {
	// Optional metrics and their configurations
	// +optional
	MetricsWithDuration []MetricsWithDuration `json:"metricsWithDuration"`
}
