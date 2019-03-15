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
	"reflect"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	kapi "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	routev1 "github.com/openshift/api/route/v1"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/controller/images"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/imageset"
	"github.com/openshift/hive/pkg/install"
)

const (
	// serviceAccountName will be a service account that can run the installer and then
	// upload artifacts to the cluster's namespace.
	serviceAccountName = "cluster-installer"
	roleName           = "cluster-installer"
	roleBindingName    = "cluster-installer"

	// deleteAfterAnnotation is the annotation that contains a duration after which the cluster should be cleaned up.
	deleteAfterAnnotation       = "hive.openshift.io/delete-after"
	adminCredsSecretPasswordKey = "password"
	pullSecretKey               = ".dockercfg"
	adminSSHKeySecretKey        = "ssh-publickey"
	adminKubeconfigKey          = "kubeconfig"
	clusterVersionObjectName    = "version"
	clusterVersionUnknown       = "undef"

	clusterDeploymentGenerationAnnotation = "hive.openshift.io/cluster-deployment-generation"
	clusterImageSetNotFoundReason         = "ClusterImageSetNotFound"
	clusterImageSetFoundReason            = "ClusterImageSetFound"
)

// Add creates a new ClusterDeployment Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return AddToManager(mgr, NewReconciler(mgr))
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileClusterDeployment{
		Client:                        mgr.GetClient(),
		scheme:                        mgr.GetScheme(),
		amiLookupFunc:                 lookupAMI,
		remoteClusterAPIClientBuilder: controllerutils.BuildClusterAPIClientFromKubeconfig,
	}
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

	// remoteClusterAPIClientBuilder is a function pointer to the function that builds a client for the
	// remote cluster's cluster-api
	remoteClusterAPIClientBuilder func(string) (client.Client, error)
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
// +kubebuilder:rbac:groups=hive.openshift.io,resources=clusterimagesets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hive.openshift.io,resources=clusterimagesets/status,verbs=get;update;patch
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
	origCD := cd
	cd = cd.DeepCopy()

	// TODO: We may want to remove this fix in future.
	// Handle pre-existing clusters with older status version structs that did not have the new
	// cluster version mandatory fields defined.
	controllerutils.FixupEmptyClusterVersionFields(&cd.Status.ClusterVersionStatus)
	if !reflect.DeepEqual(origCD.Status, cd.Status) {
		cdLog.Info("correcting empty cluster version fields")
		err := r.Status().Update(context.TODO(), cd)
		if err != nil {
			cdLog.WithError(err).Error("error updating cluster deployment")
		}
		return reconcile.Result{}, err
	}

	_, err = r.setupClusterInstallServiceAccount(cd.Namespace, cdLog)
	if err != nil {
		cdLog.WithError(err).Error("error setting up service account and role")
		return reconcile.Result{}, err
	}

	imageSet, err := r.getClusterImageSet(cd, cdLog)
	if err != nil {
		return reconcile.Result{}, err
	}

	hiveImage := r.getHiveImage(cd, imageSet, cdLog)
	releaseImage := r.getReleaseImage(cd, imageSet, cdLog)

	if cd.DeletionTimestamp != nil {
		if !controllerutils.HasFinalizer(cd, hivev1.FinalizerDeprovision) {
			return reconcile.Result{}, nil
		}
		return r.syncDeletedClusterDeployment(cd, hiveImage, cdLog)
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

	if !controllerutils.HasFinalizer(cd, hivev1.FinalizerDeprovision) {
		cdLog.Debugf("adding clusterdeployment finalizer")
		return reconcile.Result{}, r.addClusterDeploymentFinalizer(cd)
	}

	if cd.Spec.Platform.AWS != nil {
		if !isDefaultAMISet(cd) {
			// Ensure we have an AMI set, if not lookup the latest:
			// TODO: this will be obsolete by the design for OS image lifecycle, we will no
			// longer need to lookup, pin, and set on machine sets. In cluster components will do
			// this for us.
			cdLog.Debugf("looking up a default AMI for cluster")
			ami, err := r.amiLookupFunc(cd)
			if err != nil {
				cdLog.WithError(err).Error("error looking up default AMI for cluster")
				return reconcile.Result{}, err
			}
			setDefaultAMI(cd, ami)
			cdLog.WithField("AMI", ami).Infof("set default machine platform AMI")
			controllerutils.FixupEmptyClusterVersionFields(&cd.Status.ClusterVersionStatus)
			err = r.Update(context.TODO(), cd)
			if err != nil {
				cdLog.WithError(err).Error("error updating default machine annotation")
			}
			return reconcile.Result{}, err
		}
		cdLog.Debug("default AMI already set")
	}

	cdLog.Debug("loading SSH key secret")
	if cd.Spec.SSHKey == nil {
		cdLog.Error("cluster has no ssh key set, unable to launch install")
		return reconcile.Result{}, fmt.Errorf("cluster has no ssh key set, unable to launch install")
	}
	sshKey, err := r.loadSecretData(cd.Spec.SSHKey.Name,
		cd.Namespace, adminSSHKeySecretKey)
	if err != nil {
		cdLog.WithError(err).Error("unable to load ssh key from secret")
		return reconcile.Result{}, err
	}

	cdLog.Debug("loading pull secret secret")
	pullSecret, err := r.loadSecretData(cd.Spec.PullSecret.Name, cd.Namespace, pullSecretKey)
	if err != nil {
		cdLog.WithError(err).Error("unable to load pull secret from secret")
		return reconcile.Result{}, err
	}

	if cd.Status.InstallerImage == nil {
		return reconcile.Result{}, r.resolveInstallerImage(cd, hiveImage, cdLog)
	}

	job, cfgMap, err := install.GenerateInstallerJob(
		cd,
		hiveImage,
		releaseImage,
		serviceAccountName,
		sshKey,
		pullSecret)
	if err != nil {
		cdLog.WithError(err).Error("error generating install job")
		return reconcile.Result{}, err
	}

	if err = controllerutil.SetControllerReference(cd, job, r.scheme); err != nil {
		cdLog.WithError(err).Error("error setting controller reference on job")
		return reconcile.Result{}, err
	}
	if err = controllerutil.SetControllerReference(cd, cfgMap, r.scheme); err != nil {
		cdLog.WithError(err).Error("error setting controller reference on config map")
		return reconcile.Result{}, err
	}

	cdLog = cdLog.WithField("job", job.Name)

	cdLog.Debug("checking if install-config.yaml config map exists")
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
		cdLog.Infof("cluster job exists, cluster deployment status: %v", cd.Status.Installed)
		if !cd.Status.Installed {
			if existingJob.Annotations != nil && cfgMap.Annotations != nil {
				didGenerationChange, err := r.updateOutdatedConfigurations(cd.Generation, existingJob, cfgMap, cdLog)
				if err != nil {
					return reconcile.Result{}, err
				} else if didGenerationChange {
					return reconcile.Result{Requeue: true}, nil
				}
			}
		}
	}

	err = r.updateClusterDeploymentStatus(cd, origCD, existingJob, cdLog)
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

// getHiveImage looks for a Hive image to use in clusterdeployment jobs in the following order:
// 1 - specified in the cluster deployment spec.images.hiveImage
// 2 - referenced in the cluster deployment spec.imageSet
// 3 - specified via environment variable to the hive controller
// 4 - fallback default hardcoded image reference
func (r *ReconcileClusterDeployment) getHiveImage(cd *hivev1.ClusterDeployment, imageSet *hivev1.ClusterImageSet, cdLog log.FieldLogger) string {
	if cd.Spec.Images.HiveImage != "" {
		return cd.Spec.Images.HiveImage
	}
	if imageSet != nil && imageSet.Spec.HiveImage != nil {
		return *imageSet.Spec.HiveImage
	}
	return images.GetHiveImage(cdLog)
}

// getReleaseImage looks for a a release image in clusterdeployment or its corresponding imageset in the following order:
// 1 - specified in the cluster deployment spec.images.releaseImage
// 2 - referenced in the cluster deployment spec.imageSet
func (r *ReconcileClusterDeployment) getReleaseImage(cd *hivev1.ClusterDeployment, imageSet *hivev1.ClusterImageSet, cdLog log.FieldLogger) string {
	if cd.Spec.Images.ReleaseImage != "" {
		return cd.Spec.Images.ReleaseImage
	}
	if imageSet != nil && imageSet.Spec.ReleaseImage != nil {
		return *imageSet.Spec.ReleaseImage
	}
	return ""
}

func (r *ReconcileClusterDeployment) getClusterImageSet(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (*hivev1.ClusterImageSet, error) {
	if cd.Spec.ImageSet == nil {
		return nil, nil
	}
	imageSet := &hivev1.ClusterImageSet{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: cd.Spec.ImageSet.Name}, imageSet)
	switch {
	case errors.IsNotFound(err):
		cdLog.WithField("clusterimageset", cd.Spec.ImageSet.Name).Warning("clusterdeployment references non-existent clusterimageset")
		return nil, nil
	case err != nil:
		cdLog.WithError(err).WithField("clusterimageset", cd.Spec.ImageSet.Name).Error("unexpected error retrieving clusterimageset")
		return nil, err
	default:
		return imageSet, nil
	}
}

func (r *ReconcileClusterDeployment) statusUpdate(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) error {
	err := r.Status().Update(context.TODO(), cd)
	if err != nil {
		cdLog.WithError(err).Error("cannot update clusterdeployment status")
	}
	return err
}

func (r *ReconcileClusterDeployment) resolveInstallerImage(cd *hivev1.ClusterDeployment, hiveImage string, cdLog log.FieldLogger) error {
	if len(cd.Spec.Images.InstallerImage) > 0 {
		cdLog.WithField("image", cd.Spec.Images.InstallerImage).
			Debug("setting status.InstallerImage to the value in spec.images.installerImage")
		cd.Status.InstallerImage = &cd.Spec.Images.InstallerImage
		return r.statusUpdate(cd, cdLog)
	}
	if cd.Spec.ImageSet == nil {
		// In the future, not having an ImageSet or an override installer image set should not
		// be allowed. For now, we'll set a default one.
		cdLog.Warn("no imageset reference or override installer image found on cluster deployment. Using default")
		cd.Status.InstallerImage = strPtr(install.DefaultInstallerImage)
		return r.statusUpdate(cd, cdLog)
	}
	imageSet := &hivev1.ClusterImageSet{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: cd.Spec.ImageSet.Name}, imageSet)
	if errors.IsNotFound(err) {
		cdLog.WithField("clusterimageset", cd.Spec.ImageSet.Name).Debug("clusterimageset not found, setting a condition to indicate the error")
		_, err := r.setImageSetNotFoundCondition(cd, true, cdLog)
		return err
	}
	if err != nil {
		cdLog.WithField("clusterimageset", cd.Spec.ImageSet.Name).Error("cannot get clusterimageset")
		return err
	}
	modified, err := r.setImageSetNotFoundCondition(cd, false, cdLog)
	if modified {
		return err
	}
	if imageSet.Spec.InstallerImage != nil {
		cd.Status.InstallerImage = imageSet.Spec.InstallerImage
		cdLog.WithField("imageset", imageSet.Name).Debug("setting status.InstallerImage using imageSet.Spec.InstallerImage")
		return r.statusUpdate(cd, cdLog)
	}
	if imageSet.Spec.ReleaseImage == nil {
		// This is not expected to happen, but will be logged just in case.
		cdLog.WithField("imageset", imageSet.Name).Error("invalid ClusterImageSet: no releaseImage specified")
		// No need to requeue right away
		return nil
	}
	cliImage := images.GetCLIImage(cdLog)
	job := imageset.GenerateImageSetJob(cd, imageSet, serviceAccountName, imageset.AlwaysPullImage(cliImage), imageset.AlwaysPullImage(hiveImage))
	if err = controllerutil.SetControllerReference(cd, job, r.scheme); err != nil {
		cdLog.WithError(err).Error("error setting controller reference on job")
		return err
	}

	jobName := types.NamespacedName{Name: job.Name, Namespace: job.Namespace}
	jobLog := cdLog.WithField("job", jobName)

	existingJob := &batchv1.Job{}
	err = r.Get(context.TODO(), jobName, existingJob)
	switch {
	// If job exists and is finished, delete so we can recreate it
	case err == nil && controllerutils.IsFinished(existingJob):
		jobLog.WithField("successful", controllerutils.IsSuccessful(existingJob)).
			Warning("Finished job found, but installer image is not yet resolved. Deleting.")
		err := r.Delete(context.Background(), existingJob,
			client.PropagationPolicy(metav1.DeletePropagationForeground))
		if err != nil {
			jobLog.WithError(err).Error("cannot delete job")
		}
		return err
	case errors.IsNotFound(err):
		jobLog.Info("creating imageset job")
		err = r.Create(context.TODO(), job)
		if err != nil {
			jobLog.WithError(err).Error("error creating job")
		}
		return err
	case err != nil:
		jobLog.WithError(err).Error("cannot get job")
		return err
	default:
		jobLog.Debug("job exists and is in progress")
	}
	return nil
}

func (r *ReconcileClusterDeployment) setImageSetNotFoundCondition(cd *hivev1.ClusterDeployment, isNotFound bool, cdLog log.FieldLogger) (modified bool, err error) {
	original := cd.DeepCopy()
	status := corev1.ConditionFalse
	reason := clusterImageSetFoundReason
	message := fmt.Sprintf("ClusterImageSet %s is available", cd.Spec.ImageSet.Name)
	if isNotFound {
		status = corev1.ConditionTrue
		reason = clusterImageSetNotFoundReason
		message = fmt.Sprintf("ClusterImageSet %s is not available", cd.Spec.ImageSet.Name)
	}
	cd.Status.Conditions = controllerutils.SetClusterDeploymentCondition(
		cd.Status.Conditions,
		hivev1.ClusterImageSetNotFoundCondition,
		status,
		reason,
		message,
		controllerutils.UpdateConditionNever)
	if !reflect.DeepEqual(original.Status.Conditions, cd.Status.Conditions) {
		cdLog.Debug("setting ClusterImageSetNotFoundCondition to %v", status)
		err := r.Status().Update(context.TODO(), cd)
		if err != nil {
			cdLog.WithError(err).Error("cannot update status conditions")
		}
		return true, err
	}
	return false, nil
}

// Deletes the job if it exists and its generation does not match the cluster deployment's
// genetation. Updates the config map if it is outdated too
func (r *ReconcileClusterDeployment) updateOutdatedConfigurations(cdGeneration int64, existingJob *batchv1.Job, cfgMap *corev1.ConfigMap, cdLog log.FieldLogger) (bool, error) {
	var err error
	var didGenerationChange bool
	if jobGeneration, ok := existingJob.Annotations[clusterDeploymentGenerationAnnotation]; ok {
		convertedJobGeneration, _ := strconv.ParseInt(jobGeneration, 10, 64)
		if convertedJobGeneration < cdGeneration {
			didGenerationChange = true
			err = r.Delete(context.TODO(), existingJob, client.PropagationPolicy(metav1.DeletePropagationForeground))
			if err != nil {
				cdLog.WithError(err).Errorf("error deleting outdated install job")
				return didGenerationChange, err
			}
		}
	}
	if cfgMapGeneration, ok := cfgMap.Annotations[clusterDeploymentGenerationAnnotation]; ok {
		convertedMapGeneration, _ := strconv.ParseInt(cfgMapGeneration, 10, 64)
		if convertedMapGeneration < cdGeneration {
			didGenerationChange = true
			err = r.Update(context.TODO(), cfgMap)
			if err != nil {
				cdLog.WithError(err).Errorf("error deleting outdated config map")
				return didGenerationChange, err
			}
		}
	}
	return didGenerationChange, err
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

func (r *ReconcileClusterDeployment) updateClusterDeploymentStatus(cd *hivev1.ClusterDeployment, origCD *hivev1.ClusterDeployment, job *batchv1.Job, cdLog log.FieldLogger) error {
	cdLog.Debug("updating cluster deployment status")
	if job != nil && job.Name != "" && job.Namespace != "" {
		// Job exists, check it's status:
		cd.Status.Installed = controllerutils.IsSuccessful(job)
	}

	// The install manager sets this secret name, but we don't consider it a critical failure and
	// will attempt to heal it here, as the value is predictable.
	if cd.Status.Installed && cd.Status.AdminKubeconfigSecret.Name == "" {
		cd.Status.AdminKubeconfigSecret = corev1.LocalObjectReference{Name: fmt.Sprintf("%s-admin-kubeconfig", cd.Name)}
	}

	if cd.Status.AdminKubeconfigSecret.Name != "" {
		adminKubeconfigSecret := &corev1.Secret{}
		err := r.Get(context.Background(), types.NamespacedName{Namespace: cd.Namespace, Name: cd.Status.AdminKubeconfigSecret.Name}, adminKubeconfigSecret)
		if err != nil {
			if errors.IsNotFound(err) {
				log.Warn("admin kubeconfig does not yet exist")
			} else {
				return err
			}
		} else {
			err = r.setAdminKubeconfigStatus(cd, adminKubeconfigSecret, cdLog)
			if err != nil {
				return err
			}
		}
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

// setAdminKubeconfigStatus sets all cluster status fields that depend on the admin kubeconfig.
func (r *ReconcileClusterDeployment) setAdminKubeconfigStatus(cd *hivev1.ClusterDeployment, adminKubeconfigSecret *corev1.Secret, cdLog log.FieldLogger) error {
	remoteClusterAPIClient, err := r.remoteClusterAPIClientBuilder(string(adminKubeconfigSecret.Data[adminKubeconfigKey]))
	if err != nil {
		cdLog.WithError(err).Error("error building remote cluster-api client connection")
		return err
	}
	if cd.Status.WebConsoleURL == "" || cd.Status.APIURL == "" {
		// Parse the admin kubeconfig for the server URL:
		config, err := clientcmd.Load(adminKubeconfigSecret.Data["kubeconfig"])
		if err != nil {
			return err
		}
		cluster, ok := config.Clusters[cd.Spec.ClusterName]
		if !ok {
			return fmt.Errorf("error parsing admin kubeconfig secret data")
		}

		// We should be able to assume only one cluster in here:
		server := cluster.Server
		cdLog.Debugf("found cluster API URL in kubeconfig: %s", server)
		cd.Status.APIURL = server
		routeObject := &routev1.Route{}
		err = remoteClusterAPIClient.Get(context.Background(),
			types.NamespacedName{Namespace: "openshift-console", Name: "console"}, routeObject)
		if err != nil {
			cdLog.WithError(err).Error("error fetching remote route object")
			return err
		}
		cdLog.Debugf("read remote route object: %s", routeObject)
		cd.Status.WebConsoleURL = "https://" + routeObject.Spec.Host
	}
	return nil
}

func (r *ReconcileClusterDeployment) syncDeletedClusterDeployment(cd *hivev1.ClusterDeployment, hiveImage string, cdLog log.FieldLogger) (reconcile.Result, error) {

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

	// Skips creation of uninstall job if PreserveOnDelete is true and cluster is installed
	if cd.Spec.PreserveOnDelete {
		if cd.Status.Installed {
			cdLog.Warn("skipping creation of uninstall job for installed cluster due to PreserveOnDelete=true")
			if controllerutils.HasFinalizer(cd, hivev1.FinalizerDeprovision) {
				err = r.removeClusterDeploymentFinalizer(cd)
				if err != nil {
					cdLog.WithError(err).Error("error removing finalizer")
				}
				return reconcile.Result{}, err
			}
			return reconcile.Result{}, nil
		}
		// Overriding PreserveOnDelete because we might have deleted the cluster deployment before it finished
		// installing, which can cause AWS resources to leak
		cdLog.Infof("PreserveOnDelete=true but launching uninstall as cluster was never successfully provisioned")
	}

	if cd.Status.InfraID == "" {
		cdLog.Warn("skipping uninstall for cluster that never had clusterID set")
		err = r.removeClusterDeploymentFinalizer(cd)
		if err != nil {
			cdLog.WithError(err).Error("error removing finalizer")
		}
	} else {
		// Generate an uninstall job:
		uninstallJob, err := install.GenerateUninstallerJob(cd, hiveImage)
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
		if controllerutils.IsSuccessful(existingJob) {
			cdLog.Infof("uninstall job successful, removing finalizer")
			err = r.removeClusterDeploymentFinalizer(cd)
			if err != nil {
				cdLog.WithError(err).Error("error removing finalizer")
			}
			return reconcile.Result{}, err
		}

		cdLog.Infof("uninstall job not yet successful")
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileClusterDeployment) addClusterDeploymentFinalizer(cd *hivev1.ClusterDeployment) error {
	cd = cd.DeepCopy()
	controllerutils.AddFinalizer(cd, hivev1.FinalizerDeprovision)
	return r.Update(context.TODO(), cd)
}

func (r *ReconcileClusterDeployment) removeClusterDeploymentFinalizer(cd *hivev1.ClusterDeployment) error {
	cd = cd.DeepCopy()
	controllerutils.DeleteFinalizer(cd, hivev1.FinalizerDeprovision)
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

func strPtr(s string) *string {
	return &s
}
