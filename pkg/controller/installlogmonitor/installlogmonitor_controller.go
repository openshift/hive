/*
Copyright 2019 The Kubernetes Authors.

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

package installlogmonitor

import (
	"context"
	"fmt"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	controllerName      = "installlogmonitor"
	processedAnnotation = "hive.openshift.io/install-log-processed"
	regexConfigMapName  = "install-log-regexes"
	hiveNamespace       = "hive"
	unknownReason       = "UnknownError"
	unknownMessage      = "Cluster install failed but no known errors found in logs"
	successReason       = "ClusterInstalled"
	successMessage      = "Cluster install completed successfully"
)

var (
	metricInstallErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hive_install_errors",
		Help: "Counter incremented every time we observe certain errors strings in install logs.",
	},
		[]string{"cluster_type", "reason"},
	)
)

func init() {
	metrics.Registry.MustRegister(metricInstallErrors)
}

// Add creates a new InstallLogMonitor Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileInstallLog{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("installlogmonitor-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to ConfigMap
	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileInstallLog{}

// ReconcileInstallLog reconciles an install log ConfigMap object uploaded by the hive installmanager pod.
type ReconcileInstallLog struct {
	client.Client
	scheme *runtime.Scheme
}

func (r *ReconcileInstallLog) isHiveInstallLog(cm *corev1.ConfigMap) (isInstallLog bool, needsMigration bool) {
	if _, ok := cm.Labels[hivev1.HiveInstallLogLabel]; ok {
		return true, false
	}

	// If it looks like one of our install logs that predates install manager images, return true
	// and indicate it needs migration.
	if _, ok := cm.Data["log"]; ok && strings.HasSuffix(cm.Name, "-install-log") {
		return true, true
	}

	return false, false
}

// migrateInstallLog is a temporary function that will apply the new labels to pre-existing install log configmaps.
func (r *ReconcileInstallLog) migrateInstallLog(cm *corev1.ConfigMap) error {
	if cm.Labels == nil {
		cm.Labels = map[string]string{}
	}

	cm.Labels[hivev1.HiveInstallLogLabel] = "true"

	// Use the owner ref to determine cluster deployment name.
	var clusterDeploymentName string
	for _, ownerRef := range cm.OwnerReferences {
		if ownerRef.Kind == "ClusterDeployment" {
			clusterDeploymentName = ownerRef.Name
		}
	}
	if clusterDeploymentName == "" {
		return fmt.Errorf("unable to find a ClusterDeployment owner reference on configmap %s", cm.Name)
	}
	cm.Labels[hivev1.HiveClusterDeploymentNameLabel] = clusterDeploymentName

	return r.Client.Update(context.Background(), cm)
}

// Reconcile parses install log to monitor for known issues.
// +kubebuilder:rbac:groups=core,resources=serviceaccounts;secrets;configmaps,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileInstallLog) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	iLog := log.WithFields(log.Fields{
		"configMap":  request.NamespacedName.String(),
		"controller": controllerName,
	})

	// Load the regex configmap, if we don't have one, there's not much point proceeding here.
	regexCM := &corev1.ConfigMap{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: regexConfigMapName, Namespace: hiveNamespace}, regexCM)
	if err != nil {
		if errors.IsNotFound(err) {
			iLog.Debugf("%s configmap does not exist, nothing to scan for", regexConfigMapName)
			return reconcile.Result{}, nil
		}
		iLog.WithError(err).Errorf("error loading %s configmap", regexConfigMapName)
		return reconcile.Result{}, err
	}
	ilRegexes, err := loadInstallLogRegexes(regexCM, iLog)
	if err != nil {
		iLog.WithError(err).Error("error loading regex configmap")
		return reconcile.Result{}, err
	}

	cm := &corev1.ConfigMap{}
	err = r.Get(context.TODO(), request.NamespacedName, cm)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			iLog.Debug("ConfigMap not found, skipping")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		iLog.WithError(err).Error("cannot get clusterdeprovisionrequest")
		return reconcile.Result{}, err
	}

	if !cm.DeletionTimestamp.IsZero() {
		iLog.Debug("configmap being deleted, skipping")
		return reconcile.Result{}, nil
	}

	// Temporary migration of install logs created by hiveutil images prior to the labels being added.
	// TODO: Remove this and watch for ConfigMaps with our labels once the main ClusterImageSets in use are
	// new enough to populate the Hive labels on install log ConfigMaps.
	isInstallLog, needsMigration := r.isHiveInstallLog(cm)
	if !isInstallLog {
		return reconcile.Result{}, nil
	}

	if needsMigration {
		err := r.migrateInstallLog(cm)
		if err != nil {
			iLog.WithError(err).Error("error migrating install-log configmap")
		}
		return reconcile.Result{}, err
	}

	cd := &hivev1.ClusterDeployment{}
	cdName, ok := cm.Labels[hivev1.HiveClusterDeploymentNameLabel]
	if !ok {
		iLog.Error("cannot reconcile install log configmap without cluster-deployment-name label")
		return reconcile.Result{}, fmt.Errorf("install log configmap %s has no %s label",
			request.NamespacedName.String(),
			hivev1.HiveClusterDeploymentNameLabel)
	}
	cdNSName := types.NamespacedName{Namespace: cm.Namespace, Name: cdName}
	err = r.Get(context.TODO(), cdNSName, cd)
	if err != nil {
		iLog.WithError(err).Error("unable to lookup cluster deployment for configmap")
		return reconcile.Result{}, err
	}

	// Check if we've processed this configmap before and skip if so. The install manager currently
	// replaces the log on a retry (full delete + recreate).
	if _, ok := cm.Annotations[processedAnnotation]; ok {
		iLog.Debugf("skipping install log that has already been processed (%s annotation present)", processedAnnotation)
		return reconcile.Result{}, nil
	}

	iLog.Info("processing new install log")

	// Apply the processed annotation and save before we report anything. If we succeed here and fail later
	// we accept that we will lose processing of that install log. This is for tracking/metrics and not
	// anything critical.
	if cm.ObjectMeta.Annotations == nil {
		cm.ObjectMeta.Annotations = map[string]string{}
	}
	cm.ObjectMeta.Annotations[processedAnnotation] = "true"
	if err := r.Client.Update(context.Background(), cm); err != nil {
		iLog.WithError(err).Error("error saving processed annotation")
		return reconcile.Result{}, err
	}

	success, successKeyReported := cm.Data["success"]

	foundError := false
	if !successKeyReported || success == "false" {
		log := cm.Data["log"]
		// Log each line separately, this brings all our install logs from many namespaces into
		// the main hive log where we can aggregate search results.
		installLogLines := strings.Split(log, "\n")
		for _, l := range installLogLines {
			iLog.WithField("line", l).Info("install log line")
		}

		// Scan log contents for known errors if the install was not reported as a success:
		for _, ilr := range ilRegexes {
			for _, re := range ilr.SearchRegexes {
				if re.Match([]byte(log)) {
					iLog.WithField("reason", ilr.InstallFailingReason).Info("found known install failure string")
					cd.Status.Conditions = controllerutils.SetClusterDeploymentCondition(cd.Status.Conditions, hivev1.InstallFailingCondition,
						corev1.ConditionTrue, ilr.InstallFailingReason, ilr.InstallFailingMessage, controllerutils.UpdateConditionAlways)
					// Increment a counter metric for this cluster type and error reason:
					metricInstallErrors.WithLabelValues(hivemetrics.GetClusterDeploymentType(cd), ilr.InstallFailingReason).Inc()
					foundError = true
					break
				}
			}
			if foundError {
				break
			}
		}
	}

	// If we reach this point, we have found no errors. If the install log configmap specifies that the install failed,
	// we will still report InstallFailed condition with reason unknown. Note that older install manager pods will not
	// set success, so for these we will just assume success and no condition will be set.
	if !foundError && successKeyReported && success == "false" {
		iLog.Info("install failed but no error strings found, reporting unknown error")
		cd.Status.Conditions = controllerutils.SetClusterDeploymentCondition(cd.Status.Conditions, hivev1.InstallFailingCondition,
			corev1.ConditionTrue, unknownReason, unknownMessage, controllerutils.UpdateConditionAlways)
		metricInstallErrors.WithLabelValues(hivemetrics.GetClusterDeploymentType(cd), unknownReason).Inc()
	} else if !foundError {
		// Assume success and clear the condition if it is present.
		iLog.Info("install successful, clearing InstallFailing condition if set")
		cd.Status.Conditions = controllerutils.SetClusterDeploymentCondition(cd.Status.Conditions, hivev1.InstallFailingCondition,
			corev1.ConditionFalse, successReason, successMessage, controllerutils.UpdateConditionIfReasonOrMessageChange)
	}

	// Save and return on first error, we don't store more than one.
	err = r.Client.Status().Update(context.Background(), cd)
	if err != nil {
		iLog.WithError(err).Error("error saving condition onto cluster deployment, install log processing will be lost")
	}
	iLog.Info("reconcile complete")
	return reconcile.Result{}, nil
}
