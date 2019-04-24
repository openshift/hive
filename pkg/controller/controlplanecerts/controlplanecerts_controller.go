/*
Copyright 2018 The Kubernetes Authors.

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

package controlplanecerts

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

	configv1 "github.com/openshift/api/config/v1"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/resource"
)

const (
	controllerName           = "controlPlaneCerts"
	openshiftConfigNamespace = "openshift-config"

	certsNotFoundReason  = "ControlPlaneCertificatesNotFound"
	certsNotFoundMessage = "One or more serving certificates for the cluster control plane are missing"
	certsFoundReason     = "ControlPlaneCertificatesFound"
	certsFoundMessage    = "Control plane certificates are present"
)

var (
	secretCheckInterval = 2 * time.Minute
)

type applier interface {
	ApplyRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (resource.ApplyResult, error)
}

// Add creates a new ControlPlaneCerts Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return AddToManager(mgr, NewReconciler(mgr))
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager) reconcile.Reconciler {
	logger := log.WithField("controller", controllerName)
	helper := resource.NewHelperFromRESTConfig(mgr.GetConfig(), logger)
	return &ReconcileControlPlaneCerts{
		Client:  mgr.GetClient(),
		scheme:  mgr.GetScheme(),
		applier: helper,
	}
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("controlplanecerts-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: 100})
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

var _ reconcile.Reconciler = &ReconcileControlPlaneCerts{}

// ReconcileControlPlaneCerts reconciles a ClusterDeployment object
type ReconcileControlPlaneCerts struct {
	client.Client
	scheme  *runtime.Scheme
	applier applier
}

// Reconcile reads that state of the cluster for a ClusterDeployment object and makes changes based on the state read
// and what is in the ClusterDeployment.Spec
func (r *ReconcileControlPlaneCerts) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the ClusterDeployment instance
	cd := &hivev1.ClusterDeployment{}
	err := r.Get(context.TODO(), request.NamespacedName, cd)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	cdLog := log.WithFields(log.Fields{
		"clusterDeployment": request.NamespacedName.String(),
		"controller":        controllerName,
	})

	cdLog.Info("reconciling cluster deployment")
	existingSyncSet := &hivev1.SyncSet{}
	existingSyncSetNamespacedName := types.NamespacedName{Namespace: cd.Namespace, Name: controlPlaneCertsSyncSetName(cd.Name)}
	err = r.Get(context.TODO(), existingSyncSetNamespacedName, existingSyncSet)
	if err != nil && !errors.IsNotFound(err) {
		cdLog.WithError(err).Error("failed to retrieve existing control plane certs syncset")
		return reconcile.Result{}, err
	}
	if errors.IsNotFound(err) {
		existingSyncSet = nil
	}

	secrets, secretsAvailable, err := r.getControlPlaneSecrets(cd, cdLog)
	if err != nil {
		cdLog.WithError(err).Error("failed to check cert secret availability")
		return reconcile.Result{}, err
	}
	if !secretsAvailable {
		cdLog.Debug("cert secrets are not available yet, setting condition on clusterdeployment")
		updated, err := r.setCertsNotFoundCondition(cd, true, cdLog)
		if err != nil {
			cdLog.WithError(err).Error("cannot update cluster deployment secrets not found condition")
			return reconcile.Result{}, err
		}
		if updated {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{Requeue: true, RequeueAfter: secretCheckInterval}, nil
	}

	// clear condition if certs were found
	updated, err := r.setCertsNotFoundCondition(cd, false, cdLog)
	if err != nil {
		cdLog.WithError(err).Error("cannot update cluster deployment secrets not found condition")
		return reconcile.Result{}, err
	}
	if updated {
		return reconcile.Result{}, nil
	}

	if len(secrets) == 0 && existingSyncSet == nil {
		cdLog.Debug("no control plane certs needed, and no syncset exists, nothing to do")
		return reconcile.Result{}, nil
	}

	desiredSyncSet, err := r.generateControlPlaneCertsSyncSet(cd, secrets, cdLog)
	if err != nil {
		cdLog.WithError(err).Error("failed to generate control plane certs syncset")
		return reconcile.Result{}, err
	}

	if _, err = r.applier.ApplyRuntimeObject(desiredSyncSet, r.scheme); err != nil {
		cdLog.WithError(err).Error("failed to apply control plane certificates syncset")
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileControlPlaneCerts) getControlPlaneSecrets(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) ([]*corev1.Secret, bool, error) {
	secretsNeeded, err := getControlPlaneSecretNames(cd, cdLog)
	if err != nil {
		return nil, false, err
	}
	if len(secretsNeeded) == 0 {
		cdLog.Debug("the control plane does not require any cert bundles")
		return nil, true, nil
	}
	secrets := []*corev1.Secret{}
	for _, secretName := range secretsNeeded {
		secret := &corev1.Secret{}
		err := r.Get(context.TODO(), types.NamespacedName{Namespace: cd.Namespace, Name: secretName}, secret)
		if err != nil {
			if errors.IsNotFound(err) {
				cdLog.WithField("secret", secretName).Debug("certificate secret is not available yet, will check later")
				return nil, false, nil
			}
			cdLog.WithError(err).WithField("secret", secretName).Debug("error retrieving certificate secret")
			return nil, false, err
		}
		secrets = append(secrets, secret)
	}

	cdLog.Debug("all required certificate secrets are available")
	return secrets, true, nil
}

func getControlPlaneSecretNames(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) ([]string, error) {
	certs := sets.NewString()
	if cd.Spec.ControlPlaneConfig.ServingCertificates.Default != "" {
		certs.Insert(cd.Spec.ControlPlaneConfig.ServingCertificates.Default)
	}

	for _, additional := range cd.Spec.ControlPlaneConfig.ServingCertificates.Additional {
		certs.Insert(additional.Name)
	}
	if certs.Len() == 0 {
		return nil, nil
	}
	cdLog.WithField("certbundles", certs.List()).Debug("cert bundles used by the control plane")

	secretsNeeded := sets.NewString()
	for _, cert := range certs.List() {
		bundle := certificateBundle(cd, cert)
		if bundle == nil {
			// should not happen if clusterdeployment was validated
			return nil, fmt.Errorf("no certificate bundle was found for %s", cert)
		}
		secretsNeeded.Insert(bundle.SecretRef.Name)
	}
	cdLog.WithField("secrets", secretsNeeded.List()).Debug("certificate secrets needed by the control plane")
	return secretsNeeded.List(), nil
}

func (r *ReconcileControlPlaneCerts) generateControlPlaneCertsSyncSet(cd *hivev1.ClusterDeployment, secrets []*corev1.Secret, cdLog log.FieldLogger) (*hivev1.SyncSet, error) {
	cdLog.Debug("generating syncset for control plane secrets")
	syncSet := &hivev1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      controlPlaneCertsSyncSetName(cd.Name),
			Namespace: cd.Namespace,
		},
		Spec: hivev1.SyncSetSpec{
			SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
				ResourceApplyMode: hivev1.SyncResourceApplyMode,
			},
			ClusterDeploymentRefs: []corev1.LocalObjectReference{
				{
					Name: cd.Name,
				},
			},
		},
	}
	resources := []runtime.RawExtension{}
	for _, secret := range secrets {
		cdLog.WithField("secret", secret.Name).Debug("adding secret to resource list")
		targetSecret := getCertSecret(secret, cd)
		resources = append(resources, runtime.RawExtension{Object: targetSecret})
	}
	apiServerConfig := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
	}
	if cd.Spec.ControlPlaneConfig.ServingCertificates.Default != "" {
		cdLog.Debug("setting default serving certificate for control plane")
		bundle := certificateBundle(cd, cd.Spec.ControlPlaneConfig.ServingCertificates.Default)
		apiServerConfig.Spec.ServingCerts.DefaultServingCertificate.Name = remoteSecretName(bundle.SecretRef.Name, cd)
	}
	for _, additional := range cd.Spec.ControlPlaneConfig.ServingCertificates.Additional {
		cdLog.WithField("name", additional.Name).Debug("adding named certificate to control plane config")
		bundle := certificateBundle(cd, additional.Name)
		apiServerConfig.Spec.ServingCerts.NamedCertificates = append(apiServerConfig.Spec.ServingCerts.NamedCertificates, configv1.APIServerNamedServingCert{
			Names: []string{additional.Domain},
			ServingCertificate: configv1.SecretNameReference{
				Name: remoteSecretName(bundle.SecretRef.Name, cd),
			},
		})
	}
	resources = append(resources, runtime.RawExtension{Object: apiServerConfig})

	var err error
	resources, err = controllerutils.AddTypeMeta(resources, r.scheme)
	if err != nil {
		cdLog.WithError(err).Error("cannot add typemeta to syncset resources")
		return nil, err
	}
	syncSet.Spec.Resources = resources

	// ensure the syncset gets cleaned up when the clusterdeployment is deleted
	if err := controllerutil.SetControllerReference(cd, syncSet, r.scheme); err != nil {
		cdLog.WithError(err).Error("error setting owner reference")
		return nil, err
	}

	return syncSet, nil
}

func (r *ReconcileControlPlaneCerts) setCertsNotFoundCondition(cd *hivev1.ClusterDeployment, notFound bool, cdLog log.FieldLogger) (bool, error) {

	origCD := cd.DeepCopy()

	var status corev1.ConditionStatus
	var reason, message string

	if notFound {
		status = corev1.ConditionTrue
		reason = certsNotFoundReason
		message = certsNotFoundMessage
	} else {
		status = corev1.ConditionFalse
		reason = certsFoundReason
		message = certsFoundMessage
	}

	cd.Status.Conditions = controllerutils.SetClusterDeploymentCondition(
		cd.Status.Conditions,
		hivev1.ControlPlaneCertificateNotFoundCondition,
		status,
		reason,
		message,
		controllerutils.UpdateConditionNever,
	)

	if reflect.DeepEqual(origCD.Status, cd.Status) {
		return false, nil
	}
	return true, r.Status().Update(context.TODO(), cd)
}

// getCertSecret generates a secret to be created on the target cluster based
// on a local secret.
func getCertSecret(secret *corev1.Secret, cd *hivev1.ClusterDeployment) *corev1.Secret {
	targetSecret := secret.DeepCopy()
	// Only copy over metadata fields that are not generated by the system
	// and set namespace to the target namespace
	targetSecret.ObjectMeta = metav1.ObjectMeta{
		Name:        remoteSecretName(secret.Name, cd),
		Namespace:   openshiftConfigNamespace,
		Labels:      secret.Labels,
		Annotations: secret.Annotations,
	}
	return targetSecret
}

func remoteSecretName(secretName string, cd *hivev1.ClusterDeployment) string {
	return fmt.Sprintf("%s-%s", cd.Name, secretName)
}

func certificateBundle(cd *hivev1.ClusterDeployment, name string) *hivev1.CertificateBundleSpec {
	for i, bundle := range cd.Spec.CertificateBundles {
		if bundle.Name == name {
			return &cd.Spec.CertificateBundles[i]
		}
	}
	return nil
}

func controlPlaneCertsSyncSetName(name string) string {
	return fmt.Sprintf("%s-cp-certs", name)
}
