package metrics

import (
	"sort"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/apis/hive/v1/metricsconfig"
	"github.com/openshift/hive/pkg/constants"
)

// metricsWithDynamicLabels encompasses metrics that can have optional labels.
type metricsWithDynamicLabels interface {
	ObserveMetricWithDynamicLabels(metav1.Object, log.FieldLogger, map[string]string, float64)
}

type DynamicLabels struct {
	// FixedLabels just need to be a slice used for defining the metric.
	FixedLabels []string
	// OptionalLabels will be used both for defining the metric and by the Observe method
	OptionalLabels map[string]string
}

type CounterVecWithDynamicLabels struct {
	*prometheus.CounterVec
	*DynamicLabels
}

// ObserveMetricWithDynamicLabels logs the counter metric. It simply increments the value, so last parameter isn't used.
func (c CounterVecWithDynamicLabels) ObserveMetricWithDynamicLabels(obj metav1.Object, log log.FieldLogger,
	fixedLabels map[string]string, _ float64) {
	fixedLabels = verifyAllLabelsPresent(fixedLabels, c.FixedLabels)
	curriedVec, err := c.CounterVec.CurryWith(fixedLabels)
	// Maps cannot be sorted, however we would want to loop through them in a sorted fashion to ensure all the metric
	// labels are added in an expected order. Get the slice of map keys first, sort them and then use it to loop over
	// the map
	labels := make(prometheus.Labels, len(c.OptionalLabels))
	for _, v := range GetSortedKeys(c.OptionalLabels) {
		labels[v] = GetLabelValue(obj, c.OptionalLabels[v])
	}
	curriedVec, err = curriedVec.CurryWith(labels)
	if err != nil {
		// The possibility of hitting an error here is low, but report it and don't observe the metric in case it happens
		log.WithField("metric", c.CounterVec).WithError(err).Error("error observing metric")
	} else {
		curriedVec.WithLabelValues().Inc()
	}
}

// verifyAllLabelsPresent ensures all fixed labels for a metric are present. If missing, it sets the corresponding label
// to a default value
// Note: Avoid relying on this method, as it cannot ensure the labels would get curried in the expected order
func verifyAllLabelsPresent(labels map[string]string, fixedLabels []string) map[string]string {
	if len(labels) < len(fixedLabels) {
		for _, v := range fixedLabels {
			if _, ok := labels[v]; !ok {
				labels[v] = constants.MetricLabelDefaultValue
			}
		}
	}
	return labels
}

type HistogramVecWithDynamicLabels struct {
	*prometheus.HistogramVec
	*DynamicLabels
}

// ObserveMetricWithDynamicLabels observes the histogram metric with val value.
func (h HistogramVecWithDynamicLabels) ObserveMetricWithDynamicLabels(obj metav1.Object, log log.FieldLogger,
	fixedLabels map[string]string, val float64) {
	fixedLabels = verifyAllLabelsPresent(fixedLabels, h.FixedLabels)
	curriedVec, err := h.HistogramVec.CurryWith(fixedLabels)
	// Maps cannot be sorted, however we would want to loop through them in a sorted fashion to ensure all the metric
	// labels are added in an expected order. Get the slice of map keys first, sort them and then use it to loop over
	// the map
	labels := make(prometheus.Labels, len(h.OptionalLabels))
	for _, v := range GetSortedKeys(h.OptionalLabels) {
		labels[v] = GetLabelValue(obj, h.OptionalLabels[v])
	}
	curriedVec, err = curriedVec.CurryWith(labels)
	if err != nil {
		// The possibility of hitting an error here is low, but report it and don't observe the metric in case it happens
		log.WithField("metric", h.HistogramVec).WithError(err).Error("error observing metric")
	} else {
		curriedVec.WithLabelValues().Observe(val)
	}
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
	if len(mConfig.AdditionalClusterDeploymentLabels) == 0 {
		mapClusterTypeLabelToValue["cluster_type"] = hivev1.HiveClusterTypeLabel
	}
	for k, v := range mConfig.AdditionalClusterDeploymentLabels {
		mapClusterTypeLabelToValue[k] = v
	}
	return mapClusterTypeLabelToValue
}

// GetSortedKeys gets the keys of a map, sorts them and returns the same.
func GetSortedKeys(mapToSort map[string]string) []string {
	var keys []string
	for k := range mapToSort {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
