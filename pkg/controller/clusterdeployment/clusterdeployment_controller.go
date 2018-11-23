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

package clusterdeployment

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"reflect"
	"time"

	"github.com/pborman/uuid"
	log "github.com/sirupsen/logrus"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	kapi "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/clientcmd"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/install"
)

const (
	// serviceAccountName will be a service account that can run the installer and then
	// upload artifacts to the cluster's namespace.
	serviceAccountName = "cluster-installer"
	roleName           = "cluster-installer"
	roleBindingName    = "cluster-installer"

	// deleteAfterAnnotation is the annotation that contains a duration after which the cluster should be cleaned up.
	deleteAfterAnnotation = "hive.openshift.io/delete-after"

	adminCredsSecretPasswordKey = "password"
	pullSecretKey               = ".dockercfg"
	adminSSHKeySecretKey        = "ssh-publickey"
)

// Add creates a new ClusterDeployment Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return AddToManager(mgr, NewReconciler(mgr))
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileClusterDeployment{Client: mgr.GetClient(), scheme: mgr.GetScheme(), amiLookupFunc: lookupAMI}
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("clusterdeployment-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterDeployment
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for jobs created by a ClusterDeployment:
	err = c.Watch(&source.Kind{Type: &batchv1.Job{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &hivev1.ClusterDeployment{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileClusterDeployment{}

// ReconcileClusterDeployment reconciles a ClusterDeployment object
type ReconcileClusterDeployment struct {
	client.Client
	scheme        *runtime.Scheme
	amiLookupFunc func(cd *hivev1.ClusterDeployment) (string, error)
}

// Reconcile reads that state of the cluster for a ClusterDeployment object and makes changes based on the state read
// and what is in the ClusterDeployment.Spec
//
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
//
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=serviceaccounts;secrets;configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hive.openshift.io,resources=clusterdeployments;clusterdeployments/status;clusterdeployments/finalizers,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileClusterDeployment) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the ClusterDeployment instance
	cd := &hivev1.ClusterDeployment{}
	err := r.Get(context.TODO(), request.NamespacedName, cd)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}
	cdLog := log.WithFields(log.Fields{
		"clusterDeployment": cd.Name,
		"namespace":         cd.Namespace,
	})
	cdLog.Info("reconciling cluster deployment")
	cd = cd.DeepCopy()

	if cd.Spec.ClusterUUID == "" {
		return reconcile.Result{}, r.setClusterUUID(cd, cdLog)
	}

	_, err = r.setupClusterInstallServiceAccount(cd.Namespace, cdLog)
	if err != nil {
		cdLog.WithError(err).Error("error setting up service account and role")
		return reconcile.Result{}, err
	}

	if cd.DeletionTimestamp != nil {
		if !HasFinalizer(cd, hivev1.FinalizerDeprovision) {
			return reconcile.Result{}, nil
		}
		return r.syncDeletedClusterDeployment(cd, cdLog)
	}

	// requeueAfter will be used to determine if cluster should be requeued after
	// reconcile has completed
	var requeueAfter time.Duration
	// Check for the delete-after annotation, and if the cluster has expired, delete it
	deleteAfter, ok := cd.Annotations[deleteAfterAnnotation]
	if ok {
		cdLog.Debugf("found delete after annotation: %s", deleteAfter)
		dur, err := time.ParseDuration(deleteAfter)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("error parsing %s as a duration: %v", deleteAfterAnnotation, err)
		}
		if !cd.CreationTimestamp.IsZero() {
			expiry := cd.CreationTimestamp.Add(dur)
			cdLog.Debugf("cluster expires at: %s", expiry)
			if time.Now().After(expiry) {
				cdLog.WithField("expiry", expiry).Info("cluster has expired, issuing delete")
				r.Delete(context.TODO(), cd)
				return reconcile.Result{}, nil
			}

			// We have an expiry time but we're not expired yet. Set requeueAfter for just after expiry time
			// so that we requeue cluster for deletion once reconcile has completed
			requeueAfter = expiry.Sub(time.Now()) + 60*time.Second
		}
	}

	if !HasFinalizer(cd, hivev1.FinalizerDeprovision) {
		cdLog.Debugf("adding clusterdeployment finalizer")
		return reconcile.Result{}, r.addClusterDeploymentFinalizer(cd)
	}

	// Ensure we have an AMI set, if not lookup the latest:
	if cd.Spec.Config.Platform.AWS != nil && !isDefaultAMISet(cd) {
		cdLog.Debugf("looking up a default AMI for cluster")
		if cd.Spec.Config.Platform.AWS.DefaultMachinePlatform == nil {
			cd.Spec.Config.AWS.DefaultMachinePlatform = &hivev1.AWSMachinePoolPlatform{}
		}
		ami, err := r.amiLookupFunc(cd)
		if err != nil {
			return reconcile.Result{}, err
		}
		cd.Spec.Config.AWS.DefaultMachinePlatform.AMIID = ami
		cdLog.WithField("AMI", ami).Infof("set default machine platform AMI")
		return reconcile.Result{}, r.Update(context.TODO(), cd)
	}

	cdLog.Debug("loading admin secret")
	adminPassword, err := r.loadSecretData(cd.Spec.Config.Admin.Password.Name,
		cd.Namespace, adminCredsSecretPasswordKey)
	if err != nil {
		cdLog.WithError(err).Error("unable to load admin password from secret")
		return reconcile.Result{}, err
	}

	cdLog.Debug("loading SSH key secret")
	sshKey, err := r.loadSecretData(cd.Spec.Config.Admin.SSHKey.Name,
		cd.Namespace, adminSSHKeySecretKey)
	if err != nil {
		cdLog.WithError(err).Error("unable to load ssh key from secret")
		return reconcile.Result{}, err
	}

	cdLog.Debug("loading pull secret secret")
	pullSecret, err := r.loadSecretData(cd.Spec.Config.PullSecret.Name, cd.Namespace, pullSecretKey)
	if err != nil {
		cdLog.WithError(err).Error("unable to load pull secret from secret")
		return reconcile.Result{}, err
	}

	job, cfgMap, err := install.GenerateInstallerJob(
		cd,
		serviceAccountName,
		adminPassword,
		sshKey,
		pullSecret)
	if err != nil {
		cdLog.WithError(err).Error("error generating install job")
		return reconcile.Result{}, err
	}

	if err := controllerutil.SetControllerReference(cd, job, r.scheme); err != nil {
		cdLog.Errorf("error setting controller reference on job", err)
		return reconcile.Result{}, err
	}
	if err := controllerutil.SetControllerReference(cd, cfgMap, r.scheme); err != nil {
		cdLog.Errorf("error setting controller reference on config map", err)
		return reconcile.Result{}, err
	}

	cdLog = cdLog.WithField("job", job.Name)

	cdLog.Debug("checking if install-config.yml config map exists")
	// Check if the ConfigMap already exists for this ClusterDeployment:
	existingCfgMap := &kapi.ConfigMap{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: cfgMap.Name, Namespace: cfgMap.Namespace}, existingCfgMap)
	if err != nil && errors.IsNotFound(err) {
		cdLog.WithField("configMap", cfgMap.Name).Infof("creating config map")
		err = r.Create(context.TODO(), cfgMap)
		if err != nil {
			cdLog.Errorf("error creating config map: %v", err)
			return reconcile.Result{}, err
		}
	} else if err != nil {
		cdLog.Errorf("error getting config map: %v", err)
		return reconcile.Result{}, err
	}

	// Check if the Job already exists for this ClusterDeployment:
	existingJob := &batchv1.Job{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, existingJob)
	if err != nil && errors.IsNotFound(err) {
		// If the ClusterDeployment is already installed, we do not need to create a new job:
		if cd.Status.Installed {
			cdLog.Debug("cluster is already installed, no job needed")
		} else {
			cdLog.Infof("creating install job")
			err = r.Create(context.TODO(), job)
			if err != nil {
				cdLog.Errorf("error creating job: %v", err)
				return reconcile.Result{}, err
			}
		}
	} else if err != nil {
		cdLog.Errorf("error getting job: %v", err)
		return reconcile.Result{}, err
	} else {
		cdLog.Infof("cluster job exists, successful: %v", cd.Status.Installed)
	}

	err = r.updateClusterDeploymentStatus(cd, existingJob, cdLog)
	if err != nil {
		cdLog.WithError(err).Errorf("error updating cluster deployment status")
		return reconcile.Result{}, err
	}

	cdLog.Debugf("reconcile complete")
	// Check for requeueAfter duration
	if requeueAfter != 0 {
		cdLog.Debugf("cluster will re-sync due to expiry time in: %v", requeueAfter)
		return reconcile.Result{Requeue: true, RequeueAfter: requeueAfter}, nil
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileClusterDeployment) loadSecretData(secretName, namespace, dataKey string) (string, error) {
	s := &kapi.Secret{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: namespace}, s)
	if err != nil {
		return "", err
	}
	retStr, ok := s.Data[dataKey]
	if !ok {
		return "", fmt.Errorf("secret %s did not contain key %s", secretName, dataKey)
	}
	return string(retStr), nil
}

func (r *ReconcileClusterDeployment) updateClusterDeploymentStatus(cd *hivev1.ClusterDeployment, job *batchv1.Job, cdLog log.FieldLogger) error {
	cdLog.Debug("updating cluster deployment status")
	origCD := cd
	cd = cd.DeepCopy()
	if job != nil && job.Name != "" && job.Namespace != "" {
		// Job exists, check it's status:
		cd.Status.Installed = isSuccessful(job)
	}

	// The install manager sets this secret name, but we don't consider it a critical failure and
	// will attempt to heal it here, as the value is predictable.
	if cd.Status.Installed && cd.Status.AdminKubeconfigSecret.Name == "" {
		cd.Status.AdminKubeconfigSecret = corev1.LocalObjectReference{Name: fmt.Sprintf("%s-admin-kubeconfig", cd.Name)}
	}

	if cd.Status.AdminKubeconfigSecret.Name != "" &&
		(cd.Status.WebConsoleURL == "" || cd.Status.APIURL == "") {

		adminKubeconfigSecret := &corev1.Secret{}
		err := r.Get(context.Background(), types.NamespacedName{Namespace: cd.Namespace, Name: cd.Status.AdminKubeconfigSecret.Name}, adminKubeconfigSecret)
		if err != nil {
			return err
		}

		// Parse the admin kubeconfig for the server URL:
		config, err := clientcmd.Load(adminKubeconfigSecret.Data["kubeconfig"])
		if err != nil {
			return err
		}
		cluster, ok := config.Clusters[cd.Name]
		if !ok {
			return fmt.Errorf("error parsing admin kubeconfig secret data")
		}

		// We should be able to assume only one cluster in here:
		server := cluster.Server
		cdLog.Debugf("found cluster API URL in kubeconfig: %s", server)
		u, err := url.Parse(server)
		if err != nil {
			return err
		}
		cd.Status.APIURL = server
		u.Path = path.Join(u.Path, "console")
		cd.Status.WebConsoleURL = u.String()
	}

	// Update cluster deployment status if changed:
	if !reflect.DeepEqual(cd.Status, origCD.Status) {
		cdLog.Infof("status has changed, updating cluster deployment")
		cdLog.Debugf("orig: %v", origCD)
		cdLog.Debugf("new : %v", cd.Status)
		err := r.Status().Update(context.TODO(), cd)
		if err != nil {
			cdLog.Errorf("error updating cluster deployment: %v", err)
			return err
		}
	} else {
		cdLog.Infof("cluster deployment status unchanged")
	}

	return nil
}

// setClusterUUID sets the
func (r *ReconcileClusterDeployment) setClusterUUID(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) error {
	cdLog.Debug("setting cluster UUID")
	cd = cd.DeepCopy()

	if cd.Spec.ClusterUUID != "" {
		return fmt.Errorf("cluster UUID already set")
	}

	cd.Spec.ClusterUUID = uuid.New()
	cdLog.WithField("clusterUUID", cd.Spec.ClusterUUID).Info("generated new cluster UUID")
	err := r.Update(context.TODO(), cd)
	if err != nil {
		cdLog.WithError(err).Errorf("error updating cluster deployment")
		return err
	}

	return nil
}
func (r *ReconcileClusterDeployment) syncDeletedClusterDeployment(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (reconcile.Result, error) {

	// Delete the install job in case it's still running:
	installJob := &batchv1.Job{}
	err := r.Get(context.Background(),
		types.NamespacedName{
			Name:      install.GetInstallJobName(cd),
			Namespace: cd.Namespace,
		},
		installJob)
	if err != nil && errors.IsNotFound(err) {
		cdLog.Debug("install job no longer exists, nothing to cleanup")
	} else if err != nil {
		cdLog.WithError(err).Errorf("error getting existing install job for deleted cluster deployment")
		return reconcile.Result{}, err
	} else {
		err = r.Delete(context.Background(), installJob,
			client.PropagationPolicy(metav1.DeletePropagationForeground))
		if err != nil {
			cdLog.WithError(err).Errorf("error deleting existing install job for deleted cluster deployment")
			return reconcile.Result{}, err
		}
		cdLog.WithField("jobName", installJob.Name).Info("install job deleted")
	}

	// Generate an uninstall job:
	uninstallJob, err := install.GenerateUninstallerJob(cd)
	if err != nil {
		cdLog.Errorf("error generating uninstaller job: %v", err)
		return reconcile.Result{}, err
	}

	err = controllerutil.SetControllerReference(cd, uninstallJob, r.scheme)
	if err != nil {
		cdLog.Errorf("error setting controller reference on job: %v", err)
		return reconcile.Result{}, err
	}

	// Check if uninstall job already exists:
	existingJob := &batchv1.Job{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: uninstallJob.Name, Namespace: uninstallJob.Namespace}, existingJob)
	if err != nil && errors.IsNotFound(err) {
		err = r.Create(context.TODO(), uninstallJob)
		if err != nil {
			cdLog.Errorf("error creating uninstall job: %v", err)
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	} else if err != nil {
		cdLog.Errorf("error getting uninstall job: %v", err)
		return reconcile.Result{}, err
	}

	// Uninstall job exists, check it's status and if successful, remove the finalizer:
	if isSuccessful(existingJob) {
		cdLog.Infof("uninstall job successful, removing finalizer")
		return reconcile.Result{}, r.removeClusterDeploymentFinalizer(cd)
	}

	cdLog.Infof("uninstall job not yet successful")
	return reconcile.Result{}, nil
}

func (r *ReconcileClusterDeployment) addClusterDeploymentFinalizer(cd *hivev1.ClusterDeployment) error {
	cd = cd.DeepCopy()
	AddFinalizer(cd, hivev1.FinalizerDeprovision)
	return r.Update(context.TODO(), cd)
}

func (r *ReconcileClusterDeployment) removeClusterDeploymentFinalizer(cd *hivev1.ClusterDeployment) error {
	cd = cd.DeepCopy()
	DeleteFinalizer(cd, hivev1.FinalizerDeprovision)
	return r.Update(context.TODO(), cd)
}

// setupClusterInstallServiceAccount ensures a service account exists which can upload
// the required artifacts after running the installer in a pod. (metadata, admin kubeconfig)
func (r *ReconcileClusterDeployment) setupClusterInstallServiceAccount(namespace string, cdLog log.FieldLogger) (*kapi.ServiceAccount, error) {
	// create new serviceaccount if it doesn't already exist
	currentSA := &kapi.ServiceAccount{}
	err := r.Client.Get(context.Background(), client.ObjectKey{Name: serviceAccountName, Namespace: namespace}, currentSA)
	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("error checking for existing serviceaccount")
	}

	if errors.IsNotFound(err) {
		currentSA = &kapi.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceAccountName,
				Namespace: namespace,
			},
		}
		err = r.Client.Create(context.Background(), currentSA)

		if err != nil {
			return nil, fmt.Errorf("error creating serviceaccount: %v", err)
		}
		cdLog.WithField("name", serviceAccountName).Info("created service account")
	} else {
		cdLog.WithField("name", serviceAccountName).Debug("service account already exists")
	}

	expectedRole := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"secrets", "configmaps"},
				Verbs:     []string{"create", "delete", "get", "list", "update"},
			},
			{
				APIGroups: []string{"hive.openshift.io"},
				Resources: []string{"clusterdeployments", "clusterdeployments/finalizers", "clusterdeployments/status"},
				Verbs:     []string{"create", "delete", "get", "list", "update"},
			},
		},
	}
	currentRole := &rbacv1.Role{}
	err = r.Client.Get(context.Background(), client.ObjectKey{Name: roleName, Namespace: namespace}, currentRole)
	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("error checking for existing role: %v", err)
	}
	if errors.IsNotFound(err) {
		err = r.Client.Create(context.Background(), expectedRole)
		if err != nil {
			return nil, fmt.Errorf("error creating role: %v", err)
		}
		cdLog.WithField("name", roleName).Info("created role")
	} else {
		cdLog.WithField("name", roleName).Debug("role already exists")
	}

	// create rolebinding for the serviceaccount
	currentRB := &rbacv1.RoleBinding{}
	err = r.Client.Get(context.Background(), client.ObjectKey{Name: roleBindingName, Namespace: namespace}, currentRB)

	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("error checking for existing rolebinding: %v", err)
	}

	if errors.IsNotFound(err) {
		rb := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      roleBindingName,
				Namespace: namespace,
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      currentSA.Name,
					Namespace: namespace,
				},
			},
			RoleRef: rbacv1.RoleRef{
				Name: roleName,
				Kind: "Role",
			},
		}

		err = r.Client.Create(context.Background(), rb)
		if err != nil {
			return nil, fmt.Errorf("error creating rolebinding: %v", err)
		}
		cdLog.WithField("name", roleBindingName).Info("created rolebinding")
	} else {
		cdLog.WithField("name", roleBindingName).Debug("rolebinding already exists")
	}

	return currentSA, nil
}

// getJobConditionStatus gets the status of the condition in the job. If the
// condition is not found in the job, then returns False.
func getJobConditionStatus(job *batchv1.Job, conditionType batchv1.JobConditionType) kapi.ConditionStatus {
	for _, condition := range job.Status.Conditions {
		if condition.Type == conditionType {
			return condition.Status
		}
	}
	return kapi.ConditionFalse
}

func isSuccessful(job *batchv1.Job) bool {
	return getJobConditionStatus(job, batchv1.JobComplete) == kapi.ConditionTrue
}

func isFailed(job *batchv1.Job) bool {
	return getJobConditionStatus(job, batchv1.JobFailed) == kapi.ConditionTrue
}

// HasFinalizer returns true if the given object has the given finalizer
func HasFinalizer(object metav1.Object, finalizer string) bool {
	for _, f := range object.GetFinalizers() {
		if f == finalizer {
			return true
		}
	}
	return false
}

// AddFinalizer adds a finalizer to the given object
func AddFinalizer(object metav1.Object, finalizer string) {
	finalizers := sets.NewString(object.GetFinalizers()...)
	finalizers.Insert(finalizer)
	object.SetFinalizers(finalizers.List())
}

// DeleteFinalizer removes a finalizer from the given object
func DeleteFinalizer(object metav1.Object, finalizer string) {
	finalizers := sets.NewString(object.GetFinalizers()...)
	finalizers.Delete(finalizer)
	object.SetFinalizers(finalizers.List())
}
