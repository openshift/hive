package install

import (
	"fmt"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	apihelpers "github.com/openshift/hive/pkg/apis/helpers"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/controller/images"
)

const (
	azureAuthDir       = "/.azure"
	azureAuthFile      = azureAuthDir + "/osServicePrincipal.json"
	gcpAuthDir         = "/.gcp"
	gcpAuthFile        = gcpAuthDir + "/" + constants.GCPCredentialsName
	openStackCloudsDir = "/etc/openstack"
	vsphereCloudsDir   = "/vsphere"
	ovirtCloudsDir     = "/.ovirt"
	ovirtCADir         = "/.ovirt-ca"

	// SSHPrivateKeyDir is the directory where the generated Job will mount the ssh secret to
	SSHPrivateKeyDir = "/sshkeys"

	// LibvirtSSHPrivateKeyDir is the directory where the generated Job will mount the libvirt ssh secret to
	LibvirtSSHPrivateKeyDir = "/libvirtsshkeys"
)

var (
	// SSHPrivateKeyFilePath is the path to the private key contents (from the SSH secret)
	SSHPrivateKeyFilePath = fmt.Sprintf("%s/%s", SSHPrivateKeyDir, constants.SSHPrivateKeySecretKey)

	// LibvirtSSHPrivateKeyFilePath is the path to the private key contents (from the libvirt SSH secret)
	LibvirtSSHPrivateKeyFilePath = fmt.Sprintf("%s/%s", LibvirtSSHPrivateKeyDir, constants.SSHPrivateKeySecretKey)
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

	if cd.Spec.Provisioning == nil {
		return nil, fmt.Errorf("ClusterDeployment.Provisioning not set")
	}

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
	}
	volumes := []corev1.Volume{
		{
			Name: "output",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "installconfig",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: cd.Spec.Provisioning.InstallConfigSecretRef.Name,
				},
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
			Name:      "output",
			MountPath: "/output",
		},
		{
			Name:      "installconfig",
			MountPath: "/installconfig",
		},
		{
			Name:      "pullsecret",
			MountPath: "/pullsecret",
		},
	}

	switch {
	case cd.Spec.Platform.AWS != nil:
		env = append(
			env,
			corev1.EnvVar{
				Name: "AWS_ACCESS_KEY_ID",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: cd.Spec.Platform.AWS.CredentialsSecretRef,
						Key:                  constants.AWSAccessKeyIDSecretKey,
					},
				},
			},
			corev1.EnvVar{
				Name: "AWS_SECRET_ACCESS_KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: cd.Spec.Platform.AWS.CredentialsSecretRef,
						Key:                  constants.AWSSecretAccessKeySecretKey,
					},
				},
			},
		)
	case cd.Spec.Platform.Azure != nil:
		volumes = append(volumes, corev1.Volume{
			Name: "azure",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: cd.Spec.Platform.Azure.CredentialsSecretRef.Name,
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
	case cd.Spec.Platform.GCP != nil:
		volumes = append(volumes, corev1.Volume{
			Name: "gcp",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: cd.Spec.Platform.GCP.CredentialsSecretRef.Name,
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
	case cd.Spec.Platform.OpenStack != nil:
		volumes = append(volumes, corev1.Volume{
			Name: "openstack",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: cd.Spec.Platform.OpenStack.CredentialsSecretRef.Name,
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "openstack",
			MountPath: openStackCloudsDir,
		})

		// TODO: design/implement a system to sync over ClusterImageSet base OS images into OpenStack
		// so that each cluster install doesn't involve copying down, then uploading 2GB QCOW
		// images.
		// env = append(env, corev1.EnvVar{
		// 	Name:  "OPENSHIFT_INSTALL_OS_IMAGE_OVERRIDE",
		// 	Value: "jdiaz-rhcos-4.3",
		// })
	case cd.Spec.Platform.VSphere != nil:
		volumes = append(volumes, corev1.Volume{
			Name: "vsphere-certificates",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: cd.Spec.Platform.VSphere.CertificatesSecretRef.Name,
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "vsphere-certificates",
			MountPath: vsphereCloudsDir,
		})
		env = append(env, vSphereCredsEnvVars(cd.Spec.Platform.VSphere.CredentialsSecretRef.Name)...)
	case cd.Spec.Platform.Ovirt != nil:
		volumes = append(volumes,
			corev1.Volume{
				Name: "ovirt-credentials",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: cd.Spec.Platform.Ovirt.CredentialsSecretRef.Name,
					},
				},
			},
			corev1.Volume{
				Name: "ovirt-certificates",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: cd.Spec.Platform.Ovirt.CertificatesSecretRef.Name,
					},
				},
			},
		)
		volumeMounts = append(volumeMounts,
			corev1.VolumeMount{
				Name:      "ovirt-credentials",
				MountPath: ovirtCloudsDir,
			},
			corev1.VolumeMount{
				Name:      "ovirt-certificates",
				MountPath: ovirtCADir,
			},
		)
		env = append(env, oVirtCredsEnvVars(cd.Spec.Platform.Ovirt.CredentialsSecretRef.Name)...)
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

	if cd.Spec.Provisioning.ManifestsConfigMapRef != nil {
		volumes = append(
			volumes,
			corev1.Volume{
				Name: "manifests",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: cd.Spec.Provisioning.ManifestsConfigMapRef.Name,
						},
					},
				},
			},
		)
		volumeMounts = append(
			volumeMounts,
			corev1.VolumeMount{
				Name:      "manifests",
				MountPath: "/manifests",
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

	if cd.Spec.Provisioning.SSHPrivateKeySecretRef != nil {
		volumes = append(volumes, corev1.Volume{
			Name: "sshkeys",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: cd.Spec.Provisioning.SSHPrivateKeySecretRef.Name,
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "sshkeys",
			MountPath: SSHPrivateKeyDir,
		})
		env = append(env, corev1.EnvVar{
			Name:  constants.SSHPrivKeyPathEnvVar,
			Value: SSHPrivateKeyFilePath,
		})
	}

	if cd.Spec.Platform.BareMetal != nil && cd.Spec.Platform.BareMetal.LibvirtSSHPrivateKeySecretRef.Name != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "libvirtsshkeys",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: cd.Spec.Platform.BareMetal.LibvirtSSHPrivateKeySecretRef.Name,
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "libvirtsshkeys",
			MountPath: LibvirtSSHPrivateKeyDir,
		})
		env = append(env, corev1.EnvVar{
			Name:  constants.LibvirtSSHPrivKeyPathEnvVar,
			Value: LibvirtSSHPrivateKeyFilePath,
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

	hiveArg := fmt.Sprintf("/usr/bin/hiveutil install-manager --work-dir /output --log-level debug %s %s", cd.Namespace, provisionName)
	if cd.Spec.Platform.VSphere != nil {
		// Add vSphere certificates to CA trust.
		hiveArg = fmt.Sprintf("cp -vr %s/. /etc/pki/ca-trust/source/anchors/ && update-ca-trust && %s", vsphereCloudsDir, hiveArg)
	}
	if cd.Spec.Platform.Ovirt != nil {
		// Add oVirt certificates to CA trust.
		hiveArg = fmt.Sprintf("cp -vr %s/. /etc/pki/ca-trust/source/anchors/ && update-ca-trust && %s", ovirtCADir, hiveArg)
	}

	// This is used when scheduling the installer pod. It ensures that installer pods don't overwhelm
	// a given node's memory.
	memoryRequest := resource.MustParse("800Mi")

	// This container just needs to copy the required install binaries to the shared emptyDir volume,
	// where our container will run them. This is effectively downloading the all-in-one installer.
	containers := []corev1.Container{
		{
			Name:            "installer",
			Image:           installerImage,
			ImagePullPolicy: corev1.PullIfNotPresent,
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
			ImagePullPolicy: corev1.PullIfNotPresent,
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
			Env:             append(env, cd.Spec.Provisioning.InstallerEnv...),
			Command:         []string{"/bin/sh", "-c"},
			Args:            []string{hiveArg},
			VolumeMounts:    volumeMounts,
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: memoryRequest,
				},
			},
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

// GenerateUninstallerJobForDeprovision generates an uninstaller job for a given deprovision request
func GenerateUninstallerJobForDeprovision(
	req *hivev1.ClusterDeprovision) (*batchv1.Job, error) {

	restartPolicy := corev1.RestartPolicyOnFailure

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
			ObjectMeta: metav1.ObjectMeta{
				Labels: labels,
			},
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
	case req.Spec.Platform.OpenStack != nil:
		completeOpenStackDeprovisionJob(req, job)
	case req.Spec.Platform.VSphere != nil:
		completeVSphereDeprovisionJob(req, job)
	case req.Spec.Platform.Ovirt != nil:
		completeOvirtDeprovisionJob(req, job)
	default:
		return nil, errors.New("deprovision requests currently not supported for platform")
	}

	return job, nil
}

func completeAWSDeprovisionJob(req *hivev1.ClusterDeprovision, job *batchv1.Job) {
	credentialsSecret := ""
	if len(req.Spec.Platform.AWS.CredentialsSecretRef.Name) > 0 {
		credentialsSecret = req.Spec.Platform.AWS.CredentialsSecretRef.Name
	}
	containers := []corev1.Container{
		{
			Name:            "deprovision",
			Image:           images.GetHiveImage(),
			ImagePullPolicy: images.GetHiveImagePullPolicy(),
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
	if len(credentialsSecret) > 0 {
		containers[0].VolumeMounts = []corev1.VolumeMount{
			{
				Name:      "aws-creds",
				MountPath: constants.AWSCredsMount,
			},
		}
		job.Spec.Template.Spec.Volumes = []corev1.Volume{
			{
				Name: "aws-creds",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: credentialsSecret,
					},
				},
			},
		}
	}
}

func completeAzureDeprovisionJob(req *hivev1.ClusterDeprovision, job *batchv1.Job) {
	volumes := []corev1.Volume{}
	volumeMounts := []corev1.VolumeMount{}
	env := []corev1.EnvVar{}
	volumes = append(volumes, corev1.Volume{
		Name: "azure",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: req.Spec.Platform.Azure.CredentialsSecretRef.Name,
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

func completeGCPDeprovisionJob(req *hivev1.ClusterDeprovision, job *batchv1.Job) {
	volumes := []corev1.Volume{}
	volumeMounts := []corev1.VolumeMount{}
	env := []corev1.EnvVar{}
	volumes = append(volumes, corev1.Volume{
		Name: "gcp",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: req.Spec.Platform.GCP.CredentialsSecretRef.Name,
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
				req.Spec.InfraID,
			},
			VolumeMounts: volumeMounts,
		},
	}
	job.Spec.Template.Spec.Containers = containers
	job.Spec.Template.Spec.Volumes = volumes
}

func completeOpenStackDeprovisionJob(req *hivev1.ClusterDeprovision, job *batchv1.Job) {
	volumes := []corev1.Volume{}
	volumeMounts := []corev1.VolumeMount{}
	env := []corev1.EnvVar{}
	volumes = append(volumes, corev1.Volume{
		Name: "openstack",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: req.Spec.Platform.OpenStack.CredentialsSecretRef.Name,
			},
		},
	})
	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:      "openstack",
		MountPath: openStackCloudsDir,
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
				"openstack",
				"--loglevel",
				"debug",
				"--cloud",
				req.Spec.Platform.OpenStack.Cloud,
				req.Spec.InfraID,
			},
			VolumeMounts: volumeMounts,
		},
	}
	job.Spec.Template.Spec.Containers = containers
	job.Spec.Template.Spec.Volumes = volumes
}

func completeVSphereDeprovisionJob(req *hivev1.ClusterDeprovision, job *batchv1.Job) {
	volumes := []corev1.Volume{}
	volumeMounts := []corev1.VolumeMount{}
	volumes = append(volumes, corev1.Volume{
		Name: "vsphere-certificates",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: req.Spec.Platform.VSphere.CertificatesSecretRef.Name,
			},
		},
	})
	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:      "vsphere-certificates",
		MountPath: vsphereCloudsDir,
	})

	env := vSphereCredsEnvVars(req.Spec.Platform.VSphere.CredentialsSecretRef.Name)

	containers := []corev1.Container{
		{
			Name:            "deprovision",
			Image:           images.GetHiveImage(),
			ImagePullPolicy: images.GetHiveImagePullPolicy(),
			Env:             env,
			Command:         []string{"/bin/sh", "-c"},
			Args:            []string{fmt.Sprintf("cp -vr %s/. /etc/pki/ca-trust/source/anchors/ && update-ca-trust && /usr/bin/hiveutil deprovision vsphere --vsphere-vcenter %s --loglevel debug %s", vsphereCloudsDir, req.Spec.Platform.VSphere.VCenter, req.Spec.InfraID)},
			VolumeMounts:    volumeMounts,
		},
	}
	job.Spec.Template.Spec.Containers = containers
	job.Spec.Template.Spec.Volumes = volumes
}

func completeOvirtDeprovisionJob(req *hivev1.ClusterDeprovision, job *batchv1.Job) {
	volumes := []corev1.Volume{}
	volumeMounts := []corev1.VolumeMount{}
	volumes = append(volumes,
		corev1.Volume{
			Name: "ovirt-credentials",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: req.Spec.Platform.Ovirt.CredentialsSecretRef.Name,
				},
			},
		},
		corev1.Volume{
			Name: "ovirt-certificates",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: req.Spec.Platform.Ovirt.CertificatesSecretRef.Name,
				},
			},
		},
	)
	volumeMounts = append(volumeMounts,
		corev1.VolumeMount{
			Name:      "ovirt-credentials",
			MountPath: ovirtCloudsDir,
		},
		corev1.VolumeMount{
			Name:      "ovirt-certificates",
			MountPath: ovirtCADir,
		},
	)

	env := oVirtCredsEnvVars(req.Spec.Platform.Ovirt.CredentialsSecretRef.Name)

	containers := []corev1.Container{
		{
			Name:            "deprovision",
			Image:           images.GetHiveImage(),
			ImagePullPolicy: images.GetHiveImagePullPolicy(),
			Env:             env,
			Command:         []string{"/bin/sh", "-c"},
			Args:            []string{fmt.Sprintf("cp -vr %s/. /etc/pki/ca-trust/source/anchors/ && update-ca-trust && /usr/bin/hiveutil deprovision ovirt --ovirt-cluster-id %s --loglevel debug %s", ovirtCADir, req.Spec.Platform.Ovirt.ClusterID, req.Spec.InfraID)},
			VolumeMounts:    volumeMounts,
		},
	}
	job.Spec.Template.Spec.Containers = containers
	job.Spec.Template.Spec.Volumes = volumes
}

func vSphereCredsEnvVars(credentialsSecret string) []corev1.EnvVar {
	env := []corev1.EnvVar{}
	env = append(
		env,
		corev1.EnvVar{
			Name: constants.VSphereUsernameEnvVar,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: credentialsSecret},
					Key:                  constants.UsernameSecretKey,
				},
			},
		},
		corev1.EnvVar{
			Name: constants.VSpherePasswordEnvVar,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: credentialsSecret},
					Key:                  constants.PasswordSecretKey,
				},
			},
		},
	)
	return env
}

func oVirtCredsEnvVars(credentialsSecret string) []corev1.EnvVar {
	env := []corev1.EnvVar{}
	env = append(
		env,
		corev1.EnvVar{
			Name:  constants.OvirtConfigEnvVar,
			Value: ovirtCloudsDir + "/" + constants.OvirtCredentialsName,
		},
	)
	return env
}
