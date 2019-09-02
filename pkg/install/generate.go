package install

import (
	"fmt"

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

	tryUninstallOnceAnnotation = "hive.openshift.io/try-uninstall-once"

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

	if cd.Spec.PlatformSecrets.AWS != nil && len(cd.Spec.PlatformSecrets.AWS.Credentials.Name) > 0 {
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
				"--region", cd.Spec.Platform.AWS.Region,
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

// GetLegacyInstallJobName returns the expected name of the legacy install job for a cluster deployment.
func GetLegacyInstallJobName(cd *hivev1.ClusterDeployment) string {
	return apihelpers.GetResourceName(cd.Name, "install")
}

// GetUninstallJobName returns the expected name of the deprovision job for a cluster deployment.
func GetUninstallJobName(name string) string {
	return apihelpers.GetResourceName(name, "uninstall")
}

// GenerateUninstallerJobForClusterDeployment creates a job to uninstall an OpenShift cluster
// given a ClusterDeployment and an installer image.
func GenerateUninstallerJobForClusterDeployment(cd *hivev1.ClusterDeployment) (*batchv1.Job, error) {

	if cd.Spec.PreserveOnDelete {
		if cd.Spec.Installed {
			return nil, fmt.Errorf("no creation of uninstaller job, because of PreserveOnDelete")
		}
	}

	if cd.Spec.AWS == nil {
		return nil, fmt.Errorf("only AWS ClusterDeployments currently supported")
	}

	tryOnce := false
	if cd.Annotations != nil {
		value, exists := cd.Annotations[tryUninstallOnceAnnotation]
		tryOnce = exists && value == "true"
	}

	credentialsSecret := ""
	if cd.Spec.PlatformSecrets.AWS != nil && len(cd.Spec.PlatformSecrets.AWS.Credentials.Name) > 0 {
		credentialsSecret = cd.Spec.PlatformSecrets.AWS.Credentials.Name
	}

	infraID := cd.Status.InfraID
	clusterID := cd.Status.ClusterID

	return GenerateUninstallerJob(
		cd.Namespace,
		cd.Name,
		tryOnce,
		cd.Spec.AWS.Region,
		credentialsSecret,
		infraID,
		clusterID,
	), nil
}

// GenerateUninstallerJobForDeprovisionRequest generates an uninstaller job for a given deprovision request
func GenerateUninstallerJobForDeprovisionRequest(
	req *hivev1.ClusterDeprovisionRequest) (*batchv1.Job, error) {

	if req.Spec.Platform.AWS == nil {
		return nil, fmt.Errorf("only AWS deprovision requests currently supported")
	}

	tryOnce := false
	if req.Annotations != nil {
		value, exists := req.Annotations[tryUninstallOnceAnnotation]
		tryOnce = exists && value == "true"
	}

	credentialsSecret := ""
	if len(req.Spec.Platform.AWS.Credentials.Name) > 0 {
		credentialsSecret = req.Spec.Platform.AWS.Credentials.Name
	}

	return GenerateUninstallerJob(
		req.Namespace,
		req.Name,
		tryOnce,
		req.Spec.Platform.AWS.Region,
		credentialsSecret,
		req.Spec.InfraID,
		req.Spec.ClusterID,
	), nil
}

// GenerateUninstallerJob generates a new uninstaller job
func GenerateUninstallerJob(
	namespace string,
	clusterName string,
	tryOnce bool,
	region string,
	credentialsSecret string,
	infraID string,
	clusterID string,
) *batchv1.Job {

	env := []corev1.EnvVar{}
	if len(credentialsSecret) > 0 {
		env = append(env, []corev1.EnvVar{
			{
				Name: "AWS_ACCESS_KEY_ID",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: credentialsSecret},
						Key:                  "aws_access_key_id",
					},
				},
			},
			{
				Name: "AWS_SECRET_ACCESS_KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: credentialsSecret},
						Key:                  "aws_secret_access_key",
					},
				},
			},
		}...)
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
				region,
				fmt.Sprintf("kubernetes.io/cluster/%s=owned", infraID),
			},
		},
	}
	if len(clusterID) > 0 {
		// Also cleanup anything with the tag for the legacy cluster ID (credentials still using this for example)
		containers[0].Args = append(containers[0].Args, fmt.Sprintf("openshiftClusterID=%s", clusterID))
	}

	restartPolicy := corev1.RestartPolicyOnFailure
	if tryOnce {
		restartPolicy = corev1.RestartPolicyNever
	}

	podSpec := corev1.PodSpec{
		DNSPolicy:     corev1.DNSClusterFirst,
		RestartPolicy: restartPolicy,
		Containers:    containers,
	}

	completions := int32(1)
	backoffLimit := int32(123456) // effectively limitless
	labels := map[string]string{
		constants.UninstallJobLabel:          "true",
		constants.ClusterDeploymentNameLabel: clusterName,
	}

	job := &batchv1.Job{}
	job.Name = GetUninstallJobName(clusterName)
	job.Namespace = namespace
	job.ObjectMeta.Labels = labels
	job.ObjectMeta.Annotations = map[string]string{}
	job.Spec = batchv1.JobSpec{
		Completions:  &completions,
		BackoffLimit: &backoffLimit,
		Template: corev1.PodTemplateSpec{
			Spec: podSpec,
		},
	}

	return job
}
