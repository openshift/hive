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
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/install"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

var (
	metricClusterDeploymentsTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_cluster_deployments",
		Help: "Total number of cluster deployments.",
	}, []string{"cluster_type", "age_lt"})
	metricClusterDeploymentsInstalledTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_cluster_deployments_installed",
		Help: "Total number of cluster deployments that are successfully installed.",
	}, []string{"cluster_type", "age_lt"})
	metricClusterDeploymentsUninstalledTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_cluster_deployments_uninstalled",
		Help: "Total number of cluster deployments that are not yet installed by type and bucket for length of time in this state.",
	},
		[]string{"cluster_type", "age_lt", "uninstalled_gt"},
	)
	metricClusterDeploymentsWithConditionTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_cluster_deployments_conditions",
		Help: "Total number of cluster deployments by type with conditions.",
	}, []string{"cluster_type", "age_lt", "condition"})

	metricInstallJobsRunningTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_install_jobs_running",
		Help: "Total number of install jobs running by cluster type.",
	}, []string{"cluster_type"})
	metricInstallJobsFailedTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_install_jobs_failed",
		Help: "Total number of install jobs failed by cluster type.",
	}, []string{"cluster_type"})
	metricUninstallJobsRunningTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_uninstall_jobs_running",
		Help: "Total number of uninstall jobs running by cluster type..",
	}, []string{"cluster_type"})
	metricUninstallJobsFailedTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_uninstall_jobs_failed",
		Help: "Total number of uninstall jobs failed by cluster type.",
	}, []string{"cluster_type"})
)

func init() {
	metrics.Registry.MustRegister(metricClusterDeploymentsTotal)
	metrics.Registry.MustRegister(metricClusterDeploymentsInstalledTotal)
	metrics.Registry.MustRegister(metricClusterDeploymentsUninstalledTotal)
	metrics.Registry.MustRegister(metricClusterDeploymentsWithConditionTotal)
	metrics.Registry.MustRegister(metricInstallJobsRunningTotal)
	metrics.Registry.MustRegister(metricInstallJobsFailedTotal)
	metrics.Registry.MustRegister(metricUninstallJobsRunningTotal)
	metrics.Registry.MustRegister(metricUninstallJobsFailedTotal)
}

// Add creates a new metrics Calculator and adds it to the Manager.
func Add(mgr manager.Manager) error {
	mc := &Calculator{
		Client:   mgr.GetClient(),
		Interval: 2 * time.Minute,
	}
	err := mgr.Add(mc)
	if err != nil {
		return err
	}

	return nil
}

// Calculator runs in a goroutine and periodically calculates and publishes
// Prometheus metrics which will be exposed at our /metrics endpoint. Note that this is not
// a standard controller watching Kube resources, it runs periodically and then goes to sleep.
//
// This should be used for metrics which do not fit well into controller reconcile loops,
// things that are calculated globally rather than metrics related to specific reconciliations.
type Calculator struct {
	Client client.Client

	// Interval is the length of time we sleep between metrics calculations.
	Interval time.Duration
}

// Start begins the metrics calculation loop.
func (mc *Calculator) Start(stopCh <-chan struct{}) error {
	log.Info("started metrics calculator goroutine")

	// Run forever, sleep at the end:
	wait.Until(func() {
		start := time.Now()
		mcLog := log.WithField("controller", "metrics")
		mcLog.Info("calculating metrics across all ClusterDeployments")
		// Load all ClusterDeployments so we can accumulate facts about them.
		clusterDeployments := &hivev1.ClusterDeploymentList{}
		err := mc.Client.List(context.Background(), &client.ListOptions{}, clusterDeployments)
		if err != nil {
			log.WithError(err).Error("error listing cluster deployments")
		} else {
			mcLog.WithField("total", len(clusterDeployments.Items)).Debug("loaded cluster deployments")

			accumulator, err := newClusterAccumulator(infinity, []string{"0h", "1h", "2h", "8h", "24h", "72h"})
			if err != nil {
				mcLog.WithError(err).Error("unable to calculate metrics")
				return
			}
			for _, cd := range clusterDeployments.Items {
				accumulator.processCluster(&cd)
			}

			accumulator.setMetrics(metricClusterDeploymentsTotal,
				metricClusterDeploymentsInstalledTotal,
				metricClusterDeploymentsUninstalledTotal,
				metricClusterDeploymentsWithConditionTotal,
				mcLog)

			// Also add metrics only for clusters created in last 48h
			accumulator, err = newClusterAccumulator("48h", []string{"0h", "1h", "2h", "8h", "24h"})
			if err != nil {
				mcLog.WithError(err).Error("unable to calculate metrics")
				return
			}
			for _, cd := range clusterDeployments.Items {
				accumulator.processCluster(&cd)
			}

			accumulator.setMetrics(metricClusterDeploymentsTotal,
				metricClusterDeploymentsInstalledTotal,
				metricClusterDeploymentsUninstalledTotal,
				metricClusterDeploymentsWithConditionTotal,
				mcLog)
		}
		mcLog.Info("calculating metrics across all install jobs")

		// install job metrics
		installJobs := &batchv1.JobList{}
		installJobLabelSelector := map[string]string{install.InstallJobLabel: "true"}
		err = mc.Client.List(context.Background(), client.MatchingLabels(installJobLabelSelector), installJobs)
		if err != nil {
			log.WithError(err).Error("error listing install jobs")
		} else {
			runningTotal, failedTotal := processJobs(installJobs.Items)
			for k, v := range runningTotal {
				mcLog.WithFields(log.Fields{
					"clusterType": k,
					"total":       v,
				}).Debug("calculated running install jobs total")
				metricInstallJobsRunningTotal.WithLabelValues(k).Set(float64(v))
			}
			for k, v := range failedTotal {
				mcLog.WithFields(log.Fields{
					"clusterType": k,
					"total":       v,
				}).Debug("calculated failed install jobs total")
				metricInstallJobsFailedTotal.WithLabelValues(k).Set(float64(v))
			}
		}

		mcLog.Info("calculating metrics across all uninstall jobs")
		// uninstall job metrics
		uninstallJobs := &batchv1.JobList{}
		uninstallJobLabelSelector := map[string]string{install.UninstallJobLabel: "true"}
		err = mc.Client.List(context.Background(), client.MatchingLabels(uninstallJobLabelSelector), uninstallJobs)
		if err != nil {
			log.WithError(err).Error("error listing uninstall jobs")
		} else {
			runningTotal, failedTotal := processJobs(uninstallJobs.Items)
			for k, v := range runningTotal {
				mcLog.WithFields(log.Fields{
					"clusterType": k,
					"total":       v,
				}).Debug("calculated running uninstall jobs total")
				metricUninstallJobsRunningTotal.WithLabelValues(k).Set(float64(v))
			}
			for k, v := range failedTotal {
				mcLog.WithFields(log.Fields{
					"clusterType": k,
					"total":       v,
				}).Debug("calculated failed uninstall jobs total")
				metricUninstallJobsFailedTotal.WithLabelValues(k).Set(float64(v))
			}
		}

		elapsed := time.Since(start)
		mcLog.WithField("elapsed", elapsed).Info("metrics calculation complete")
	}, mc.Interval, stopCh)

	return nil
}

func processJobs(jobs []batchv1.Job) (runningTotal, failedTotal map[string]int) {
	running := map[string]int{}
	failed := map[string]int{}
	for _, job := range jobs {
		clusterType := GetClusterDeploymentTypeForJob(job)
		if _, ok := running[clusterType]; !ok {
			running[clusterType] = 0
		}
		if _, ok := failed[clusterType]; !ok {
			failed[clusterType] = 0
		}
		if job.Status.CompletionTime == nil {
			if job.Status.Failed > 0 {
				failed[clusterType]++
			} else {
				running[clusterType]++
			}
		}
	}
	return running, failed
}

// clusterAccumulator is an object used to process cluster deployments and sort them so we can
// increment the appropriate metrics counter based on it's type, installed state, length of time
// it has been uninstalled, and the conditions it has.
type clusterAccumulator struct {
	// ageFilter can optionally be specified to skip processing clusters older than this duration. If this is not desired,
	// specify "0h" to include all.
	ageFilter    string
	ageFilterDur time.Duration

	// total maps cluster type to counter.
	total map[string]int

	// installed maps cluster type to counter.
	installed map[string]int

	// uninstalled maps a "greater than" duration string (i.e. 8h) to
	// cluster type to counter. Specify 0h if you want a bucket for the smallest duration.
	uninstalled map[string]map[string]int

	// conditions maps conditions to cluster type to counter.
	conditions map[hivev1.ClusterDeploymentConditionType]map[string]int
}

const (
	infinity = "Inf"
)

// newClusterAccumulator initializes a new cluster accumulator. Use "0h" for the age filter if you want to include
// all clusters.
func newClusterAccumulator(ageFilter string, uninstalledDurationBuckets []string) (*clusterAccumulator, error) {
	ca := &clusterAccumulator{
		ageFilter:   ageFilter,
		total:       map[string]int{},
		installed:   map[string]int{},
		uninstalled: map[string]map[string]int{},
		conditions:  map[hivev1.ClusterDeploymentConditionType]map[string]int{},
	}
	var err error
	if ageFilter != infinity {
		ca.ageFilterDur, err = time.ParseDuration(ageFilter)
		if err != nil {
			return nil, err
		}
	}

	for _, durStr := range uninstalledDurationBuckets {
		// Make sure all the strings parse as durations, we ignore errors below.
		_, err := time.ParseDuration(durStr)
		if err != nil {
			return nil, err
		}
		ca.uninstalled[durStr] = map[string]int{}
	}

	for _, cdct := range hivev1.AllClusterDeploymentConditions {
		ca.conditions[cdct] = map[string]int{}
	}
	return ca, nil
}

func (ca *clusterAccumulator) ensureClusterTypeBuckets(clusterType string) {
	// Make sure an entry exists for this cluster type in all relevant maps:

	_, ok := ca.total[clusterType]
	if !ok {
		ca.total[clusterType] = 0
	}

	_, ok = ca.installed[clusterType]
	if !ok {
		ca.installed[clusterType] = 0
	}

	for k, v := range ca.uninstalled {
		_, ok := v[clusterType]
		if !ok {
			ca.uninstalled[k][clusterType] = 0
		}
	}
	for k, v := range ca.conditions {
		_, ok := v[clusterType]
		if !ok {
			ca.conditions[k][clusterType] = 0
		}
	}
}

func (ca *clusterAccumulator) processCluster(cd *hivev1.ClusterDeployment) {
	if ca.ageFilter != infinity && time.Since(cd.CreationTimestamp.Time) > ca.ageFilterDur {
		return
	}

	clusterType := GetClusterDeploymentType(cd)
	ca.ensureClusterTypeBuckets(clusterType)

	ca.total[clusterType]++

	if cd.Status.Installed {
		ca.installed[clusterType]++
	} else {
		// Sort uninstall clusters into buckets based on how long since
		// they were created. The larger the bucket the more serious the problem.
		uninstalledDur := time.Since(cd.CreationTimestamp.Time)

		for k := range ca.uninstalled {
			// We already error checked that these parse in constructor func:
			gtDurBucket, _ := time.ParseDuration(k)
			if uninstalledDur > gtDurBucket {
				ca.uninstalled[k][clusterType]++
			}
		}
	}

	// Process conditions regardless if installed or not:
	for _, cond := range cd.Status.Conditions {
		if cond.Status == corev1.ConditionTrue {
			ca.conditions[cond.Type][clusterType]++
		}
	}
}

func (ca *clusterAccumulator) setMetrics(total, installed, uninstalled, conditions *prometheus.GaugeVec, mcLog log.FieldLogger) {

	for k, v := range ca.total {
		total.WithLabelValues(k, ca.ageFilter).Set(float64(v))
		mcLog.WithFields(log.Fields{
			"clusterType": k,
			"age_lt":      ca.ageFilter,
			"total":       v,
		}).Debug("calculated total cluster deployments metric")
	}
	for k, v := range ca.installed {
		installed.WithLabelValues(k, ca.ageFilter).Set(float64(v))
		mcLog.WithFields(log.Fields{
			"clusterType": k,
			"age_lt":      ca.ageFilter,
			"total":       v,
		}).Debug("calculated total cluster deployments installed metric")
	}
	for k, v := range ca.uninstalled {
		for k1, v1 := range v {
			uninstalled.WithLabelValues(k1, ca.ageFilter, k).Set(float64(v1))
			mcLog.WithFields(log.Fields{
				"clusterType":    k1,
				"age_lt":         ca.ageFilter,
				"uninstalled_gt": k,
				"total":          v1,
			}).Debug("calculated total cluster deployments uninstalled metric")
		}
	}
	for k, v := range ca.conditions {
		for k1, v1 := range v {
			conditions.WithLabelValues(k1, ca.ageFilter, string(k)).Set(float64(v1))
			mcLog.WithFields(log.Fields{
				"clusterType": k1,
				"age_lt":      ca.ageFilter,
				"condition":   string(k),
				"total":       v1,
			}).Debug("calculated total cluster deployments with condition metric")
		}
	}
}

// GetClusterDeploymentType returns the value of the hive.openshift.io/cluster-type label if set,
// otherwise a default value.
func GetClusterDeploymentType(cd *hivev1.ClusterDeployment) string {
	if cd.Labels == nil {
		return hivev1.DefaultClusterType
	}
	typeStr, ok := cd.Labels[hivev1.HiveClusterTypeLabel]
	if !ok {
		return hivev1.DefaultClusterType
	}
	return typeStr
}

// GetClusterDeploymentTypeForJob returns the value of the hive.openshift.io/cluster-type label if set,
// otherwise a default value.
func GetClusterDeploymentTypeForJob(job batchv1.Job) string {
	if job.Labels == nil {
		return hivev1.DefaultClusterType
	}
	typeStr, ok := job.Labels[hivev1.HiveClusterTypeLabel]
	if !ok {
		return hivev1.DefaultClusterType
	}
	return typeStr
}
