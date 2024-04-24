package imageset

import (
	"time"

	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	log "github.com/sirupsen/logrus"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	apihelpers "github.com/openshift/hive/apis/helpers"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/controller/images"
)

const (
	// ImagesetJobLabel is the label used for counting the number of imageset jobs in Hive
	ImagesetJobLabel = "hive.openshift.io/imageset"
)

// GenerateImageSetJob creates a job to determine the installer image for a ClusterImageSet
// given a release image
func GenerateImageSetJob(cd *hivev1.ClusterDeployment, releaseImage, serviceAccountName, httpProxy, httpsProxy, noProxy string, nodeSelector map[string]string, tolerations []corev1.Toleration) *batchv1.Job {
	logger := log.WithFields(log.Fields{
		"clusterdeployment": types.NamespacedName{Namespace: cd.Namespace, Name: cd.Name}.String(),
	})

	logger.Debug("generating cluster image set job")

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "common",
			MountPath: "/common",
		},
	}

	podSpec := corev1.PodSpec{
		NodeSelector:  nodeSelector,
		Tolerations:   tolerations,
		RestartPolicy: corev1.RestartPolicyOnFailure,
		InitContainers: []corev1.Container{
			{
				Name:            "release",
				Image:           releaseImage,
				ImagePullPolicy: corev1.PullIfNotPresent,
				Command:         []string{"/bin/sh", "-c"},
				Args:            []string{"cp -v /release-manifests/image-references /release-manifests/release-metadata  /common/"},
				VolumeMounts:    volumeMounts,
			},
		},
		Containers: []corev1.Container{
			{
				Name:            "hiveutil",
				Image:           images.GetHiveImage(),
				ImagePullPolicy: images.GetHiveImagePullPolicy(),
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
		},
		Volumes: []corev1.Volume{
			{
				Name: "common",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		},
		ServiceAccountName: serviceAccountName,
		ImagePullSecrets:   []corev1.LocalObjectReference{{Name: constants.GetMergedPullSecretName(cd)}},
	}

	completions := int32(1)
	// make sure the deadline is small enough so that the controller can
	// react to job failing and provide appropiate status update to the
	// user.
	// The cluster version operator that pulls the release image similar to this
	// job has active deadline of 2 minutes (https://github.com/openshift/cluster-version-operator/blob/84b3884e422739dbbfa33078e62349752b5afa18/pkg/cvo/updatepayload.go#L146)
	// and that should be good to follow here too.
	deadline := int64((2 * time.Minute).Seconds())
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
	controllerutils.SetProxyEnvVars(&podSpec, httpProxy, httpsProxy, noProxy)

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
	controllerutils.AddLogFieldsEnvVar(cd, job)

	return job
}

// GetImageSetJobName returns the expected name of the imageset job for a ClusterImageSet.
func GetImageSetJobName(cdName string) string {
	return apihelpers.GetResourceName(cdName, "imageset")
}
