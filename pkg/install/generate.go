package install

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	apihelpers "github.com/openshift/hive/apis/helpers"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/controller/images"
	"github.com/openshift/hive/pkg/controller/utils"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	azureAuthDir          = "/.azure"
	azureAuthFile         = azureAuthDir + "/osServicePrincipal.json"
	gcpAuthDir            = "/.gcp"
	gcpAuthFile           = gcpAuthDir + "/" + constants.GCPCredentialsName
	boundSASigningKeyDir  = "/boundsasigningkey"
	boundSASigningKeyFile = boundSASigningKeyDir + "/" + constants.BoundServiceAccountSigningKeyFile
	openStackCloudsDir    = "/etc/openstack"
	openStackCADir        = "/etc/openstack-ca"
	vsphereCloudsDir      = "/vsphere"
	ovirtCloudsDir        = "/.ovirt"
	ovirtCADir            = "/.ovirt-ca"
	ibmCloudsDir          = "/ibmcloud"

	// SSHPrivateKeyDir is the directory where the generated Job will mount the ssh secret to
	SSHPrivateKeyDir = "/sshkeys"

	// LibvirtSSHPrivateKeyDir is the directory where the generated Job will mount the libvirt ssh secret to
	LibvirtSSHPrivateKeyDir = "/libvirtsshkeys"

	// provisionJobDeadline is the maximum time that provision job will be allowed to run.
	// when this deadline is reached, the provision attempt will be marked failed.
	// since provision jobs can include cleanup before attempting installation, this should
	// include maximum of both these durations to allow for a successfull attempt.
	provisionJobDeadline = 3 * time.Hour
	// deprovisionJobDeadline is the maximum time that deprovision job will be allowed to run.
	// when this deadline is reached, the deprovision attempt will be marked failed.
	deprovisionJobDeadline = 1 * time.Hour
)

var (
	// SSHPrivateKeyFilePath is the path to the private key contents (from the SSH secret)
	SSHPrivateKeyFilePath = fmt.Sprintf("%s/%s", SSHPrivateKeyDir, constants.SSHPrivateKeySecretKey)

	// LibvirtSSHPrivateKeyFilePath is the path to the private key contents (from the libvirt SSH secret)
	LibvirtSSHPrivateKeyFilePath = fmt.Sprintf("%s/%s", LibvirtSSHPrivateKeyDir, constants.SSHPrivateKeySecretKey)
)

func AWSAssumeRoleSecretName(secretPrefix string) string {
	return secretPrefix + "-aws-assume-role-config"
}

// CopyAWSServiceProviderSecret copies the AWS service provider secret to the dest namespace
// when HiveAWSServiceProviderCredentialsSecretRefEnvVar is set in envVars. The secret
// name in the dest namespace will be the value set in HiveAWSServiceProviderCredentialsSecretRefEnvVar.
func CopyAWSServiceProviderSecret(client client.Client, destNamespace string, envVars []corev1.EnvVar, owner metav1.Object, scheme *runtime.Scheme) error {
	hiveNS := controllerutils.GetHiveNamespace()

	spSecretName := os.Getenv(constants.HiveAWSServiceProviderCredentialsSecretRefEnvVar)
	if spSecretName == "" {
		// If the src secret reference wasn't found, then don't attempt to copy the secret.
		return nil
	}

	foundDest := false
	var destSecretName string
	for _, envVar := range envVars {
		if envVar.Name == constants.HiveAWSServiceProviderCredentialsSecretRefEnvVar {
			destSecretName = envVar.Value
			foundDest = true
		}
	}
	if !foundDest {
		// If the dest secret reference wasn't found, then don't attempt to copy the secret.
		return nil
	}

	src := types.NamespacedName{Name: spSecretName, Namespace: hiveNS}
	dest := types.NamespacedName{Name: destSecretName, Namespace: destNamespace}
	return controllerutils.CopySecret(client, src, dest, owner, scheme)
}

// AWSAssumeRoleCLIConfig creates a secret that can assume the role using the hiveutil
// credential_process helper.
func AWSAssumeRoleCLIConfig(client client.Client, role *hivev1aws.AssumeRole, secretName, secretNamespace string, owner metav1.Object, scheme *runtime.Scheme) error {
	cmd := "/usr/bin/hiveutil"
	args := []string{"install-manager", "aws-credentials"}
	args = append(args, []string{"--namespace", secretNamespace}...)
	args = append(args, []string{"--role-arn", role.RoleARN}...)
	if role.ExternalID != "" {
		args = append(args, []string{"--external-id", role.ExternalID}...)
	}

	cmd = fmt.Sprintf("%s %s", cmd, strings.Join(args, " "))

	template := `[default]
credential_process = %s
`
	data := fmt.Sprintf(template, cmd)

	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: secretNamespace,
			Name:      secretName,
		},
		Data: map[string][]byte{
			constants.AWSConfigSecretKey: []byte(data),
		},
	}
	if err := controllerutil.SetOwnerReference(owner, secret, scheme); err != nil {
		return nil
	}

	return client.Create(context.TODO(), secret)
}

// InstallerPodSpec generates a spec for an installer pod.
func InstallerPodSpec(
	cd *hivev1.ClusterDeployment,
	provisionName,
	releaseImage,
	serviceAccountName,
	httpProxy, httpsProxy, noProxy string,
	extraEnvVars []corev1.EnvVar,
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

	env = append(env, extraEnvVars...)

	volumes := []corev1.Volume{
		{
			Name: "output",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "logs",
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
			Name:      "logs",
			MountPath: "/logs",
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
	case cd.Spec.Platform.AlibabaCloud != nil:
		env = append(
			env,
			corev1.EnvVar{
				Name: "ALIBABA_CLOUD_ACCESS_KEY_ID",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: cd.Spec.Platform.AlibabaCloud.CredentialsSecretRef,
						Key:                  constants.AlibabaCloudAccessKeyIDSecretKey,
						Optional:             pointer.BoolPtr(false),
					},
				},
			},
			corev1.EnvVar{
				Name: "ALIBABA_CLOUD_ACCESS_KEY_SECRET",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: cd.Spec.Platform.AlibabaCloud.CredentialsSecretRef,
						Key:                  constants.AlibabaCloudAccessKeySecretSecretKey,
						Optional:             pointer.BoolPtr(false),
					},
				},
			})
	case cd.Spec.Platform.AWS != nil:
		credentialRef := cd.Spec.Platform.AWS.CredentialsSecretRef
		if credentialRef.Name == "" {
			credentialRef = corev1.LocalObjectReference{Name: AWSAssumeRoleSecretName(cd.Name)}
		}
		env = append(
			env,
			corev1.EnvVar{
				Name: "AWS_ACCESS_KEY_ID",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: credentialRef,
						Key:                  constants.AWSAccessKeyIDSecretKey,
						Optional:             pointer.BoolPtr(true),
					},
				},
			},
			corev1.EnvVar{
				Name: "AWS_SECRET_ACCESS_KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: credentialRef,
						Key:                  constants.AWSSecretAccessKeySecretKey,
						Optional:             pointer.BoolPtr(true),
					},
				},
			},
			corev1.EnvVar{
				Name:  "AWS_SDK_LOAD_CONFIG",
				Value: "true",
			},
			corev1.EnvVar{
				Name:  "AWS_CONFIG_FILE",
				Value: filepath.Join(constants.AWSCredsMount, constants.AWSConfigSecretKey),
			},
		)

		volumes = append(volumes, corev1.Volume{
			Name: "aws",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: credentialRef.Name,
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "aws",
			MountPath: constants.AWSCredsMount,
		})
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
		if cd.Spec.Platform.OpenStack.CertificatesSecretRef != nil && cd.Spec.Platform.OpenStack.CertificatesSecretRef.Name != "" {
			volumes = append(volumes, corev1.Volume{
				Name: "openstack-certificates",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: cd.Spec.Platform.OpenStack.CertificatesSecretRef.Name,
					},
				},
			})
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      "openstack-certificates",
				MountPath: openStackCADir,
			})
		}
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
	case cd.Spec.Platform.IBMCloud != nil:
		env = append(env, corev1.EnvVar{
			Name: constants.IBMCloudAPIKeyEnvVar,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: cd.Spec.Platform.IBMCloud.CredentialsSecretRef,
					Key:                  constants.IBMCloudAPIKeySecretKey,
					Optional:             pointer.BoolPtr(false),
				},
			},
		})
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

	// Mount additional manifests if supplied via configmap or secret
	if cmr, sr := cd.Spec.Provisioning.ManifestsConfigMapRef, cd.Spec.Provisioning.ManifestsSecretRef; cmr != nil || sr != nil {
		var vs corev1.VolumeSource
		// Webhooks should prevent both of these getting set; but if somehow they do, the secret gets precedence
		if sr != nil {
			vs = corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: sr.Name,
				},
			}
		} else {
			vs = corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cmr.Name,
					},
				},
			}
		}
		volumes = append(
			volumes,
			corev1.Volume{
				Name:         "manifests",
				VolumeSource: vs,
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

	// If this cluster is using a custom BoundServiceAccountSigningKey, mount volume for the bound service account signing key:
	if cd.Spec.BoundServiceAccountSignkingKeySecretRef != nil {
		volumes = append(volumes, corev1.Volume{
			Name: "bound-token-signing-key",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: cd.Spec.BoundServiceAccountSignkingKeySecretRef.Name,
					Items: []corev1.KeyToPath{
						{
							Key:  constants.BoundServiceAccountSigningKeyFile,
							Path: constants.BoundServiceAccountSigningKeyFile,
						},
					},
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "bound-token-signing-key",
			MountPath: boundSASigningKeyDir,
		})
		env = append(env, corev1.EnvVar{
			Name:  constants.BoundServiceAccountSigningKeyEnvVar,
			Value: boundSASigningKeyFile,
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

	// Signal to fake an installation:
	if utils.IsFakeCluster(cd) {
		env = append(env, corev1.EnvVar{
			Name:  constants.FakeClusterInstallEnvVar,
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

	hiveArg := fmt.Sprintf("/usr/bin/hiveutil install-manager --work-dir /output --log-level debug %s %s", cd.Namespace, provisionName)
	if cd.Spec.Platform.VSphere != nil {
		// Add vSphere certificates to CA trust.
		hiveArg = fmt.Sprintf("cp -vr %s/. /etc/pki/ca-trust/source/anchors/ && update-ca-trust && %s", vsphereCloudsDir, hiveArg)
	}
	if cd.Spec.Platform.Ovirt != nil {
		// Add oVirt certificates to CA trust.
		hiveArg = fmt.Sprintf("cp -vr %s/. /etc/pki/ca-trust/source/anchors/ && update-ca-trust && %s", ovirtCADir, hiveArg)
	}
	if cd.Spec.Platform.OpenStack != nil && cd.Spec.Platform.OpenStack.CertificatesSecretRef != nil && cd.Spec.Platform.OpenStack.CertificatesSecretRef.Name != "" {
		// Add OpenStack certificates to CA trust.
		hiveArg = fmt.Sprintf("cp -vr %s/. /etc/pki/ca-trust/source/anchors/ && update-ca-trust && %s", openStackCADir, hiveArg)
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
			ImagePullPolicy: images.GetHiveClusterProvisionImagePullPolicy(),
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

	podSpec := &corev1.PodSpec{
		DNSPolicy:          corev1.DNSClusterFirst,
		RestartPolicy:      corev1.RestartPolicyNever,
		Containers:         containers,
		Volumes:            volumes,
		ServiceAccountName: serviceAccountName,
		ImagePullSecrets:   []corev1.LocalObjectReference{{Name: constants.GetMergedPullSecretName(cd)}},
	}
	controllerutils.SetProxyEnvVars(podSpec, httpProxy, httpsProxy, noProxy)
	return podSpec, nil
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
			BackoffLimit:          pointer.Int32Ptr(0),
			Completions:           pointer.Int32Ptr(1),
			ActiveDeadlineSeconds: pointer.Int64Ptr(int64(provisionJobDeadline.Seconds())),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: provision.Spec.PodSpec,
			},
		},
	}
	utils.AddLogFieldsEnvVar(provision, job)

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
	req *hivev1.ClusterDeprovision,
	serviceAccountName, httpProxy, httpsProxy, noProxy string,
	extraEnvVars []corev1.EnvVar) (*batchv1.Job, error) {

	restartPolicy := corev1.RestartPolicyOnFailure

	podSpec := corev1.PodSpec{
		DNSPolicy:          corev1.DNSClusterFirst,
		RestartPolicy:      restartPolicy,
		ServiceAccountName: serviceAccountName,
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
		Completions:           &completions,
		BackoffLimit:          &backoffLimit,
		ActiveDeadlineSeconds: pointer.Int64Ptr(int64(deprovisionJobDeadline.Seconds())),
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: labels,
			},
			Spec: podSpec,
		},
	}

	switch {
	case req.Spec.Platform.AlibabaCloud != nil:
		completeAlibabaCloudDeprovisionJob(req, job)
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
	case req.Spec.Platform.IBMCloud != nil:
		completeIBMCloudDeprovisionJob(req, job)
	default:
		return nil, errors.New("deprovision requests currently not supported for platform")
	}

	for idx := range job.Spec.Template.Spec.Containers {
		job.Spec.Template.Spec.Containers[idx].Env = append(job.Spec.Template.Spec.Containers[idx].Env, extraEnvVars...)
	}
	controllerutils.SetProxyEnvVars(&job.Spec.Template.Spec, httpProxy, httpsProxy, noProxy)
	utils.AddLogFieldsEnvVar(req, job)

	return job, nil
}

func completeAWSDeprovisionJob(req *hivev1.ClusterDeprovision, job *batchv1.Job) {
	credentialRef := *req.Spec.Platform.AWS.CredentialsSecretRef
	if credentialRef.Name == "" {
		credentialRef = corev1.LocalObjectReference{Name: AWSAssumeRoleSecretName(req.Name)}
	}
	containers := []corev1.Container{
		{
			Name:            "deprovision",
			Image:           images.GetHiveImage(),
			ImagePullPolicy: images.GetHiveImagePullPolicy(),
			Env: []corev1.EnvVar{{
				Name: "AWS_ACCESS_KEY_ID",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: credentialRef,
						Key:                  constants.AWSAccessKeyIDSecretKey,
						Optional:             pointer.BoolPtr(true),
					},
				},
			}, {
				Name: "AWS_SECRET_ACCESS_KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: credentialRef,
						Key:                  constants.AWSSecretAccessKeySecretKey,
						Optional:             pointer.BoolPtr(true),
					},
				},
			}, {
				Name:  "AWS_SDK_LOAD_CONFIG",
				Value: "true",
			}, {
				Name:  "AWS_CONFIG_FILE",
				Value: filepath.Join(constants.AWSCredsMount, constants.AWSConfigSecretKey),
			}},
			Command: []string{"/usr/bin/hiveutil"},
			Args: []string{
				"aws-tag-deprovision",
				"--creds-dir",
				constants.AWSCredsMount,
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
					SecretName: credentialRef.Name,
				},
			},
		},
	}

	job.Spec.Template.Spec.Containers = containers
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
				req.Spec.InfraID,
				"--loglevel",
				"debug",
				"--creds-dir",
				azureAuthDir,
			},
			VolumeMounts: volumeMounts,
		},
	}
	if req.Spec.Platform.Azure.CloudName != nil {
		containers[0].Args = append(containers[0].Args, "--azure-cloud-name", req.Spec.Platform.Azure.CloudName.Name())
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
				"--creds-dir",
				gcpAuthDir,
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
	if req.Spec.Platform.OpenStack.CertificatesSecretRef != nil && req.Spec.Platform.OpenStack.CertificatesSecretRef.Name != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "openstack-certificates",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: req.Spec.Platform.OpenStack.CertificatesSecretRef.Name,
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "openstack-certificates",
			MountPath: openStackCADir,
		})
	}
	cmd := []string{"/usr/bin/hiveutil"}
	args := []string{
		"deprovision",
		"openstack",
		"--loglevel",
		"debug",
		"--creds-dir",
		openStackCloudsDir,
		"--cloud",
		req.Spec.Platform.OpenStack.Cloud,
		req.Spec.InfraID,
	}
	if req.Spec.Platform.OpenStack.CertificatesSecretRef != nil && req.Spec.Platform.OpenStack.CertificatesSecretRef.Name != "" {
		args = []string{fmt.Sprintf("cp -vr %s/. /etc/pki/ca-trust/source/anchors/ && update-ca-trust && %s", openStackCADir, strings.Join(append(cmd, args...), " "))}
		cmd = []string{"/bin/sh", "-c"}
	}
	containers := []corev1.Container{
		{
			Name:            "deprovision",
			Image:           images.GetHiveImage(),
			ImagePullPolicy: images.GetHiveImagePullPolicy(),
			Env:             env,
			Command:         cmd,
			Args:            args,
			VolumeMounts:    volumeMounts,
		},
	}
	job.Spec.Template.Spec.Containers = containers
	job.Spec.Template.Spec.Volumes = volumes
}

func completeVSphereDeprovisionJob(req *hivev1.ClusterDeprovision, job *batchv1.Job) {
	const vsphereCredsDir = "/vsphere-creds"
	volumes := []corev1.Volume{}
	volumeMounts := []corev1.VolumeMount{}
	volumes = append(volumes,
		corev1.Volume{
			Name: "vsphere-creds",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: req.Spec.Platform.VSphere.CredentialsSecretRef.Name,
				},
			},
		},
		corev1.Volume{
			Name: "vsphere-certificates",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: req.Spec.Platform.VSphere.CertificatesSecretRef.Name,
				},
			},
		},
	)
	volumeMounts = append(volumeMounts,
		corev1.VolumeMount{
			Name:      "vsphere-creds",
			MountPath: vsphereCredsDir,
		},
		corev1.VolumeMount{
			Name:      "vsphere-certificates",
			MountPath: vsphereCloudsDir,
		},
	)

	env := vSphereCredsEnvVars(req.Spec.Platform.VSphere.CredentialsSecretRef.Name)

	containers := []corev1.Container{
		{
			Name:            "deprovision",
			Image:           images.GetHiveImage(),
			ImagePullPolicy: images.GetHiveImagePullPolicy(),
			Env:             env,
			Command:         []string{"/bin/sh", "-c"},
			Args: []string{
				fmt.Sprintf(
					"cp -vr %s/. /etc/pki/ca-trust/source/anchors/ && "+
						"update-ca-trust && "+
						"/usr/bin/hiveutil deprovision vsphere --vsphere-vcenter %s --loglevel debug --creds-dir=%s %s",
					vsphereCloudsDir,
					req.Spec.Platform.VSphere.VCenter,
					vsphereCredsDir,
					req.Spec.InfraID,
				),
			},
			VolumeMounts: volumeMounts,
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
			Args: []string{
				fmt.Sprintf(
					"cp -vr %s/. /etc/pki/ca-trust/source/anchors/ && "+
						"update-ca-trust && "+
						"/usr/bin/hiveutil deprovision ovirt --ovirt-cluster-id %s --loglevel debug --creds-dir=%s %s",
					ovirtCADir,
					req.Spec.Platform.Ovirt.ClusterID,
					ovirtCloudsDir,
					req.Spec.InfraID,
				),
			},
			VolumeMounts: volumeMounts,
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

func completeIBMCloudDeprovisionJob(req *hivev1.ClusterDeprovision, job *batchv1.Job) {
	env := []corev1.EnvVar{}
	env = append(env, corev1.EnvVar{
		Name: constants.IBMCloudAPIKeyEnvVar,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: req.Spec.Platform.IBMCloud.CredentialsSecretRef,
				Key:                  constants.IBMCloudAPIKeySecretKey,
				Optional:             pointer.BoolPtr(false),
			},
		},
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
				"ibmcloud",
				req.Spec.InfraID,
				"--region",
				req.Spec.Platform.IBMCloud.Region,
				"--base-domain",
				req.Spec.Platform.IBMCloud.BaseDomain,
				"--cluster-name",
				req.Spec.ClusterName,
				"--loglevel",
				"debug",
			},
		},
	}
	job.Spec.Template.Spec.Containers = containers
}

func completeAlibabaCloudDeprovisionJob(req *hivev1.ClusterDeprovision, job *batchv1.Job) {
	env := []corev1.EnvVar{}
	env = append(
		env,
		corev1.EnvVar{
			Name: "ALIBABA_CLOUD_ACCESS_KEY_ID",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: req.Spec.Platform.AlibabaCloud.CredentialsSecretRef,
					Key:                  constants.AlibabaCloudAccessKeyIDSecretKey,
					Optional:             pointer.BoolPtr(false),
				},
			},
		},
		corev1.EnvVar{
			Name: "ALIBABA_CLOUD_ACCESS_KEY_SECRET",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: req.Spec.Platform.AlibabaCloud.CredentialsSecretRef,
					Key:                  constants.AlibabaCloudAccessKeySecretSecretKey,
					Optional:             pointer.BoolPtr(false),
				},
			},
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
				"alibabacloud",
				req.Spec.InfraID,
				"--region",
				req.Spec.Platform.AlibabaCloud.Region,
				"--cluster-name",
				req.Spec.ClusterName,
				"--base-domain",
				req.Spec.Platform.AlibabaCloud.BaseDomain,
				"--loglevel",
				"debug",
			},
		},
	}
	job.Spec.Template.Spec.Containers = containers
}
