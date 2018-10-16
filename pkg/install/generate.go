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
	"fmt"
	"time"

	"github.com/ghodss/yaml"
	log "github.com/sirupsen/logrus"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
)

const (
	defaultInstallerImage           = "registry.svc.ci.openshift.org/openshift/origin-v4.0:installer"
	defaultInstallerImagePullPolicy = corev1.PullAlways
	defaultHiveImage                = "hive-controller:latest"
	defaultHiveImagePullPolicy      = corev1.PullNever
)

// GenerateInstallerJob creates a job to install an OpenShift cluster
// given a ClusterDeployment and an installer image.
func GenerateInstallerJob(
	cd *hivev1.ClusterDeployment,
	serviceAccountName string) (*batchv1.Job, *corev1.ConfigMap, error) {

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

	cfgMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-installconfig", cd.Name),
			Namespace: cd.Namespace,
		},
		Data: map[string]string{
			"installconfig.yaml": installConfig,
		},
	}

	env := []corev1.EnvVar{
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
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: cd.Spec.Config.Admin.Password,
					Key:                  "password",
				},
			},
		},
		{
			Name: "OPENSHIFT_INSTALL_PULL_SECRET",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: cd.Spec.Config.PullSecret,
					Key:                  ".dockercfg",
				},
			},
		},
	}
	if cd.Spec.Config.AWS != nil {
		env = append(env, []corev1.EnvVar{
			{
				Name:  "OPENSHIFT_INSTALL_AWS_REGION",
				Value: cd.Spec.Config.AWS.Region,
			},
			{
				Name:  "OPENSHIFT_INSTALL_PLATFORM",
				Value: "aws",
			},
		}...)
	}
	if cd.Spec.PlatformSecrets.AWS != nil && len(cd.Spec.PlatformSecrets.AWS.Credentials.Name) > 0 {
		env = append(env, []corev1.EnvVar{
			{
				Name: "AWS_ACCESS_KEY_ID",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: cd.Spec.PlatformSecrets.AWS.Credentials,
						Key:                  "awsAccessKeyId",
					},
				},
			},
			{
				Name: "AWS_SECRET_ACCESS_KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: cd.Spec.PlatformSecrets.AWS.Credentials,
						Key:                  "awsSecretAccessKey",
					},
				},
			},
		}...)
	}

	if cd.Spec.Config.Admin.SSHKey != nil {
		env = append(env, corev1.EnvVar{
			Name: "OPENSHIFT_INSTALL_SSH_PUB_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: *cd.Spec.Config.Admin.SSHKey,
					Key:                  "ssh-publickey",
				},
			},
		})
	}

	volumes := []corev1.Volume{
		{
			Name: "install",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "installconfig",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cfgMap.Name,
					},
				},
			},
		},
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "install",
			MountPath: "/output",
		},
		{
			Name:      "installconfig",
			MountPath: "/output/config",
		},
	}

	installerImage := defaultInstallerImage
	if cd.Spec.Images.InstallerImage != "" {
		installerImage = cd.Spec.Images.InstallerImage
	}

	installerImagePullPolicy := defaultInstallerImagePullPolicy
	if cd.Spec.Images.InstallerImagePullPolicy != "" {
		installerImagePullPolicy = cd.Spec.Images.InstallerImagePullPolicy
	}

	hiveImage := defaultHiveImage
	if cd.Spec.Images.HiveImage != "" {
		hiveImage = cd.Spec.Images.HiveImage
	}

	hiveImagePullPolicy := defaultHiveImagePullPolicy
	if cd.Spec.Images.HiveImagePullPolicy != "" {
		hiveImagePullPolicy = cd.Spec.Images.HiveImagePullPolicy
	}

	// This container just needs to copy the required install binaries to the shared emptyDir volume,
	// where our container will run them. This is effectively downloading the all-in-one installer.
	containers := []corev1.Container{
		{
			Name:            "installer",
			Image:           installerImage,
			ImagePullPolicy: installerImagePullPolicy,
			Env:             env,
			Command:         []string{"/bin/sh", "-c"},
			Args:            []string{"cp -v /bin/openshift-install /output && cp -v /bin/terraform /output && ls -la /output"},
			VolumeMounts:    volumeMounts,
		},
		{
			Name:            "hive",
			Image:           hiveImage,
			ImagePullPolicy: hiveImagePullPolicy,
			Env:             env,
			Command:         []string{"/usr/bin/hiveutil"},
			Args:            []string{"install-manager", "--work-dir", "/output", "--log-level", "debug", cd.Namespace, cd.Name},
			VolumeMounts:    volumeMounts,
		},
	}

	podSpec := corev1.PodSpec{
		DNSPolicy:          corev1.DNSClusterFirst,
		RestartPolicy:      corev1.RestartPolicyOnFailure,
		Containers:         containers,
		Volumes:            volumes,
		ServiceAccountName: serviceAccountName,
	}

	completions := int32(1)
	deadline := int64((24 * time.Hour).Seconds())
	backoffLimit := int32(123456) // effectively limitless

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-install", cd.Name),
			Namespace: cd.Namespace,
		},
		Spec: batchv1.JobSpec{
			Completions:           &completions,
			ActiveDeadlineSeconds: &deadline,
			BackoffLimit:          &backoffLimit,
			Template: corev1.PodTemplateSpec{
				Spec: podSpec,
			},
		},
	}

	return job, cfgMap, nil
}

// GenerateUninstallerJob creates a job to uninstall an OpenShift cluster
// given a ClusterDeployment and an installer image.
func GenerateUninstallerJob(
	cd *hivev1.ClusterDeployment) (*batchv1.Job, error) {

	if cd.Spec.Config.AWS == nil {
		return nil, fmt.Errorf("only AWS ClusterDeployments currently supported")
	}

	env := []corev1.EnvVar{}
	if cd.Spec.PlatformSecrets.AWS != nil && len(cd.Spec.PlatformSecrets.AWS.Credentials.Name) > 0 {
		env = append(env, []corev1.EnvVar{
			{
				Name: "AWS_ACCESS_KEY_ID",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: cd.Spec.PlatformSecrets.AWS.Credentials,
						Key:                  "awsAccessKeyId",
					},
				},
			},
			{
				Name: "AWS_SECRET_ACCESS_KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: cd.Spec.PlatformSecrets.AWS.Credentials,
						Key:                  "awsSecretAccessKey",
					},
				},
			},
		}...)
	}

	volumes := []corev1.Volume{
		{
			Name: "metadata",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						// TODO: This matches what is done in the hiveutil install-manager command.
						// We should explicitly link the two.
						Name: fmt.Sprintf("%s-metadata", cd.Name),
					},
				},
			},
		},
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "metadata",
			MountPath: "/cluster/metadata",
		},
	}

	args := []string{"destroy-cluster", "--dir", "/cluster/metadata", "--log-level", "debug"}

	installerImage := defaultInstallerImage
	if cd.Spec.Images.InstallerImage != "" {
		installerImage = cd.Spec.Images.InstallerImage
	}

	installerImagePullPolicy := defaultInstallerImagePullPolicy
	if cd.Spec.Images.InstallerImagePullPolicy != "" {
		installerImagePullPolicy = cd.Spec.Images.InstallerImagePullPolicy
	}

	containers := []corev1.Container{
		{
			Name:            "installer",
			Image:           installerImage,
			ImagePullPolicy: installerImagePullPolicy,
			Env:             env,
			Args:            args,
			VolumeMounts:    volumeMounts,
		},
	}

	podSpec := corev1.PodSpec{
		DNSPolicy:     corev1.DNSClusterFirst,
		RestartPolicy: corev1.RestartPolicyOnFailure,
		Containers:    containers,
		Volumes:       volumes,
	}

	completions := int32(1)
	deadline := int64((24 * time.Hour).Seconds())
	backoffLimit := int32(123456) // effectively limitless

	job := &batchv1.Job{}
	job.Name = fmt.Sprintf("%s-uninstall", cd.Name)
	job.Namespace = cd.Namespace
	job.Spec = batchv1.JobSpec{
		Completions:           &completions,
		ActiveDeadlineSeconds: &deadline,
		BackoffLimit:          &backoffLimit,
		Template: corev1.PodTemplateSpec{
			Spec: podSpec,
		},
	}

	return job, nil
}
