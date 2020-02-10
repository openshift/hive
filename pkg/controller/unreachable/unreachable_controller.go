/*
Copyright (C) 2019 Red Hat, Inc.

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

// Package unreachable provides a controller which periodically checks if a remote cluster is reachable
// and maintains a condition on the cluster as a result. If the unreachable condition is true, other controllers
// can skip attempts to reach the cluster which require a 30 second timeout.
package unreachable

import (
	"context"
	"time"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/remoteclient"
)

const (
	controllerName = "unreachable"

	maxUnreachableDuration      = 2 * time.Hour
	noOfAttemptsWhenUnreachable = 4
)

// Add creates a new Unreachable Controller and adds it to the Manager with default RBAC. The Manager will set fields on the
// Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return AddToManager(mgr, NewReconciler(mgr))
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager) reconcile.Reconciler {
	r := &ReconcileRemoteMachineSet{
		Client: controllerutils.NewClientWithMetricsOrDie(mgr, controllerName),
		scheme: mgr.GetScheme(),
		logger: log.WithField("controller", controllerName),
	}
	r.remoteClusterAPIClientBuilder = func(cd *hivev1.ClusterDeployment) remoteclient.Builder {
		return remoteclient.NewBuilder(r.Client, cd, controllerName)
	}
	return r
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("unreachable-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: controllerutils.GetConcurrentReconciles()})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterDeployment
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileRemoteMachineSet{}

// ReconcileRemoteMachineSet reconciles the MachineSets generated from a ClusterDeployment object
type ReconcileRemoteMachineSet struct {
	client.Client
	scheme *runtime.Scheme

	logger log.FieldLogger

	// remoteClusterAPIClientBuilder is a function pointer to the function that gets a builder for building a client
	// for the remote cluster's API server
	remoteClusterAPIClientBuilder func(cd *hivev1.ClusterDeployment) remoteclient.Builder
}

// Reconcile checks if we can establish an API client connection to the remote cluster and maintains the unreachable condition as a result.
func (r *ReconcileRemoteMachineSet) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	cdLog := log.WithFields(log.Fields{
		"clusterDeployment": request.Name,
		"namespace":         request.Namespace,
		"controller":        controllerName,
	})

	// For logging, we need to see when the reconciliation loop starts and ends.
	cdLog.Info("reconciling cluster deployment")
	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(controllerName).Observe(dur.Seconds())
		cdLog.WithField("elapsed", dur).Info("reconcile complete")
	}()

	cd := &hivev1.ClusterDeployment{}
	err := r.Get(context.TODO(), request.NamespacedName, cd)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}

		// Error reading the object - requeue the request
		log.WithError(err).Error("error looking up cluster deployment")
		return reconcile.Result{}, err
	}

	// If the clusterdeployment is deleted, do not reconcile.
	if cd.DeletionTimestamp != nil {
		cdLog.Debug("cluster has deletion timestamp")
		return reconcile.Result{}, nil
	}

	if !cd.Spec.Installed {
		cdLog.Debug("cluster installation is not complete")
		return reconcile.Result{}, nil
	}

	if cd.Spec.ClusterMetadata == nil {
		cdLog.Error("installed cluster with no cluster metadata")
		return reconcile.Result{}, nil
	}

	// Check if we're due for rechecking cluster's connectivity
	cond := controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.UnreachableCondition)
	if cond != nil {
		if !isTimeForConnectivityCheck(cond) {
			cdLog.WithFields(log.Fields{
				"sinceLastTransition": time.Since(cond.LastTransitionTime.Time),
				"sinceLastProbe":      time.Since(cond.LastProbeTime.Time),
			}).Debug("skipping unreachable check")
			return reconcile.Result{}, nil
		}
	}

	cdLog.Info("checking if cluster is reachable")
	remoteClientBuilder := r.remoteClusterAPIClientBuilder(cd)
	hasOverride := cd.Spec.ControlPlaneConfig.APIURLOverride != ""
	var (
		unreachableError          error
		activeAPIURLOverrideError error
	)
	if _, primaryErr := remoteClientBuilder.UsePrimaryAPIURL().Build(); primaryErr != nil {
		if hasOverride {
			cdLog.WithError(primaryErr).Info("unable to create remote API client using API URL override")
			activeAPIURLOverrideError = primaryErr
			if _, secondaryErr := remoteClientBuilder.UseSecondaryAPIURL().Build(); secondaryErr != nil {
				cdLog.WithError(secondaryErr).Warn("unable to create remote API client with either the initial API URL or the API URL override, marking cluster unreachable")
				unreachableError = utilerrors.NewAggregate([]error{primaryErr, secondaryErr})
			}
		} else {
			cdLog.WithError(primaryErr).Warn("unable to create remote API client, marking cluster unreachable")
			unreachableError = primaryErr
		}
	}
	unreachableChanged := setUnreachableCond(cd, unreachableError)
	activeAPIURLOverrideChanged := setActiveAPIURLOverrideCond(cd, hasOverride, activeAPIURLOverrideError)

	if !unreachableChanged && !activeAPIURLOverrideChanged {
		return reconcile.Result{}, nil
	}

	if hasOverride {
		if activeAPIURLOverrideError == nil {
			cdLog.Info("cluster is reachable via API URL override")
		} else if unreachableError == nil {
			cdLog.Info("cluster is reachable via initial API URL")
		}
	} else {
		if unreachableError == nil {
			cdLog.Info("cluster is reachable")
		}
	}

	err = r.Status().Update(context.TODO(), cd)
	if err != nil {
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error updating cluster deployment with unreachable condition")
	}
	return reconcile.Result{}, err
}

func setUnreachableCond(cd *hivev1.ClusterDeployment, connectionError error) (condsChanged bool) {
	status := corev1.ConditionFalse
	reason := "ClusterReachable"
	message := "cluster is reachable"
	updateCheck := controllerutils.UpdateConditionNever
	if connectionError != nil {
		status = corev1.ConditionTrue
		reason = "ErrorConnectingToCluster"
		message = connectionError.Error()
		updateCheck = controllerutils.UpdateConditionIfReasonOrMessageChange
	}
	cd.Status.Conditions, condsChanged = controllerutils.SetClusterDeploymentConditionWithChangeCheck(
		cd.Status.Conditions,
		hivev1.UnreachableCondition,
		status,
		reason,
		message,
		updateCheck,
	)
	return
}

func setActiveAPIURLOverrideCond(cd *hivev1.ClusterDeployment, hasOverride bool, connectionError error) (condsChanged bool) {
	if !hasOverride {
		return
	}
	if existingCond := controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.ActiveAPIURLOverrideCondition); existingCond == nil {
		// This adds a dummy ActiveAPIURLOverride condition that will be updated when setting the condition later.
		// We want an explicit ActiveAPIURLOverride condition even when the condition is false so that the user
		// can see the details of the connection error.
		cd.Status.Conditions = append(cd.Status.Conditions, hivev1.ClusterDeploymentCondition{Type: hivev1.ActiveAPIURLOverrideCondition})
	}
	status := corev1.ConditionTrue
	reason := "ClusterReachable"
	message := "cluster is reachable"
	updateCheck := controllerutils.UpdateConditionNever
	if connectionError != nil {
		status = corev1.ConditionFalse
		reason = "ErrorConnectingToCluster"
		message = connectionError.Error()
		updateCheck = controllerutils.UpdateConditionIfReasonOrMessageChange
	}
	cd.Status.Conditions, condsChanged = controllerutils.SetClusterDeploymentConditionWithChangeCheck(
		cd.Status.Conditions,
		hivev1.ActiveAPIURLOverrideCondition,
		status,
		reason,
		message,
		updateCheck,
	)
	return
}

// isTimeForConnectivityCheck returns true as per the condition to check if cluster is reachable/unreachable
// else it returns false
func isTimeForConnectivityCheck(condition *hivev1.ClusterDeploymentCondition) bool {
	sinceLastTransition := time.Since(condition.LastTransitionTime.Time)
	sinceLastProbe := time.Since(condition.LastProbeTime.Time)
	if sinceLastProbe < (sinceLastTransition/noOfAttemptsWhenUnreachable) && sinceLastProbe < maxUnreachableDuration {
		return false
	}
	return true
}
