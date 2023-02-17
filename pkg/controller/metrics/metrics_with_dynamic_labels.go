package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/apis/hive/v1/metricsconfig"
	"github.com/openshift/hive/pkg/constants"
)

// metricsWithDynamicLabels encompasses metrics that can have optional labels.
type metricsWithDynamicLabels interface {
	Observe(metav1.Object, map[string]string, float64)
	Register()
}

type dynamicLabels struct {
	// fixedLabels defines the label keys that are always registered on the metric.
	fixedLabels []string
	// optionalLabels is generated dynamically based on HiveConfig.Spec.MetricsConfig.MetricsWithDynamicLabels, and
	// represents a map of metric label keys to ClusterDeployment metadata.label keys from which values will be gleaned
	// when the metric is observed.
	optionalLabels map[string]string
}

// getLabelList combines the fixed and optional labels and returns the list with just the labels that can be used to
// define the metric
func (d *dynamicLabels) getLabelList() []string {
	var finalList []string
	finalList = append(finalList, d.fixedLabels...)
	for label := range d.optionalLabels {
		finalList = append(finalList, label)
	}
	return finalList
}

// getRepeatedLabels detects and returns the optional labels that overlap with another label.
func (d *dynamicLabels) getRepeatedLabels() []string {
	var returnList []string
	keys := sets.Set[string]{}
	for _, val := range d.fixedLabels {
		keys.Insert(val)
	}
	for key := range d.optionalLabels {
		if keys.Has(key) {
			returnList = append(returnList, key)
		} else {
			keys.Insert(key)
		}
	}
	return returnList
}

// buildLabels fetches the optional label values and merges the maps of fixedLabels and optionalLabels. The resultant
// map can be used for observing the metric.
func (d *dynamicLabels) buildLabels(fixedLabels map[string]string, obj metav1.Object) prometheus.Labels {
	labels := make(map[string]string, len(d.fixedLabels)+len(d.optionalLabels))
	for _, label := range d.fixedLabels {
		if value := fixedLabels[label]; value != "" {
			labels[label] = value
		} else {
			labels[label] = constants.MetricLabelDefaultValue
		}
	}
	for key, value := range d.optionalLabels {
		labels[key] = GetLabelValue(obj, value)
	}
	return labels
}

type CounterVecWithDynamicLabels struct {
	*prometheus.CounterOpts
	metric *prometheus.CounterVec
	*dynamicLabels
}

func NewCounterVecWithDynamicLabels(counterOpts *prometheus.CounterOpts, fixedLabels []string,
	optionalLabels map[string]string) *CounterVecWithDynamicLabels {
	counterVecMetric := &CounterVecWithDynamicLabels{
		CounterOpts: counterOpts,
		dynamicLabels: &dynamicLabels{
			fixedLabels:    fixedLabels,
			optionalLabels: optionalLabels,
		},
	}
	if repeatedLabels := counterVecMetric.getRepeatedLabels(); repeatedLabels != nil {
		panic(fmt.Sprintf("Label(s) %v in HiveConfig.Spec.AdditionalClusterDeploymentLabels conflict with fixed label(s) for the metric %s. Please rename your label.", repeatedLabels, counterOpts.Name))
	}
	counterVecMetric.metric = prometheus.NewCounterVec(*counterVecMetric.CounterOpts, counterVecMetric.getLabelList())
	return counterVecMetric
}

// Register registers the CounterVec metric.
func (c CounterVecWithDynamicLabels) Register() {
	metrics.Registry.MustRegister(c.metric)
}

// Observe logs the counter metric. It simply increments the value, so last parameter isn't used.
func (c CounterVecWithDynamicLabels) Observe(obj metav1.Object, fixedLabels map[string]string, _ float64) {
	c.metric.With(c.buildLabels(fixedLabels, obj)).Inc()
}

type HistogramVecWithDynamicLabels struct {
	*prometheus.HistogramOpts
	metric *prometheus.HistogramVec
	*dynamicLabels
}

func NewHistogramVecWithDynamicLabels(histogramOpts *prometheus.HistogramOpts, fixedLabels []string,
	optionalLabels map[string]string) *HistogramVecWithDynamicLabels {
	histogramVecMetric := &HistogramVecWithDynamicLabels{
		HistogramOpts: histogramOpts,
		dynamicLabels: &dynamicLabels{
			fixedLabels:    fixedLabels,
			optionalLabels: optionalLabels,
		},
	}
	if repeatedLabels := histogramVecMetric.getRepeatedLabels(); repeatedLabels != nil {
		panic(fmt.Sprintf("Label(s) %v in HiveConfig.Spec.AdditionalClusterDeploymentLabels conflict with fixed label(s) for the metric %s. Please rename your label.", repeatedLabels, histogramOpts.Name))
	}
	histogramVecMetric.metric = prometheus.NewHistogramVec(*histogramVecMetric.HistogramOpts, histogramVecMetric.getLabelList())
	return histogramVecMetric
}

// Register registers the HistogramVec metric.
func (h HistogramVecWithDynamicLabels) Register() {
	metrics.Registry.MustRegister(h.metric)
}

// Observe observes the histogram metric with val value.
func (h HistogramVecWithDynamicLabels) Observe(obj metav1.Object, fixedLabels map[string]string, val float64) {
	h.metric.With(h.buildLabels(fixedLabels, obj)).Observe(val)
}

var _ metricsWithDynamicLabels = CounterVecWithDynamicLabels{}
var _ metricsWithDynamicLabels = HistogramVecWithDynamicLabels{}

// GetOptionalClusterTypeLabels reads the AdditionalClusterDeploymentLabels from the metrics config and returns the same
// as a map
func GetOptionalClusterTypeLabels(mConfig *metricsconfig.MetricsConfig) map[string]string {
	var mapClusterTypeLabelToValue = make(map[string]string)
	// "cluster_type" is a non-optional label for these metrics, so if the config is empty (default), set the
	// "cluster_type" label
	if mConfig.AdditionalClusterDeploymentLabels == nil {
		mapClusterTypeLabelToValue["cluster_type"] = hivev1.HiveClusterTypeLabel
	} else {
		for k, v := range *mConfig.AdditionalClusterDeploymentLabels {
			mapClusterTypeLabelToValue[k] = v
		}
	}
	return mapClusterTypeLabelToValue
}
