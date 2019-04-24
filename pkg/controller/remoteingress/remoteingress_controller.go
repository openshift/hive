/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package remoteingress

import (
	"context"
	"fmt"
	"reflect"
	"time"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	ingresscontroller "github.com/openshift/api/operator/v1"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/resource"
)

const (
	controllerName = "remoteingress"

	// namespace where the ingressController objects must be created
	remoteIngressControllerNamespace = "openshift-ingress-operator"

	// while the IngressController objects live in openshift-ingress-operator
	// the secrets that the ingressControllers refer to must live in openshift-ingress
	remoteIngressConrollerSecretsNamespace = "openshift-ingress"

	ingressCertificateNotFoundReason = "IngressCertificateNotFound"
	ingressCertificateFoundReason    = "IngressCertificateFound"

	// requeueAfter2 is just a static 2 minute delay for when to requeue
	// for the case when a necessary secret is missing
	requeueAfter2 = time.Minute * 2
)

// kubeCLIApplier knows how to ApplyRuntimeObject.
type kubeCLIApplier interface {
	ApplyRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (resource.ApplyResult, error)
}

// Add creates a new RemoteMachineSet Controller and adds it to the Manager with default RBAC. The Manager will set fields on the
// Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return AddToManager(mgr, NewReconciler(mgr))
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager) reconcile.Reconciler {
	logger := log.WithField("controller", controllerName)
	helper := resource.NewHelperFromRESTConfig(mgr.GetConfig(), logger)
	return &ReconcileRemoteClusterIngress{
		Client:  mgr.GetClient(),
		scheme:  mgr.GetScheme(),
		logger:  log.WithField("controller", controllerName),
		kubeCLI: helper,
	}
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("remoteingress-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: 100})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterDeployment
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

type reconcileContext struct {
	clusterDeployment *hivev1.ClusterDeployment
	certBundleSecrets []*corev1.Secret
	logger            log.FieldLogger
}

var _ reconcile.Reconciler = &ReconcileRemoteClusterIngress{}

// ReconcileRemoteClusterIngress reconciles the ingress objects defined in a ClusterDeployment object
type ReconcileRemoteClusterIngress struct {
	client.Client
	scheme  *runtime.Scheme
	kubeCLI kubeCLIApplier

	logger log.FieldLogger
}

// Reconcile reads that state of the cluster for a ClusterDeployment object and sets up
// any needed ClusterIngress objects up for syncing to the remote cluster.
//
// +kubebuilder:rbac:groups=hive.openshift.io,resources=clusterdeployments,verbs=get;watch;update
// +kubebuilder:rbac:groups=hive.openshift.io,resources=syncsets,verbs=get;create;update;delete;patch;list;watch
func (r *ReconcileRemoteClusterIngress) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	rContext := &reconcileContext{}

	// Fetch the ClusterDeployment instance
	cd := &hivev1.ClusterDeployment{}
	err := r.Get(context.TODO(), request.NamespacedName, cd)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found (must have been deleted), return
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request
		log.WithError(err).Error("error looking up cluster deployment")
		return reconcile.Result{}, err
	}
	rContext.clusterDeployment = cd

	cdLog := r.logger.WithFields(log.Fields{
		"clusterDeployment": cd.Name,
		"namespace":         cd.Namespace,
	})
	rContext.logger = cdLog

	if cd.Spec.Ingress == nil {
		// the addmission controller will ensure that we get valid-looking
		// Spec.Ingress (ie no missing 'default', no going from a defined
		// ingress list to an empty list, etc)
		rContext.logger.Debug("no ingress objects defined. using default intaller behavior.")
		return reconcile.Result{}, nil
	}

	// can't proceed if the secret(s) referred to doesn't exist
	certBundleSecrets, err := r.getIngressSecrets(rContext)
	if err != nil {
		rContext.logger.WithError(err).Error("will need to retry until able to find all certBundle secrets")
		conditionErr := r.setIngressCertificateNotFoundCondition(rContext, true, err.Error())
		if conditionErr != nil {
			rContext.logger.WithError(conditionErr).Error("unable to set IngressCertNotFound condition")
			return reconcile.Result{}, conditionErr
		}

		// no error return b/c we just need to wait for the certificate/secret to appear
		// which is out of our control.
		return reconcile.Result{
			Requeue:      true,
			RequeueAfter: requeueAfter2,
		}, nil
	}
	if err := r.setIngressCertificateNotFoundCondition(rContext, false, ""); err != nil {
		rContext.logger.WithError(err).Error("error setting clusterDeployment condition")
		return reconcile.Result{}, err
	}

	rContext.certBundleSecrets = certBundleSecrets

	if err := r.syncClusterIngress(rContext); err != nil {
		cdLog.Errorf("error syncing clusterIngress syncset: %v", err)
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// syncClusterIngress will create the syncSet with all the needed secrets and
// ingressController objects to sync to the remote cluster
func (r *ReconcileRemoteClusterIngress) syncClusterIngress(rContext *reconcileContext) error {
	rContext.logger.Info("reconciling ClusterIngress for cluster deployment")

	rawList := rawExtensionsFromClusterDeployment(rContext)

	return r.syncSyncSet(rContext, rawList)
}

// rawExtensionsFromClusterDeployment will return the slice of runtime.RawExtension objects
// (really the syncSet.Spec.Resources) to satisfy the ingress config for the clusterDeployment
func rawExtensionsFromClusterDeployment(rContext *reconcileContext) []runtime.RawExtension {
	rawList := []runtime.RawExtension{}

	// first the certBundle secrets
	for _, cbSecret := range rContext.certBundleSecrets {
		secret := createSecret(rContext, cbSecret)
		raw := runtime.RawExtension{Object: secret}
		rawList = append(rawList, raw)
	}

	// then the ingressControllers
	for _, ingress := range rContext.clusterDeployment.Spec.Ingress {
		ingressObj := createIngressController(rContext.clusterDeployment, ingress)
		raw := runtime.RawExtension{Object: ingressObj}
		rawList = append(rawList, raw)
	}

	return rawList
}

func newSyncSetSpec(cd *hivev1.ClusterDeployment, rawExtensions []runtime.RawExtension) *hivev1.SyncSetSpec {
	ssSpec := &hivev1.SyncSetSpec{
		SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
			Resources:         rawExtensions,
			ResourceApplyMode: "sync",
		},
		ClusterDeploymentRefs: []corev1.LocalObjectReference{
			{
				Name: cd.Name,
			},
		},
	}
	return ssSpec
}

// syncSyncSet builds up a syncSet object with the passed-in rawExtensions as the spec.Resources
func (r *ReconcileRemoteClusterIngress) syncSyncSet(rContext *reconcileContext, rawExtensions []runtime.RawExtension) error {
	ssName := fmt.Sprintf("%v-clusteringress", rContext.clusterDeployment.Name)

	newSyncSetSpec := newSyncSetSpec(rContext.clusterDeployment, rawExtensions)
	syncSet := &hivev1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ssName,
			Namespace: rContext.clusterDeployment.Namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "SyncSet",
			APIVersion: hivev1.SchemeGroupVersion.String(),
		},
		Spec: *newSyncSetSpec,
	}

	// ensure the syncset gets cleaned up when the clusterdeployment is deleted
	if err := controllerutil.SetControllerReference(rContext.clusterDeployment, syncSet, r.scheme); err != nil {
		r.logger.WithError(err).Error("error setting owner reference")
		return err
	}

	if _, err := r.kubeCLI.ApplyRuntimeObject(syncSet, r.scheme); err != nil {
		rContext.logger.WithError(err).Error("failed to apply syncset")
		return err
	}

	return nil
}

// createSecret returns the secret that needs to be synced to the remote cluster
// to satisfy any ingressController object that depends on the secret.
func createSecret(rContext *reconcileContext, cbSecret *corev1.Secret) *corev1.Secret {
	newSecret := cbSecret.DeepCopy()

	// don't want all the local object meta (eg creation/uuid/etc), so replace it
	// with a clean one with just the data we want.
	newSecret.ObjectMeta = metav1.ObjectMeta{
		Name:        remoteSecretNameForCertificateBundleSecret(cbSecret.Name, rContext.clusterDeployment),
		Namespace:   remoteIngressConrollerSecretsNamespace,
		Labels:      cbSecret.Labels,
		Annotations: cbSecret.Annotations,
	}

	return newSecret
}

// createIngressController will return an ingressController based on a clusterDeployment's
// spec.Ingress object
func createIngressController(cd *hivev1.ClusterDeployment, ingress hivev1.ClusterIngress) *ingresscontroller.IngressController {
	newIngress := ingresscontroller.IngressController{
		TypeMeta: metav1.TypeMeta{
			Kind:       "IngressController",
			APIVersion: ingresscontroller.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingress.Name,
			Namespace: remoteIngressControllerNamespace,
		},
		Spec: ingresscontroller.IngressControllerSpec{
			Domain:            ingress.Domain,
			RouteSelector:     ingress.RouteSelector,
			NamespaceSelector: ingress.NamespaceSelector,
		},
	}

	// if the ingress entry references a certBundle, make sure to put the appropriate looking
	// entry in the ingressController object
	if ingress.ServingCertificate != "" {
		for _, cb := range cd.Spec.CertificateBundles {
			// assume we're going to find the certBundle as we would've errored earlier
			if cb.Name == ingress.ServingCertificate {
				newIngress.Spec.DefaultCertificate = &corev1.LocalObjectReference{
					Name: remoteSecretNameForCertificateBundleSecret(cb.SecretRef.Name, cd),
				}
				break
			}
		}
	}

	return &newIngress
}

func (r *ReconcileRemoteClusterIngress) getIngressSecrets(rContext *reconcileContext) ([]*corev1.Secret, error) {
	certSet := sets.NewString()

	for _, ingress := range rContext.clusterDeployment.Spec.Ingress {
		if ingress.ServingCertificate != "" {
			certSet.Insert(ingress.ServingCertificate)
		}
	}

	cbSecrets := []*corev1.Secret{}

	for _, cert := range certSet.List() {
		foundCertBundle := false
		for _, cb := range rContext.clusterDeployment.Spec.CertificateBundles {
			if cb.Name == cert {
				foundCertBundle = true
				cbSecret := &corev1.Secret{}
				searchKey := types.NamespacedName{
					Name:      cb.SecretRef.Name,
					Namespace: rContext.clusterDeployment.Namespace,
				}

				if err := r.Get(context.TODO(), searchKey, cbSecret); err != nil {
					if errors.IsNotFound(err) {
						msg := fmt.Sprintf("secret %v for certbundle %v was not found", cb.SecretRef.Name, cb.Name)
						rContext.logger.Error(msg)
						return cbSecrets, fmt.Errorf(msg)
					}
					rContext.logger.WithError(err).Error("error while gathering certBundle secret")
					return cbSecrets, err
				}

				cbSecrets = append(cbSecrets, cbSecret)
			}
		}
		if !foundCertBundle {
			return cbSecrets, fmt.Errorf("didn't find expected certbundle %v", cert)
		}
	}

	return cbSecrets, nil
}

// setIngressCertificateNotFoundCondition will set/unset the condition indicating whether all certificates required
// by the clusterDeployment ingress objects were found. Returns any error encountered while setting the condition.
func (r *ReconcileRemoteClusterIngress) setIngressCertificateNotFoundCondition(rContext *reconcileContext, notFound bool, missingSecretMessage string) error {
	var (
		msg, reason string
		status      corev1.ConditionStatus
		updateCheck utils.UpdateConditionCheck
	)

	origCD := rContext.clusterDeployment.DeepCopy()

	if notFound {
		msg = missingSecretMessage
		status = corev1.ConditionTrue
		reason = ingressCertificateNotFoundReason
		updateCheck = utils.UpdateConditionIfReasonOrMessageChange
	} else {
		msg = fmt.Sprintf("all secrets for ingress found")
		status = corev1.ConditionFalse
		reason = ingressCertificateFoundReason
		updateCheck = utils.UpdateConditionNever
	}

	rContext.clusterDeployment.Status.Conditions = utils.SetClusterDeploymentCondition(rContext.clusterDeployment.Status.Conditions,
		hivev1.IngressCertificateNotFoundCondition, status, reason, msg, updateCheck)

	if !reflect.DeepEqual(rContext.clusterDeployment.Status.Conditions, origCD.Status.Conditions) {
		if err := r.Status().Update(context.TODO(), rContext.clusterDeployment); err != nil {
			rContext.logger.WithError(err).Error("error updating clusterDeployment condition")
			return err
		}
	}

	return nil
}

// remoteSecretNameForCertificateBundleSecret just stitches together a secret name consisting of
// the original certificateBundle's secret name pre-pended with the clusterDeployment.Name
func remoteSecretNameForCertificateBundleSecret(secretName string, cd *hivev1.ClusterDeployment) string {
	return fmt.Sprintf("%s-%s", cd.Name, secretName)
}
