package install

import (
	"fmt"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	apihelpers "github.com/openshift/hive/pkg/apis/helpers"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/controller/images"
)

const (
	// DefaultInstallerImage is the image that will be used to install a ClusterDeployment if no
	// image is specified through a ClusterImageSet reference or on the ClusterDeployment itself.
	DefaultInstallerImage = "registry.svc.ci.openshift.org/openshift/origin-v4.0:installer"

	defaultInstallerImagePullPolicy = corev1.PullAlways
	tryUninstallOnceAnnotation      = "hive.openshift.io/try-uninstall-once"
	azureAuthDir                    = "/.azure"
	azureAuthFile                   = azureAuthDir + "/osServicePrincipal.json"
	gcpAuthDir                      = "/.gcp"
	gcpAuthFile                     = gcpAuthDir + "/osServiceAccount.json"

	// SSHPrivateKeyDir is the directory where the generated Job will mount the ssh secret to
	SSHPrivateKeyDir = "/sshkeys"

	// SSHSecretPrivateKeyName is the key name holding the private key in the SSH secret
	SSHSecretPrivateKeyName = "ssh-privatekey"
)

var (
	// SSHPrivateKeyFilePath is the path to the private key contents (from the SSH secret)
	SSHPrivateKeyFilePath = fmt.Sprintf("%s/%s", SSHPrivateKeyDir, SSHSecretPrivateKeyName)
)

// InstallerPodSpec generates a spec for an installer pod.
func InstallerPodSpec(
	cd *hivev1.ClusterDeployment,
	provisionName string,
	releaseImage string,
	serviceAccountName string,
	pvcName string,
	skipGatherLogs bool,
) (*corev1.PodSpec, error) {

	pLog := log.WithFields(log.Fields{
		"clusterProvision": provisionName,
		"namespace":        cd.Namespace,
	})

	pLog.Debug("generating installer pod spec")

	env := []corev1.EnvVar{
		{
			// OPENSHIFT_INSTALL_INVOKER allows setting who launched the installer
			// (it is stored in the configmap openshift-config/openshift-install)
			Name:  "OPENSHIFT_INSTALL_INVOKER",
			Value: "hive",
		},
		{
			Name: "PULL_SECRET",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: constants.GetMergedPullSecretName(cd)},
					Key:                  corev1.DockerConfigJsonKey,
				},
			},
		},
		{
			Name: "SSH_PUB_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: cd.Spec.SSHKey,
					Key:                  "ssh-publickey",
				},
			},
		},
		// ok when the private key isn't in the secret, as the installmanager
		// will just gracefully handle the file not being present
		{
			Name:  "SSH_PRIV_KEY_PATH",
			Value: SSHPrivateKeyFilePath,
		},
	}
	volumes := []corev1.Volume{
		{
			Name: "sshkeys",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: cd.Spec.SSHKey.Name,
				},
			},
		},
		{
			Name: "output",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "sshkeys",
			MountPath: SSHPrivateKeyDir,
		},
		{
			Name:      "output",
			MountPath: "/output",
		},
	}

	switch {
	case cd.Spec.PlatformSecrets.AWS != nil:
		if len(cd.Spec.PlatformSecrets.AWS.Credentials.Name) > 0 {
			env = append(
				env,
				corev1.EnvVar{
					Name: "AWS_ACCESS_KEY_ID",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: cd.Spec.PlatformSecrets.AWS.Credentials,
							Key:                  "aws_access_key_id",
						},
					},
				},
				corev1.EnvVar{
					Name: "AWS_SECRET_ACCESS_KEY",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: cd.Spec.PlatformSecrets.AWS.Credentials,
							Key:                  "aws_secret_access_key",
						},
					},
				},
			)
		}
	case cd.Spec.PlatformSecrets.Azure != nil:
		if len(cd.Spec.PlatformSecrets.Azure.Credentials.Name) > 0 {
			volumes = append(volumes, corev1.Volume{
				Name: "azure",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: cd.Spec.PlatformSecrets.Azure.Credentials.Name,
					},
				},
			})
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      "azure",
				MountPath: azureAuthDir,
			})
			env = append(env, corev1.EnvVar{
				Name:  "AZURE_AUTH_LOCATION",
				Value: azureAuthFile,
			})
		}
	case cd.Spec.PlatformSecrets.GCP != nil:
		if len(cd.Spec.PlatformSecrets.GCP.Credentials.Name) > 0 {
			volumes = append(volumes, corev1.Volume{
				Name: "gcp",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: cd.Spec.PlatformSecrets.GCP.Credentials.Name,
					},
				},
			})
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      "gcp",
				MountPath: gcpAuthDir,
			})
			env = append(env, corev1.EnvVar{
				Name:  "GOOGLE_CREDENTIALS",
				Value: gcpAuthFile,
			})
		}
	}

	if releaseImage != "" {
		env = append(
			env,
			corev1.EnvVar{
				Name:  "OPENSHIFT_INSTALL_RELEASE_IMAGE_OVERRIDE",
				Value: releaseImage,
			},
		)
	}

	if !skipGatherLogs {
		// Add a volume where we will store full logs from both the installer, and the
		// cluster itself (assuming we made it far enough).
		volumes = append(volumes, corev1.Volume{
			Name: "logs",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcName,
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "logs",
			MountPath: "/logs",
		})
	} else {
		env = append(env, corev1.EnvVar{
			Name:  constants.SkipGatherLogsEnvVar,
			Value: "true",
		})
	}

	if cd.Status.InstallerImage == nil {
		return nil, fmt.Errorf("installer image not resolved")
	}
	installerImage := *cd.Status.InstallerImage

	if cd.Status.CLIImage == nil {
		return nil, fmt.Errorf("cli image not resolved")
	}
	cliImage := *cd.Status.CLIImage

	installerImagePullPolicy := defaultInstallerImagePullPolicy
	if cd.Spec.Images.InstallerImagePullPolicy != "" {
		installerImagePullPolicy = cd.Spec.Images.InstallerImagePullPolicy
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
			// Large file copy here has shown to cause problems in clusters under load, safer to copy then rename to the file the install manager is waiting for
			// so it doesn't try to run a partially copied binary.
			Args:         []string{"cp -v /bin/openshift-install /output/openshift-install.tmp && mv -v /output/openshift-install.tmp /output/openshift-install && ls -la /output"},
			VolumeMounts: volumeMounts,
		},
		{
			Name:            "cli",
			Image:           cliImage,
			ImagePullPolicy: installerImagePullPolicy,
			Env:             env,
			Command:         []string{"/bin/sh", "-c"},
			// Large file copy here has shown to cause problems in clusters under load, safer to copy then rename to the file the install manager is waiting for
			// so it doesn't try to run a partially copied binary.
			Args:         []string{"cp -v /usr/bin/oc /output/oc.tmp && mv -v /output/oc.tmp /output/oc && ls -la /output"},
			VolumeMounts: volumeMounts,
		},
		{
			Name:            "hive",
			Image:           images.GetHiveImage(),
			ImagePullPolicy: images.GetHiveImagePullPolicy(),
			Env:             env,
			Command:         []string{"/usr/bin/hiveutil"},
			Args: []string{"install-manager",
				"--work-dir", "/output",
				"--log-level", "debug",
				cd.Namespace, provisionName,
			},
			VolumeMounts: volumeMounts,
		},
	}

	return &corev1.PodSpec{
		DNSPolicy:          corev1.DNSClusterFirst,
		RestartPolicy:      corev1.RestartPolicyNever,
		Containers:         containers,
		Volumes:            volumes,
		ServiceAccountName: serviceAccountName,
		ImagePullSecrets:   []corev1.LocalObjectReference{{Name: constants.GetMergedPullSecretName(cd)}},
	}, nil
}

// GenerateInstallerJob creates a job to install an OpenShift cluster
// given a ClusterDeployment and an installer image.
func GenerateInstallerJob(provision *hivev1.ClusterProvision) (*batchv1.Job, error) {

	pLog := log.WithFields(log.Fields{
		"clusterProvision": provision.Name,
		"namespace":        provision.Namespace,
	})

	pLog.Debug("generating installer job")

	labels := provision.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	labels[constants.InstallJobLabel] = "true"

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetInstallJobName(provision),
			Namespace: provision.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: pointer.Int32Ptr(0),
			Completions:  pointer.Int32Ptr(1),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: provision.Spec.PodSpec,
			},
		},
	}

	return job, nil
}

// GetInstallJobName returns the expected name of the install job for a cluster provision.
func GetInstallJobName(provision *hivev1.ClusterProvision) string {
	return apihelpers.GetResourceName(provision.Name, "provision")
}

// GetUninstallJobName returns the expected name of the deprovision job for a cluster deployment.
func GetUninstallJobName(name string) string {
	return apihelpers.GetResourceName(name, "uninstall")
}

// GenerateUninstallerJobForDeprovisionRequest generates an uninstaller job for a given deprovision request
func GenerateUninstallerJobForDeprovisionRequest(
	req *hivev1.ClusterDeprovisionRequest) (*batchv1.Job, error) {

	tryOnce := false
	if req.Annotations != nil {
		value, exists := req.Annotations[tryUninstallOnceAnnotation]
		tryOnce = exists && value == "true"
	}

	restartPolicy := corev1.RestartPolicyOnFailure
	if tryOnce {
		restartPolicy = corev1.RestartPolicyNever
	}

	podSpec := corev1.PodSpec{
		DNSPolicy:     corev1.DNSClusterFirst,
		RestartPolicy: restartPolicy,
	}

	completions := int32(1)
	backoffLimit := int32(123456) // effectively limitless
	labels := map[string]string{
		constants.UninstallJobLabel:          "true",
		constants.ClusterDeploymentNameLabel: req.Name,
	}

	job := &batchv1.Job{}
	job.Name = GetUninstallJobName(req.Name)
	job.Namespace = req.Namespace
	job.ObjectMeta.Labels = labels
	job.ObjectMeta.Annotations = map[string]string{}
	job.Spec = batchv1.JobSpec{
		Completions:  &completions,
		BackoffLimit: &backoffLimit,
		Template: corev1.PodTemplateSpec{
			Spec: podSpec,
		},
	}

	switch {
	case req.Spec.Platform.AWS != nil:
		completeAWSDeprovisionJob(req, job)
	case req.Spec.Platform.Azure != nil:
		completeAzureDeprovisionJob(req, job)
	case req.Spec.Platform.GCP != nil:
		completeGCPDeprovisionJob(req, job)
	default:
		return nil, errors.New("deprovision requests currently not supported for platform")
	}

	return job, nil
}

func completeAWSDeprovisionJob(req *hivev1.ClusterDeprovisionRequest, job *batchv1.Job) {
	credentialsSecret := ""
	if len(req.Spec.Platform.AWS.Credentials.Name) > 0 {
		credentialsSecret = req.Spec.Platform.AWS.Credentials.Name
	}
	env := []corev1.EnvVar{}
	if len(credentialsSecret) > 0 {
		env = append(
			env,
			corev1.EnvVar{
				Name: "AWS_ACCESS_KEY_ID",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: credentialsSecret},
						Key:                  "aws_access_key_id",
					},
				},
			},
			corev1.EnvVar{
				Name: "AWS_SECRET_ACCESS_KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: credentialsSecret},
						Key:                  "aws_secret_access_key",
					},
				},
			},
		)
	}
	containers := []corev1.Container{
		{
			Name:            "deprovision",
			Image:           images.GetHiveImage(),
			ImagePullPolicy: images.GetHiveImagePullPolicy(),
			Env:             env,
			Command:         []string{"/usr/bin/hiveutil"},
			Args: []string{
				"aws-tag-deprovision",
				"--loglevel",
				"debug",
				"--region",
				req.Spec.Platform.AWS.Region,
				fmt.Sprintf("kubernetes.io/cluster/%s=owned", req.Spec.InfraID),
			},
		},
	}
	if len(req.Spec.ClusterID) > 0 {
		// Also cleanup anything with the tag for the legacy cluster ID (credentials still using this for example)
		containers[0].Args = append(containers[0].Args, fmt.Sprintf("openshiftClusterID=%s", req.Spec.ClusterID))
	}
	job.Spec.Template.Spec.Containers = containers
}

func completeAzureDeprovisionJob(req *hivev1.ClusterDeprovisionRequest, job *batchv1.Job) {
	volumes := []corev1.Volume{}
	volumeMounts := []corev1.VolumeMount{}
	env := []corev1.EnvVar{}
	volumes = append(volumes, corev1.Volume{
		Name: "azure",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: req.Spec.Platform.Azure.Credentials.Name,
			},
		},
	})
	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:      "azure",
		MountPath: azureAuthDir,
	})
	env = append(env, corev1.EnvVar{
		Name:  "AZURE_AUTH_LOCATION",
		Value: azureAuthFile,
	})
	containers := []corev1.Container{
		{
			Name:            "deprovision",
			Image:           images.GetHiveImage(),
			ImagePullPolicy: images.GetHiveImagePullPolicy(),
			Env:             env,
			Command:         []string{"/usr/bin/hiveutil"},
			Args: []string{
				"deprovision",
				"azure",
				"--loglevel",
				"debug",
				req.Spec.InfraID,
			},
			VolumeMounts: volumeMounts,
		},
	}
	job.Spec.Template.Spec.Containers = containers
	job.Spec.Template.Spec.Volumes = volumes

}

func completeGCPDeprovisionJob(req *hivev1.ClusterDeprovisionRequest, job *batchv1.Job) {
	volumes := []corev1.Volume{}
	volumeMounts := []corev1.VolumeMount{}
	env := []corev1.EnvVar{}
	volumes = append(volumes, corev1.Volume{
		Name: "gcp",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: req.Spec.Platform.GCP.Credentials.Name,
			},
		},
	})
	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:      "gcp",
		MountPath: gcpAuthDir,
	})
	env = append(env, corev1.EnvVar{
		Name:  "GOOGLE_CREDENTIALS",
		Value: gcpAuthFile,
	})
	containers := []corev1.Container{
		{
			Name:            "deprovision",
			Image:           images.GetHiveImage(),
			ImagePullPolicy: images.GetHiveImagePullPolicy(),
			Env:             env,
			Command:         []string{"/usr/bin/hiveutil"},
			Args: []string{
				"deprovision",
				"gcp",
				"--loglevel",
				"debug",
				"--region",
				req.Spec.Platform.GCP.Region,
				"--gcp-project-id",
				req.Spec.Platform.GCP.ProjectID,
				req.Spec.InfraID,
			},
			VolumeMounts: volumeMounts,
		},
	}
	job.Spec.Template.Spec.Containers = containers
	job.Spec.Template.Spec.Volumes = volumes
}
