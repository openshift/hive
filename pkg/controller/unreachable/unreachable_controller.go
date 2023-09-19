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
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/client-go/util/workqueue"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/remoteclient"
)

const (
	ControllerName = hivev1.UnreachableControllerName

	maxUnreachableDuration = 2 * time.Hour
)

// clusterDeploymentUnreachableConditions are the cluster deployment conditions controlled by
// Unreachable controller
var clusterDeploymentUnreachableConditions = []hivev1.ClusterDeploymentConditionType{
	hivev1.ActiveAPIURLOverrideCondition,
	hivev1.UnreachableCondition,
}

// Add creates a new Unreachable Controller and adds it to the Manager with default RBAC. The Manager will set fields on the
// Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	logger := log.WithField("controller", ControllerName)
	concurrentReconciles, clientRateLimiter, queueRateLimiter, err := controllerutils.GetControllerConfig(mgr.GetClient(), ControllerName)
	if err != nil {
		logger.WithError(err).Error("could not get controller configurations")
		return err
	}
	return AddToManager(mgr, NewReconciler(mgr, clientRateLimiter), concurrentReconciles, queueRateLimiter)
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager, rateLimiter flowcontrol.RateLimiter) reconcile.Reconciler {
	r := &ReconcileRemoteMachineSet{
		Client: controllerutils.NewClientWithMetricsOrDie(mgr, ControllerName, &rateLimiter),
		scheme: mgr.GetScheme(),
		logger: log.WithField("controller", ControllerName),
	}
	r.remoteClusterAPIClientBuilder = func(cd *hivev1.ClusterDeployment) remoteclient.Builder {
		return remoteclient.NewBuilder(r.Client, cd, ControllerName)
	}
	return r
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler, concurrentReconciles int, rateLimiter workqueue.RateLimiter) error {
	// Create a new controller
	c, err := controller.New("unreachable-controller", mgr, controller.Options{
		Reconciler:              controllerutils.NewDelayingReconciler(r, log.WithField("controller", ControllerName)),
		MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter:             rateLimiter,
	})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterDeployment
	err = c.Watch(source.Kind(mgr.GetCache(), &hivev1.ClusterDeployment{}), &handler.EnqueueRequestForObject{})
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
func (r *ReconcileRemoteMachineSet) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	cdLog := controllerutils.BuildControllerLogger(ControllerName, "clusterDeployment", request.NamespacedName)
	cdLog.Info("reconciling cluster deployment")
	recobsrv := hivemetrics.NewReconcileObserver(ControllerName, cdLog)
	defer recobsrv.ObserveControllerReconcileTime()

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
	cdLog = controllerutils.AddLogFields(controllerutils.MetaObjectLogTagger{Object: cd}, cdLog)

	if paused, err := strconv.ParseBool(cd.Annotations[constants.ReconcilePauseAnnotation]); err == nil && paused {
		cdLog.Info("skipping reconcile due to ClusterDeployment pause annotation")
		return reconcile.Result{}, nil
	}

	// Initialize cluster deployment conditions if not present
	newConditions, changed := controllerutils.InitializeClusterDeploymentConditions(cd.Status.Conditions, clusterDeploymentUnreachableConditions)
	if changed {
		cd.Status.Conditions = newConditions
		cdLog.Info("initializing unreachable controller conditions")
		if err := r.Status().Update(context.TODO(), cd); err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "failed to update cluster deployment status")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
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

	// Check whether, prior to this reconciliation, the remote cluster was considered unreachable. Also, get the
	// last time that the unreachable check was performed.
	wasUnreachable, lastCheck := remoteclient.Unreachable(cd)
	// Check whether, prior to this reconciliation, connectivity to the remote cluster was using the preferred API URL.
	wasPrimaryActive := remoteclient.IsPrimaryURLActive(cd)
	// Determine the amount of time to wait before rechecking connectivity to a reachable remote cluster.
	connectivityRecheckDelay := maxUnreachableDuration - time.Since(lastCheck)
	// Determine if it is time to recheck connectivity.
	connectivityRecheckNeeded := wasUnreachable || connectivityRecheckDelay <= 0*time.Second

	// If the remote cluster was reachable via the preferred API URL and it is not time to recheck connectivity,
	// then requeue the ClusterDeployment to be synced again when it is time for the next connectivity check.
	// If connectivity was not being made via the preferred API URL, the reconciliation needs to proceed in order to
	// check connectivity to the preferred API URL.
	if !connectivityRecheckNeeded && wasPrimaryActive {
		cdLog.WithField("delay", connectivityRecheckDelay).Debug("waiting to check connectivity")
		return reconcile.Result{RequeueAfter: connectivityRecheckDelay}, nil
	}

	cdLog.Info("checking if cluster is reachable")
	remoteClientBuilder := r.remoteClusterAPIClientBuilder(cd)
	var unreachableError error
	updateUnreachable := true
	var primaryErr error
	// Attempt to connect to the remote cluster using the preferred API URL.
	_, primaryErr = remoteClientBuilder.UsePrimaryAPIURL().Build()
	if primaryErr != nil {
		// If the remote cluster is not accessible via the preferred API URL, check if there is a fallback API URL to use.
		if hasOverride(cd) {
			cdLog.WithError(primaryErr).Info("unable to create remote API client using API URL override")
			// If a connectivity recheck is needed or the remote cluster was reachable via the preferred API URL prior
			// to this reconciliation, then attempt to connect to the remote cluster using the fallback API URL.
			// Even when the controller continues to reconcile a ClusterDeployment waiting for the preferred API URL to
			// become accessible, the controller should not recheck connectivity via the fallback API URL more often
			// than once every 2 hours.
			if connectivityRecheckNeeded || wasPrimaryActive {
				if _, secondaryErr := remoteClientBuilder.UseSecondaryAPIURL().Build(); secondaryErr != nil {
					cdLog.WithError(secondaryErr).Warn("unable to create remote API client with either the initial API URL or the API URL override, marking cluster unreachable")
					unreachableError = utilerrors.NewAggregate([]error{primaryErr, secondaryErr})
				}
			} else {
				updateUnreachable = false
			}
		} else {
			cdLog.WithError(primaryErr).Warn("unable to create remote API client, marking cluster unreachable")
			unreachableError = primaryErr
		}
	}

	// Update conditions to reflect the current state of connectivity to the remote cluster.
	unreachableChanged := false
	if updateUnreachable {
		unreachableChanged = remoteclient.SetUnreachableCondition(cd, unreachableError)
	}
	overrideChanged := setActiveAPIURLOverrideCond(cd, primaryErr)

	// Determine when to requeue the ClusterDeployment. If there is no connectivity to the remote cluster via the
	// preferred API URL, then requeue the ClusterDeployment using the backoff. If there is connectivity via the
	// preferred API URL, then requeue the ClusterDeployment to sync again in 2 hours for the next connectivity re-check.
	result := reconcile.Result{Requeue: primaryErr != nil}
	if !result.Requeue {
		result.RequeueAfter = maxUnreachableDuration
	}

	// If none of the conditions have changed, stop the reconciliation now without updating the ClusterDeployment.
	if !unreachableChanged && !overrideChanged {
		return result, nil
	}

	// Log an info entry when the remote cluster becomes reachable.
	transitionedToReachable := wasUnreachable && unreachableError == nil
	isPrimaryActive := primaryErr == nil
	transitionedToPrimaryActive := !wasPrimaryActive && isPrimaryActive
	if transitionedToReachable || transitionedToPrimaryActive {
		switch {
		case !hasOverride(cd):
			cdLog.Info("cluster is reachable")
		case isPrimaryActive:
			cdLog.Info("cluster is reachable via API URL override")
		default:
			cdLog.Info("cluster is reachable via initial API URL")
		}
	}

	err = r.Status().Update(context.TODO(), cd)
	if err != nil {
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error updating cluster deployment with unreachable condition")
	}
	return result, err
}

func setActiveAPIURLOverrideCond(cd *hivev1.ClusterDeployment, connectionError error) (condsChanged bool) {
	if !hasOverride(cd) {
		return
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

func hasOverride(cd *hivev1.ClusterDeployment) bool {
	return cd.Spec.ControlPlaneConfig.APIURLOverride != ""
}
