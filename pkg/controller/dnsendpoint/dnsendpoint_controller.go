package dnsendpoint

import (
	"context"
	"os"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/controller/dnsendpoint/nameserver"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/manageddns"
)

const (
	controllerName = "dnsendpoint"
)

// Add creates a new DNSZone Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	logger := log.WithField("controller", controllerName)
	c := controllerutils.NewClientWithMetricsOrDie(mgr, controllerName)

	managedDomains, err := manageddns.ReadManagedDomainsFile()
	if err != nil {
		logger.WithError(err).Error("could not read managed domains file")
		return errors.Wrap(err, "could not read managed domains file")
	}
	if len(managedDomains) == 0 {
		logger.Info("no managed domains for external DNS; controller disabled")
		return nil
	}

	nameServerQuery := createNameServerQuery(c, logger)
	if nameServerQuery == nil {
		logger.Info("no platform found for external DNS; controller disabled")
		return nil
	}

	nameServerChanges := make(chan event.GenericEvent, 1024)

	registerNameServerChange := func(objectKey client.ObjectKey) {
		nameServerChanges <- event.GenericEvent{
			Meta: &metav1.ObjectMeta{
				Namespace: objectKey.Namespace,
				Name:      objectKey.Name,
			},
		}
	}
	nameServerScraper := newNameServerScraper(logger, nameServerQuery, managedDomains, registerNameServerChange)
	if err := mgr.Add(nameServerScraper); err != nil {
		return err
	}

	reconciler := &ReconcileDNSEndpoint{
		Client:            c,
		scheme:            mgr.GetScheme(),
		logger:            logger,
		nameServerScraper: nameServerScraper,
		nameServerQuery:   nameServerQuery,
	}
	ctrl, err := controller.New(
		controllerName,
		mgr,
		controller.Options{
			Reconciler:              reconciler,
			MaxConcurrentReconciles: controllerutils.GetConcurrentReconciles(),
		},
	)
	if err != nil {
		return err
	}
	if err := ctrl.Watch(&source.Kind{Type: &hivev1.DNSEndpoint{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}
	return ctrl.Watch(&source.Channel{Source: nameServerChanges}, &handler.EnqueueRequestForObject{})
}

var _ reconcile.Reconciler = &ReconcileDNSEndpoint{}

// ReconcileDNSEndpoint reconciles a DNSEndpoint object
type ReconcileDNSEndpoint struct {
	client.Client
	scheme            *runtime.Scheme
	logger            log.FieldLogger
	nameServerScraper *nameServerScraper
	nameServerQuery   nameserver.Query
}

// Reconcile reads that state of the cluster for a DNSEndpoint object and makes changes based on the state read
// and what is in the DNSEndpoint.Spec
func (r *ReconcileDNSEndpoint) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	dnsLog := r.logger.WithFields(log.Fields{
		"dnsendpoint": request.Name,
		"namespace":   request.Namespace,
	})

	// For logging, we need to see when the reconciliation loop starts and ends.
	dnsLog.Info("reconciling dns endpoint")
	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(controllerName).Observe(dur.Seconds())
		dnsLog.WithField("elapsed", dur).Info("reconcile complete")
	}()

	// Fetch the DNSEndpoint object
	instance := &hivev1.DNSEndpoint{}
	if err := r.Get(context.TODO(), request.NamespacedName, instance); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		dnsLog.WithError(err).Error("Error fetching dnsendpoint object")
		return reconcile.Result{}, err
	}

	if !isValidDNSEndpoint(instance, dnsLog) {
		return reconcile.Result{}, nil
	}
	domain := instance.Spec.Endpoints[0].DNSName
	dnsLog = dnsLog.WithField("domain", domain)

	if instance.DeletionTimestamp != nil {
		if !controllerutils.HasFinalizer(instance, hivev1.FinalizerDNSEndpoint) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, r.syncDeletedEndpoint(instance, dnsLog)
	}

	if !r.nameServerScraper.HasBeenScraped(domain) {
		return reconcile.Result{}, errors.New("name servers have not yet been scraped")
	}

	if !controllerutils.HasFinalizer(instance, hivev1.FinalizerDNSEndpoint) {
		controllerutils.AddFinalizer(instance, hivev1.FinalizerDNSEndpoint)
		if err := r.Update(context.TODO(), instance); err != nil {
			dnsLog.WithError(err).Log(controllerutils.LogLevel(err), "error adding finalizer")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	desiredNameServers := sets.NewString(instance.Spec.Endpoints[0].Targets...)
	rootDomain, currentNameServers := r.nameServerScraper.GetEndpoint(domain)
	if currentNameServers.Equal(desiredNameServers) {
		dnsLog.Debug("NS record is up to date")
		return reconcile.Result{}, r.observeCurrentGeneration(instance, dnsLog)
	}

	dnsLog.Info("creating NS record")
	if err := r.nameServerQuery.Create(rootDomain, domain, desiredNameServers); err != nil {
		dnsLog.WithError(err).Error("error creating NS record")
		return reconcile.Result{}, err
	}
	r.nameServerScraper.AddEndpoint(request.NamespacedName, domain, desiredNameServers)

	return reconcile.Result{}, r.observeCurrentGeneration(instance, dnsLog)
}

func (r *ReconcileDNSEndpoint) syncDeletedEndpoint(instance *hivev1.DNSEndpoint, dnsLog log.FieldLogger) error {
	dnsLog.Info("deleting NS record")
	domain := instance.Spec.Endpoints[0].DNSName
	rootDomain, currentNameServers := r.nameServerScraper.GetEndpoint(domain)
	if err := r.nameServerQuery.Delete(rootDomain, domain, currentNameServers); err != nil {
		dnsLog.WithError(err).Error("error deleting NS record")
		return err
	}
	r.nameServerScraper.RemoveEndpoint(domain)
	controllerutils.DeleteFinalizer(instance, hivev1.FinalizerDNSEndpoint)
	if err := r.Update(context.Background(), instance); err != nil {
		dnsLog.WithError(err).Log(controllerutils.LogLevel(err), "error deleting finalizer")
		return err
	}
	return nil
}

func (r *ReconcileDNSEndpoint) observeCurrentGeneration(instance *hivev1.DNSEndpoint, dnsLog log.FieldLogger) error {
	if instance.Generation == instance.Status.ObservedGeneration {
		return nil
	}
	instance.Status.ObservedGeneration = instance.Generation
	return errors.Wrap(
		r.Status().Update(context.Background(), instance),
		"error setting observed generation",
	)
}

func isValidDNSEndpoint(instance *hivev1.DNSEndpoint, dnsLog log.FieldLogger) bool {
	if len(instance.Spec.Endpoints) != 1 {
		dnsLog.Error("dnsendpoint does not contain exactly 1 endpoint")
		return false
	}
	if instance.Spec.Endpoints[0].RecordType != "NS" {
		dnsLog.Error("dnsendpoint is not a NS")
		return false
	}
	return true
}

func createNameServerQuery(c client.Client, logger log.FieldLogger) nameserver.Query {
	awsCredsSecretName := os.Getenv(constants.ExternalDNSAWSCredsEnvVar)
	if awsCredsSecretName != "" {
		logger.Infof("using aws creds for external DNS stored in %q secret", awsCredsSecretName)
		return nameserver.NewAWSQuery(c, awsCredsSecretName)
	}

	gcpCredsSecretName := os.Getenv(constants.ExternalDNSGCPCredsEnvVar)
	if gcpCredsSecretName != "" {
		logger.Infof("using gcp creds for external DNS stored in %q secret", gcpCredsSecretName)
		return nameserver.NewGCPQuery(c, gcpCredsSecretName)
	}

	return nil
}
