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

package imageset

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
)

// ImageSpec specifies an image reference and associated pull policy
type ImageSpec struct {
	Image      string
	PullPolicy corev1.PullPolicy
}

const (
	extractImageScript = `#/bin/bash
echo "About to run oc adm release info"
if oc adm release info --image-for="installer" --registry-config "${PULL_SECRET}" "${RELEASE_IMAGE}" > /common/installer-image.txt 2> /common/error.log; then
  echo "The command succeeded"  
  echo "1" > /common/success
else
  echo "The command failed"
  echo "0" > /common/success
fi
`
)

// GenerateImageSetJob creates a job to determine the installer image for a ClusterImageSet
// given a release image
func GenerateImageSetJob(cd *hivev1.ClusterDeployment, imageSet *hivev1.ClusterImageSet, serviceAccountName string, cli, hive ImageSpec) *batchv1.Job {

	logger := log.WithFields(log.Fields{
		"clusterdeployment": types.NamespacedName{Namespace: cd.Namespace, Name: cd.Name}.String(),
	})

	logger.Debug("generating cluster image set job")

	env := []corev1.EnvVar{
		{
			Name:  "RELEASE_IMAGE",
			Value: *imageSet.Spec.ReleaseImage,
		},
		{
			Name:  "PULL_SECRET",
			Value: "/run/release-pull-secret/" + corev1.DockerConfigKey,
		},
	}

	volumes := []corev1.Volume{
		{
			Name: "common",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "pullsecret",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: cd.Spec.PullSecret.Name,
				},
			},
		},
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "common",
			MountPath: "/common",
		},
		{
			Name:      "pullsecret",
			MountPath: "/run/release-pull-secret",
		},
	}

	// This container just needs to copy the required install binaries to the shared emptyDir volume,
	// where our container will run them. This is effectively downloading the all-in-one installer.
	containers := []corev1.Container{
		{
			Name:            "release",
			Image:           cli.Image,
			ImagePullPolicy: cli.PullPolicy,
			Env:             env,
			Command:         []string{"/bin/sh", "-c"},
			Args:            []string{extractImageScript},
			VolumeMounts:    volumeMounts,
		},
		{
			Name:            "hiveutil",
			Image:           hive.Image,
			ImagePullPolicy: hive.PullPolicy,
			Env:             env,
			Command:         []string{"/usr/bin/hiveutil"},
			Args: []string{
				"update-installer-image",
				"--work-dir",
				"/common",
				"--log-level",
				"debug",
				"--cluster-deployment-name",
				cd.Name,
				"--cluster-deployment-namespace",
				cd.Namespace,
			},
			VolumeMounts: volumeMounts,
		},
	}

	restartPolicy := corev1.RestartPolicyOnFailure

	podSpec := corev1.PodSpec{
		RestartPolicy:      restartPolicy,
		Containers:         containers,
		Volumes:            volumes,
		ServiceAccountName: serviceAccountName,
	}

	completions := int32(1)
	deadline := int64((24 * time.Hour).Seconds())
	backoffLimit := int32(123456)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetImageSetJobName(cd.Name),
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

	return job
}

// GetImageSetJobName returns the expected name of the imageset job for a ClusterImageSet.
func GetImageSetJobName(cdName string) string {
	return fmt.Sprintf("%s-imageset", cdName)
}

// AlwaysPullImage returns an ImageSpec with a PullAlways pull policy
func AlwaysPullImage(name string) ImageSpec {
	return ImageSpec{
		Image:      name,
		PullPolicy: corev1.PullAlways,
	}
}
