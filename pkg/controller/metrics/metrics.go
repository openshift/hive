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

	"github.com/openshift/hive/pkg/imageset"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
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
	metricClusterDeploymentsDeprovisioningTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_cluster_deployments_deprovisioning",
		Help: "Total number of cluster deployments in process of being deprovisioned.",
	}, []string{"cluster_type", "age_lt", "deprovisioning_gt"})
	metricClusterDeploymentsWithConditionTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_cluster_deployments_conditions",
		Help: "Total number of cluster deployments by type with conditions.",
	}, []string{"cluster_type", "age_lt", "condition"})
	metricInstallJobsTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_install_jobs",
		Help: "Total number of install jobs running by cluster type and state.",
	}, []string{"cluster_type", "state"})
	metricUninstallJobsTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_uninstall_jobs",
		Help: "Total number of uninstall jobs running by cluster type and state.",
	}, []string{"cluster_type", "state"})
	metricImagesetJobsTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_imageset_jobs",
		Help: "Total number of imageset jobs running by cluster type and state.",
	}, []string{"cluster_type", "state"})

	// MetricClusterDeploymentProvisionUnderwaySeconds is a prometheus metric for the number of seconds
	// between when a still provisioning cluster was created and now.
	MetricClusterDeploymentProvisionUnderwaySeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "hive_cluster_deployment_provision_underway_seconds",
			Help: "Length of time a cluster has been provisioning. Goes to 0 on succesful install and then will no longer be reported.",
		},
		[]string{"cluster_deployment", "namespace", "cluster_type"},
	)
	// MetricClusterDeploymentDeprovisioningUnderwaySeconds is a prometheus metric for the number of seconds
	// between when a still deprovisioning cluster was created and now.
	MetricClusterDeploymentDeprovisioningUnderwaySeconds = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "hive_cluster_deployment_deprovision_underway_seconds",
			// Will clear once hive restarts.
			Help: "Length of time a cluster has been deprovisioning. Goes to 0 on succesful deprovision and then will no longer be reported.",
		},
		[]string{"cluster_deployment", "namespace", "cluster_type"},
	)
)

func init() {
	metrics.Registry.MustRegister(metricClusterDeploymentsTotal)
	metrics.Registry.MustRegister(metricClusterDeploymentsInstalledTotal)
	metrics.Registry.MustRegister(metricClusterDeploymentsUninstalledTotal)
	metrics.Registry.MustRegister(metricClusterDeploymentsDeprovisioningTotal)
	metrics.Registry.MustRegister(metricClusterDeploymentsWithConditionTotal)
	metrics.Registry.MustRegister(metricInstallJobsTotal)
	metrics.Registry.MustRegister(metricUninstallJobsTotal)
	metrics.Registry.MustRegister(metricImagesetJobsTotal)

	metrics.Registry.MustRegister(MetricClusterDeploymentProvisionUnderwaySeconds)
	metrics.Registry.MustRegister(MetricClusterDeploymentDeprovisioningUnderwaySeconds)
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

				if cd.DeletionTimestamp != nil {
					if controllerutils.HasFinalizer(&cd, hivev1.FinalizerDeprovision) {
						// Deprovision still underway, report metric for this cluster.
						// Note that the clusterdeployment_controller is responsible for clearing
						// this value to 0, as it is the only place where we know we first observe
						// a deletion completed. (when we clear the finalizer successfully)
						MetricClusterDeploymentDeprovisioningUnderwaySeconds.WithLabelValues(
							cd.Name,
							cd.Namespace,
							GetClusterDeploymentType(&cd)).Set(
							time.Since(cd.DeletionTimestamp.Time).Seconds())
					}
				} else {
					if !cd.Status.Installed {
						// Similarly for installing clusters we report the seconds since
						// cluster was created. clusterdeployment_controller should set to 0
						// once we know we've first observed install success, and then this
						// metric should no longer be reported.
						MetricClusterDeploymentProvisionUnderwaySeconds.WithLabelValues(
							cd.Name,
							cd.Namespace,
							GetClusterDeploymentType(&cd)).Set(
							time.Since(cd.CreationTimestamp.Time).Seconds())
					}
				}

			}

			accumulator.setMetrics(metricClusterDeploymentsTotal,
				metricClusterDeploymentsInstalledTotal,
				metricClusterDeploymentsUninstalledTotal,
				metricClusterDeploymentsDeprovisioningTotal,
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
				metricClusterDeploymentsDeprovisioningTotal,
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
			runningTotal, succeededTotal, failedTotal := processJobs(installJobs.Items)
			for k, v := range runningTotal {
				mcLog.WithFields(log.Fields{
					"clusterType": k,
					"total":       v,
				}).Debug("calculated running install jobs total")
				metricInstallJobsTotal.WithLabelValues(k, stateRunning).Set(float64(v))
			}
			for k, v := range succeededTotal {
				mcLog.WithFields(log.Fields{
					"clusterType": k,
					"total":       v,
				}).Debug("calculated succeeded install jobs total")
				metricInstallJobsTotal.WithLabelValues(k, stateSucceeded).Set(float64(v))
			}
			for k, v := range failedTotal {
				mcLog.WithFields(log.Fields{
					"clusterType": k,
					"total":       v,
				}).Debug("calculated failed install jobs total")
				metricInstallJobsTotal.WithLabelValues(k, stateFailed).Set(float64(v))
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
			runningTotal, succeededTotal, failedTotal := processJobs(uninstallJobs.Items)
			for k, v := range runningTotal {
				mcLog.WithFields(log.Fields{
					"clusterType": k,
					"total":       v,
				}).Debug("calculated running uninstall jobs total")
				metricUninstallJobsTotal.WithLabelValues(k, stateRunning).Set(float64(v))
			}
			for k, v := range succeededTotal {
				mcLog.WithFields(log.Fields{
					"clusterType": k,
					"total":       v,
				}).Debug("calculated succeeded uninstall jobs total")
				metricUninstallJobsTotal.WithLabelValues(k, stateSucceeded).Set(float64(v))
			}
			for k, v := range failedTotal {
				mcLog.WithFields(log.Fields{
					"clusterType": k,
					"total":       v,
				}).Debug("calculated failed uninstall jobs total")
				metricUninstallJobsTotal.WithLabelValues(k, stateFailed).Set(float64(v))
			}
		}

		mcLog.Info("calculating metrics across all imageset jobs")
		// imageset job metrics
		imagesetJobs := &batchv1.JobList{}
		imagesetJobLabelSelector := map[string]string{imageset.ImagesetJobLabel: "true"}
		err = mc.Client.List(context.Background(), client.MatchingLabels(imagesetJobLabelSelector), imagesetJobs)
		if err != nil {
			log.WithError(err).Error("error listing imageset jobs")
		} else {
			runningTotal, succeededTotal, failedTotal := processJobs(imagesetJobs.Items)
			for k, v := range runningTotal {
				mcLog.WithFields(log.Fields{
					"clusterType": k,
					"total":       v,
				}).Debug("calculated running imageset jobs total")
				metricImagesetJobsTotal.WithLabelValues(k, stateRunning).Set(float64(v))
			}
			for k, v := range succeededTotal {
				mcLog.WithFields(log.Fields{
					"clusterType": k,
					"total":       v,
				}).Debug("calculated succeeded imageset jobs total")
				metricImagesetJobsTotal.WithLabelValues(k, stateSucceeded).Set(float64(v))
			}
			for k, v := range failedTotal {
				mcLog.WithFields(log.Fields{
					"clusterType": k,
					"total":       v,
				}).Debug("calculated failed imageset jobs total")
				metricImagesetJobsTotal.WithLabelValues(k, stateFailed).Set(float64(v))
			}
		}

		elapsed := time.Since(start)
		mcLog.WithField("elapsed", elapsed).Info("metrics calculation complete")
	}, mc.Interval, stopCh)

	return nil
}

func processJobs(jobs []batchv1.Job) (runningTotal, succeededTotal, failedTotal map[string]int) {
	running := map[string]int{}
	failed := map[string]int{}
	succeeded := map[string]int{}
	for _, job := range jobs {
		clusterType := GetClusterDeploymentTypeForJob(job)

		// Sort the jobs:
		if job.Status.Failed > 0 {
			failed[clusterType]++
		} else if job.Status.Succeeded > 0 {
			succeeded[clusterType]++
		} else {
			running[clusterType]++
		}
	}
	return running, succeeded, failed
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

	// deprovisioning maps a "greater than" duration string (i.e. 8h) to
	// cluster type to counter. Specify 0h if you want a bucket for the smallest duration.
	deprovisioning map[string]map[string]int

	// installed maps cluster type to counter.
	installed map[string]int

	// uninstalled maps a "greater than" duration string (i.e. 8h) to
	// cluster type to counter. Specify 0h if you want a bucket for the smallest duration.
	uninstalled map[string]map[string]int

	// conditions maps conditions to cluster type to counter.
	conditions map[hivev1.ClusterDeploymentConditionType]map[string]int

	// clusterTypesSet will contain every cluster type we encounter during processing.
	// Used to zero out some values which may no longer exist when setting the final metrics.
	// Maps cluster type to a meaningless bool.
	clusterTypesSet map[string]bool
}

const (
	infinity       = "+Inf"
	stateRunning   = "running"
	stateSucceeded = "succeeded"
	stateFailed    = "failed"
)

// newClusterAccumulator initializes a new cluster accumulator.
// ageFilter can be used to exclude clusters older than a certain duration. Use "0h" to include all clusters.
// durationBuckets are used to sort uninstalled, or deleted clusters into buckets based on how long they have been in that state.
func newClusterAccumulator(ageFilter string, durationBuckets []string) (*clusterAccumulator, error) {
	ca := &clusterAccumulator{
		ageFilter:       ageFilter,
		total:           map[string]int{},
		installed:       map[string]int{},
		deprovisioning:  map[string]map[string]int{},
		uninstalled:     map[string]map[string]int{},
		conditions:      map[hivev1.ClusterDeploymentConditionType]map[string]int{},
		clusterTypesSet: map[string]bool{},
	}
	var err error
	if ageFilter != infinity {
		ca.ageFilterDur, err = time.ParseDuration(ageFilter)
		if err != nil {
			return nil, err
		}
	}

	for _, durStr := range durationBuckets {
		// Make sure all the strings parse as durations, we ignore errors below.
		_, err := time.ParseDuration(durStr)
		if err != nil {
			return nil, err
		}
		ca.uninstalled[durStr] = map[string]int{}
		ca.deprovisioning[durStr] = map[string]int{}
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

	for k, v := range ca.deprovisioning {
		_, ok := v[clusterType]
		if !ok {
			ca.deprovisioning[k][clusterType] = 0
		}
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
	ca.clusterTypesSet[clusterType] = true

	ca.total[clusterType]++

	if cd.DeletionTimestamp != nil {
		// Sort deleted clusters into buckets based on how long since
		// they were deleted. The larger the bucket the more serious the problem.
		deletedDur := time.Since(cd.DeletionTimestamp.Time)

		for k := range ca.deprovisioning {
			// We already error checked that these parse in constructor func:
			gtDurBucket, _ := time.ParseDuration(k)
			if deletedDur > gtDurBucket {
				ca.deprovisioning[k][clusterType]++
			}
		}
	}

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

func (ca *clusterAccumulator) setMetrics(total, installed, uninstalled, deprovisioning, conditions *prometheus.GaugeVec, mcLog log.FieldLogger) {

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
		for clusterType := range ca.clusterTypesSet {
			if count, ok := v[clusterType]; ok {
				uninstalled.WithLabelValues(clusterType, ca.ageFilter, k).Set(float64(count))
				mcLog.WithFields(log.Fields{
					"clusterType":    clusterType,
					"age_lt":         ca.ageFilter,
					"uninstalled_gt": k,
					"total":          count,
				}).Debug("calculated total cluster deployments uninstalled metric")
			} else {
				// We need to potentially clear out old cluster types no longer showing in the list.
				// This will work so long as there is at least one cluster of that type still remaining
				// in hive somewhere.
				uninstalled.WithLabelValues(clusterType, ca.ageFilter, k).Set(float64(0))
				mcLog.WithFields(log.Fields{
					"clusterType":    clusterType,
					"age_lt":         ca.ageFilter,
					"uninstalled_gt": k,
					"total":          0,
				}).Debug("calculated total cluster deployments uninstalled metric")
			}
		}
	}
	for k, v := range ca.deprovisioning {
		for clusterType := range ca.clusterTypesSet {
			if count, ok := v[clusterType]; ok {
				deprovisioning.WithLabelValues(clusterType, ca.ageFilter, k).Set(float64(count))
				mcLog.WithFields(log.Fields{
					"clusterType":    clusterType,
					"age_lt":         ca.ageFilter,
					"uninstalled_gt": k,
					"total":          count,
				}).Debug("calculated total cluster deployments uninstalled metric")
			} else {
				// We need to potentially clear out old cluster types no longer showing in the list.
				// This will work so long as there is at least one cluster of that type still remaining
				// in hive somewhere.
				deprovisioning.WithLabelValues(clusterType, ca.ageFilter, k).Set(float64(0))
				mcLog.WithFields(log.Fields{
					"clusterType":    clusterType,
					"age_lt":         ca.ageFilter,
					"uninstalled_gt": k,
					"total":          0,
				}).Debug("calculated total cluster deployments deprovisioning metric")
			}
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
