package clusterpool

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
	"k8s.io/apimachinery/pkg/types"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"

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
	"github.com/openshift/hive/pkg/gcpclient"
)

const (
	ControllerName = "clusterpool"
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
		scheme: mgr.GetScheme(),
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

	r.(*ReconcileClusterPool).dynamicClient, err = dynamic.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}

	r.(*ReconcileClusterPool).discoveryClient, err = discovery.NewDiscoveryClientForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileClusterPool{}

// ReconcileClusterPool reconciles a CLusterLeasePool object
type ReconcileClusterPool struct {
	client.Client
	scheme          *runtime.Scheme
	dynamicClient   dynamic.Interface
	discoveryClient discovery.DiscoveryInterface
}

// Reconcile reads the state of the ClusterPool, checks if we currently have enough ClusterDeployments waiting, and
// attempts to reach the desired state if not.
func (r *ReconcileClusterPool) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	logger := log.WithFields(log.Fields{
		"clusterPool": request.Name,
		"controller":  ControllerName,
	})

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

	// If the pool is deleted, do not reconcile.
	if clp.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	// Find all ClusterDeployments from this pool:
	allCDs := &hivev1.ClusterDeploymentList{}
	if err := r.Client.List(context.Background(), allCDs); err != nil {
		logger.WithError(err).Error("error listing ClusterDeployments")
		return reconcile.Result{}, err
	}
	var poolCDs []*hivev1.ClusterDeployment
	for i := range allCDs.Items {
		cd := &allCDs.Items[i]
		if cd.Spec.ClusterPoolRef != nil && cd.Spec.ClusterPoolRef.Name == clp.Name && cd.Spec.ClusterPoolRef.Namespace == clp.Namespace {
			poolCDs = append(poolCDs, cd)
		}
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

	// If too many, delete some.
	// TODO: improve logic here, delete oldest, or delete still installing in favor of those that are ready
	if len(poolCDs)-deleting > clp.Spec.Size {
		deletionsNeeded := len(poolCDs) - deleting - clp.Spec.Size
		if err := r.deleteExcessClusters(clp, poolCDs, deletionsNeeded, logger); err != nil {
			return reconcile.Result{}, err
		}
	} else if len(poolCDs)-deleting < clp.Spec.Size {
		// If too few, create new InstallConfig and ClusterDeployment.
		if err := r.addClusters(clp, clp.Spec.Size-len(poolCDs)+deleting, logger); err != nil {
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

	ns, err := r.obtainRandomNamespace(clp)
	if err != nil {
		logger.WithError(err).Error("error obtaining random namespace")
		return err
	}
	logger.Infof("Creating new cluster in namespace: %s", ns.Name)

	// We will use this unique random namespace name for our cluster name.
	builder := &clusterresource.Builder{
		Name:       ns.Name,
		Namespace:  ns.Name,
		BaseDomain: clp.Spec.BaseDomain,
		// TODO:
		ReleaseImage:     "quay.io/openshift-release-dev/ocp-release:4.3.3-x86_64",
		WorkerNodesCount: int64(3),
		MachineNetwork:   "10.0.0.0/16",
		ClusterPool:      types.NamespacedName{Namespace: clp.Namespace, Name: clp.Name},
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
	var credsSecretName string
	if clp.Spec.Platform.AWS != nil {
		credsSecretName = clp.Spec.Platform.AWS.CredentialsSecretRef.Name
		// Lookup the platform creds secret for this pool:
		credsSecret := &corev1.Secret{}
		if err := r.Client.Get(context.Background(), types.NamespacedName{Namespace: clp.Namespace, Name: credsSecretName}, credsSecret); err != nil {
			logger.WithError(err).Error("error looking up credentials secret for pool in hive namespace")
			return err
		}

		accessKeyID := credsSecret.Data[constants.AWSAccessKeyIDSecretKey]
		secretAccessKey := credsSecret.Data[constants.AWSSecretAccessKeySecretKey]
		awsProvider := &clusterresource.AWSCloudBuilder{
			AccessKeyID:     string(accessKeyID),
			SecretAccessKey: string(secretAccessKey),
		}
		builder.CloudBuilder = awsProvider

	} else if clp.Spec.Platform.GCP != nil {
		credsSecretName = clp.Spec.Platform.GCP.CredentialsSecretRef.Name
		// Lookup the platform creds secret for this pool:
		credsSecret := &corev1.Secret{}
		if err := r.Client.Get(context.Background(), types.NamespacedName{Namespace: clp.Namespace, Name: credsSecretName}, credsSecret); err != nil {
			logger.WithError(err).Error("error looking up credentials secret for pool in hive namespace")
			return err
		}

		gcpSA := credsSecret.Data[constants.GCPCredentialsName]

		projectID, err := gcpclient.ProjectID(gcpSA)
		if err != nil {
			return errors.Wrap(err, "error loading GCP project ID from service account json")
		}

		gcpProvider := &clusterresource.GCPCloudBuilder{
			ServiceAccount: gcpSA,
			ProjectID:      projectID,
		}
		builder.CloudBuilder = gcpProvider

	} else if clp.Spec.Platform.Azure != nil {
		credsSecretName = clp.Spec.Platform.Azure.CredentialsSecretRef.Name
		// Lookup the platform creds secret for this pool:
		credsSecret := &corev1.Secret{}
		if err := r.Client.Get(context.Background(), types.NamespacedName{Namespace: clp.Namespace, Name: credsSecretName}, credsSecret); err != nil {
			logger.WithError(err).Error("error looking up credentials secret for pool in hive namespace")
			return err
		}
		azureSP := credsSecret.Data[constants.AzureCredentialsName]

		azureProvider := &clusterresource.AzureCloudBuilder{
			ServicePrincipal:            azureSP,
			BaseDomainResourceGroupName: clp.Spec.Platform.Azure.BaseDomainResourceGroupName,
		}
		builder.CloudBuilder = azureProvider
	}
	// TODO: OpenStack, VMware, and Ovirt.

	objs, err := builder.Build()
	if err != nil {
		return errors.Wrap(err, "error building resources")
	}
	for _, obj := range objs {
		if err := r.Client.Create(context.Background(), obj); err != nil {
			return err
		}
	}

	return nil
}

func (r *ReconcileClusterPool) obtainRandomNamespace(clp *hivev1.ClusterPool) (*corev1.Namespace, error) {
	namespaceName := apihelpers.GetResourceName(clp.Name, utilrand.String(5))
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
			Labels: map[string]string{
				// Should never be removed.
				constants.OriginClusterPoolNameLabel: clp.Name,
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

	logger.Infof("too many clusters, searching for %d to delete", deletionsNeeded)
	counter := 0
	for _, cd := range poolCDs {
		cdLog := logger.WithField("cluster", fmt.Sprintf("%s/%s", cd.Namespace, cd.Name))
		if cd.DeletionTimestamp != nil {
			cdLog.WithFields(log.Fields{"cdName": cd.Name, "cdNamespace": cd.Namespace}).Debug("cluster already deleting")
			continue
		}
		cdLog.Info("deleting cluster deployment")
		if err := r.Client.Delete(context.Background(), cd); err != nil {
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
