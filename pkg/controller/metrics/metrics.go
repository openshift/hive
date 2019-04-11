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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/openshift/hive/pkg/install"
	batchv1 "k8s.io/api/batch/v1"
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
		mcLog := log.WithField("controller", "metrics")
		// Load all ClusterDeployments so we can accumulate facts about them.
		clusterDeployments := &hivev1.ClusterDeploymentList{}
		err := mc.Client.List(context.Background(), &client.ListOptions{}, clusterDeployments)
		if err != nil {
			log.WithError(err).Error("error listing cluster deployments")
		} else {
			mcLog.WithField("totalClusterDeployments", len(clusterDeployments.Items)).Debug("loaded cluster deployments")
			total := 0
			installedTotal := 0
			for _, cd := range clusterDeployments.Items {
				total = total + 1
				if cd.Status.Installed {
					installedTotal = installedTotal + 1
				}
			}
			metricClusterDeploymentsTotal.Set(float64(total))
			metricClusterDeploymentsInstalledTotal.Set(float64(installedTotal))
		}
		mcLog.Debug("calculating jobs metrics")

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
	}, mc.Interval, stopCh)

	return nil
}

func processJobs(jobs []batchv1.Job) (runningTotal, failedTotal int) {
	var running int
	var failed int
	for _, job := range jobs {
		if job.Status.CompletionTime == nil {
			if job.Status.Failed > 0 {
				failed += 1
			} else {
				running += 1
			}
		}
	}
	return running, failed
}
