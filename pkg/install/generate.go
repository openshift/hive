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
	defaultHiveImage                = "registry.svc.ci.openshift.org/openshift/hive-v4.0:hive"
	defaultHiveImagePullPolicy      = corev1.PullAlways

	tryInstallOnceAnnotation = "hive.openshift.io/try-install-once"
)

// GenerateInstallerJob creates a job to install an OpenShift cluster
// given a ClusterDeployment and an installer image.
func GenerateInstallerJob(
	cd *hivev1.ClusterDeployment,
	serviceAccountName string,
	sshKey string,
	pullSecret string) (*batchv1.Job, *corev1.ConfigMap, error) {

	cdLog := log.WithFields(log.Fields{
		"clusterDeployment": cd.Name,
		"namespace":         cd.Namespace,
	})

	cdLog.Debug("generating installer job")

	ic, err := GenerateInstallConfig(cd, sshKey, pullSecret)
	if err != nil {
		return nil, nil, err
	}

	tryOnce := false
	if cd.Annotations != nil {
		value, exists := cd.Annotations[tryInstallOnceAnnotation]
		tryOnce = exists && value == "true"
	}

	d, err := yaml.Marshal(ic)
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
			// Filename should match installer default:
			"install-config.yaml": installConfig,
		},
	}

	env := []corev1.EnvVar{
		{
			Name:  "OPENSHIFT_INSTALL_BASE_DOMAIN",
			Value: cd.Spec.BaseDomain,
		},
		{
			Name:  "OPENSHIFT_INSTALL_CLUSTER_NAME",
			Value: cd.Name,
		},
		{
			Name: "OPENSHIFT_INSTALL_PULL_SECRET",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: cd.Spec.PullSecret,
					Key:                  ".dockercfg",
				},
			},
		},
	}
	if cd.Spec.AWS != nil {
		env = append(env, []corev1.EnvVar{
			{
				Name:  "OPENSHIFT_INSTALL_AWS_REGION",
				Value: cd.Spec.AWS.Region,
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
						Key:                  "aws_access_key_id",
					},
				},
			},
			{
				Name: "AWS_SECRET_ACCESS_KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: cd.Spec.PlatformSecrets.AWS.Credentials,
						Key:                  "aws_secret_access_key",
					},
				},
			},
		}...)
	}
	if cd.Spec.Images.ReleaseImage != "" {
		env = append(env, []corev1.EnvVar{
			{
				Name:  "OPENSHIFT_INSTALL_RELEASE_IMAGE_OVERRIDE",
				Value: cd.Spec.Images.ReleaseImage,
			},
		}...)
	}

	if cd.Spec.SSHKey != nil {
		env = append(env, corev1.EnvVar{
			Name: "OPENSHIFT_INSTALL_SSH_PUB_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: *cd.Spec.SSHKey,
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
			MountPath: "/installconfig",
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
			Args:            []string{"cp -v /bin/openshift-install /output && ls -la /output"},
			VolumeMounts:    volumeMounts,
		},
		{
			Name:            "hive",
			Image:           hiveImage,
			ImagePullPolicy: hiveImagePullPolicy,
			Env:             env,
			Command:         []string{"/usr/bin/hiveutil"},
			Args: []string{
				"install-manager",
				"--work-dir",
				"/output",
				"--log-level",
				"debug",
				"--install-config",
				"/installconfig/install-config.yaml",
				"--region",
				cd.Spec.Platform.AWS.Region,
				cd.Namespace,
				cd.Name,
			},
			VolumeMounts: volumeMounts,
		},
	}

	restartPolicy := corev1.RestartPolicyOnFailure
	if tryOnce {
		restartPolicy = corev1.RestartPolicyNever
	}

	podSpec := corev1.PodSpec{
		DNSPolicy:          corev1.DNSClusterFirst,
		RestartPolicy:      restartPolicy,
		Containers:         containers,
		Volumes:            volumes,
		ServiceAccountName: serviceAccountName,
	}

	completions := int32(1)
	deadline := int64((24 * time.Hour).Seconds())
	backoffLimit := int32(123456) // effectively limitless
	if tryOnce {
		backoffLimit = int32(0)
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetInstallJobName(cd),
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

// GetInstallJobName returns the expected name of the install job for a cluster deployment.
func GetInstallJobName(cd *hivev1.ClusterDeployment) string {
	return fmt.Sprintf("%s-install", cd.Name)
}

// GenerateUninstallerJob creates a job to uninstall an OpenShift cluster
// given a ClusterDeployment and an installer image.
func GenerateUninstallerJob(
	cd *hivev1.ClusterDeployment) (*batchv1.Job, error) {

	if cd.Spec.PreserveOnDelete {
		return nil, fmt.Errorf("no creation of uninstaller job, because of PreserveOnDelete")
	}

	if cd.Spec.AWS == nil {
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
						Key:                  "aws_access_key_id",
					},
				},
			},
			{
				Name: "AWS_SECRET_ACCESS_KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: cd.Spec.PlatformSecrets.AWS.Credentials,
						Key:                  "aws_secret_access_key",
					},
				},
			},
		}...)
	}

	hiveImage := defaultHiveImage
	if cd.Spec.Images.HiveImage != "" {
		hiveImage = cd.Spec.Images.HiveImage
	}

	hiveImagePullPolicy := defaultHiveImagePullPolicy
	if cd.Spec.Images.HiveImagePullPolicy != "" {
		hiveImagePullPolicy = cd.Spec.Images.HiveImagePullPolicy
	}

	containers := []corev1.Container{
		{
			Name:            "deprovision",
			Image:           hiveImage,
			ImagePullPolicy: hiveImagePullPolicy,
			Env:             env,
			Command:         []string{"/usr/bin/hiveutil"},
			Args: []string{
				"aws-tag-deprovision",
				"--loglevel",
				"debug",
				"--region",
				cd.Spec.AWS.Region,
				fmt.Sprintf("openshiftClusterID=%s", cd.Status.ClusterID),
				fmt.Sprintf("kubernetes.io/cluster/%s=owned", cd.Spec.ClusterName),
			},
		},
	}

	podSpec := corev1.PodSpec{
		DNSPolicy:     corev1.DNSClusterFirst,
		RestartPolicy: corev1.RestartPolicyOnFailure,
		Containers:    containers,
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
