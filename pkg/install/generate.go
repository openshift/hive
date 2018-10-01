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

package install

import (
	"time"

	log "github.com/sirupsen/logrus"

	kbatch "k8s.io/api/batch/v1"
	kapi "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
)

// GenerateInstallerJob creates a job to install an OpenShift cluster
// given a ClusterDeployment and an installer image.
func GenerateInstallerJob(
	name string,
	cd *hivev1.ClusterDeployment,
	installerImage string,
	installerImagePullPolicy kapi.PullPolicy,
	uninstall bool,
	serviceAccount *kapi.ServiceAccount,
	scheme *runtime.Scheme) *kbatch.Job {

	cdLog := log.WithFields(log.Fields{
		"clusterDeployment": cd.Name,
		"namespace":         cd.Namespace,
	})

	cdLog.Debug("generating installer job")

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
			{
				Name:  "OPENSHIFT_INSTALL_BASE_DOMAIN",
				Value: cd.Spec.Config.BaseDomain,
			},
			{
				Name:  "OPENSHIFT_INSTALL_CLUSTER_NAME",
				Value: cd.Name,
			},
			{
				Name:  "OPENSHIFT_INSTALL_EMAIL_ADDRESS",
				Value: cd.Spec.Config.Admin.Email,
			},
			{
				Name: "OPENSHIFT_INSTALL_PASSWORD",
				ValueFrom: &kapi.EnvVarSource{
					SecretKeyRef: &kapi.SecretKeySelector{
						LocalObjectReference: cd.Spec.Config.Admin.Password,
						Key:                  "password",
					},
				},
			},
			{
				Name:  "OPENSHIFT_INSTALL_PLATFORM",
				Value: "aws",
			},
			{
				Name: "OPENSHIFT_INSTALL_PULL_SECRET",
				ValueFrom: &kapi.EnvVarSource{
					SecretKeyRef: &kapi.SecretKeySelector{
						LocalObjectReference: cd.Spec.Config.PullSecret,
						Key:                  ".dockercfg",
					},
				},
			},
			{
				Name: "OPENSHIFT_INSTALL_SSH_PUB_KEY",
				ValueFrom: &kapi.EnvVarSource{
					SecretKeyRef: &kapi.SecretKeySelector{
						LocalObjectReference: *cd.Spec.Config.Admin.SSHKey,
						Key:                  "ssh-publickey",
					},
				},
			},
			{
				Name:  "OPENSHIFT_INSTALL_AWS_REGION",
				Value: cd.Spec.Config.AWS.Region,
			},
		}...)
	}

	volumes := []kapi.Volume{
		{
			Name: "install",
			VolumeSource: kapi.VolumeSource{
				EmptyDir: &kapi.EmptyDirVolumeSource{},
			},
		},
	}

	volumeMounts := []kapi.VolumeMount{
		{
			Name:      "install",
			MountPath: "/output",
		},
	}

	command := []string{}
	if uninstall {
		command = []string{"echo", "this would have been an uninstall"}
	}
	args := []string{}
	if !uninstall {
		args = []string{"cluster"}
	}

	containers := []kapi.Container{
		{
			Name:            "installer",
			Image:           installerImage,
			ImagePullPolicy: installerImagePullPolicy,
			Env:             env,
			Command:         command,
			Args:            args,
			VolumeMounts:    volumeMounts,
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

	return job
}
