package imageset

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	apihelpers "github.com/openshift/hive/pkg/apis/helpers"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/controller/images"
)

// ImageSpec specifies an image reference and associated pull policy
type ImageSpec struct {
	Image      string
	PullPolicy corev1.PullPolicy
}

const (
	// extractImageScriptTemplate is a minimal shell script we run in the imageset job to determine the images to
	// use for various OpenShift components involved in the provisioning. A string value will be formatted in
	// for which installer image we need to use. (regular vs baremetal)
	extractImageScriptTemplate = `#/bin/bash
echo "About to run oc adm release info"
if oc adm release info --image-for="%s" --registry-config "${PULL_SECRET}" "${RELEASE_IMAGE}" > /common/installer-image.txt 2> /common/error.log; then
  echo "installer image resolved successfully"
else
  echo "installer image resolution failed"
  echo "0" > /common/success
  exit 1
fi

if oc adm release info --image-for="cli" --registry-config "${PULL_SECRET}" "${RELEASE_IMAGE}" > /common/cli-image.txt 2> /common/error.log; then
  echo "cli image resolved successfully"
else
  echo "cli image resolution failed"
  echo "0" > /common/success
  exit 1
fi

echo "1" > /common/success
exit 0
`
	// ImagesetJobLabel is the label used for counting the number of imageset jobs in Hive
	ImagesetJobLabel = "hive.openshift.io/imageset"
)

// GenerateImageSetJob creates a job to determine the installer image for a ClusterImageSet
// given a release image
func GenerateImageSetJob(cd *hivev1.ClusterDeployment, releaseImage, serviceAccountName string, cli ImageSpec) *batchv1.Job {
	logger := log.WithFields(log.Fields{
		"clusterdeployment": types.NamespacedName{Namespace: cd.Namespace, Name: cd.Name}.String(),
	})

	logger.Debug("generating cluster image set job")

	env := []corev1.EnvVar{
		{
			Name:  "RELEASE_IMAGE",
			Value: releaseImage,
		},
		{
			Name:  "PULL_SECRET",
			Value: "/run/release-pull-secret/" + corev1.DockerConfigJsonKey,
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
					SecretName: constants.GetMergedPullSecretName(cd),
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

	installerImageKey := "installer"
	// If this is a bare metal install, we need to get the openshift-install binary from a different image with
	// bare metal functionality compiled in. The binary is named the same and in the same location, so after swapping
	// out what image to get it from, we can proceed with the code as we normally would.
	if cd.Spec.Platform.BareMetal != nil {
		installerImageKey = "baremetal-installer"
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
			Args:            []string{fmt.Sprintf(extractImageScriptTemplate, installerImageKey)},
			VolumeMounts:    volumeMounts,
		},
		{
			Name:            "hiveutil",
			Image:           images.GetHiveImage(),
			ImagePullPolicy: images.GetHiveImagePullPolicy(),
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
	labels := map[string]string{
		ImagesetJobLabel:                     "true",
		constants.ClusterDeploymentNameLabel: cd.Name,
	}
	if cd.Labels != nil {
		typeStr, ok := cd.Labels[hivev1.HiveClusterTypeLabel]
		if ok {
			labels[hivev1.HiveClusterTypeLabel] = typeStr
		}
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetImageSetJobName(cd.Name),
			Namespace: cd.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			Completions:           &completions,
			ActiveDeadlineSeconds: &deadline,
			BackoffLimit:          &backoffLimit,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: podSpec,
			},
		},
	}

	return job
}

// GetImageSetJobName returns the expected name of the imageset job for a ClusterImageSet.
func GetImageSetJobName(cdName string) string {
	return apihelpers.GetResourceName(cdName, "imageset")
}

// AlwaysPullImage returns an ImageSpec with a PullAlways pull policy
func AlwaysPullImage(name string) ImageSpec {
	return ImageSpec{
		Image:      name,
		PullPolicy: corev1.PullAlways,
	}
}
