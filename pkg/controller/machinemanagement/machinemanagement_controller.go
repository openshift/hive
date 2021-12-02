package machinemanagement

import (
	"context"
	"reflect"

	"github.com/pkg/errors"
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
	k8sannotations "github.com/openshift/hive/pkg/util/annotations"
)

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
	return r
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler, concurrentReconciles int, rateLimiter workqueue.RateLimiter) error {
	c, err := controller.New("machinemanagement-controller", mgr, controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter:             rateLimiter,
	})
	if err != nil {
		log.WithField("controller", ControllerName).WithError(err).Error("could not create controller")
		return err
	}

	// Watch for changes to ClusterDeployment
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}},
		controllerutils.NewRateLimitedUpdateEventHandler(&handler.EnqueueRequestForObject{}, controllerutils.IsClusterDeploymentErrorUpdateEvent))
	if err != nil {
		log.WithField("controller", ControllerName).WithError(err).Error("Error watching cluster deployment")
		return err
	}

	err = mgr.GetFieldIndexer().IndexField(context.TODO(), &hivev1.ClusterDeployment{}, "spec.secrets.secretName", func(o client.Object) []string {
		var res []string
		cd := o.(*hivev1.ClusterDeployment)
		if utils.CredentialsSecretName(cd) != "" {
			res = append(res, utils.CredentialsSecretName(cd))
		}
		if cd.Spec.PullSecretRef != nil {
			res = append(res, cd.Spec.PullSecretRef.Name)
		}
		if cd.Spec.Provisioning != nil && cd.Spec.Provisioning.SSHPrivateKeySecretRef != nil {
			res = append(res, cd.Spec.Provisioning.SSHPrivateKeySecretRef.Name)
		}
		return res
	})
	if err != nil {
		log.WithField("controller", ControllerName).WithError(err).Error("Error indexing cluster deployment secrets")
		return err
	}

	// Watch for changes to Secret referenced by cluster deployment
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, handler.EnqueueRequestsFromMapFunc(func(a client.Object) []reconcile.Request {
		retval := []reconcile.Request{}

		secret, ok := a.(*corev1.Secret)
		if !ok {
			// Wasn't a Secret, bail out. This should not happen.
			log.Errorf("Error converting MapObject.Object to Secret. Value: %+v", a)
			return retval
		}

		cdsWithSecrets := &hivev1.ClusterDeploymentList{}
		_ = mgr.GetClient().List(context.Background(), cdsWithSecrets, client.MatchingFields{"spec.secrets.secretName": secret.Name}, client.InNamespace(secret.Namespace))
		for _, cd := range cdsWithSecrets.Items {
			retval = append(retval, reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      cd.Name,
				Namespace: cd.Namespace,
			}})
		}
		return retval
	}))
	if err != nil {
		log.WithField("controller", ControllerName).WithError(err).Error("Error watching cluster deployment secrets")
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
}

// Reconcile reads settings within ClusterDeployment.Spec.MachineManagement and creates/copies resources necessary for
// managing machines centrally when requested.
func (r *ReconcileMachineManagement) Reconcile(ctx context.Context, request reconcile.Request) (result reconcile.Result, returnErr error) {
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

	if cd.Spec.MachineManagement == nil || cd.Spec.MachineManagement.Central == nil {
		return reconcile.Result{}, nil
	}

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
					return reconcile.Result{}, errors.Wrapf(err, "failed to delete target namespace %q", cd.Spec.MachineManagement.TargetNamespace)
				}
			}
			// Remove finalizer from cluster deployment
			controllerutil.RemoveFinalizer(cd, hivev1.FinalizerMachineManagementTargetNamespace)
			if err := r.Update(context.TODO(), cd); err != nil {
				return reconcile.Result{}, errors.Wrap(err, "failed to remove finalizer from cluster deployment")
			}
		}
		return reconcile.Result{}, nil
	}

	if cd.Spec.MachineManagement.TargetNamespace == "" {
		cd.Spec.MachineManagement.TargetNamespace = apihelpers.GetResourceName(cd.Name+"-targetns", utilrand.String(5))

		// Ensure the cluster deployment has a finalizer for cleanup
		if !controllerutil.ContainsFinalizer(cd, hivev1.FinalizerMachineManagementTargetNamespace) {
			controllerutil.AddFinalizer(cd, hivev1.FinalizerMachineManagementTargetNamespace)
		}

		if err := r.Update(context.TODO(), cd); err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "failed to update cluster deployment with MachineManagement.TargetNamespace")
			return reconcile.Result{Requeue: true}, nil
		}
	}

	ns := &corev1.Namespace{}
	err = r.Get(context.Background(), types.NamespacedName{Name: cd.Spec.MachineManagement.TargetNamespace}, ns)
	switch {
	case apierrors.IsNotFound(err):
		cdLog.Info("Creating MachineManagement.TargetNamespace ", cd.Spec.MachineManagement.TargetNamespace)
		ns.Name = cd.Spec.MachineManagement.TargetNamespace
		if err := r.Create(context.TODO(), ns); err != nil && !apierrors.IsAlreadyExists(err) {
			return reconcile.Result{}, errors.Wrapf(err, "failed to create MachineManagement.TargetNamespace: %q", cd.Spec.MachineManagement.TargetNamespace)
		}
	case err != nil:
		return reconcile.Result{}, errors.Wrapf(err, "failed to get namespace")
	}

	// Ensure targetNamespace has machine management annotation
	if err := r.addAnnotationToTargetNamespace(cd, cdLog, cd.Spec.MachineManagement.TargetNamespace); err != nil {
		return reconcile.Result{}, err
	}

	// Sync credentials secret to targetNamespace
	if err := r.createOrUpdateSecretInTargetNamespace(utils.CredentialsSecretName(cd), cd, cdLog); err != nil {
		return reconcile.Result{}, err
	}

	// Sync pull secret to targetNamespace
	if err := r.createOrUpdateSecretInTargetNamespace(cd.Spec.PullSecretRef.Name, cd, cdLog); err != nil {
		return reconcile.Result{}, err
	}

	// Sync SSH key secret to targetNamespace
	if cd.Spec.Provisioning != nil && cd.Spec.Provisioning.SSHPrivateKeySecretRef != nil {
		if err := r.createOrUpdateSecretInTargetNamespace(cd.Spec.Provisioning.SSHPrivateKeySecretRef.Name, cd, cdLog); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

// createOrUpdateSecretInTargetNamespace
func (r *ReconcileMachineManagement) createOrUpdateSecretInTargetNamespace(secretName string, cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) error {
	targetNamespace := cd.Spec.MachineManagement.TargetNamespace

	secret := &corev1.Secret{}
	err := r.Get(context.Background(), types.NamespacedName{Name: secretName, Namespace: targetNamespace}, secret)
	// Create secret in targetNamespace

	if err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to get secret: %s", secretName)
		}

		cdLog.WithFields(log.Fields{"secret": secretName, "targetNamespace": targetNamespace}).Info("Creating secret in the target namespace")
		if err := r.Get(context.Background(), types.NamespacedName{Namespace: cd.Namespace, Name: secretName}, secret); err != nil {
			return errors.Wrapf(err, "failed to get secret: %s", secretName)
		}
		secret.Namespace = targetNamespace
		secret.ResourceVersion = ""
		if err := r.Create(context.TODO(), secret); err != nil {
			return errors.Wrap(err, "failed to create secret in target namespace")
		}
		return nil
	}

	origSecret := &corev1.Secret{}
	err = r.Get(context.Background(), types.NamespacedName{Name: secretName, Namespace: cd.Namespace}, origSecret)
	if err != nil {
		return errors.Wrapf(err, "failed to get secret: %s", secretName)
	}
	if reflect.DeepEqual(origSecret.Data, secret.Data) {
		return nil
	}

	// Update secret in targetNamespace
	cdLog.Infof("Updating secret %s in the target namespace %s", secretName, targetNamespace)
	secret.Data = origSecret.Data
	err = r.Update(context.Background(), secret)
	if err != nil {
		return errors.Wrapf(err, "failed to update secret: %s", secretName)
	}

	return nil
}

// addAnnotationToTargetNamespace adds a constants.MachineManagementAnnotation=cd.Name annotation to target namespace
// constants.MachineManagementAnnotation=cd.Name is used by the Cluster API to know to watch the target namespace
func (r *ReconcileMachineManagement) addAnnotationToTargetNamespace(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger, name string) error {
	cdLog = cdLog.WithField("namespace", name)

	namespace := &corev1.Namespace{}
	if err := r.Get(context.Background(), types.NamespacedName{Name: name}, namespace); err != nil {
		cdLog.WithError(err).Error("failed to get namespace")
		return err
	}

	if namespace.Annotations[constants.MachineManagementAnnotation] != cd.Name {
		cdLog.Debug("Setting annotation on target namespace")
		namespace.Annotations = k8sannotations.AddAnnotation(namespace.Annotations, constants.MachineManagementAnnotation, cd.Name)

		cdLog.Info("namespace has been modified, updating")
		if err := r.Update(context.TODO(), namespace); err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error updating namespace")
			return err
		}
	}

	return nil
}
