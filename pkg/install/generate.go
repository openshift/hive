package install

import (
	"fmt"
	"os"
	"strconv"

	log "github.com/sirupsen/logrus"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apihelpers "github.com/openshift/hive/pkg/apis/helpers"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/constants"
)

const (
	// DefaultInstallerImage is the image that will be used to install a ClusterDeployment if no
	// image is specified through a ClusterImageSet reference or on the ClusterDeployment itself.
	DefaultInstallerImage = "registry.svc.ci.openshift.org/openshift/origin-v4.0:installer"

	defaultInstallerImagePullPolicy = corev1.PullAlways
	defaultHiveImagePullPolicy      = corev1.PullAlways

	tryInstallOnceAnnotation              = "hive.openshift.io/try-install-once"
	tryUninstallOnceAnnotation            = "hive.openshift.io/try-uninstall-once"
	clusterDeploymentGenerationAnnotation = "hive.openshift.io/cluster-deployment-generation"
	// InstallJobLabel is the label used for counting the number of install jobs in Hive
	InstallJobLabel = "hive.openshift.io/install"

	// UninstallJobLabel is the label used for counting the number of uninstall jobs in Hive
	UninstallJobLabel = "hive.openshift.io/uninstall"

	// ClusterDeploymentNameLabel is the label that is used to identify the installer pod of a particular cluster deployment
	ClusterDeploymentNameLabel = "hive.openshift.io/cluster-deployment-name"

	// SSHPrivateKeyDir is the directory where the generated Job will mount the ssh secret to
	SSHPrivateKeyDir = "/sshkeys"

	// SSHSecretPrivateKeyName is the key name holding the private key in the SSH secret
	SSHSecretPrivateKeyName = "ssh-privatekey"
)

var (
	// SSHPrivateKeyFilePath is the path to the private key contents (from the SSH secret)
	SSHPrivateKeyFilePath = fmt.Sprintf("%s/%s", SSHPrivateKeyDir, SSHSecretPrivateKeyName)
)

// GenerateInstallerJob creates a job to install an OpenShift cluster
// given a ClusterDeployment and an installer image.
func GenerateInstallerJob(
	cd *hivev1.ClusterDeployment,
	hiveImage, releaseImage string,
	serviceAccountName string,
	sshKey string) (*batchv1.Job, *corev1.PersistentVolumeClaim, error) {

	cdLog := log.WithFields(log.Fields{
		"clusterDeployment": cd.Name,
		"namespace":         cd.Namespace,
	})

	gatherLogs := os.Getenv(constants.GatherLogsEnvVar) == "true"
	gatherLogsStr := "false"
	if gatherLogs {
		gatherLogsStr = "true"
	}

	cdLog.Debug("generating installer job")
	tryOnce := false
	if cd.Annotations != nil {
		value, exists := cd.Annotations[tryInstallOnceAnnotation]
		tryOnce = exists && value == "true"
	}
	installJobName := GetInstallJobName(cd)
	env := []corev1.EnvVar{
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
			Name:  constants.GatherLogsEnvVar,
			Value: gatherLogsStr,
		},
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
	if releaseImage != "" {
		env = append(env, []corev1.EnvVar{
			{
				Name:  "OPENSHIFT_INSTALL_RELEASE_IMAGE_OVERRIDE",
				Value: releaseImage,
			},
		}...)
	}

	if cd.Spec.SSHKey != nil {
		env = append(env, corev1.EnvVar{
			Name: "SSH_PUB_KEY",
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
			Name: "output",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}
	if cd.Spec.SSHKey != nil {
		volumes = append(volumes, corev1.Volume{
			Name: "sshkeys",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: cd.Spec.SSHKey.Name,
				},
			},
		})
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "output",
			MountPath: "/output",
		},
	}

	if gatherLogs {
		// Add a volume where we will store full logs from both the installer, and the
		// cluster itself (assuming we made it far enough).
		volumes = append(volumes, corev1.Volume{
			Name: "logs",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: installJobName,
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "logs",
			MountPath: "/logs",
		})
	}

	if cd.Spec.SSHKey != nil {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "sshkeys",
			MountPath: SSHPrivateKeyDir,
		})

		// ok when the private key isn't in the secret, as the installmanager
		// will just gracefully handle the file not being present
		env = append(env, corev1.EnvVar{
			Name:  "SSH_PRIV_KEY_PATH",
			Value: SSHPrivateKeyFilePath,
		})
	}

	if cd.Status.InstallerImage == nil {
		return nil, nil, fmt.Errorf("installer image not resolved")
	}
	installerImage := *cd.Status.InstallerImage

	installerImagePullPolicy := defaultInstallerImagePullPolicy
	if cd.Spec.Images.InstallerImagePullPolicy != "" {
		installerImagePullPolicy = cd.Spec.Images.InstallerImagePullPolicy
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
			// Large file copy here has shown to cause problems in clusters under load, safer to copy then rename to the file the install manager is waiting for
			// so it doesn't try to run a partially copied binary.
			Args:         []string{"cp -v /bin/openshift-install /output/openshift-install.tmp && mv -v /output/openshift-install.tmp /output/openshift-install && ls -la /output"},
			VolumeMounts: volumeMounts,
		},
		{
			Name:            "hive",
			Image:           hiveImage,
			ImagePullPolicy: hiveImagePullPolicy,
			Env:             env,
			Command:         []string{"/bin/sh"},
			Args: []string{"-c",
				// Inlining a script to be run, we cannot assume a script to be in older images, nor that older images will output
				// a sleep-seconds.txt file. If one is written, we will sleep that number of seconds. This allows exponential backoff
				// for failing installs.
				fmt.Sprintf(
					"/usr/bin/hiveutil install-manager --work-dir /output --log-level debug --region %s %s %s; installer_result=$?; if [ -f /output/sleep-seconds.txt ]; then sleep_seconds=$(cat /output/sleep-seconds.txt); echo \"sleeping for $sleep_seconds seconds until next retry\"; sleep $sleep_seconds; fi; exit $installer_result",
					cd.Spec.Platform.AWS.Region,
					cd.Namespace,
					cd.Name),
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
		ImagePullSecrets:   []corev1.LocalObjectReference{{Name: constants.GetMergedPullSecretName(cd)}},
	}

	completions := int32(1)
	backoffLimit := int32(123456) // effectively limitless
	if tryOnce {
		backoffLimit = int32(0)
	}

	labels := map[string]string{
		InstallJobLabel:            "true",
		ClusterDeploymentNameLabel: cd.Name,
	}
	if cd.Labels != nil {
		typeStr, ok := cd.Labels[hivev1.HiveClusterTypeLabel]
		if ok {
			labels[hivev1.HiveClusterTypeLabel] = typeStr
		}
	}

	annotations := map[string]string{
		clusterDeploymentGenerationAnnotation: strconv.FormatInt(cd.Generation, 10),
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        installJobName,
			Namespace:   cd.Namespace,
			Annotations: annotations,
			Labels:      labels,
		},
		Spec: batchv1.JobSpec{
			Completions:  &completions,
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: podSpec,
			},
		},
	}

	// Return nil for the PVC if log gathering is disabled.
	var pvc *corev1.PersistentVolumeClaim
	if gatherLogs {
		pvc = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      installJobName, // re-use the job name
				Namespace: cd.Namespace,
				Labels:    labels,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("1Gi"),
					},
				},
			},
		}
	}

	return job, pvc, nil
}

// GetInstallJobName returns the expected name of the install job for a cluster deployment.
func GetInstallJobName(cd *hivev1.ClusterDeployment) string {
	return apihelpers.GetResourceName(cd.Name, "install")
}

// GetUninstallJobName returns the expected name of the deprovision job for a cluster deployment.
func GetUninstallJobName(name string) string {
	return apihelpers.GetResourceName(name, "uninstall")
}

// GenerateUninstallerJobForClusterDeployment creates a job to uninstall an OpenShift cluster
// given a ClusterDeployment and an installer image.
func GenerateUninstallerJobForClusterDeployment(
	cd *hivev1.ClusterDeployment, hiveImage string) (*batchv1.Job, error) {

	if cd.Spec.PreserveOnDelete {
		if cd.Status.Installed {
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

	hiveImagePullPolicy := defaultHiveImagePullPolicy
	if cd.Spec.Images.HiveImagePullPolicy != "" {
		hiveImagePullPolicy = cd.Spec.Images.HiveImagePullPolicy
	}

	infraID := cd.Status.InfraID
	clusterID := cd.Status.ClusterID
	name := GetUninstallJobName(cd.Name)

	return GenerateUninstallerJob(
		cd.Namespace,
		name,
		tryOnce,
		cd.Spec.AWS.Region,
		credentialsSecret,
		infraID,
		clusterID,
		hiveImage,
		hiveImagePullPolicy), nil
}

// GenerateUninstallerJobForDeprovisionRequest generates an uninstaller job for a given deprovision request
func GenerateUninstallerJobForDeprovisionRequest(
	req *hivev1.ClusterDeprovisionRequest, hiveImage string) (*batchv1.Job, error) {

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

	name := GetUninstallJobName(req.Name)

	return GenerateUninstallerJob(
		req.Namespace,
		name,
		tryOnce,
		req.Spec.Platform.AWS.Region,
		credentialsSecret,
		req.Spec.InfraID,
		req.Spec.ClusterID,
		hiveImage,
		defaultHiveImagePullPolicy), nil
}

// GenerateUninstallerJob generates a new uninstaller job
func GenerateUninstallerJob(
	namespace string,
	name string,
	tryOnce bool,
	region string,
	credentialsSecret string,
	infraID string,
	clusterID string,
	hiveImage string,
	hiveImagePullPolicy corev1.PullPolicy) *batchv1.Job {

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
			Image:           hiveImage,
			ImagePullPolicy: hiveImagePullPolicy,
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
	labels := map[string]string{UninstallJobLabel: "true"}

	job := &batchv1.Job{}
	job.Name = name
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
