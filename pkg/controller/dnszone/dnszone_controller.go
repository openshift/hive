package dnszone

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/miekg/dns"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	awsclient "github.com/openshift/hive/pkg/awsclient"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	gcpclient "github.com/openshift/hive/pkg/gcpclient"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	controllerName                  = "dnszone"
	zoneResyncDuration              = 2 * time.Hour
	domainAvailabilityCheckInterval = 30 * time.Second
	dnsClientTimeout                = 30 * time.Second
	resolverConfigFile              = "/etc/resolv.conf"
	zoneCheckDNSServersEnvVar       = "ZONE_CHECK_DNS_SERVERS"
)

// Add creates a new DNSZone Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileDNSZone{
		Client:    controllerutils.NewClientWithMetricsOrDie(mgr, controllerName),
		scheme:    mgr.GetScheme(),
		logger:    log.WithField("controller", controllerName),
		soaLookup: lookupSOARecord,
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

	// soaLookup is a function that looks up a zone's SOA record
	soaLookup func(string, log.FieldLogger) (bool, error)
}

// Reconcile reads that state of the cluster for a DNSZone object and makes changes based on the state read
// and what is in the DNSZone.Spec
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
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		dnsLog.WithError(err).Error("Error fetching dnszone object")
		return reconcile.Result{}, err
	}

	// See if we need to sync. This is what rate limits our dns provider API usage, but allows for immediate syncing
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

	actuator, err := r.getActuator(desiredState, dnsLog)
	if err != nil {
		// Handle an edge case here where if the DNSZone has been deleted, it has its finalizer, the actuator couldn't be
		// created (presumably because creds secret is absent), and our namespace is terminated, we know we've entered a bad state
		// where we must give up and remove the finalizer. A followup fix should prevent this problem from
		// happening but we need to cleanup stuck DNSZones regardless.
		if desiredState.DeletionTimestamp != nil && controllerutils.HasFinalizer(desiredState, hivev1.FinalizerDNSZone) {
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

				// This returns whether there was an error or not.
				// This is desired so that on success, this dnszone is NOT requeued. Falling through
				// would cause a requeue because of the actuator erroring.
				return reconcile.Result{}, err
			}
		}

		dnsLog.WithError(err).Error("error instantiating actuator")
		return reconcile.Result{}, err
	}

	// Actually reconcile desired state with current state
	dnsLog.WithFields(log.Fields{
		"delta":              delta,
		"currentGeneration":  desiredState.Generation,
		"lastSyncGeneration": desiredState.Status.LastSyncGeneration,
	}).Info("Syncing DNS Zone")
	result, err := r.reconcileDNSProvider(actuator, desiredState)
	if err != nil {
		dnsLog.WithError(err).Error("Encountered error while attempting to reconcile")
	}
	return result, err
}

// ReconcileDNSProvider attempts to make the current state reflect the desired state. It does this idempotently.
func (r *ReconcileDNSZone) reconcileDNSProvider(actuator Actuator, dnsZone *hivev1.DNSZone) (reconcile.Result, error) {
	r.logger.Debug("Retrieving current state")
	err := actuator.Refresh()
	if err != nil {
		r.logger.WithError(err).Error("Failed to retrieve hosted zone and corresponding tags")
		return reconcile.Result{}, err
	}

	zoneFound, err := actuator.Exists()
	if err != nil {
		r.logger.WithError(err).Error("Failed while checking if hosted zone exists")
		return reconcile.Result{}, err
	}

	if dnsZone.DeletionTimestamp != nil {
		if zoneFound {
			r.logger.Debug("DNSZone resource is deleted, deleting hosted zone")
			err := actuator.Delete()
			if err != nil {
				return reconcile.Result{}, err
			}
		}
		if controllerutils.HasFinalizer(dnsZone, hivev1.FinalizerDNSZone) {
			// Remove the finalizer from the DNSZone. It will be persisted when we persist status
			r.logger.Info("Removing DNSZone finalizer")
			controllerutils.DeleteFinalizer(dnsZone, hivev1.FinalizerDNSZone)
			err := r.Client.Update(context.TODO(), dnsZone)
			if err != nil {
				r.logger.WithError(err).Log(controllerutils.LogLevel(err), "Failed to remove DNSZone finalizer")
			}
		}
		return reconcile.Result{}, err
	}
	if !controllerutils.HasFinalizer(dnsZone, hivev1.FinalizerDNSZone) {
		r.logger.Info("DNSZone does not have a finalizer. Adding one.")
		controllerutils.AddFinalizer(dnsZone, hivev1.FinalizerDNSZone)
		err := r.Client.Update(context.TODO(), dnsZone)
		if err != nil {
			r.logger.WithError(err).Log(controllerutils.LogLevel(err), "Failed to add finalizer to DNSZone")
		}
		return reconcile.Result{}, err
	}
	if !zoneFound {
		r.logger.Info("No corresponding hosted zone found on cloud provider, creating one")
		err := actuator.Create()
		if err != nil {
			r.logger.WithError(err).Error("Failed to create hosted zone")
			return reconcile.Result{}, err
		}
	} else {
		r.logger.Info("Existing hosted zone found. Syncing with DNSZone resource")
		err := actuator.UpdateMetadata()
		if err != nil {
			r.logger.WithError(err).Error("failed to sync tags for hosted zone")
			return reconcile.Result{}, err
		}
	}

	nameServers, err := actuator.GetNameServers()
	if err != nil {
		r.logger.WithError(err).Error("Failed to get hosted zone name servers")
		return reconcile.Result{}, err
	}

	isZoneSOAAvailable, err := r.soaLookup(dnsZone.Spec.Zone, r.logger)
	if err != nil {
		r.logger.WithError(err).Error("error looking up SOA record for zone")
	}

	reconcileResult := reconcile.Result{}
	if !isZoneSOAAvailable {
		r.logger.Info("SOA record for DNS zone not available")
		reconcileResult.RequeueAfter = domainAvailabilityCheckInterval
	}

	r.logger.Debug("Letting the actuator modify the DNSZone status before sending it to kube.")
	err = actuator.ModifyStatus()
	if err != nil {
		r.logger.WithError(err).Error("error modifying DNSZone status")
		reconcileResult.RequeueAfter = domainAvailabilityCheckInterval
		return reconcileResult, err
	}

	return reconcileResult, r.updateStatus(nameServers, isZoneSOAAvailable, dnsZone)
}

func shouldSync(desiredState *hivev1.DNSZone) (bool, time.Duration) {
	if desiredState.DeletionTimestamp != nil && !controllerutils.HasFinalizer(desiredState, hivev1.FinalizerDNSZone) {
		return false, 0 // No finalizer means our cleanup has been completed. There's nothing left to do.
	}

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

func (r *ReconcileDNSZone) getActuator(dnsZone *hivev1.DNSZone, dnsLog log.FieldLogger) (Actuator, error) {
	if dnsZone.Spec.AWS != nil {
		secret := &corev1.Secret{}
		err := r.Get(context.TODO(),
			types.NamespacedName{
				Name:      dnsZone.Spec.AWS.CredentialsSecretRef.Name,
				Namespace: dnsZone.Namespace,
			},
			secret)
		if err != nil {
			return nil, err
		}

		return NewAWSActuator(dnsLog, secret, dnsZone, awsclient.NewClientFromSecret)
	}

	if dnsZone.Spec.GCP != nil {
		secret := &corev1.Secret{}
		err := r.Get(context.TODO(),
			types.NamespacedName{
				Name:      dnsZone.Spec.GCP.CredentialsSecretRef.Name,
				Namespace: dnsZone.Namespace,
			},
			secret)
		if err != nil {
			return nil, err
		}

		return NewGCPActuator(dnsLog, secret, dnsZone, gcpclient.NewClientFromSecret)
	}

	return nil, errors.New("unable to determine which actuator to use")
}

func (r *ReconcileDNSZone) updateStatus(nameServers []string, isSOAAvailable bool, dnsZone *hivev1.DNSZone) error {
	orig := dnsZone.DeepCopy()
	r.logger.Debug("Updating DNSZone status")

	dnsZone.Status.NameServers = nameServers

	var availableStatus corev1.ConditionStatus
	var availableReason, availableMessage string
	if isSOAAvailable {
		// We need to keep track of the last time we synced to rate limit our dns provider calls.
		tmpTime := metav1.Now()
		dnsZone.Status.LastSyncTimestamp = &tmpTime

		availableStatus = corev1.ConditionTrue
		availableReason = "ZoneAvailable"
		availableMessage = "DNS SOA record for zone is reachable"
	} else {
		availableStatus = corev1.ConditionFalse
		availableReason = "ZoneUnavailable"
		availableMessage = "DNS SOA record for zone is not reachable"
	}
	dnsZone.Status.LastSyncGeneration = dnsZone.ObjectMeta.Generation
	dnsZone.Status.Conditions = controllerutils.SetDNSZoneCondition(
		dnsZone.Status.Conditions,
		hivev1.ZoneAvailableDNSZoneCondition,
		availableStatus,
		availableReason,
		availableMessage,
		controllerutils.UpdateConditionNever)

	if !reflect.DeepEqual(orig.Status, dnsZone.Status) {
		err := r.Client.Status().Update(context.TODO(), dnsZone)
		if err != nil {
			r.logger.WithError(err).Log(controllerutils.LogLevel(err), "Cannot update DNSZone status")
		}
		return err
	}
	return nil
}

func lookupSOARecord(zone string, logger log.FieldLogger) (bool, error) {
	// TODO: determine if there's a better way to obtain resolver endpoints
	clientConfig, _ := dns.ClientConfigFromFile(resolverConfigFile)
	client := dns.Client{Timeout: dnsClientTimeout}

	dnsServers := []string{}
	serversFromEnv := os.Getenv(zoneCheckDNSServersEnvVar)
	if len(serversFromEnv) > 0 {
		dnsServers = strings.Split(serversFromEnv, ",")
		// Add port to servers with unspecified port
		for i := range dnsServers {
			if !strings.Contains(dnsServers[i], ":") {
				dnsServers[i] = dnsServers[i] + ":53"
			}
		}
	} else {
		for _, s := range clientConfig.Servers {
			dnsServers = append(dnsServers, fmt.Sprintf("%s:%s", s, clientConfig.Port))
		}
	}
	logger.WithField("servers", dnsServers).Info("looking up domain SOA record")

	m := &dns.Msg{}
	m.SetQuestion(zone+".", dns.TypeSOA)
	for _, s := range dnsServers {
		in, rtt, err := client.Exchange(m, s)
		if err != nil {
			logger.WithError(err).WithField("server", s).Info("query for SOA record failed")
			continue
		}
		log.WithField("server", s).Infof("SOA query duration: %v", rtt)
		if len(in.Answer) > 0 {
			for _, rr := range in.Answer {
				soa, ok := rr.(*dns.SOA)
				if !ok {
					logger.Info("Record returned is not an SOA record: %#v", rr)
					continue
				}
				if soa.Hdr.Name != controllerutils.Dotted(zone) {
					logger.WithField("zone", soa.Hdr.Name).Info("SOA record returned but it does not match the lookup zone")
					return false, nil
				}
				logger.WithField("zone", soa.Hdr.Name).Info("SOA record returned, zone is reachable")
				return true, nil
			}
		}
		logger.WithField("server", s).Info("no answer for SOA record returned")
		return false, nil
	}
	return false, nil
}
