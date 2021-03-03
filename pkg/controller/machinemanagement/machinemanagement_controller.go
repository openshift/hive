package machinemanagement

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	apihelpers "github.com/openshift/hive/apis/helpers"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	"github.com/openshift/hive/pkg/controller/utils"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/remoteclient"
	k8sannotations "github.com/openshift/hive/pkg/util/annotations"
)

// controllerKind contains the schema.GroupVersionKind for this controller type.
var controllerKind = hivev1.SchemeGroupVersion.WithKind("ClusterDeployment")

const (
	ControllerName = hivev1.MachineManagementControllerName
)

// Add creates a new ClusterDeployment controller and adds it to the manager with default RBAC.
func Add(mgr manager.Manager) error {
	logger := log.WithField("controller", ControllerName)
	concurrentReconciles, clientRateLimiter, queueRateLimiter, err := controllerutils.GetControllerConfig(mgr.GetClient(), ControllerName)
	if err != nil {
		logger.WithError(err).Error("could not get controller configurations")
		return err
	}
	return AddToManager(mgr, NewReconciler(mgr, logger, clientRateLimiter), concurrentReconciles, queueRateLimiter)
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager, logger log.FieldLogger, rateLimiter flowcontrol.RateLimiter) reconcile.Reconciler {
	r := &ReconcileMachineManagement{
		Client: controllerutils.NewClientWithMetricsOrDie(mgr, ControllerName, &rateLimiter),
		scheme: mgr.GetScheme(),
		logger: logger,
	}
	r.remoteClusterAPIClientBuilder = func(cd *hivev1.ClusterDeployment) remoteclient.Builder {
		return remoteclient.NewBuilder(r.Client, cd, ControllerName)
	}
	return r
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler, concurrentReconciles int, rateLimiter workqueue.RateLimiter) error {
	c, err := controller.New("clusterdeployment-controller", mgr, controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter:             rateLimiter,
	})
	if err != nil {
		log.WithField("controller", ControllerName).WithError(err).Error("Error getting new cluster deployment")
		return err
	}

	// Watch for changes to ClusterDeployment
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		log.WithField("controller", ControllerName).WithError(err).Error("Error watching cluster deployment")
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileMachineManagement{}

// ReconcileMachineManagement reconciles a ClusterDeployment object
type ReconcileMachineManagement struct {
	client.Client
	scheme *runtime.Scheme
	logger log.FieldLogger

	// remoteClusterAPIClientBuilder is a function pointer to the function that gets a builder for building a client
	// for the remote cluster's API server
	remoteClusterAPIClientBuilder func(cd *hivev1.ClusterDeployment) remoteclient.Builder
}

// Reconcile reads that state of the cluster for a ClusterDeployment object and makes changes based on the state read
// and what is in the ClusterDeployment.Spec
//
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
//
func (r *ReconcileMachineManagement) Reconcile(request reconcile.Request) (result reconcile.Result, returnErr error) {
	cdLog := controllerutils.BuildControllerLogger(ControllerName, "clusterDeployment", request.NamespacedName)
	cdLog.Info("reconciling cluster deployment")
	recobsrv := hivemetrics.NewReconcileObserver(ControllerName, cdLog)
	defer recobsrv.ObserveControllerReconcileTime()

	// Fetch the ClusterDeployment instance
	cd := &hivev1.ClusterDeployment{}
	err := r.Get(context.TODO(), request.NamespacedName, cd)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			cdLog.Info("cluster deployment Not Found")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		cdLog.WithError(err).Error("Error getting cluster deployment")
		return reconcile.Result{}, err
	}

	return r.reconcile(request, cd, cdLog)
}

func (r *ReconcileMachineManagement) reconcile(request reconcile.Request, cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (result reconcile.Result, returnErr error) {
	// Return early if cluster deployment was deleted
	if !cd.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(cd, hivev1.FinalizerMachineManagementTargetNamespace) {
			if cd.Spec.MachineManagement.TargetNamespace != "" {
				// Clean up namespace
				cdLog.Info("Deleting target namespace ", cd.Spec.MachineManagement.TargetNamespace)
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: cd.Spec.MachineManagement.TargetNamespace},
				}
				if err := r.Delete(context.TODO(), ns); err != nil && !apierrors.IsNotFound(err) {
					return reconcile.Result{}, fmt.Errorf("failed to delete namespace: %w", err)
				}
			}
			// Remove finalizer from cluster deployment
			controllerutil.RemoveFinalizer(cd, hivev1.FinalizerMachineManagementTargetNamespace)
			if err := r.Update(context.TODO(), cd); err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to remove finalizer from cluster deployment: %w", err)
			}
		}
		return reconcile.Result{}, nil
	}

	if cd.Spec.MachineManagement != nil && cd.Spec.MachineManagement.Central != nil {
		targetNamespace := cd.Spec.MachineManagement.TargetNamespace
		if targetNamespace == "" {
			targetNamespace = apihelpers.GetResourceName(cd.Name+"-targetns", utilrand.String(5))
		}

		if cd.Spec.MachineManagement.TargetNamespace == "" {
			cd.Spec.MachineManagement.TargetNamespace = targetNamespace

			// Ensure the cluster deployment has a finalizer for cleanup
			if !controllerutil.ContainsFinalizer(cd, hivev1.FinalizerMachineManagementTargetNamespace) {
				controllerutil.AddFinalizer(cd, hivev1.FinalizerMachineManagementTargetNamespace)
			}

			if err := r.Update(context.TODO(), cd); err != nil {
				cdLog.WithError(err).Log(controllerutils.LogLevel(err), "failed to update cluster deployment")
				return reconcile.Result{Requeue: true}, nil
			}
		}

		ns := &corev1.Namespace{}
		if err := r.Get(context.Background(), types.NamespacedName{Name: targetNamespace}, ns); err != nil && apierrors.IsNotFound(err) {
			cdLog.Info("Creating the target namespace ", targetNamespace)
			ns.Name = targetNamespace
			if err := r.Create(context.TODO(), ns); err != nil && !apierrors.IsAlreadyExists(err) {
				return reconcile.Result{}, fmt.Errorf("failed to create target namespace %q: %w", targetNamespace, err)
			}
		}

		// Ensure targetNamespace has machine management annotation
		if err := r.addAnnotationToTargetNamespace(cd, cdLog, targetNamespace); err != nil {
			return reconcile.Result{}, err
		}

		credentialsSecretName := utils.CredentialsSecretName(cd)
		credentialsSecret := &corev1.Secret{}
		if err := r.Get(context.Background(), types.NamespacedName{Name: credentialsSecretName, Namespace: targetNamespace}, credentialsSecret); err != nil && apierrors.IsNotFound(err) {
			cdLog.Infof("Creating credentials secret in the target namespace %s", targetNamespace)
			err := r.Get(context.Background(), types.NamespacedName{Namespace: cd.Namespace, Name: credentialsSecretName}, credentialsSecret)
			if err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to get provider creds %s: %w", credentialsSecretName, err)
			}
			credentialsSecret.Namespace = targetNamespace
			credentialsSecret.ResourceVersion = ""
			if err := r.Create(context.TODO(), credentialsSecret); err != nil && !apierrors.IsAlreadyExists(err) {
				return reconcile.Result{}, fmt.Errorf("failed to create provider creds secret: %v", err)
			}
		}

		pullSecretName := cd.Spec.PullSecretRef.Name
		pullSecret := &corev1.Secret{}
		if err := r.Get(context.Background(), types.NamespacedName{Name: pullSecretName, Namespace: targetNamespace}, pullSecret); err != nil && apierrors.IsNotFound(err) {
			cdLog.Infof("Creating pull secret in the target namespace %s", targetNamespace)
			err = r.Get(context.Background(), types.NamespacedName{Namespace: cd.Namespace, Name: pullSecretName}, pullSecret)
			if err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to get pull secret %s: %w", pullSecretName, err)
			}
			pullSecret.Namespace = targetNamespace
			pullSecret.ResourceVersion = ""
			if err := r.Create(context.TODO(), pullSecret); err != nil && !apierrors.IsAlreadyExists(err) {
				return reconcile.Result{}, fmt.Errorf("failed to create pull secret: %v", err)
			}
		}

		if cd.Spec.Provisioning.SSHPrivateKeySecretRef != nil {
			SSHKeySecretName := cd.Spec.Provisioning.SSHPrivateKeySecretRef.Name
			SSHKeySecret := &corev1.Secret{}
			if err := r.Get(context.Background(), types.NamespacedName{Name: SSHKeySecretName, Namespace: targetNamespace}, SSHKeySecret); err != nil && apierrors.IsNotFound(err) {
				cdLog.Infof("Creating ssh key secret in the target namespace %s", targetNamespace)
				err = r.Get(context.Background(), types.NamespacedName{Namespace: cd.Namespace, Name: SSHKeySecretName}, SSHKeySecret)
				if err != nil {
					return reconcile.Result{}, fmt.Errorf("failed to get pull secret %s: %w", SSHKeySecretName, err)
				}
				SSHKeySecret.Namespace = targetNamespace
				SSHKeySecret.ResourceVersion = ""
				if err := r.Create(context.TODO(), SSHKeySecret); err != nil && !apierrors.IsAlreadyExists(err) {
					return reconcile.Result{}, fmt.Errorf("failed to create ssh key secret: %v", err)
				}
			}
		}
	}
	return reconcile.Result{}, nil
}

// addAnnotationToTargetNamespace adds annotation to cluster deployment
func (r *ReconcileMachineManagement) addAnnotationToTargetNamespace(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger, name string) error {
	cdLog = cdLog.WithField("namespace", name)

	namespace := &corev1.Namespace{}
	if err := r.Get(context.Background(), types.NamespacedName{Name: name}, namespace); err != nil {
		cdLog.WithError(err).Error("failed to get namespace")
		return err
	}

	annotationAdded := false
	if namespace.Annotations[constants.MachineManagementAnnotation] != cd.Name {
		cdLog.Debug("Setting annotation on target namespace")
		namespace.Annotations = k8sannotations.AddAnnotation(namespace.Annotations, constants.MachineManagementAnnotation, cd.Name)
		annotationAdded = true
	}

	if annotationAdded {
		cdLog.Info("namespace has been modified, updating")
		if err := r.Update(context.TODO(), namespace); err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error updating namespace")
			return err
		}
	}

	return nil
}
