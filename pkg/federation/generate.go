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

package federation

import (
	"fmt"
	"time"

	// "github.com/ghodss/yaml"
	log "github.com/sirupsen/logrus"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
)

const (
	// TODO: Replace with an official image containing federation CLI"
	defaultFederationImage = "quay.io/csrwng/federation-cli:latest"
)

// GenerateFederationJob creates a job to federate an OpenShift cluster
// given a ClusterDeployment and its kubeconfig
func GenerateFederationJob(
	cd *hivev1.ClusterDeployment,
	kubeconfig []byte,
	serviceAccountName string) (*batchv1.Job, *corev1.Secret, error) {

	cdLog := log.WithFields(log.Fields{
		"clusterDeployment": cd.Name,
		"namespace":         cd.Namespace,
	})

	cdLog.Debug("generating federation job")

	mergedKubeconfig, err := GenerateCombinedKubeconfig(kubeconfig, cd.Name)
	if err != nil {
		cdLog.WithError(err).Error("error occurred generating merged kubeconfig")
		return nil, nil, err
	}

	secret := &corev1.Secret{}
	secret.Name = fmt.Sprintf("%s-federation", cd.Name)
	secret.Namespace = cd.Namespace
	secret.Data = map[string][]byte{
		"kubeconfig": mergedKubeconfig,
	}

	volumes := []corev1.Volume{
		{
			Name: "kubeconfig",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: secret.Name,
				},
			},
		},
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "kubeconfig",
			MountPath: "/data",
		},
	}

	federationImage := defaultFederationImage

	containers := []corev1.Container{
		{
			Name:            "federation",
			Image:           federationImage,
			ImagePullPolicy: corev1.PullAlways,
			Command:         []string{"/usr/bin/kubefed2"},
			Args: []string{
				"join",
				cd.Name,
				"--cluster-context",
				cd.Name,
				"--host-cluster-context",
				"hive",
				"--add-to-registry",
				"--v=2",
				"--kubeconfig",
				"/data/kubeconfig",
			},
			VolumeMounts: volumeMounts,
		},
	}

	podSpec := corev1.PodSpec{
		DNSPolicy:          corev1.DNSClusterFirst,
		RestartPolicy:      corev1.RestartPolicyNever,
		Containers:         containers,
		Volumes:            volumes,
		ServiceAccountName: serviceAccountName,
	}

	completions := int32(1)
	deadline := int64((1 * time.Hour).Seconds())
	backoffLimit := int32(0) // only allow the pod to fail once

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetFederationJobName(cd),
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

	return job, secret, nil
}

// GetFederationJobName returns the expected name of the federation job for a cluster deployment.
func GetFederationJobName(cd *hivev1.ClusterDeployment) string {
	return fmt.Sprintf("%s-federation", cd.Name)
}
