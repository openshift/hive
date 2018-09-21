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
	"github.com/ghodss/yaml"
	"reflect"
	"time"

	log "github.com/sirupsen/logrus"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"

	kbatch "k8s.io/api/batch/v1"
	kapi "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	installerImage = "registry.svc.ci.openshift.org/openshift/origin-v4.0:installer"
)

// Add creates a new ClusterDeployment Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileClusterDeployment{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
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
	err = c.Watch(&source.Kind{Type: &kbatch.Job{}}, &handler.EnqueueRequestForOwner{
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
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a ClusterDeployment object and makes changes based on the state read
// and what is in the ClusterDeployment.Spec
//
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
//
// TODO: RBAC for jobs instead of deployments here:
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hive.openshift.io,resources=clusterdeployments,verbs=get;list;watch;create;update;patch;delete
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
	origCD := cd.DeepCopy()

	job, cfgMap, err := generateInstallerJob("mytestjob", cd, installerImage, kapi.PullIfNotPresent, nil, r.scheme)
	if err != nil {
		cdLog.Errorf("error generating install job", err)
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

	// Check if the ConfigMap already exists for this ClusterDeployment:
	existingCfgMap := &kapi.ConfigMap{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: cfgMap.Name, Namespace: cfgMap.Namespace}, existingCfgMap)
	if err != nil && errors.IsNotFound(err) {
		cdLog.Infof("creating config map")
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
	existingJob := &kbatch.Job{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, existingJob)
	if err != nil && errors.IsNotFound(err) {
		cdLog.Infof("creating job")
		err = r.Create(context.TODO(), job)
		if err != nil {
			cdLog.Errorf("error creating job: %v", err)
			return reconcile.Result{}, err
		}
	} else if err != nil {
		cdLog.Errorf("error getting job: %v", err)
		return reconcile.Result{}, err
	} else {
		// Job exists, check it's status:
		cd.Status.Installed = isSuccessful(existingJob)
	}

	// Update cluster deployment status if changed:
	if !reflect.DeepEqual(cd.Status, origCD.Status) {
		cdLog.Infof("status has changed, updating cluster deployment")
		err = r.Update(context.TODO(), cd)
		if err != nil {
			cdLog.Errorf("error updating cluster deployment: %v", err)
			return reconcile.Result{}, err
		}
	} else {
		cdLog.Infof("cluster deployment status unchanged")
	}

	cdLog.Debugf("reconcile complete")
	return reconcile.Result{}, nil
}

func generateInstallerJob(
	name string,
	cd *hivev1.ClusterDeployment,
	installerImage string,
	installerImagePullPolicy kapi.PullPolicy,

	serviceAccount *kapi.ServiceAccount,
	scheme *runtime.Scheme) (*kbatch.Job, *kapi.ConfigMap, error) {

	cdLog := log.WithFields(log.Fields{
		"clusterDeployment": cd.Name,
		"namespace":         cd.Namespace,
	})

	cdLog.Debug("generating installer job")

	d, err := yaml.Marshal(cd.Spec.Config)
	if err != nil {
		return nil, nil, err
	}
	installConfig := string(d)
	cdLog.Infof("Generated installConfig: \n\n\n%s\n\n\n", installConfig)

	cfgMap := &kapi.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-installconfig", name),
			Namespace: cd.Namespace,
		},
		Data: map[string]string{
			"installconfig.yaml": installConfig,
		},
	}

	env := []kapi.EnvVar{}
	if cd.Spec.PlatformSecrets.AWS != nil && len(cd.Spec.PlatformSecrets.AWS.Credentials.Name) > 0 {
		env = append(env, []kapi.EnvVar{
			{
				Name: "AWS_ACCESS_KEY_ID",
				ValueFrom: &kapi.EnvVarSource{
					SecretKeyRef: &kapi.SecretKeySelector{
						LocalObjectReference: cd.Spec.PlatformSecrets.AWS.Credentials,
						Key:                  "awsAccessKeyId",
					},
				},
			},
			{
				Name: "AWS_SECRET_ACCESS_KEY",
				ValueFrom: &kapi.EnvVarSource{
					SecretKeyRef: &kapi.SecretKeySelector{
						LocalObjectReference: cd.Spec.PlatformSecrets.AWS.Credentials,
						Key:                  "awsSecretAccessKey",
					},
				},
			},
		}...)
	}

	volumes := make([]kapi.Volume, 0, 1)
	volumeMounts := make([]kapi.VolumeMount, 0, 1)

	volumeMounts = append(volumeMounts, kapi.VolumeMount{
		Name:      "installconfig",
		MountPath: "/home/user/installerinput",
	})
	volumes = append(volumes, kapi.Volume{
		Name: "installconfig",
		VolumeSource: kapi.VolumeSource{
			ConfigMap: &kapi.ConfigMapVolumeSource{
				LocalObjectReference: kapi.LocalObjectReference{
					Name: cfgMap.Name,
				},
			},
		},
	})

	/*
		if len(hardware.SSHSecret.Name) > 0 {
			volumeMounts = append(volumeMounts, kapi.VolumeMount{
				Name:      "sshkey",
				MountPath: "/ansible/ssh/",
			})
			// sshKeyFileMode is used to set the file permissions for the private SSH key
			sshKeyFileMode := int32(0600)
			volumes = append(volumes, kapi.Volume{
				Name: "sshkey",
				VolumeSource: kapi.VolumeSource{
					Secret: &kapi.SecretVolumeSource{
						SecretName: hardware.SSHSecret.Name,
						Items: []kapi.KeyToPath{
							{
								Key:  "ssh-privatekey",
								Path: "privatekey.pem",
								Mode: &sshKeyFileMode,
							},
							{
								Key:  "ssh-publickey",
								Path: "publickey.pub",
								Mode: &sshKeyFileMode,
							},
						},
					},
				},
			})
		}
		if len(hardware.SSLSecret.Name) > 0 {
			volumeMounts = append(volumeMounts, kapi.VolumeMount{
				Name:      "sslkey",
				MountPath: "/ansible/ssl/",
			})
			volumes = append(volumes, kapi.Volume{
				Name: "sslkey",
				VolumeSource: kapi.VolumeSource{
					Secret: &kapi.SecretVolumeSource{
						SecretName: hardware.SSLSecret.Name,
					},
				},
			})
		}
	*/

	containers := []kapi.Container{
		{
			Name:            "installer",
			Image:           installerImage,
			ImagePullPolicy: installerImagePullPolicy,
			Env:             env,
			VolumeMounts:    volumeMounts,
			Command:         []string{"cat", "/home/user/installerinput/installconfig.yaml"},
			//Command:      []string{"/home/user/installer/tectonic", "init", "--config", "/home/user/installerinput/installconfig.yaml"},
		},
	}

	podSpec := kapi.PodSpec{
		DNSPolicy:     kapi.DNSClusterFirst,
		RestartPolicy: kapi.RestartPolicyOnFailure,
		Containers:    containers,
		Volumes:       volumes,
	}

	if serviceAccount != nil {
		podSpec.ServiceAccountName = serviceAccount.Name
	}

	completions := int32(1)
	deadline := int64((24 * time.Hour).Seconds())
	backoffLimit := int32(123456) // effectively limitless

	job := &kbatch.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cd.Namespace,
		},
		Spec: kbatch.JobSpec{
			Completions:           &completions,
			ActiveDeadlineSeconds: &deadline,
			BackoffLimit:          &backoffLimit,
			Template: kapi.PodTemplateSpec{
				Spec: podSpec,
			},
		},
	}

	return job, cfgMap, nil
}

// getJobConditionStatus gets the status of the condition in the job. If the
// condition is not found in the job, then returns False.
func getJobConditionStatus(job *kbatch.Job, conditionType kbatch.JobConditionType) kapi.ConditionStatus {
	for _, condition := range job.Status.Conditions {
		if condition.Type == conditionType {
			return condition.Status
		}
	}
	return kapi.ConditionFalse
}

func isSuccessful(job *kbatch.Job) bool {
	return getJobConditionStatus(job, kbatch.JobComplete) == kapi.ConditionTrue
}

func isFailed(job *kbatch.Job) bool {
	return getJobConditionStatus(job, kbatch.JobFailed) == kapi.ConditionTrue
}
