package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/apis/hive/v1/metricsconfig"
	"github.com/openshift/hive/pkg/constants"
)

// metricsWithDynamicLabels encompasses metrics that can have optional labels.
type metricsWithDynamicLabels interface {
	ObserveMetricWithDynamicLabels(metav1.Object, log.FieldLogger, map[string]string, float64)
	RegisterMetricWithDynamicLabels()
}

type dynamicLabels struct {
	// fixedLabels defines the label keys that are always registered on the metric.
	fixedLabels []string
	// optionalLabels is generated dynamically based on HiveConfig.Spec.MetricsConfig.MetricsWithDynamicLabels, and
	// represents a map of metric label keys to ClusterDeployment metadata.label keys from which values will be gleaned
	// when the metric is observed.
	optionalLabels map[string]string
}

func WithFixedLabels(labels []string) func(*dynamicLabels) {
	return func(d *dynamicLabels) {
		d.fixedLabels = labels
	}
}

func WithOptionalLabels(labels map[string]string) func(*dynamicLabels) {
	return func(d *dynamicLabels) {
		d.optionalLabels = labels
	}
}

// getLabelList combines fixed and optional labels and returns the list without duplicates.
func (d *dynamicLabels) getLabelList() []string {
	var finalList []string

	// Ensure there isn't an overlap between fixed and optional labels
	keys := make(map[string]bool)
	for _, entry := range append(d.fixedLabels, getKeys(d.optionalLabels)...) {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			finalList = append(finalList, entry)
		}
	}
	return finalList
}

// buildLabels fetches the optional label values and merges the maps of fixedLabels and optionalLabels without
// duplicates. The resultant map can be used for observing the metric.
func (d *dynamicLabels) buildLabels(fixedLabels map[string]string, obj metav1.Object) prometheus.Labels {
	optionalLabels := make(map[string]string, len(d.optionalLabels))
	for _, v := range getKeys(d.optionalLabels) {
		optionalLabels[v] = GetLabelValue(obj, d.optionalLabels[v])
	}
	labels := make(map[string]string, len(d.getLabelList()))
	for label, value := range fixedLabels {
		labels[label] = value
	}
	for label, value := range optionalLabels {
		labels[label] = value
	}
	return labels
}

// defaultMissingLabels ensures a value is present for all corresponding labels of a metric. If missing, it sets it to a
// default value
func defaultMissingLabels(labelKeys []string, valueMap map[string]string) map[string]string {
	if len(valueMap) < len(labelKeys) {
		for _, v := range labelKeys {
			if _, ok := valueMap[v]; !ok {
				valueMap[v] = constants.MetricLabelDefaultValue
			}
		}
	}
	return valueMap
}

type CounterVecWithDynamicLabels struct {
	*prometheus.CounterOpts
	metric *prometheus.CounterVec
	*dynamicLabels
}

func NewCounterVecWithDynamicLabels(counterOpts *prometheus.CounterOpts,
	labelOptions ...func(labels *dynamicLabels)) *CounterVecWithDynamicLabels {
	counterVecMetric := &CounterVecWithDynamicLabels{
		CounterOpts:   counterOpts,
		dynamicLabels: &dynamicLabels{},
	}
	for _, o := range labelOptions {
		o(counterVecMetric.dynamicLabels)
	}
	counterVecMetric.metric = prometheus.NewCounterVec(*counterVecMetric.CounterOpts, counterVecMetric.getLabelList())
	return counterVecMetric
}

// RegisterMetricWithDynamicLabels registers the CounterVec metric.
func (c CounterVecWithDynamicLabels) RegisterMetricWithDynamicLabels() {
	metrics.Registry.MustRegister(c.metric)
}

// ObserveMetricWithDynamicLabels logs the counter metric. It simply increments the value, so last parameter isn't used.
func (c CounterVecWithDynamicLabels) ObserveMetricWithDynamicLabels(obj metav1.Object, log log.FieldLogger,
	fixedLabels map[string]string, _ float64) {
	labels := defaultMissingLabels(c.getLabelList(), c.buildLabels(fixedLabels, obj))
	c.metric.With(labels).Inc()
}

type HistogramVecWithDynamicLabels struct {
	*prometheus.HistogramOpts
	metric *prometheus.HistogramVec
	*dynamicLabels
}

func NewHistogramVecWithDynamicLabels(histogramOpts *prometheus.HistogramOpts,
	labelOptions ...func(labels *dynamicLabels)) *HistogramVecWithDynamicLabels {
	histogramVecMetric := &HistogramVecWithDynamicLabels{
		HistogramOpts: histogramOpts,
		dynamicLabels: &dynamicLabels{},
	}
	for _, o := range labelOptions {
		o(histogramVecMetric.dynamicLabels)
	}
	histogramVecMetric.metric = prometheus.NewHistogramVec(*histogramVecMetric.HistogramOpts,
		histogramVecMetric.getLabelList())
	return histogramVecMetric
}

// RegisterMetricWithDynamicLabels registers the HistogramVec metric.
func (h HistogramVecWithDynamicLabels) RegisterMetricWithDynamicLabels() {
	metrics.Registry.MustRegister(h.metric)
}

// ObserveMetricWithDynamicLabels observes the histogram metric with val value.
func (h HistogramVecWithDynamicLabels) ObserveMetricWithDynamicLabels(obj metav1.Object, log log.FieldLogger,
	fixedLabels map[string]string, val float64) {
	labels := defaultMissingLabels(h.getLabelList(), h.buildLabels(fixedLabels, obj))
	h.metric.With(labels).Observe(val)
}

var _ metricsWithDynamicLabels = CounterVecWithDynamicLabels{}
var _ metricsWithDynamicLabels = HistogramVecWithDynamicLabels{}

// GetOptionalClusterTypeLabels reads the AdditionalClusterDeploymentLabels from the metrics config and returns the same
// as a map
func GetOptionalClusterTypeLabels(mConfig *metricsconfig.MetricsConfig) map[string]string {
	var mapClusterTypeLabelToValue = make(map[string]string)
	// Currently "cluster_type" is a non-optional label for these metrics, so if the config is empty (default), set the
	// "cluster_type" label
	// TODO: remove as a part of https://issues.redhat.com/browse/HIVE-2077
	if mConfig.AdditionalClusterDeploymentLabels == nil {
		mapClusterTypeLabelToValue["cluster_type"] = hivev1.HiveClusterTypeLabel
	} else {
		for k, v := range *mConfig.AdditionalClusterDeploymentLabels {
			mapClusterTypeLabelToValue[k] = v
		}
	}
	return mapClusterTypeLabelToValue
}

// getKeys gets the keys of a map
func getKeys(labelMap map[string]string) []string {
	var keys []string
	for k := range labelMap {
		keys = append(keys, k)
	}
	return keys
}
