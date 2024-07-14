package install

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	// provisionJobDeadline is the maximum time that provision job will be allowed to run.
	// when this deadline is reached, the provision attempt will be marked failed.
	// since provision jobs can include cleanup before attempting installation, this should
	// include maximum of both these durations to allow for a successfull attempt.
	provisionJobDeadline = 3 * time.Hour
	// deprovisionJobDeadline is the maximum time that deprovision job will be allowed to run.
	// when this deadline is reached, the deprovision attempt will be marked failed.
	deprovisionJobDeadline = 1 * time.Hour
)

func AWSAssumeRoleSecretName(secretPrefix string) string {
	return secretPrefix + "-aws-assume-role-config"
}

// CopyAWSServiceProviderSecret copies the AWS service provider secret to the dest namespace
// when HiveAWSServiceProviderCredentialsSecretRefEnvVar is set in envVars. The secret
// name in the dest namespace will be the value set in HiveAWSServiceProviderCredentialsSecretRefEnvVar.
func CopyAWSServiceProviderSecret(client client.Client, destNamespace string, envVars []corev1.EnvVar, owner metav1.Object, scheme *runtime.Scheme) error {
	hiveNS := controllerutils.GetHiveNamespace()

	spSecretName := controllerutils.AWSServiceProviderSecretName("")
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

// AWSAssumeRoleConfig creates or updates a secret with an AWS credentials file containing:
// - Role configuration for AssumeRole, pointing to...
// - A profile containing the source credentials for AssumeRole.
func AWSAssumeRoleConfig(client client.Client, role *hivev1aws.AssumeRole, secretName, secretNamespace string, owner metav1.Object, scheme *runtime.Scheme) error {

	// Credentials source
	credsSecret := &corev1.Secret{}
	credsSecretName := controllerutils.AWSServiceProviderSecretName(owner.GetName())
	if err := client.Get(
		context.TODO(),
		types.NamespacedName{
			Namespace: secretNamespace,
			Name:      credsSecretName,
		},
		credsSecret); err != nil {
		return errors.Wrapf(err, "failed to load credentials source secret %s", credsSecretName)
	}
	// The old credential_process flow documented creating this with [default].
	// For backward compatibility, accept that, but convert to [profile source].
	sourceProfile := strings.Replace(string(credsSecret.Data[constants.AWSConfigSecretKey]), `[default]`, `[profile source]`, 1)

	extID := ""
	if role.ExternalID != "" {
		extID = fmt.Sprintf("external_id = %s\n", role.ExternalID)
	}

	// Build the config file
	configFile := fmt.Sprintf(`[default]
source_profile = source
role_arn = %s
%s
%s
`,
		role.RoleARN, extID, sourceProfile)

	// Load the config secret if it already exists
	configSecret := &corev1.Secret{}
	if err := client.Get(context.TODO(), types.NamespacedName{Namespace: secretNamespace, Name: secretName}, configSecret); err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to load config secret %s", secretName)
		}
		// Not found -- build it
		configSecret = &corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				APIVersion: corev1.SchemeGroupVersion.String(),
				Kind:       "Secret",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: secretNamespace,
				Name:      secretName,
			},
			Data: map[string][]byte{
				constants.AWSConfigSecretKey: []byte(configFile),
			},
		}
		if err := controllerutil.SetOwnerReference(owner, configSecret, scheme); err != nil {
			return err
		}
		return client.Create(context.TODO(), configSecret)
	}

	// Secret exists -- do we need to update it? Compare data and owner references.
	origSecret := configSecret.DeepCopy()
	configSecret.Data[constants.AWSConfigSecretKey] = []byte(configFile)
	// SetOwnerReference is a no-op if the owner is already registered
	if err := controllerutil.SetOwnerReference(owner, configSecret, scheme); err != nil {
		return err
	}
	if reflect.DeepEqual(origSecret, configSecret) {
		return nil
	}

	return client.Update(context.TODO(), configSecret)
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
		{
			// The command needs to load secrets and configmaps (e.g. for cloud
			// credentials) from the same namespace as the CD.
			Name:  "CLUSTERDEPLOYMENT_NAMESPACE",
			Value: cd.Namespace,
		},
		{
			Name:  "INSTALLCONFIG_SECRET_NAME",
			Value: cd.Spec.Provisioning.InstallConfigSecretRef.Name,
		},
		{
			Name:  "PULLSECRET_SECRET_NAME",
			Value: constants.GetMergedPullSecretName(cd),
		},
	}

	env = append(env, extraEnvVars...)

	// The installmanager pod expects several empty directories to exist for its use
	emptyDirs := map[string]string{
		"output":        "/output",
		"logs":          "/logs",
		"installconfig": "/installconfig",
		"pullsecret":    "/pullsecret",
	}

	var credentialRef, certificateRef string

	switch {
	case cd.Spec.Platform.AlibabaCloud != nil:
		credentialRef = cd.Spec.Platform.AlibabaCloud.CredentialsSecretRef.Name
	case cd.Spec.Platform.AWS != nil:
		credentialRef = cd.Spec.Platform.AWS.CredentialsSecretRef.Name
		if credentialRef == "" {
			credentialRef = AWSAssumeRoleSecretName(cd.Name)
		}
		emptyDirs["aws"] = constants.AWSCredsMount
	case cd.Spec.Platform.Azure != nil:
		credentialRef = cd.Spec.Platform.Azure.CredentialsSecretRef.Name
		emptyDirs["azure"] = constants.AzureCredentialsDir
	case cd.Spec.Platform.GCP != nil:
		credentialRef = cd.Spec.Platform.GCP.CredentialsSecretRef.Name
		emptyDirs["gcp"] = constants.GCPCredentialsDir
	case cd.Spec.Platform.OpenStack != nil:
		credentialRef = cd.Spec.Platform.OpenStack.CredentialsSecretRef.Name
		emptyDirs["openstack"] = constants.OpenStackCredentialsDir
		if cd.Spec.Platform.OpenStack.CertificatesSecretRef != nil && cd.Spec.Platform.OpenStack.CertificatesSecretRef.Name != "" {
			emptyDirs["openstack-certificates"] = constants.OpenStackCertificatesDir
			certificateRef = cd.Spec.Platform.OpenStack.CertificatesSecretRef.Name
		}
	case cd.Spec.Platform.VSphere != nil:
		credentialRef, certificateRef = cd.Spec.Platform.VSphere.CredentialsSecretRef.Name, cd.Spec.Platform.VSphere.CertificatesSecretRef.Name
		// TODO: I don't think we actually use this dir. Can we get rid of it?
		emptyDirs["vsphere-credentials"] = constants.VSphereCredentialsDir
		emptyDirs["vsphere-certificates"] = constants.VSphereCertificatesDir
	case cd.Spec.Platform.Ovirt != nil:
		credentialRef, certificateRef = cd.Spec.Platform.Ovirt.CredentialsSecretRef.Name, cd.Spec.Platform.Ovirt.CertificatesSecretRef.Name
		emptyDirs["ovirt-credentials"] = constants.OvirtCredentialsDir
		emptyDirs["ovirt-certificates"] = constants.OvirtCertificatesDir
	case cd.Spec.Platform.IBMCloud != nil:
		credentialRef = cd.Spec.Platform.IBMCloud.CredentialsSecretRef.Name
	case cd.Spec.Platform.BareMetal != nil:
		if cd.Spec.Platform.BareMetal.LibvirtSSHPrivateKeySecretRef.Name != "" {
			env = append(env, corev1.EnvVar{
				Name:  "LIBVIRT_SSH_KEYS_SECRET_NAME",
				Value: cd.Spec.Platform.BareMetal.LibvirtSSHPrivateKeySecretRef.Name,
			})
			emptyDirs["libvirtsshkeys"] = constants.LibvirtSSHPrivateKeyDir
		}
	}

	if credentialRef != "" {
		env = append(env,
			corev1.EnvVar{
				Name:  "CREDS_SECRET_NAME",
				Value: credentialRef,
			})
	}
	if certificateRef != "" {
		env = append(env,
			corev1.EnvVar{
				Name:  "CERTS_SECRET_NAME",
				Value: certificateRef,
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

	// Additional manifests if supplied via configmap or secret
	if cmr, sr := cd.Spec.Provisioning.ManifestsConfigMapRef, cd.Spec.Provisioning.ManifestsSecretRef; cmr != nil || sr != nil {
		// Webhooks should prevent both of these getting set; but if somehow they do, the secret gets precedence
		if sr != nil {
			env = append(env, corev1.EnvVar{
				Name:  "MANIFESTS_SECRET_NAME",
				Value: sr.Name,
			})
		} else {
			env = append(env, corev1.EnvVar{
				Name:  "MANIFESTS_CONFIGMAP_NAME",
				Value: cmr.Name,
			})
		}
		emptyDirs["manifests"] = "/manifests"
	}

	// If this cluster is using a custom BoundServiceAccountSigningKey, mount volume for the bound service account signing key:
	if ref := cd.Spec.BoundServiceAccountSignkingKeySecretRef; ref != nil {
		env = append(env, corev1.EnvVar{
			Name:  "BOUND_TOKEN_SIGNING_KEY_SECRET_NAME",
			Value: ref.Name,
		})
		emptyDirs["bound-token-signing-key"] = constants.BoundServiceAccountSigningKeyDir
	}

	if ref := cd.Spec.Provisioning.SSHPrivateKeySecretRef; ref != nil {
		env = append(env, corev1.EnvVar{
			Name:  "SSH_PRIVATE_KEY_SECRET_PATH",
			Value: ref.Name,
		})
		emptyDirs["sshkeys"] = constants.SSHPrivateKeyDir
	}

	// Create all the empty directories we need
	volumes, volumeMounts := baseVolumesAndMounts()
	for volname, dir := range emptyDirs {
		volumes = append(volumes, corev1.Volume{
			Name: volname,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      volname,
			MountPath: dir,
		})
	}

	// Signal to fake an installation:
	if controllerutils.IsFakeCluster(cd) {
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

	// This is used when scheduling the installer pod. It ensures that installer pods don't overwhelm
	// a given node's memory.
	memoryRequest := resource.MustParse("800Mi")

	// This container just needs to copy the required install binaries to the shared emptyDir volume,
	// where our container will run them. This is effectively downloading the all-in-one installer.
	initContainers := []corev1.Container{
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
	}
	// Don't pull the CLI image in minimal mode. This disables must-gather!
	if minimal, err := strconv.ParseBool(cd.Annotations[constants.MinimalInstallModeAnnotation]); err != nil || !minimal {
		initContainers = append(initContainers, corev1.Container{
			Name:            "cli",
			Image:           *cd.Status.CLIImage,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Env:             env,
			Command:         []string{"/bin/sh", "-c"},
			// Large file copy here has shown to cause problems in clusters under load, safer to copy then rename to the file the install manager is waiting for
			// so it doesn't try to run a partially copied binary.
			Args:         []string{"cp -v /usr/bin/oc /output/oc.tmp && mv -v /output/oc.tmp /output/oc && ls -la /output"},
			VolumeMounts: volumeMounts,
		})
	}
	containers := []corev1.Container{
		{
			Name:            "hive",
			Image:           images.GetHiveImage(),
			ImagePullPolicy: images.GetHiveClusterProvisionImagePullPolicy(),
			Env:             append(env, cd.Spec.Provisioning.InstallerEnv...),
			Command:         []string{"/usr/bin/hiveutil"},
			Args: []string{
				"install-manager",
				"--work-dir", "/output",
				"--log-level", "debug",
				cd.Namespace, provisionName,
			},
			VolumeMounts: volumeMounts,
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
		InitContainers:     initContainers,
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
	controllerutils.AddLogFieldsEnvVar(provision, job)

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
	controllerutils.AddLogFieldsEnvVar(req, job)

	return job, nil
}

func baseVolumesAndMounts() ([]corev1.Volume, []corev1.VolumeMount) {
	// All prov/deprov pods get the clusterwide certificate bundle, which includes
	// the proxy trustedCA if configured. See https://docs.openshift.com/container-platform/4.12/networking/configuring-a-custom-pki.html#certificate-injection-using-operators_configuring-a-custom-pki
	volumes := []corev1.Volume{
		{
			Name: constants.TrustedCAConfigMapName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: constants.TrustedCAConfigMapName,
					},
					Items: []corev1.KeyToPath{
						{
							Key:  constants.TrustedCABundleFile,
							Path: constants.TrustedCABundleFile,
						},
					},
				},
			},
		},
	}
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      constants.TrustedCAConfigMapName,
			MountPath: constants.TrustedCABundleDir,
		},
	}
	return volumes, volumeMounts
}

// envAndVolumes creates lists of EnvVar, Volume, and VolumeMount suitable for including in a Pod spec
// that's going to run a hiveutil deprovision command (including aws-tag-deprovision).
// Args:
//   - ns (required): The namespace in which the credsName and certsName Secrets can be found.
//   - credsVolName, credsDir: The name and mount point of an EmptyDir into which the hiveutil side knows
//     how to unpack credentials from the secret named by credsName. If either is unspecified, no creds
//     Volume/VolumeMount will be included in the return.
//   - credsName: The name of a Secret in the `ns` namespace containing credentials in whatever form
//     hiveutil expects for the given cluod provider. If specified, it is returned in a
//     "CREDS_SECRET_NAME" EnvVar; otherwise no such env var is included.
//   - certsVolName, certsDir, certsName: Same as their creds* counterparts, but for certificates.
func envAndVolumes(ns, credsVolName, credsDir, credsName, certsVolName, certsDir, certsName string) ([]corev1.EnvVar, []corev1.Volume, []corev1.VolumeMount) {
	volumes, volumeMounts := baseVolumesAndMounts()
	env := []corev1.EnvVar{
		{
			Name:  "CLUSTERDEPLOYMENT_NAMESPACE",
			Value: ns,
		},
	}
	if credsName != "" {
		env = append(env, corev1.EnvVar{
			Name:  "CREDS_SECRET_NAME",
			Value: credsName,
		})
	}
	if credsVolName != "" && credsDir != "" {
		volumes = append(volumes, corev1.Volume{
			Name: credsVolName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      credsVolName,
			MountPath: credsDir,
		})
	}
	if certsName != "" {
		env = append(env, corev1.EnvVar{
			Name:  "CERTS_SECRET_NAME",
			Value: certsName,
		})
	}
	if certsVolName != "" && certsDir != "" {
		volumes = append(volumes, corev1.Volume{
			Name: certsVolName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      certsVolName,
			MountPath: certsDir,
		})
	}
	return env, volumes, volumeMounts
}

func completeAWSDeprovisionJob(req *hivev1.ClusterDeprovision, job *batchv1.Job) {
	credentialRef := req.Spec.Platform.AWS.CredentialsSecretRef.Name
	if credentialRef == "" {
		credentialRef = AWSAssumeRoleSecretName(req.Name)
	}
	env, volumes, mounts := envAndVolumes(
		req.Namespace,
		"aws-creds", constants.AWSCredsMount, credentialRef,
		"", "", "")
	hostedZoneRole := ""
	if req.Spec.Platform.AWS.HostedZoneRole != nil {
		hostedZoneRole = *req.Spec.Platform.AWS.HostedZoneRole
	}
	args := []string{
		"aws-tag-deprovision",
		"--creds-dir", constants.AWSCredsMount,
		"--loglevel", "debug",
		"--region", req.Spec.Platform.AWS.Region,
		"--hosted-zone-role", hostedZoneRole,
	}
	// LEGACY: BaseDomain should always be set in new code. This conditional is only in case we're
	// reconciling an old ClusterDeprovision created before we added the cluster-domain arg. Such
	// deprovisions will leak DNS entries if they came from shared-VPC clusters.
	if req.Spec.BaseDomain != "" {
		args = append(args, "--cluster-domain", req.Spec.ClusterName+"."+req.Spec.BaseDomain)
	}

	args = append(args, fmt.Sprintf("kubernetes.io/cluster/%s=owned", req.Spec.InfraID))

	containers := []corev1.Container{
		{
			Name:            "deprovision",
			Image:           images.GetHiveImage(),
			ImagePullPolicy: images.GetHiveImagePullPolicy(),
			Env:             env,
			Command:         []string{"/usr/bin/hiveutil"},
			Args:            args,
			VolumeMounts:    mounts,
		},
	}
	if len(req.Spec.ClusterID) > 0 {
		// Also cleanup anything with the tag for the legacy cluster ID (credentials still using this for example)
		containers[0].Args = append(containers[0].Args, fmt.Sprintf("openshiftClusterID=%s", req.Spec.ClusterID))
	}
	job.Spec.Template.Spec.Containers = containers
	job.Spec.Template.Spec.Volumes = volumes
}

func completeAzureDeprovisionJob(req *hivev1.ClusterDeprovision, job *batchv1.Job) {
	env, volumes, volumeMounts := envAndVolumes(
		req.Namespace,
		"azure", constants.AzureCredentialsDir, req.Spec.Platform.Azure.CredentialsSecretRef.Name,
		"", "", "")
	containers := []corev1.Container{
		{
			Name:            "deprovision",
			Image:           images.GetHiveImage(),
			ImagePullPolicy: images.GetHiveImagePullPolicy(),
			Env:             env,
			Command:         []string{"/usr/bin/hiveutil"},
			Args: []string{
				"deprovision", "azure",
				req.Spec.InfraID,
				"--loglevel", "debug",
				"--creds-dir", constants.AzureCredentialsDir,
			},
			VolumeMounts: volumeMounts,
		},
	}
	if req.Spec.Platform.Azure.CloudName != nil {
		containers[0].Args = append(containers[0].Args, "--azure-cloud-name", req.Spec.Platform.Azure.CloudName.Name())
	}
	if req.Spec.Platform.Azure.ResourceGroupName != nil {
		containers[0].Args = append(containers[0].Args, "--azure-resource-group-name", *req.Spec.Platform.Azure.ResourceGroupName)
	}
	job.Spec.Template.Spec.Containers = containers
	job.Spec.Template.Spec.Volumes = volumes

}

func completeGCPDeprovisionJob(req *hivev1.ClusterDeprovision, job *batchv1.Job) {
	env, volumes, volumeMounts := envAndVolumes(
		req.Namespace,
		"gcp", constants.GCPCredentialsDir, req.Spec.Platform.GCP.CredentialsSecretRef.Name,
		"", "", "")
	npid := ""
	if req.Spec.Platform.GCP.NetworkProjectID != nil {
		npid = *req.Spec.Platform.GCP.NetworkProjectID
	}
	job.Spec.Template.Spec.Containers = []corev1.Container{
		{
			Name:            "deprovision",
			Image:           images.GetHiveImage(),
			ImagePullPolicy: images.GetHiveImagePullPolicy(),
			Env:             env,
			Command:         []string{"/usr/bin/hiveutil"},
			Args: []string{
				"deprovision", "gcp",
				"--loglevel", "debug",
				"--creds-dir", constants.GCPCredentialsDir,
				"--region", req.Spec.Platform.GCP.Region,
				"--network-project-id", npid,
				req.Spec.InfraID,
			},
			VolumeMounts: volumeMounts,
		},
	}
	job.Spec.Template.Spec.Volumes = volumes
}

func completeOpenStackDeprovisionJob(req *hivev1.ClusterDeprovision, job *batchv1.Job) {
	certRef := ""
	if req.Spec.Platform.OpenStack.CertificatesSecretRef != nil {
		// Can theoretically still be "", but that's okay.
		certRef = req.Spec.Platform.OpenStack.CertificatesSecretRef.Name
	}
	env, volumes, volumeMounts := envAndVolumes(
		req.Namespace,
		"openstack", constants.OpenStackCredentialsDir, req.Spec.Platform.OpenStack.CredentialsSecretRef.Name,
		"openstack-certificates", constants.OpenStackCertificatesDir, certRef)
	cmd := []string{"/usr/bin/hiveutil"}
	args := []string{
		"deprovision", "openstack",
		"--loglevel", "debug",
		"--creds-dir", constants.OpenStackCredentialsDir,
		"--cloud", req.Spec.Platform.OpenStack.Cloud,
		req.Spec.InfraID,
	}
	job.Spec.Template.Spec.Containers = []corev1.Container{
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
	job.Spec.Template.Spec.Volumes = volumes
}

func completeVSphereDeprovisionJob(req *hivev1.ClusterDeprovision, job *batchv1.Job) {
	env, volumes, volumeMounts := envAndVolumes(
		req.Namespace,
		"vsphere-creds", constants.VSphereCredentialsDir, req.Spec.Platform.VSphere.CredentialsSecretRef.Name,
		"vsphere-certificates", constants.VSphereCertificatesDir, req.Spec.Platform.VSphere.CertificatesSecretRef.Name)
	job.Spec.Template.Spec.Containers = []corev1.Container{
		{
			Name:            "deprovision",
			Image:           images.GetHiveImage(),
			ImagePullPolicy: images.GetHiveImagePullPolicy(),
			Env:             env,
			Command:         []string{"/usr/bin/hiveutil"},
			Args: []string{
				"deprovision", "vsphere",
				"--vsphere-vcenter", req.Spec.Platform.VSphere.VCenter,
				"--loglevel", "debug",
				"--creds-dir", constants.VSphereCredentialsDir,
				req.Spec.InfraID,
			},
			VolumeMounts: volumeMounts,
		},
	}
	job.Spec.Template.Spec.Volumes = volumes
}

func completeOvirtDeprovisionJob(req *hivev1.ClusterDeprovision, job *batchv1.Job) {
	env, volumes, volumeMounts := envAndVolumes(
		req.Namespace,
		"ovirt-credentials", constants.OvirtCredentialsDir, req.Spec.Platform.Ovirt.CredentialsSecretRef.Name,
		"ovirt-certificates", constants.OvirtCertificatesDir, req.Spec.Platform.Ovirt.CertificatesSecretRef.Name)

	job.Spec.Template.Spec.Containers = []corev1.Container{
		{
			Name:            "deprovision",
			Image:           images.GetHiveImage(),
			ImagePullPolicy: images.GetHiveImagePullPolicy(),
			Env:             env,
			Command:         []string{"/usr/bin/hiveutil"},
			Args: []string{
				"deprovision", "ovirt",
				"--ovirt-cluster-id", req.Spec.Platform.Ovirt.ClusterID,
				"--loglevel", "debug",
				"--creds-dir", constants.OvirtCredentialsDir,
				req.Spec.InfraID,
			},
			VolumeMounts: volumeMounts,
		},
	}
	job.Spec.Template.Spec.Volumes = volumes
}

func completeIBMCloudDeprovisionJob(req *hivev1.ClusterDeprovision, job *batchv1.Job) {
	env, _, _ := envAndVolumes(
		req.Namespace,
		"", "", req.Spec.Platform.IBMCloud.CredentialsSecretRef.Name,
		"", "", "")

	job.Spec.Template.Spec.Containers = []corev1.Container{
		{
			Name:            "deprovision",
			Image:           images.GetHiveImage(),
			ImagePullPolicy: images.GetHiveImagePullPolicy(),
			Env:             env,
			Command:         []string{"/usr/bin/hiveutil"},
			Args: []string{
				"deprovision", "ibmcloud",
				req.Spec.InfraID,
				"--region", req.Spec.Platform.IBMCloud.Region,
				"--base-domain", req.Spec.Platform.IBMCloud.BaseDomain,
				"--cluster-name", req.Spec.ClusterName,
				"--loglevel", "debug",
			},
		},
	}
}

func completeAlibabaCloudDeprovisionJob(req *hivev1.ClusterDeprovision, job *batchv1.Job) {
	env, _, _ := envAndVolumes(
		req.Namespace,
		"", "", req.Spec.Platform.AlibabaCloud.CredentialsSecretRef.Name,
		"", "", "")

	job.Spec.Template.Spec.Containers = []corev1.Container{
		{
			Name:            "deprovision",
			Image:           images.GetHiveImage(),
			ImagePullPolicy: images.GetHiveImagePullPolicy(),
			Env:             env,
			Command:         []string{"/usr/bin/hiveutil"},
			Args: []string{
				"deprovision", "alibabacloud",
				req.Spec.InfraID,
				"--region", req.Spec.Platform.AlibabaCloud.Region,
				"--cluster-name", req.Spec.ClusterName,
				"--base-domain", req.Spec.Platform.AlibabaCloud.BaseDomain,
				"--loglevel", "debug",
			},
		},
	}
}
