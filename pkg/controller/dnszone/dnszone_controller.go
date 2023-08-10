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
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	awsclient "github.com/openshift/hive/pkg/awsclient"
	"github.com/openshift/hive/pkg/azureclient"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	gcpclient "github.com/openshift/hive/pkg/gcpclient"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	ControllerName                  = hivev1.DNSZoneControllerName
	zoneResyncDuration              = 2 * time.Hour
	domainAvailabilityCheckInterval = 30 * time.Second
	dnsClientTimeout                = 30 * time.Second
	resolverConfigFile              = "/etc/resolv.conf"
	zoneCheckDNSServersEnvVar       = "ZONE_CHECK_DNS_SERVERS"
	accessDeniedReason              = "AccessDenied"
	accessGrantedReason             = "AccessGranted"
	authenticationFailedReason      = "AuthenticationFailed"
	authenticationSucceededReason   = "AuthenticationSucceeded"
	apiOptInRequiredReason          = "RequiredAPIsNotEnabled"
	apiOptInNotRequiredReason       = "RequiredAPIsEnabled"
	dnsCloudErrorReason             = "CloudError"
	dnsNoErrorReason                = "NoError"
)

var (
	metricDNSZonesDeleted = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hive_dnszones_deleted_total",
		Help: "Counter incremented every time we observe a deleted dnszone. Force will be true if we were unable to properly cleanup the zone.",
	},
		[]string{"force"},
	)
)

func init() {
	metrics.Registry.MustRegister(metricDNSZonesDeleted)
}

// Add creates a new DNSZone Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	logger := log.WithField("controller", ControllerName)
	concurrentReconciles, clientRateLimiter, queueRateLimiter, err := controllerutils.GetControllerConfig(mgr.GetClient(), ControllerName)
	if err != nil {
		logger.WithError(err).Error("could not get controller configurations")
		return err
	}
	return add(mgr, newReconciler(mgr, clientRateLimiter), concurrentReconciles, queueRateLimiter)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, rateLimiter flowcontrol.RateLimiter) *ReconcileDNSZone {
	return &ReconcileDNSZone{
		Client:    controllerutils.NewClientWithMetricsOrDie(mgr, ControllerName, &rateLimiter),
		logger:    log.WithField("controller", ControllerName),
		soaLookup: lookupSOARecord,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r *ReconcileDNSZone, concurrentReconciles int, rateLimiter workqueue.RateLimiter) error {
	// Create a new controller
	c, err := controller.New(ControllerName.String(), mgr, controller.Options{
		Reconciler:              controllerutils.NewDelayingReconciler(r, r.logger),
		MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter:             rateLimiter,
	})
	if err != nil {
		return err
	}

	// Watch for changes to DNSZone
	if err := c.Watch(source.Kind(mgr.GetCache(), &hivev1.DNSZone{}), controllerutils.NewRateLimitedUpdateEventHandler(&handler.EnqueueRequestForObject{}, IsErrorUpdateEvent)); err != nil {
		return err
	}

	// Watch for changes to ClusterDeployment
	if err := c.Watch(
		source.Kind(mgr.GetCache(), &hivev1.ClusterDeployment{}),
		controllerutils.NewRateLimitedUpdateEventHandler(
			controllerutils.EnqueueDNSZonesOwnedByClusterDeployment(r, r.logger),
			controllerutils.IsClusterDeploymentErrorUpdateEvent,
		),
	); err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileDNSZone{}

// ReconcileDNSZone reconciles a DNSZone object
type ReconcileDNSZone struct {
	client.Client

	logger log.FieldLogger

	// soaLookup is a function that looks up a zone's SOA record
	soaLookup func(string, log.FieldLogger) (bool, error)
}

// Reconcile reads that state of the cluster for a DNSZone object and makes changes based on the state read
// and what is in the DNSZone.Spec
func (r *ReconcileDNSZone) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	dnsLog := controllerutils.BuildControllerLogger(ControllerName, "dnsZone", request.NamespacedName)
	dnsLog.Info("reconciling dns zone")
	recobsrv := hivemetrics.NewReconcileObserver(ControllerName, dnsLog)
	defer recobsrv.ObserveControllerReconcileTime()

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
	dnsLog = controllerutils.AddLogFields(controllerutils.MetaObjectLogTagger{Object: desiredState}, dnsLog)

	// NOTE: Can race with call to same in dnsendpoint controller
	if result, err := controllerutils.ReconcileDNSZoneForRelocation(r.Client, dnsLog, desiredState, hivev1.FinalizerDNSZone); err != nil {
		var changed bool
		desiredState.Status.Conditions, changed = controllerutils.SetDNSZoneConditionWithChangeCheck(
			desiredState.Status.Conditions,
			hivev1.GenericDNSErrorsCondition,
			corev1.ConditionTrue,
			"RelocationError",
			controllerutils.ErrorScrub(err),
			controllerutils.UpdateConditionIfReasonOrMessageChange)
		if changed {
			if err := r.Status().Update(context.Background(), desiredState); err != nil {
				dnsLog.WithError(err).Log(controllerutils.LogLevel(err), "failed to update dnszone status")
			}
		}
		return reconcile.Result{}, err
	} else if result != nil {
		return *result, nil
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

	actuator, actErr := r.getActuator(desiredState, dnsLog)
	if actErr != nil {
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
				} else {
					metricDNSZonesDeleted.WithLabelValues("true").Inc()
				}

				// This returns whether there was an error or not.
				// This is desired so that on success, this dnszone is NOT requeued. Falling through
				// would cause a requeue because of the actuator erroring.
				return reconcile.Result{}, actErr
			}
		}

		dnsLog.WithError(actErr).Error("error instantiating actuator")
		var changed bool
		desiredState.Status.Conditions, changed = controllerutils.SetDNSZoneConditionWithChangeCheck(
			desiredState.Status.Conditions,
			hivev1.GenericDNSErrorsCondition,
			corev1.ConditionTrue,
			"ActuatorNotInitialized",
			"error instantiating actuator: "+controllerutils.ErrorScrub(actErr),
			controllerutils.UpdateConditionIfReasonOrMessageChange)
		if changed {
			if err := r.Status().Update(context.Background(), desiredState); err != nil {
				dnsLog.WithError(err).Log(controllerutils.LogLevel(err), "failed to update dnszone status")
			}
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, actErr
	}

	// Actually reconcile desired state with current state
	dnsLog.WithFields(log.Fields{
		"delta":              delta,
		"currentGeneration":  desiredState.Generation,
		"lastSyncGeneration": desiredState.Status.LastSyncGeneration,
	}).Info("Syncing DNS Zone")
	result, err := r.reconcileDNSProvider(actuator, desiredState, dnsLog)
	conditionsChanged := actuator.SetConditionsForError(err)

	if conditionsChanged {
		if err := r.Status().Update(context.Background(), desiredState); err != nil {
			return reconcile.Result{}, err
		}
	}
	if err != nil {
		dnsLog.WithError(err).Log(controllerutils.LogLevel(err), "Encountered error while attempting to reconcile")
	}
	return result, err
}

// ReconcileDNSProvider attempts to make the current state reflect the desired state. It does this idempotently.
func (r *ReconcileDNSZone) reconcileDNSProvider(actuator Actuator, dnsZone *hivev1.DNSZone, logger log.FieldLogger) (reconcile.Result, error) {

	// If deleted and set to PreserveOnDelete, we need to skip any
	// attempts to use the cluster's cloud credentials as they may no longer be good.
	if dnsZone.DeletionTimestamp != nil && dnsZone.Spec.PreserveOnDelete {
		logger.Info("DNSZone set to PreserveOnDelete, skipping cleanup and removing finalizer")
		err := r.removeDNSZoneFinalizer(dnsZone, logger)
		return reconcile.Result{}, err
	}

	logger.Debug("Retrieving current state")
	err := actuator.Refresh()
	if err != nil {
		logger.WithError(err).Error("Failed to retrieve hosted zone and corresponding tags")
		return reconcile.Result{}, err
	}

	zoneFound, err := actuator.Exists()
	if err != nil {
		logger.WithError(err).Error("Failed while checking if hosted zone exists")
		return reconcile.Result{}, err
	}

	if dnsZone.DeletionTimestamp != nil {
		if zoneFound {
			logger.Debug("DNSZone resource is deleted, deleting hosted zone")
			err = actuator.Delete()
			if err != nil {
				return reconcile.Result{}, err
			}
		}
		if controllerutils.HasFinalizer(dnsZone, hivev1.FinalizerDNSZone) {
			// Remove the finalizer from the DNSZone. It will be persisted when we persist status
			err = r.removeDNSZoneFinalizer(dnsZone, logger)
			if err != nil {
				return reconcile.Result{}, err
			}
			metricDNSZonesDeleted.WithLabelValues("false").Inc()
		}
		return reconcile.Result{}, err
	}
	if !controllerutils.HasFinalizer(dnsZone, hivev1.FinalizerDNSZone) {
		logger.Info("DNSZone does not have a finalizer. Adding one.")
		controllerutils.AddFinalizer(dnsZone, hivev1.FinalizerDNSZone)
		err := r.Client.Update(context.TODO(), dnsZone)
		if err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "Failed to add finalizer to DNSZone")
		}
		return reconcile.Result{}, err
	}
	if !zoneFound {
		logger.Info("No corresponding hosted zone found on cloud provider, creating one")
		err := actuator.Create()
		if err != nil {
			logger.WithError(err).Error("Failed to create hosted zone")
			return reconcile.Result{}, err
		}
	} else {
		logger.Info("Existing hosted zone found. Syncing with DNSZone resource")
		err := actuator.UpdateMetadata()
		if err != nil {
			logger.WithError(err).Error("failed to sync tags for hosted zone")
			return reconcile.Result{}, err
		}
	}

	nameServers, err := actuator.GetNameServers()
	if err != nil {
		logger.WithError(err).Error("Failed to get hosted zone name servers")
		return reconcile.Result{}, err
	}

	isZoneSOAAvailable, err := r.soaLookup(dnsZone.Spec.Zone, logger)
	if err != nil {
		logger.WithError(err).Error("error looking up SOA record for zone")
	}

	reconcileResult := reconcile.Result{}
	if !isZoneSOAAvailable {
		logger.Info("SOA record for DNS zone not available")
		reconcileResult.RequeueAfter = domainAvailabilityCheckInterval
	}

	return reconcileResult, r.updateStatus(nameServers, isZoneSOAAvailable, dnsZone, logger)
}

func (r *ReconcileDNSZone) removeDNSZoneFinalizer(dnsZone *hivev1.DNSZone, logger log.FieldLogger) error {
	// Remove the finalizer from the DNSZone. It will be persisted when we persist status
	logger.Info("Removing DNSZone finalizer")
	controllerutils.DeleteFinalizer(dnsZone, hivev1.FinalizerDNSZone)
	err := r.Client.Update(context.TODO(), dnsZone)
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "Failed to remove DNSZone finalizer")
	}
	return err
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
		availableCondition := controllerutils.FindCondition(desiredState.Status.Conditions, hivev1.ZoneAvailableDNSZoneCondition)
		if availableCondition == nil || availableCondition.Status == corev1.ConditionFalse {
			return true, 0
		} // If waiting to link to parent, sync now to check domain
	}

	delta := time.Since(desiredState.Status.LastSyncTimestamp.Time)
	if delta >= zoneResyncDuration {
		// We haven't sync'd in over zoneResyncDuration time, sync now.
		return true, delta
	}

	// We didn't meet any of the criteria above, so we should not sync.
	return false, delta
}

func (r *ReconcileDNSZone) getActuator(dnsZone *hivev1.DNSZone, dnsLog log.FieldLogger) (Actuator, error) {
	if dnsZone.Spec.AWS != nil {
		credentials := awsclient.CredentialsSource{
			Secret: &awsclient.SecretCredentialsSource{
				Ref:       &dnsZone.Spec.AWS.CredentialsSecretRef,
				Namespace: dnsZone.Namespace,
			},
			AssumeRole: &awsclient.AssumeRoleCredentialsSource{
				SecretRef: corev1.SecretReference{
					Namespace: controllerutils.GetHiveNamespace(),
					Name:      os.Getenv(constants.HiveAWSServiceProviderCredentialsSecretRefEnvVar),
				},
				Role: dnsZone.Spec.AWS.CredentialsAssumeRole,
			},
		}

		return NewAWSActuator(dnsLog, r.Client, credentials, dnsZone, awsclient.New)
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

	if dnsZone.Spec.Azure != nil {
		secret := &corev1.Secret{}
		err := r.Get(context.TODO(),
			types.NamespacedName{
				Name:      dnsZone.Spec.Azure.CredentialsSecretRef.Name,
				Namespace: dnsZone.Namespace,
			},
			secret)
		if err != nil {
			return nil, err
		}

		return NewAzureActuator(dnsLog, secret, dnsZone, azureclient.NewClientFromSecret)
	}

	return nil, errors.New("unable to determine which actuator to use")
}

func (r *ReconcileDNSZone) updateStatus(nameServers []string, isSOAAvailable bool, dnsZone *hivev1.DNSZone, logger log.FieldLogger) error {
	orig := dnsZone.DeepCopy()

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
		logger.Debug("Updating DNSZone status")
		err := r.Client.Status().Update(context.TODO(), dnsZone)
		if err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "Cannot update DNSZone status")
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

// IsErrorUpdateEvent returns true when the update event for DNSZone is from
// error state.
func IsErrorUpdateEvent(evt event.UpdateEvent) bool {
	new, ok := evt.ObjectNew.(*hivev1.DNSZone)
	if !ok {
		return false
	}
	if len(new.Status.Conditions) == 0 {
		return false
	}

	old, ok := evt.ObjectOld.(*hivev1.DNSZone)
	if !ok {
		return false
	}

	errorConds := []hivev1.DNSZoneConditionType{
		hivev1.InsufficientCredentialsCondition,
		hivev1.AuthenticationFailureCondition,
	}

	for _, cond := range errorConds {
		cn := controllerutils.FindCondition(new.Status.Conditions, cond)
		if cn != nil && cn.Status == corev1.ConditionTrue {
			co := controllerutils.FindCondition(old.Status.Conditions, cond)
			if co == nil {
				return true // newly added failure condition
			}
			if co.Status != corev1.ConditionTrue {
				return true // newly Failed failure condition
			}
			if cn.Message != co.Message ||
				cn.Reason != co.Reason {
				return true // already failing but change in error reported
			}
		}
	}

	return false
}
