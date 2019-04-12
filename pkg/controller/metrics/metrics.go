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
	metricClusterDeploymentsTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "hive_cluster_deployments_total",
		Help: "Total number of cluster deployments that exist in Hive.",
	})
	metricClusterDeploymentsInstalledTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "hive_cluster_deployments_installed_total",
		Help: "Total number of cluster deployments that are successfully installed.",
	})
	metricClusterDeploymentsUninstalledTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_cluster_deployments_uninstalled_total",
		Help: "Total number of cluster deployments that are not yet installed.",
	},
		[]string{"duration"},
	)
	metricClusterDeploymentsWithConditionTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_cluster_deployments_with_condition_total",
		Help: "Total number of cluster deployments with conditions.",
	}, []string{"condition"})
	metricInstallJobsRunningTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "hive_install_jobs_running_total",
		Help: "Total number of install jobs running in Hive.",
	})
	metricInstallJobsFailedTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "hive_install_jobs_failed_total",
		Help: "Total number of install jobs failed in Hive.",
	})
	metricUninstallJobsRunningTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "hive_uninstall_jobs_running_total",
		Help: "Total number of uninstall jobs running in Hive.",
	})
	metricUninstallJobsFailedTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "hive_uninstall_jobs_failed_total",
		Help: "Total number of uninstall jobs failed in Hive.",
	})
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
			mcLog.WithField("totalClusterDeployments", len(clusterDeployments.Items)).Debug("loaded cluster deployments")

			total,
				installedTotal,
				uninstalledUnder1h,
				uninstalledOver1h,
				uninstalledOver2h,
				uninstalledOver8h,
				uninstalledOver24h,
				conditionTotals := processClusters(clusterDeployments.Items, mcLog)

			metricClusterDeploymentsTotal.Set(float64(total))
			metricClusterDeploymentsInstalledTotal.Set(float64(installedTotal))
			metricClusterDeploymentsUninstalledTotal.WithLabelValues("under1h").Set(float64(uninstalledUnder1h))
			metricClusterDeploymentsUninstalledTotal.WithLabelValues("over1h").Set(float64(uninstalledOver1h))
			metricClusterDeploymentsUninstalledTotal.WithLabelValues("over2h").Set(float64(uninstalledOver2h))
			metricClusterDeploymentsUninstalledTotal.WithLabelValues("over8h").Set(float64(uninstalledOver8h))
			metricClusterDeploymentsUninstalledTotal.WithLabelValues("over24h").Set(float64(uninstalledOver24h))

			for k, v := range conditionTotals {
				metricClusterDeploymentsWithConditionTotal.WithLabelValues(string(k)).Set(float64(v))
			}
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
			mcLog.WithField("runningInstalls", runningTotal).Debug("calculating running install jobs metric")
			mcLog.WithField("failedInstalls", failedTotal).Debug("calculated failed install jobs metric")
			metricInstallJobsRunningTotal.Set(float64(runningTotal))
			metricInstallJobsFailedTotal.Set(float64(failedTotal))
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
			mcLog.WithField("runningUninstalls", runningTotal).Debug("calculated running uninstall jobs metric")
			mcLog.WithField("failedUninstalls", failedTotal).Debug("calculated failed uninstall jobs metric")
			metricUninstallJobsRunningTotal.Set(float64(runningTotal))
			metricUninstallJobsFailedTotal.Set(float64(failedTotal))
		}

		elapsed := time.Since(start)
		mcLog.WithField("elapsed", elapsed).Info("metrics calculation complete")
	}, mc.Interval, stopCh)

	return nil
}

func processJobs(jobs []batchv1.Job) (runningTotal, failedTotal int) {
	var running int
	var failed int
	for _, job := range jobs {
		if job.Status.CompletionTime == nil {
			if job.Status.Failed > 0 {
				failed++
			} else {
				running++
			}
		}
	}
	return running, failed
}

func processClusters(clusters []hivev1.ClusterDeployment, mcLog log.FieldLogger) (
	total,
	installedTotal,
	uninstalledUnder1h,
	uninstalledOver1h,
	uninstalledOver2h,
	uninstalledOver8h,
	uninstalledOver24h int,
	conditionTotals map[hivev1.ClusterDeploymentConditionType]int) {

	conditionTotals = map[hivev1.ClusterDeploymentConditionType]int{}
	for _, cdct := range hivev1.AllClusterDeploymentConditions {
		conditionTotals[cdct] = 0
	}

	for _, cd := range clusters {
		total = total + 1
		if cd.Status.Installed {
			installedTotal = installedTotal + 1
		} else {

			// Sort uninstall clusters into buckets based on how long since
			// they were created. The larger the bucket the more serious the problem.

			uninstalledDur := time.Since(cd.CreationTimestamp.Time)

			if uninstalledDur > 1*time.Hour {
				uninstalledOver1h++

				// Anything over 2 hours we start to consider an issue:
				if uninstalledDur > 2*time.Hour {
					mcLog.WithFields(log.Fields{
						"clusterDeployment": cd.Name,
						"created":           cd.CreationTimestamp.Time,
						"uninstalledFor":    uninstalledDur,
					}).Warn("cluster has failed to install in expected timeframe")

					uninstalledOver2h++
				}
				// Increment additional counters for other thresholds of awful:
				if uninstalledDur > 8*time.Hour {
					uninstalledOver8h++
				}
				if uninstalledDur > 24*time.Hour {
					uninstalledOver24h++
				}
			} else {
				uninstalledUnder1h++
			}
		}

		// Process conditions regardless if installed or not:
		for _, cond := range cd.Status.Conditions {
			if cond.Status == corev1.ConditionTrue {
				conditionTotals[cond.Type]++
			}
		}
	}
	return total, installedTotal, uninstalledUnder1h, uninstalledOver1h, uninstalledOver2h, uninstalledOver8h, uninstalledOver24h, conditionTotals
}
