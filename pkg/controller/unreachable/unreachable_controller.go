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
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	kapi "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	controllerName = "unreachable"

	adminKubeConfigKey          = "kubeconfig"
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
	return &ReconcileRemoteMachineSet{
		Client:                        controllerutils.NewClientWithMetricsOrDie(mgr, controllerName),
		scheme:                        mgr.GetScheme(),
		logger:                        log.WithField("controller", controllerName),
		remoteClusterAPIClientBuilder: controllerutils.BuildClusterAPIClientFromKubeconfig,
	}
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

	// remoteClusterAPIClientBuilder is a function pointer to the function that builds a client for the
	// remote cluster's cluster-api
	remoteClusterAPIClientBuilder func(string, string) (client.Client, error)
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
		if errors.IsNotFound(err) {
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

	secretName := cd.Status.AdminKubeconfigSecret.Name
	secretData, err := r.loadSecretData(secretName, cd.Namespace, adminKubeConfigKey)
	if err != nil {
		cdLog.WithError(err).Error("unable to load admin kubeconfig")
		return reconcile.Result{}, err
	}

	cdLog.Info("checking if cluster is reachable")
	_, err = r.remoteClusterAPIClientBuilder(secretData, controllerName)
	if err != nil {
		cdLog.Warn("unable to create remote API client, marking cluster unreachable")
		cd.Status.Conditions = controllerutils.SetClusterDeploymentCondition(cd.Status.Conditions, hivev1.UnreachableCondition,
			corev1.ConditionTrue, "ErrorConnectingToCluster", err.Error(), controllerutils.UpdateConditionAlways)
		err := r.Status().Update(context.TODO(), cd)
		if err != nil {
			cdLog.WithError(err).Error("error updating cluster deployment with unreachable condition (= true)")
			return reconcile.Result{}, err
		}
		cdLog.Info("cluster is unreachable")
		return reconcile.Result{}, nil
	}

	// check if cluster has condition set to true
	cond = controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.UnreachableCondition)
	if cond != nil {
		if cond.Status == corev1.ConditionTrue {
			cdLog.Infof("cluster is reachable now, %s condition will be set to %s", hivev1.UnreachableCondition, corev1.ConditionFalse)
		}
	} else {
		cdLog.Info("Cluster is reachable and unreachable condition does not exist")
		return reconcile.Result{}, nil
	}

	// If cluster already has "unreachable" condition set is true, set the "unreachable" condition to false
	// as the cluster is reachable now
	cd.Status.Conditions = controllerutils.SetClusterDeploymentCondition(cd.Status.Conditions, hivev1.UnreachableCondition,
		corev1.ConditionFalse, "ClusterReachable", "cluster is reachable", controllerutils.UpdateConditionAlways)
	err = r.Status().Update(context.TODO(), cd)
	if err != nil {
		cdLog.WithError(err).Error("error updating cluster deployment with unreachable condition (= false)")
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileRemoteMachineSet) loadSecretData(secretName, namespace, dataKey string) (string, error) {
	s := &kapi.Secret{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: namespace}, s)
	if err != nil {
		return "", err
	}
	retStr, ok := s.Data[dataKey]
	if !ok {
		return "", fmt.Errorf("secret %s did not contain key %s", secretName, dataKey)
	}
	return string(retStr), nil
}

// isTimeForisTimeForConnectivityCheck returns true as per the condition to check if cluster is reachable/unreachable
// else it returns false
func isTimeForConnectivityCheck(condition *hivev1.ClusterDeploymentCondition) bool {
	sinceLastTransition := time.Since(condition.LastTransitionTime.Time)
	sinceLastProbe := time.Since(condition.LastProbeTime.Time)
	if sinceLastProbe < (sinceLastTransition/noOfAttemptsWhenUnreachable) && sinceLastProbe < maxUnreachableDuration {
		return false
	}
	return true
}
