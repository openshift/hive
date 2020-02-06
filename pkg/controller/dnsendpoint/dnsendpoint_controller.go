package dnsendpoint

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
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

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
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
	c := controllerutils.NewClientWithMetricsOrDie(mgr, controllerName)

	reconciler, nameServerChangeNotifier, err := newReconciler(mgr, c)
	if err != nil {
		return err
	}

	if reconciler == nil {
		return nil
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

	if err := ctrl.Watch(&source.Kind{Type: &hivev1.DNSZone{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	if nameServerChangeNotifier != nil {
		if err := ctrl.Watch(&source.Channel{Source: nameServerChangeNotifier}, &handler.EnqueueRequestForObject{}); err != nil {
			log.WithField("controller", controllerName).WithError(err).Error("unable to set up watch for name server changes")
			return err
		}
	}

	return nil
}

type nameServerTool struct {
	scraper     *nameServerScraper
	queryClient nameserver.Query
}

func newReconciler(mgr manager.Manager, kubeClient client.Client) (*ReconcileDNSEndpoint, chan event.GenericEvent, error) {
	nsTools := []nameServerTool{}

	logger := log.WithField("controller", controllerName)

	reconciler := &ReconcileDNSEndpoint{
		Client:          kubeClient,
		scheme:          mgr.GetScheme(),
		logger:          logger,
		nameServerTools: nsTools,
	}

	managedDomains, err := manageddns.ReadManagedDomainsFile()
	if err != nil {
		logger.WithError(err).Error("could not read managed domains file")
		return reconciler, nil, errors.Wrap(err, "could not read managed domains file")
	}
	if len(managedDomains) == 0 {
		// allow the controller to run even when no managed domains are configured
		// (so that we can at least set the DomainNotManaged condition on existing DNSZone objects)
		logger.Info("no managed domains configured")
		return reconciler, nil, nil
	}

	nameServerChangeNotifier := make(chan event.GenericEvent, 1024)

	for _, md := range managedDomains {
		nameServerQuery := createNameServerQuery(kubeClient, logger, md)
		if nameServerQuery == nil {
			logger.WithField("domains", md.Domains).Warn("no platform found for managed DNS")
			continue
		}

		registerNameServerChange := func(objectKey client.ObjectKey) {
			nameServerChangeNotifier <- event.GenericEvent{
				Meta: &metav1.ObjectMeta{
					Namespace: objectKey.Namespace,
					Name:      objectKey.Name,
				},
			}
		}
		nameServerScraper := newNameServerScraper(logger, nameServerQuery, md.Domains, registerNameServerChange)
		if err := mgr.Add(nameServerScraper); err != nil {
			logger.WithError(err).WithField("domains", md.Domains).Warn("unable to add name server scraper for domains")
			continue
		}

		nsTools = append(nsTools, nameServerTool{
			scraper:     nameServerScraper,
			queryClient: nameServerQuery,
		})

	}

	reconciler.nameServerTools = nsTools

	return reconciler, nameServerChangeNotifier, nil
}

var _ reconcile.Reconciler = &ReconcileDNSEndpoint{}

// ReconcileDNSEndpoint reconciles a DNSEndpoint object
type ReconcileDNSEndpoint struct {
	client.Client
	scheme          *runtime.Scheme
	logger          log.FieldLogger
	nameServerTools []nameServerTool
}

// Reconcile reads that state of the cluster for a DNSEndpoint object and makes changes based on the state read
// and what is in the DNSEndpoint.Spec
func (r *ReconcileDNSEndpoint) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	dnsLog := r.logger.WithFields(log.Fields{
		"dnszone":   request.Name,
		"namespace": request.Namespace,
	})

	// For logging, we need to see when the reconciliation loop starts and ends.
	dnsLog.Info("reconciling dns endpoint")
	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(controllerName).Observe(dur.Seconds())
		dnsLog.WithField("elapsed", dur).Info("reconcile complete")
	}()

	// Fetch the DNSZone object
	instance := &hivev1.DNSZone{}
	if err := r.Get(context.TODO(), request.NamespacedName, instance); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		dnsLog.WithError(err).Error("Error fetching dnszone object")
		return reconcile.Result{}, err
	}

	if !instance.Spec.LinkToParentDomain {
		return reconcile.Result{}, nil
	}

	isDeleted := instance.DeletionTimestamp != nil
	hasFinalizer := controllerutils.HasFinalizer(instance, hivev1.FinalizerDNSEndpoint)

	if isDeleted && !hasFinalizer {
		return reconcile.Result{}, nil
	}

	fullDomain := instance.Spec.Zone

	dnsLog = dnsLog.WithField("domain", fullDomain)

	var rootDomain string
	var currentNameServers sets.String

	var nsTool nameServerTool

	for i, nst := range r.nameServerTools {
		rootDomain, currentNameServers = nst.scraper.GetEndpoint(fullDomain)
		if rootDomain != "" {
			nsTool = r.nameServerTools[i]
			break
		}
	}

	if rootDomain == "" {
		dnsLog.WithField("domain", fullDomain).Error("no scraper for domain found, skipping reconcile")
		_, err := updateDomainNotManagedCondition(r.Client, dnsLog, instance, true)
		return reconcile.Result{}, err
	}
	changed, err := updateDomainNotManagedCondition(r.Client, dnsLog, instance, false)
	if changed || err != nil {
		return reconcile.Result{}, err
	}

	if !nsTool.scraper.HasBeenScraped(rootDomain) {
		return reconcile.Result{}, errors.New("name servers have not yet been scraped")
	}

	if !hasFinalizer {
		controllerutils.AddFinalizer(instance, hivev1.FinalizerDNSEndpoint)
		if err := r.Update(context.TODO(), instance); err != nil {
			dnsLog.WithError(err).Log(controllerutils.LogLevel(err), "error adding finalizer")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	desiredNameServers := sets.NewString(instance.Status.NameServers...)

	switch {
	// NS is up-to-date
	case !isDeleted && currentNameServers.Equal(desiredNameServers):
		dnsLog.Debug("NS record is up to date")

	// NS needs to be created or updated
	case !isDeleted && len(desiredNameServers) > 0:
		dnsLog.Info("creating NS record")
		if err := nsTool.queryClient.Create(rootDomain, fullDomain, desiredNameServers); err != nil {
			dnsLog.WithError(err).Error("error creating NS record")
			return reconcile.Result{}, err
		}

		nsTool.scraper.AddEndpoint(request.NamespacedName, fullDomain, desiredNameServers)

	// NS needs to be deleted, either because the DNSZone has been deleted or because
	// there are no targets for the NS.
	default:
		dnsLog.Info("deleting NS record")
		if err := nsTool.queryClient.Delete(rootDomain, fullDomain, currentNameServers); err != nil {
			dnsLog.WithError(err).Error("error deleting NS record")
			return reconcile.Result{}, err
		}
		nsTool.scraper.RemoveEndpoint(fullDomain)
	}

	parentLinkCreated := false
	if !isDeleted && len(desiredNameServers) > 0 {
		parentLinkCreated = true
	}
	_, err = updateParentLinkCreatedCondition(r.Client, dnsLog, instance, parentLinkCreated, desiredNameServers)
	if err != nil {
		return reconcile.Result{}, err
	}

	if isDeleted {
		controllerutils.DeleteFinalizer(instance, hivev1.FinalizerDNSEndpoint)
		if err := r.Update(context.Background(), instance); err != nil {
			dnsLog.WithError(err).Log(controllerutils.LogLevel(err), "error deleting finalizer")
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

func createNameServerQuery(c client.Client, logger log.FieldLogger, managedDomain hivev1.ManageDNSConfig) nameserver.Query {
	if managedDomain.AWS != nil {
		secretName := managedDomain.AWS.CredentialsSecretRef.Name
		logger.Infof("using aws creds for managed domains stored in %q secret", secretName)
		return nameserver.NewAWSQuery(c, secretName)
	}
	if managedDomain.GCP != nil {
		secretName := managedDomain.GCP.CredentialsSecretRef.Name
		logger.Infof("using gcp creds for managed domain stored in %q secret", secretName)
		return nameserver.NewGCPQuery(c, secretName)
	}
	logger.Error("unsupported cloud for managing DNS")
	return nil
}

func updateParentLinkCreatedCondition(c client.Client, logger log.FieldLogger, dnsZone *hivev1.DNSZone, created bool, nameServers sets.String) (bool, error) {
	var status corev1.ConditionStatus
	var reason string
	var message string
	if created {
		status = corev1.ConditionTrue
		reason = "ParentLinkCreated"
		message = fmt.Sprintf("Parent link created for name servers %s", nameServers)
	} else {
		status = corev1.ConditionFalse
		reason = "ParentLinkNotCreated"
		message = "Parent link has not been created"
	}

	return updateCondition(c, logger, dnsZone, hivev1.ParentLinkCreatedCondition, status, reason, message)
}

func updateDomainNotManagedCondition(c client.Client, logger log.FieldLogger, dnsZone *hivev1.DNSZone, missing bool) (bool, error) {
	var status corev1.ConditionStatus
	var reason string
	var message string
	if missing {
		status = corev1.ConditionTrue
		reason = "DomainNotManaged"
		message = "HiveConfig missing configuration definition for domain"
	} else {
		status = corev1.ConditionFalse
		reason = "FoundManagedDomain"
		message = "Found HiveConfig settings for domain"
	}

	return updateCondition(c, logger, dnsZone, hivev1.DomainNotManaged, status, reason, message)
}

// updateCondition will update conditions if necessary and report back with a boolean indicating
// whether a change was made, and whether or not an error was encountered.
func updateCondition(c client.Client,
	logger log.FieldLogger,
	dnsZone *hivev1.DNSZone,
	condition hivev1.DNSZoneConditionType,
	status corev1.ConditionStatus,
	reason, message string) (bool, error) {

	if conds, changed := controllerutils.SetDNSZoneConditionWithChangeCheck(
		dnsZone.Status.Conditions,
		condition,
		status,
		reason,
		message,
		controllerutils.UpdateConditionIfReasonOrMessageChange,
	); changed {
		dnsZone.Status.Conditions = conds
		if err := c.Status().Update(context.Background(), dnsZone); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "could not update status conditions")
			return false, err
		}
		return true, nil
	}
	return false, nil
}
