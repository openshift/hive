package dnszone

import (
	"context"
	"time"

	log "github.com/sirupsen/logrus"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	awsclient "github.com/openshift/hive/pkg/awsclient"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	controllerName     = "dnszone"
	zoneResyncDuration = 2 * time.Hour
)

// Add creates a new DNSZone Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileDNSZone{
		Client:           controllerutils.NewClientWithMetricsOrDie(mgr, controllerName),
		scheme:           mgr.GetScheme(),
		logger:           log.WithField("controller", controllerName),
		awsClientBuilder: awsclient.NewClient,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: controllerutils.GetConcurrentReconciles()})
	if err != nil {
		return err
	}

	// Watch for changes to DNSZone
	err = c.Watch(&source.Kind{Type: &hivev1.DNSZone{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileDNSZone{}

// ReconcileDNSZone reconciles a DNSZone object
type ReconcileDNSZone struct {
	client.Client
	scheme *runtime.Scheme

	logger log.FieldLogger

	// awsClientBuilder is a function pointer to the function that builds the aws client.
	awsClientBuilder func(kClient client.Client, secretName, namespace, region string) (awsclient.Client, error)
}

// NewReconcileDNSZone creates a new reconciler for testing purposes
func NewReconcileDNSZone(client client.Client, scheme *runtime.Scheme, logger log.FieldLogger, awsClientBuilder func(kClient client.Client, secretName, namespace, region string) (awsclient.Client, error)) *ReconcileDNSZone {
	return &ReconcileDNSZone{
		Client:           client,
		scheme:           scheme,
		logger:           logger,
		awsClientBuilder: awsClientBuilder,
	}
}

// SetAWSClientBuilder sets the AWS client builder for testing purposes
func (r *ReconcileDNSZone) SetAWSClientBuilder(awsClientBuilder func(kClient client.Client, secretName, namespace, region string) (awsclient.Client, error)) {
	r.awsClientBuilder = awsClientBuilder
}

// Reconcile reads that state of the cluster for a DNSZone object and makes changes based on the state read
// and what is in the DNSZone.Spec
// Automatically generate RBAC rules to allow the Controller to read and write DNSZones
func (r *ReconcileDNSZone) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	dnsLog := r.logger.WithFields(log.Fields{
		"controller": controllerName,
		"dnszone":    request.Name,
		"namespace":  request.Namespace,
	})

	// For logging, we need to see when the reconciliation loop starts and ends.
	dnsLog.Info("reconciling dns zone")
	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(controllerName).Observe(dur.Seconds())
		dnsLog.WithField("elapsed", dur).Info("reconcile complete")
	}()

	// Fetch the DNSZone object
	desiredState := &hivev1.DNSZone{}
	err := r.Get(context.TODO(), request.NamespacedName, desiredState)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		dnsLog.WithError(err).Error("Error fetching dnszone object")
		return reconcile.Result{}, err
	}

	// Handle an edge case here where if the DNSZone has been deleted, it has it's finalizer, our AWS
	// creds secret is missing, and our namespace is terminated, we know we've entered a bad state
	// where we must give up and remove the finalizer. A followup fix should prevent this problem from
	// happening but we need to cleanup stuck DNSZones regardless.
	if desiredState.DeletionTimestamp != nil && controllerutils.HasFinalizer(desiredState, hivev1.FinalizerDNSZone) {
		if desiredState.Spec.AWS != nil && desiredState.Spec.AWS.AccountSecret.Name != "" {
			secretName := desiredState.Spec.AWS.AccountSecret.Name
			secret := &corev1.Secret{}
			err := r.Client.Get(context.TODO(),
				types.NamespacedName{
					Name:      secretName,
					Namespace: desiredState.Namespace,
				},
				secret)
			if err != nil && errors.IsNotFound(err) {
				// Check if our namespace is deleted, if so we need to give up and remove our finalizer:
				ns := &corev1.Namespace{}
				err = r.Get(context.TODO(), types.NamespacedName{Name: desiredState.Namespace}, ns)
				if err != nil {
					dnsLog.WithError(err).Error("error checking for deletionTimestamp on namespace")
					return reconcile.Result{}, err
				}
				if ns.DeletionTimestamp != nil {
					dnsLog.Warn("detected a namespace deleted before dnszone could be cleaned up, giving up and removing finalizer")
					// Remove the finalizer from the DNSZone. It will be persisted when we persist status
					dnsLog.Debug("Removing DNSZone finalizer")
					controllerutils.DeleteFinalizer(desiredState, hivev1.FinalizerDNSZone)
					err := r.Client.Update(context.TODO(), desiredState)
					if err != nil {
						dnsLog.WithError(err).Log(controllerutils.LogLevel(err), "Failed to remove DNSZone finalizer")
					}
				}
				return reconcile.Result{}, err
			} else if err != nil {
				dnsLog.WithError(err).Error("error loading AWS creds secret")
				return reconcile.Result{}, err
			}
		}
	}

	// See if we need to sync. This is what rate limits our AWS API usage, but allows for immediate syncing
	// on spec changes and deletes.
	shouldSync, delta := shouldSync(desiredState)
	if !shouldSync {
		dnsLog.WithFields(log.Fields{
			"delta":                delta,
			"currentGeneration":    desiredState.Generation,
			"lastSyncedGeneration": desiredState.Status.LastSyncGeneration,
		}).Debug("Sync not needed")

		return reconcile.Result{}, nil
	}

	awsClient, err := r.getAWSClient(desiredState, dnsLog)
	if err != nil {
		dnsLog.WithError(err).Error("Error creating aws client")
		return reconcile.Result{}, err
	}

	zr, err := NewZoneReconciler(
		desiredState,
		r.Client,
		dnsLog,
		awsClient,
		r.scheme,
	)
	if err != nil {
		dnsLog.WithError(err).Error("Error creating zone reconciler")
		return reconcile.Result{}, err
	}

	// Actually reconcile desired state with current state
	dnsLog.WithFields(log.Fields{
		"delta":              delta,
		"currentGeneration":  desiredState.Generation,
		"lastSyncGeneration": desiredState.Status.LastSyncGeneration,
	}).Info("Syncing DNS Zone")
	result, err := zr.Reconcile()
	if err != nil {
		dnsLog.WithError(err).Error("Encountered error while attempting to reconcile")
	}
	return result, err
}

func shouldSync(desiredState *hivev1.DNSZone) (bool, time.Duration) {
	if desiredState.DeletionTimestamp != nil {
		return true, 0 // We're in a deleting state, sync now.
	}

	if desiredState.Status.LastSyncTimestamp == nil {
		return true, 0 // We've never sync'd before, sync now.
	}

	if desiredState.Status.LastSyncGeneration != desiredState.Generation {
		return true, 0 // Spec has changed since last sync, sync now.
	}

	if desiredState.Spec.LinkToParentDomain {
		availableCondition := controllerutils.FindDNSZoneCondition(desiredState.Status.Conditions, hivev1.ZoneAvailableDNSZoneCondition)
		if availableCondition == nil || availableCondition.Status == corev1.ConditionFalse {
			return true, 0
		} // If waiting to link to parent, sync now to check domain
	}

	delta := time.Now().Sub(desiredState.Status.LastSyncTimestamp.Time)
	if delta >= zoneResyncDuration {
		// We haven't sync'd in over zoneResyncDuration time, sync now.
		return true, delta
	}

	// We didn't meet any of the criteria above, so we should not sync.
	return false, delta
}

// getAWSClient generates an awsclient
func (r *ReconcileDNSZone) getAWSClient(dnsZone *hivev1.DNSZone, dnsLog log.FieldLogger) (awsclient.Client, error) {
	// This allows for using host profiles for AWS auth.
	var secretName, regionName string

	if dnsZone != nil && dnsZone.Spec.AWS != nil {
		secretName = dnsZone.Spec.AWS.AccountSecret.Name
		regionName = dnsZone.Spec.AWS.Region
	}

	awsClient, err := r.awsClientBuilder(r.Client, secretName, dnsZone.Namespace, regionName)
	if err != nil {
		dnsLog.WithError(err).Error("Error creating AWSClient")
		return nil, err
	}

	return awsClient, nil
}
