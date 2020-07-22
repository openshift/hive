package clusterpool

import (
	"context"
	"sort"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	apihelpers "github.com/openshift/hive/pkg/apis/helpers"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/clusterresource"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	ControllerName = "clusterpool"
	finalizer      = "hive.openshift.io/clusters"
)

// Add creates a new ClusterPool Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return AddToManager(mgr, NewReconciler(mgr))
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager) reconcile.Reconciler {
	r := &ReconcileClusterPool{
		Client: controllerutils.NewClientWithMetricsOrDie(mgr, ControllerName),
		logger: log.WithField("controller", ControllerName),
	}
	return r
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("clusterpool-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: controllerutils.GetConcurrentReconciles()})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterPool
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterPool{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterDeployments originating from a pool:
	clusterPoolMapFn := handler.ToRequestsFunc(
		func(a handler.MapObject) []reconcile.Request {
			var requests []reconcile.Request

			cd := a.Object.(*hivev1.ClusterDeployment)
			if cd.Spec.ClusterPoolRef != nil {
				nsName := types.NamespacedName{Namespace: cd.Spec.ClusterPoolRef.Namespace, Name: cd.Spec.ClusterPoolRef.Name}
				requests = append(requests, reconcile.Request{
					NamespacedName: nsName,
				})
			}
			return requests
		})
	err = c.Watch(
		&source.Kind{Type: &hivev1.ClusterDeployment{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: clusterPoolMapFn,
		})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileClusterPool{}

// ReconcileClusterPool reconciles a CLusterLeasePool object
type ReconcileClusterPool struct {
	client.Client
	logger log.FieldLogger
}

// Reconcile reads the state of the ClusterPool, checks if we currently have enough ClusterDeployments waiting, and
// attempts to reach the desired state if not.
func (r *ReconcileClusterPool) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	logger := r.logger.WithField("clusterPool", request.Name)

	logger.Infof("reconciling cluster pool: %v", request.Name)
	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(ControllerName).Observe(dur.Seconds())
		logger.WithField("elapsed", dur).Info("reconcile complete")
	}()

	// Fetch the ClusterPool instance
	clp := &hivev1.ClusterPool{}
	err := r.Get(context.TODO(), request.NamespacedName, clp)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("pool not found")
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.WithError(err).Error("error reading cluster pool")
		return reconcile.Result{}, err
	}

	// If the pool is deleted, clear finalizer once all ClusterDeployments have been deleted.
	if clp.DeletionTimestamp != nil {
		return reconcile.Result{}, r.reconcileDeletedPool(clp, logger)
	}

	// Add finalizer if not already present
	if !controllerutils.HasFinalizer(clp, finalizer) {
		logger.Debug("adding finalizer to ClusterPool")
		controllerutils.AddFinalizer(clp, finalizer)
		if err := r.Update(context.Background(), clp); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "error adding finalizer to ClusterPool")
			return reconcile.Result{}, err
		}
	}

	// Find all ClusterDeployments from this pool:
	poolCDs, err := r.getAllUnclaimedClusterDeployments(clp, logger)
	if err != nil {
		return reconcile.Result{}, err
	}

	installing := 0
	deleting := 0
	for _, cd := range poolCDs {
		if cd.DeletionTimestamp != nil {
			deleting += 1
		} else if !cd.Spec.Installed {
			installing += 1
		}
	}
	logger.WithFields(log.Fields{
		"installing": installing,
		"deleting":   deleting,
		"total":      len(poolCDs),
		"ready":      len(poolCDs) - installing - deleting,
	}).Info("found clusters for ClusterPool")

	switch drift := len(poolCDs) - deleting - int(clp.Spec.Size); {
	// If too many, delete some.
	case drift > 0:
		if err := r.deleteExcessClusters(clp, poolCDs, drift, logger); err != nil {
			return reconcile.Result{}, err
		}
	// If too few, create new InstallConfig and ClusterDeployment.
	case drift < 0:
		if err := r.addClusters(clp, -drift, logger); err != nil {
			log.WithError(err).Error("error adding clusters")
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileClusterPool) addClusters(
	clp *hivev1.ClusterPool,
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

func (r *ReconcileClusterPool) createCluster(
	clp *hivev1.ClusterPool,
	logger log.FieldLogger) error {

	ns, err := r.createRandomNamespace(clp)
	if err != nil {
		logger.WithError(err).Error("error obtaining random namespace")
		return err
	}
	logger.WithField("cluster", ns.Name).Info("Creating new cluster")

	// We will use this unique random namespace name for our cluster name.
	builder := &clusterresource.Builder{
		Name:       ns.Name,
		Namespace:  ns.Name,
		BaseDomain: clp.Spec.BaseDomain,
		// TODO:
		ReleaseImage:     "quay.io/openshift-release-dev/ocp-release:4.3.3-x86_64",
		WorkerNodesCount: int64(3),
		MachineNetwork:   "10.0.0.0/16",
	}

	// Load the pull secret if one is specified (may not be if using a global pull secret)
	if clp.Spec.PullSecretRef != nil {
		pullSec := &corev1.Secret{}
		err := r.Client.Get(context.Background(),
			types.NamespacedName{Namespace: clp.Namespace, Name: clp.Spec.PullSecretRef.Name},
			pullSec)
		if err != nil {
			logger.WithError(err).Error("error reading pull secret")
			return err
		}
		builder.PullSecret = string(pullSec.Data[".dockerconfigjson"])
	}

	// TODO: regions being ignored throughout, and not exposed in create-cluster cmd either
	switch platform := clp.Spec.Platform; {
	case platform.AWS != nil:
		credsSecret, err := r.getCredentialsSecret(clp, platform.AWS.CredentialsSecretRef.Name, logger)
		if err != nil {
			return err
		}
		builder.CloudBuilder = clusterresource.NewAWSCloudBuilderFromSecret(credsSecret)
	case platform.GCP != nil:
		credsSecret, err := r.getCredentialsSecret(clp, platform.GCP.CredentialsSecretRef.Name, logger)
		if err != nil {
			return err
		}
		cloudBuilder, err := clusterresource.NewGCPCloudBuilderFromSecret(credsSecret)
		if err != nil {
			return err
		}
		builder.CloudBuilder = cloudBuilder
	case platform.Azure != nil:
		credsSecret, err := r.getCredentialsSecret(clp, platform.Azure.CredentialsSecretRef.Name, logger)
		if err != nil {
			return err
		}
		builder.CloudBuilder = clusterresource.NewAzureCloudBuilderFromSecret(credsSecret, platform.Azure.BaseDomainResourceGroupName)
	}
	// TODO: OpenStack, VMware, and Ovirt.

	objs, err := builder.Build()
	if err != nil {
		return errors.Wrap(err, "error building resources")
	}
	// Add the ClusterPoolRef to the ClusterDeployment, and move it to the end of the slice.
	for i, obj := range objs {
		cd, ok := obj.(*hivev1.ClusterDeployment)
		if !ok {
			continue
		}
		poolRef := poolReference(clp)
		cd.Spec.ClusterPoolRef = &poolRef
		lastIndex := len(objs) - 1
		objs[i], objs[lastIndex] = objs[lastIndex], objs[i]
	}
	// Create the resources.
	for _, obj := range objs {
		if err := r.Client.Create(context.Background(), obj); err != nil {
			return err
		}
	}

	return nil
}

func (r *ReconcileClusterPool) createRandomNamespace(clp *hivev1.ClusterPool) (*corev1.Namespace, error) {
	namespaceName := apihelpers.GetResourceName(clp.Name, utilrand.String(5))
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
			Labels: map[string]string{
				// Should never be removed.
				constants.ClusterPoolNameLabel: clp.Name,
			},
		},
	}
	err := r.Create(context.Background(), ns)
	return ns, err
}

func (r *ReconcileClusterPool) deleteExcessClusters(
	clp *hivev1.ClusterPool,
	poolCDs []*hivev1.ClusterDeployment,
	deletionsNeeded int,
	logger log.FieldLogger) error {

	logger.WithField("deletionsNeeded", deletionsNeeded).Info("deleting excess clusters")
	var installingClusters []*hivev1.ClusterDeployment
	var installedClusters []*hivev1.ClusterDeployment
	for _, cd := range poolCDs {
		logger := logger.WithField("cluster", cd.Name)
		switch {
		case cd.DeletionTimestamp != nil:
			logger.Debug("cluster already deleting")
		case cd.Spec.Installed:
			logger.Debug("cluster is installed")
			installedClusters = append(installedClusters, cd)
		default:
			logger.Debug("cluster is installing")
			installingClusters = append(installingClusters, cd)
		}
	}
	clustersToDelete := make([]*hivev1.ClusterDeployment, 0, deletionsNeeded)
	if deletionsNeeded < len(installingClusters) {
		// Sort the installing clusters in order by creation timestamp from newest to oldest. This has the effect of
		// prioritizing deleting those clusters that have the longest time until they are installed.
		sort.Slice(installingClusters, func(i, j int) bool {
			return installingClusters[i].CreationTimestamp.After(installingClusters[j].CreationTimestamp.Time)
		})
		clustersToDelete = installingClusters[:deletionsNeeded]
	} else {
		clustersToDelete = append(clustersToDelete, installingClusters...)
		deletionsOfInstalledClustersNeeded := deletionsNeeded - len(installingClusters)
		if deletionsOfInstalledClustersNeeded <= len(installedClusters) {
			clustersToDelete = append(clustersToDelete, installedClusters[:deletionsOfInstalledClustersNeeded]...)
		} else {
			logger.WithField("deletionsNeeded", deletionsNeeded).
				WithField("installingClusters", len(installingClusters)).
				WithField("installedClusters", len(installedClusters)).
				Error("trying to delete more clusters than there are available")
			clustersToDelete = append(clustersToDelete, installedClusters...)
		}
	}
	for _, cd := range clustersToDelete {
		logger := logger.WithField("cluster", cd.Name)
		logger.Info("deleting cluster deployment")
		if err := r.Client.Delete(context.Background(), cd); err != nil {
			logger.WithError(err).Error("error deleting cluster deployment")
			return err
		}
	}
	logger.Info("no more deletions required")
	return nil
}

func (r *ReconcileClusterPool) reconcileDeletedPool(pool *hivev1.ClusterPool, logger log.FieldLogger) error {
	if !controllerutils.HasFinalizer(pool, finalizer) {
		return nil
	}
	poolCDs, err := r.getAllUnclaimedClusterDeployments(pool, logger)
	if err != nil {
		return err
	}
	for _, cd := range poolCDs {
		if cd.DeletionTimestamp != nil {
			continue
		}
		if err := r.Delete(context.Background(), cd); err != nil {
			logger.WithError(err).WithField("cluster", cd.Name).Log(controllerutils.LogLevel(err), "could not delete ClusterDeployment")
			return errors.Wrap(err, "could not delete ClusterDeployment")
		}
	}
	controllerutils.DeleteFinalizer(pool, finalizer)
	if err := r.Update(context.Background(), pool); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not remove finalizer from ClusterPool")
		return errors.Wrap(err, "could not delete finalizer from ClusterPool")
	}
	return nil
}

func (r *ReconcileClusterPool) getAllUnclaimedClusterDeployments(pool *hivev1.ClusterPool, logger log.FieldLogger) ([]*hivev1.ClusterDeployment, error) {
	cdList := &hivev1.ClusterDeploymentList{}
	if err := r.Client.List(context.Background(), cdList); err != nil {
		logger.WithError(err).Error("error listing ClusterDeployments")
		return nil, err
	}
	var poolCDs []*hivev1.ClusterDeployment
	for i, cd := range cdList.Items {
		if refInCD := cd.Spec.ClusterPoolRef; refInCD != nil && *refInCD == poolReference(pool) {
			poolCDs = append(poolCDs, &cdList.Items[i])
		}
	}
	return poolCDs, nil
}

func poolReference(pool *hivev1.ClusterPool) hivev1.ClusterPoolReference {
	return hivev1.ClusterPoolReference{
		Name:      pool.Name,
		Namespace: pool.Namespace,
		State:     hivev1.ClusterPoolStateUnclaimed,
	}
}

func (r *ReconcileClusterPool) getCredentialsSecret(pool *hivev1.ClusterPool, secretName string, logger log.FieldLogger) (*corev1.Secret, error) {
	credsSecret := &corev1.Secret{}
	if err := r.Client.Get(
		context.Background(),
		client.ObjectKey{Namespace: pool.Namespace, Name: secretName},
		credsSecret,
	); err != nil {
		logger.WithError(err).Error("error looking up credentials secret for pool in hive namespace")
		return nil, err
	}
	return credsSecret, nil
}
