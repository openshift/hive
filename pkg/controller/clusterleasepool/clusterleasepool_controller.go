package clusterleasepool

import (
	"context"
	"fmt"
	"github.com/openshift/hive/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"time"

	log "github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/openshift/hive/contrib/pkg/createcluster"
	apihelpers "github.com/openshift/hive/pkg/apis/helpers"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	controllerName = "clusterleasepool"

	// lease pool label
)

// Add creates a new ClusterLeasePool Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return AddToManager(mgr, NewReconciler(mgr))
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager) reconcile.Reconciler {
	r := &ReconcileClusterLeasePool{
		Client: controllerutils.NewClientWithMetricsOrDie(mgr, controllerName),
		scheme: mgr.GetScheme(),
	}
	return r
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("clusterleasepool-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: controllerutils.GetConcurrentReconciles()})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterLeasePool
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterLeasePool{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO: watch ClusterDeployment, map to it's owning LeasePool (if any) and queue it up

	return nil
}

var _ reconcile.Reconciler = &ReconcileClusterLeasePool{}

// ReconcileClusterLeasePool reconciles a CLusterLeasePool object
type ReconcileClusterLeasePool struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state ClusterLeasePool, checks if we currently have enough ClusterDeployments waiting, and
// attempts to reach the desired state if not.
func (r *ReconcileClusterLeasePool) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	logger := log.WithFields(log.Fields{
		"clusterLeasePool": request.Name,
		"controller":       controllerName,
	})

	logger.Infof("reconciling cluster lease pool: %v", request.Name)
	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(controllerName).Observe(dur.Seconds())
		logger.WithField("elapsed", dur).Info("reconcile complete")
	}()

	// Fetch the ClusterLeasePool instance
	clp := &hivev1.ClusterLeasePool{}
	err := r.Get(context.TODO(), request.NamespacedName, clp)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("pool not found")
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}
	// If the lease pool is deleted, do not reconcile.
	if clp.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	// TODO: do stuff.

	// List all ClusterDeployments with a matching lease pool label:
	leaseCDs := &hivev1.ClusterDeploymentList{}
	if err := r.Client.List(context.Background(), leaseCDs, client.MatchingLabels(map[string]string{constants.ClusterLeasePoolNameLabel: clp.Name})); err != nil {
		logger.WithError(err).Error("error listing ClusterDeployments for lease pool")
		return reconcile.Result{}, err
	}

	installing := 0
	deleting := 0
	for _, cd := range leaseCDs.Items {
		if cd.DeletionTimestamp != nil {
			deleting += 1
		} else if !cd.Spec.Installed {
			installing += 1
		}
	}
	logger.WithFields(log.Fields{
		"installing": installing,
		"deleting":   deleting,
		"total":      len(leaseCDs.Items),
		"ready":      len(leaseCDs.Items) - installing - deleting,
	}).Info("found clusters for lease pool")

	// If too many, delete some.
	// TODO: improve logic here, delete oldest, or delete still installing in favor of those that are ready
	if len(leaseCDs.Items)-deleting > clp.Spec.Size {
		deletionsNeeded := len(leaseCDs.Items) - deleting - clp.Spec.Size
		if err := r.deleteExcessClusters(clp, leaseCDs, deletionsNeeded, logger); err != nil {
			return reconcile.Result{}, err
		}
	} else if len(leaseCDs.Items)-deleting < clp.Spec.Size {
		// If too few, create new InstallConfig and ClusterDeployment.
		if err := r.addClusters(clp, clp.Spec.Size-len(leaseCDs.Items)+deleting, logger); err != nil {
			return reconcile.Result{}, err
		}
	}

	logger.Debug("reconcile complete")
	return reconcile.Result{}, nil
}

func (r *ReconcileClusterLeasePool) addClusters(
	clp *hivev1.ClusterLeasePool,
	newClusterCount int,
	logger log.FieldLogger) error {
	logger.Infof("Adding %d clusters", newClusterCount)

	for i := 0; i < newClusterCount; i++ {
		if err := r.createCluster(clp, logger); err != nil {
			return err
		}
	}

	return nil
}

func (r *ReconcileClusterLeasePool) createCluster(
	clp *hivev1.ClusterLeasePool,
	logger log.FieldLogger) error {

	ns, err := r.obtainRandomNamespace(clp)
	if err != nil {
		logger.WithError(err).Error("error obtaining random namespace")
		return err
	}
	logger.Infof("Creating new cluster in namespace: %s", ns.Name)

	var cloudPlatform string
	var credsSecretName string
	if clp.Spec.Platform.AWS != nil {
		cloudPlatform = "aws"
		credsSecretName = clp.Spec.Platform.AWS.CredentialsSecretRef.Name
	} else if clp.Spec.Platform.GCP != nil {
		cloudPlatform = "gcp"
		credsSecretName = clp.Spec.Platform.GCP.CredentialsSecretRef.Name
	} else if clp.Spec.Platform.Azure != nil {
		cloudPlatform = "azure"
		credsSecretName = clp.Spec.Platform.Azure.CredentialsSecretRef.Name
	}

	// Lookup the platform creds secret for this pool which we are currently assuming
	// is in the hive namespace:
	credsSecret := &corev1.Secret{}
	if err := r.Client.Get(context.Background(), types.NamespacedName{Namespace: constants.HiveNamespace, Name: credsSecretName}, credsSecret); err != nil {
		logger.WithError(err).Error("error looking up credentials secret for pool in hive namespace")
		return err
	}

	// Copy the secret to the target namespace:
	clusterCredsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      apihelpers.GetResourceName(ns.Name, "cloud-creds"),
			Namespace: ns.Name,
		},
		Data: credsSecret.Data,
	}
	if err := r.Client.Create(context.Background(), clusterCredsSecret); err != nil {
		logger.WithError(err).Error("error copying cloud creds secret to cluster namespace")
		return err
	}

	// We will use this unique random namespace name for our cluster name.

	createOpts := &createcluster.Options{
		Name:        ns.Name,
		Namespace:   ns.Name,
		BaseDomain:  clp.Spec.BaseDomain,
		DeleteAfter: clp.Spec.DeleteAfter.Duration.String(),
		Cloud:       cloudPlatform,
		CredsSecret: clusterCredsSecret.Name,
		// TODO:
		ReleaseImage: "quay.io/openshift-release-dev/ocp-release:4.3.3-x86_64",
		ClusterLabels: map[string]string{
			constants.ClusterLeasePoolNameLabel: clp.Name,
		},
	}

	if err := createOpts.Run(); err != nil {
		logger.WithError(err).Error("error creating cluster artifacts")
		return err
	}

	return nil
}

func (r *ReconcileClusterLeasePool) obtainRandomNamespace(clp *hivev1.ClusterLeasePool) (*corev1.Namespace, error) {
	namespaceName := apihelpers.GetResourceName(clp.Name, utilrand.String(5))
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
			Labels: map[string]string{
				// Will be removed once assigned to a request, removing it from the pool..
				constants.ClusterLeasePoolNameLabel: clp.Name,
				// Should never be removed.
				constants.OriginClusterLeasePoolNameLabel: clp.Name,
			},
		},
	}
	err := r.Create(context.Background(), ns)
	return ns, err
}

func (r *ReconcileClusterLeasePool) deleteExcessClusters(
	clp *hivev1.ClusterLeasePool,
	leaseCDs *hivev1.ClusterDeploymentList,
	deletionsNeeded int,
	logger log.FieldLogger) error {

	logger.Infof("too many clusters, searching for %d to delete", deletionsNeeded)
	counter := 0
	for _, cd := range leaseCDs.Items {
		cdLog := logger.WithField("cluster", fmt.Sprintf("%s/%s", cd.Namespace, cd.Name))
		if cd.DeletionTimestamp != nil {
			cdLog.WithFields(log.Fields{"cdName": cd.Name, "cdNamespace": cd.Namespace}).Debug("cluster already deleting")
			continue
		}
		cdLog.Info("deleting cluster deployment")
		if err := r.Client.Delete(context.Background(), &cd); err != nil {
			cdLog.WithError(err).Error("error deleting cluster deployment")
			return err
		}
		counter += 1
		if counter == deletionsNeeded {
			logger.Info("no more deletions required")
			break
		}
	}
	return nil
}
