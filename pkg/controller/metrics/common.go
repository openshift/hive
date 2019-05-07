/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"

	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Common metrics used by more than one controller

var (
	// MetricUninstallJobDuration is a metric set by both the ClusterDeployment and ClusterDeprovisionRequest controllers
	MetricUninstallJobDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "hive_cluster_deployment_uninstall_job_duration_seconds",
			Help:    "Distribution of the runtime of completed uninstall jobs.",
			Buckets: []float64{60, 300, 600, 1200, 1800, 2400, 3000, 3600},
		},
	)
)

func init() {
	metrics.Registry.MustRegister(MetricUninstallJobDuration)
}
