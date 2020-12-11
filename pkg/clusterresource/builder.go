package clusterresource

import (
	"fmt"
	"time"

	"github.com/ghodss/yaml"
	"github.com/openshift/installer/pkg/ipnet"
	installertypes "github.com/openshift/installer/pkg/types"
	"github.com/openshift/installer/pkg/validate"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
)

const (
	deleteAfterAnnotation    = "hive.openshift.io/delete-after"
	tryInstallOnceAnnotation = "hive.openshift.io/try-install-once"
)

// Builder can be used to build all artifacts required for to create a ClusterDeployment.
type Builder struct {
	// Name is the name of your Cluster. Will be used for both the ClusterDeployment.Name and the
	// ClusterDeployment.Spec.ClusterName, which encompasses the subdomain and cloud provider resource
	// tagging.
	Name string

	// Namespace where the ClusterDeployment and all associated artifacts will be created.
	Namespace string

	// Labels are labels to be added to the ClusterDeployment.
	Labels map[string]string

	// CloudBuilder encapsulates logic for building the objects for a specific cloud.
	CloudBuilder CloudBuilder

	// PullSecret is the secret to use when pulling images.
	PullSecret string

	// SSHPrivateKey is an optional SSH key to configure on hosts in the cluster. This would
	// typically be read from ~/.ssh/id_rsa.
	SSHPrivateKey string

	// SSHPublicKey is an optional public SSH key to configure on hosts in the cluster. This would
	// typically be read from ~/.ssh/id_rsa.pub. Must match the SSHPrivateKey.
	SSHPublicKey string

	// InstallOnce indicates that the provision job should not be retried on failure.
	InstallOnce bool

	// BaseDomain is the DNS base domain to be used for the cluster.
	BaseDomain string

	// WorkerNodesCount is the number of worker nodes to create in the cluster initially.
	WorkerNodesCount int64

	// ManageDNS can be set to true to enable Hive's automatic DNS zone creation and forwarding. (assuming
	// this is properly configured in HiveConfig)
	ManageDNS bool

	// DeleteAfter is the duration after which the cluster should be automatically destroyed, relative to
	// creationTimestamp. Stored as an annotation on the ClusterDeployment.
	DeleteAfter string

	// HibernateAfter is the duration after which a running cluster should be automatically hibernated.
	HibernateAfter *time.Duration

	// ServingCert is the contents of a serving certificate to be used for the cluster.
	ServingCert string

	// ServingCertKey is the contents of a key for the ServingCert.
	ServingCertKey string

	// Adopt is a flag indicating we're adopting a pre-existing cluster.
	Adopt bool

	// AdoptAdminKubeconfig is a cluster administrator admin kubeconfig typically obtained
	// from openshift-install. Required when adopting pre-existing clusters.
	AdoptAdminKubeconfig []byte

	// AdoptClusterID is the unique generated ID for a cluster being adopted.
	// Required when adopting pre-existing clusters.
	AdoptClusterID string

	// AdoptInfraID is the unique generated infrastructure ID for a cluster being adopted.
	// Required when adopting pre-existing clusters.
	AdoptInfraID string

	// AdoptAdminUsername is the admin username for an adopted cluster, typically written to disk
	// after openshift-install create-cluster. This field is optional when adopting.
	AdoptAdminUsername string

	// AdoptAdminPassword is the admin password for an adopted cluster, typically written to disk
	// after openshift-install create-cluster. This field is optional when adopting.
	AdoptAdminPassword string

	// InstallerManifests is a map of filename strings to bytes for files to inject into the installers
	// manifests dir before launching create-cluster.
	InstallerManifests map[string][]byte

	// ImageSet is the ClusterImageSet to use for this cluster.
	ImageSet string

	// ReleaseImage is a specific OpenShift release image to install this cluster with. Will override
	// ImageSet.
	ReleaseImage string

	// MachineNetwork is the subnet to use for the cluster's machine network.
	MachineNetwork string

	// SkipMachinePools should be true if you do not want Hive to manage MachineSets in the spoke cluster once it is installed.
	SkipMachinePools bool

	// AdditionalTrustBundle is a PEM-encoded X.509 certificate bundle
	// that will be added to the nodes' trusted certificate store.
	AdditionalTrustBundle string

	// InstallConfig Secret to be used as template for deployment install-config
	InstallConfigTemplate string
}

// Validate ensures that the builder's fields are logically configured and usable to generate the cluster resources.
func (o *Builder) Validate() error {
	if len(o.Name) == 0 {
		return fmt.Errorf("name is required")
	}
	if len(o.BaseDomain) == 0 {
		return fmt.Errorf("BaseDomain is required")
	}
	if o.CloudBuilder == nil {
		return fmt.Errorf("no CloudBuilder configured for this Builder")
	}
	if len(o.ImageSet) > 0 && len(o.ReleaseImage) > 0 {
		return fmt.Errorf("cannot set both ImageSet and ReleaseImage")
	}
	if len(o.ImageSet) == 0 && len(o.ReleaseImage) == 0 {
		return fmt.Errorf("must set either image set or release image")
	}

	if len(o.ServingCert) > 0 && len(o.ServingCertKey) == 0 {
		return fmt.Errorf("must set serving cert key to use with serving cert")
	}

	if o.Adopt {
		if len(o.AdoptAdminKubeconfig) == 0 || o.AdoptInfraID == "" || o.AdoptClusterID == "" {
			return fmt.Errorf("must specify the following fields to adopt a cluster: AdoptAdminKubeConfig AdoptInfraID AdoptClusterID")
		}

		if (o.AdoptAdminUsername != "" || o.AdoptAdminPassword != "") && !(o.AdoptAdminUsername != "" && o.AdoptAdminPassword != "") {
			return fmt.Errorf("either both AdoptAdminPassword and AdoptAdminUsername must be set, or neither")
		}
	} else {
		if len(o.AdoptAdminKubeconfig) > 0 || o.AdoptInfraID != "" || o.AdoptClusterID != "" || o.AdoptAdminUsername != "" || o.AdoptAdminPassword != "" {
			return fmt.Errorf("cannot set adoption fields if Adopt is false")
		}
	}

	if len(o.AdditionalTrustBundle) > 0 {
		if err := validate.CABundle(o.AdditionalTrustBundle); err != nil {
			return fmt.Errorf("AdditionalTrustBundle is not valid: %s", err.Error())
		}
	}

	return nil
}

// Build generates all resources using the fields configured.
func (o *Builder) Build() ([]runtime.Object, error) {

	if err := o.Validate(); err != nil {
		return nil, err
	}

	var allObjects []runtime.Object
	allObjects = append(allObjects, o.generateClusterDeployment())

	if mp := o.generateMachinePool(); mp != nil && !o.SkipMachinePools {
		allObjects = append(allObjects, o.generateMachinePool())
	}

	if o.InstallConfigTemplate != "" {
		installConfigSecret, err := o.mergeInstallConfigTemplate()
		if err != nil {
			return nil, fmt.Errorf("Encountered problems merging InstallConfigTemplate: %s", err.Error())
		}
		allObjects = append(allObjects, installConfigSecret)
	} else {
		installConfigSecret, err := o.generateInstallConfigSecret()
		if err != nil {
			return nil, err
		}
		allObjects = append(allObjects, installConfigSecret)
	}

	// TODO: maintain "include secrets" flag functionality? possible this should just be removed
	if len(o.PullSecret) != 0 {
		allObjects = append(allObjects, o.GeneratePullSecretSecret())
	}
	if o.SSHPrivateKey != "" {
		allObjects = append(allObjects, o.generateSSHPrivateKeySecret())
	}
	if o.ServingCertKey != "" && o.ServingCert != "" {
		allObjects = append(allObjects, o.generateServingCertSecret())
	}
	cloudCredsSecret := o.CloudBuilder.GenerateCredentialsSecret(o)
	if cloudCredsSecret != nil {
		allObjects = append(allObjects, cloudCredsSecret)
	}

	cloudCertificatesSecret := o.CloudBuilder.generateCloudCertificatesSecret(o)
	if cloudCertificatesSecret != nil {
		allObjects = append(allObjects, cloudCertificatesSecret)
	}

	if o.InstallerManifests != nil {
		allObjects = append(allObjects, o.generateInstallerManifestsConfigMap())
	}

	if o.Adopt {
		allObjects = append(allObjects, o.generateAdminKubeconfigSecret())
		if o.AdoptAdminUsername != "" {
			allObjects = append(allObjects, o.generateAdoptedAdminPasswordSecret())
		}
	}

	return allObjects, nil
}

func (o *Builder) generateClusterDeployment() *hivev1.ClusterDeployment {
	cd := &hivev1.ClusterDeployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterDeployment",
			APIVersion: hivev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        o.Name,
			Namespace:   o.Namespace,
			Annotations: map[string]string{},
			Labels:      o.Labels,
		},
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterName:  o.Name,
			BaseDomain:   o.BaseDomain,
			ManageDNS:    o.ManageDNS,
			Provisioning: &hivev1.Provisioning{},
		},
	}

	if o.SSHPrivateKey != "" {
		cd.Spec.Provisioning.SSHPrivateKeySecretRef = &corev1.LocalObjectReference{Name: o.getSSHPrivateKeySecretName()}
	}

	if o.InstallOnce {
		cd.Annotations[tryInstallOnceAnnotation] = "true"
	}

	if o.PullSecret != "" {
		cd.Spec.PullSecretRef = &corev1.LocalObjectReference{Name: o.GetPullSecretSecretName()}
	}

	if len(o.ServingCert) > 0 {
		cd.Spec.CertificateBundles = []hivev1.CertificateBundleSpec{
			{
				Name: "serving-cert",
				CertificateSecretRef: corev1.LocalObjectReference{
					Name: fmt.Sprintf("%s-serving-cert", o.Name),
				},
			},
		}
		cd.Spec.ControlPlaneConfig.ServingCertificates.Default = "serving-cert"
		cd.Spec.Ingress = []hivev1.ClusterIngress{
			{
				Name:               "default",
				Domain:             fmt.Sprintf("apps.%s.%s", o.Name, o.BaseDomain),
				ServingCertificate: "serving-cert",
			},
		}
	}

	if o.DeleteAfter != "" {
		cd.ObjectMeta.Annotations[deleteAfterAnnotation] = o.DeleteAfter
	}

	if o.HibernateAfter != nil {
		cd.Spec.HibernateAfter = &metav1.Duration{Duration: *o.HibernateAfter}
	}

	if o.Adopt {
		cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{
			ClusterID:                o.AdoptClusterID,
			InfraID:                  o.AdoptInfraID,
			AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: o.getAdoptAdminKubeconfigSecretName()},
		}
		cd.Spec.Installed = true
		if o.AdoptAdminUsername != "" {
			cd.Spec.ClusterMetadata.AdminPasswordSecretRef = corev1.LocalObjectReference{
				Name: o.getAdoptAdminPasswordSecretName(),
			}
		}
	}

	if o.InstallerManifests != nil {
		cd.Spec.Provisioning.ManifestsConfigMapRef = &corev1.LocalObjectReference{
			Name: o.getManifestsConfigMapName(),
		}
	}

	if o.ReleaseImage != "" {
		cd.Spec.Provisioning.ReleaseImage = o.ReleaseImage
	} else if o.ImageSet != "" {
		cd.Spec.Provisioning.ImageSetRef = &hivev1.ClusterImageSetReference{Name: o.ImageSet}
	}

	cd.Spec.Provisioning.InstallConfigSecretRef = corev1.LocalObjectReference{Name: o.getInstallConfigSecretName()}
	cd.Spec.Platform = o.CloudBuilder.GetCloudPlatform(o)

	return cd
}

func (o *Builder) generateInstallConfigSecret() (*corev1.Secret, error) {
	installConfig := &installertypes.InstallConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: o.Name,
		},
		TypeMeta: metav1.TypeMeta{
			APIVersion: installertypes.InstallConfigVersion,
		},
		SSHKey:     o.SSHPublicKey,
		BaseDomain: o.BaseDomain,
		Networking: &installertypes.Networking{
			NetworkType:    "OpenShiftSDN",
			ServiceNetwork: []ipnet.IPNet{*ipnet.MustParseCIDR("172.30.0.0/16")},
			ClusterNetwork: []installertypes.ClusterNetworkEntry{
				{
					CIDR:       *ipnet.MustParseCIDR("10.128.0.0/14"),
					HostPrefix: 23,
				},
			},
			MachineNetwork: []installertypes.MachineNetworkEntry{
				{
					CIDR: *ipnet.MustParseCIDR(o.MachineNetwork),
				},
			},
		},
		ControlPlane: &installertypes.MachinePool{
			Name:     "master",
			Replicas: pointer.Int64Ptr(3),
		},
		Compute: []installertypes.MachinePool{
			{
				Name:     "worker",
				Replicas: &o.WorkerNodesCount,
			},
		},
		AdditionalTrustBundle: o.AdditionalTrustBundle,
	}

	o.CloudBuilder.addInstallConfigPlatform(o, installConfig)

	d, err := yaml.Marshal(installConfig)
	if err != nil {
		return nil, err
	}
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      o.getInstallConfigSecretName(),
			Namespace: o.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"install-config.yaml": string(d),
		},
	}, nil
}

func (o *Builder) mergeInstallConfigTemplate() (*corev1.Secret, error) {
	ic := new(InstallConfigTemplate)
	err := yaml.Unmarshal([]byte(o.InstallConfigTemplate), ic)
	if err != nil {
		return nil, fmt.Errorf("Error parsing installconfigtemplate: %s", err.Error())
	}
	ic.BaseDomain = o.BaseDomain
	ic.MetaData.Name = o.Name

	d, err := yaml.Marshal(ic)
	if err != nil {
		return nil, err
	}

	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      o.getInstallConfigSecretName(),
			Namespace: o.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"install-config.yaml": string(d),
		},
	}, nil
}

func (o *Builder) generateMachinePool() *hivev1.MachinePool {
	mp := &hivev1.MachinePool{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MachinePool",
			APIVersion: hivev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-worker", o.Name),
			Namespace: o.Namespace,
		},
		Spec: hivev1.MachinePoolSpec{
			ClusterDeploymentRef: corev1.LocalObjectReference{
				Name: o.Name,
			},
			Name:     "worker",
			Replicas: pointer.Int64Ptr(o.WorkerNodesCount),
		},
	}
	o.CloudBuilder.addMachinePoolPlatform(o, mp)
	return mp
}

func (o *Builder) getInstallConfigSecretName() string {
	return fmt.Sprintf("%s-install-config", o.Name)
}

// GeneratePullSecretSecret returns a Kubernetes Secret containing the pull secret to be
// used for pulling images.
func (o *Builder) GeneratePullSecretSecret() *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      o.GetPullSecretSecretName(),
			Namespace: o.Namespace,
		},
		Type: corev1.SecretTypeDockerConfigJson,
		StringData: map[string]string{
			corev1.DockerConfigJsonKey: o.PullSecret,
		},
	}
}

// generateSSHPrivateKeySecret returns a Kubernetes Secret containing the SSH private
// key to be used.
func (o *Builder) generateSSHPrivateKeySecret() *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      o.getSSHPrivateKeySecretName(),
			Namespace: o.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			constants.SSHPrivateKeySecretKey: o.SSHPrivateKey,
		},
	}
}

func (o *Builder) generateServingCertSecret() *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      o.getServingCertSecretName(),
			Namespace: o.Namespace,
		},
		Type: corev1.SecretTypeTLS,
		StringData: map[string]string{
			constants.TLSCrtSecretKey: o.ServingCert,
			constants.TLSKeySecretKey: o.ServingCertKey,
		},
	}
}

func (o *Builder) generateAdminKubeconfigSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      o.getAdoptAdminKubeconfigSecretName(),
			Namespace: o.Namespace,
		},
		Data: map[string][]byte{
			constants.KubeconfigSecretKey:    o.AdoptAdminKubeconfig,
			constants.RawKubeconfigSecretKey: o.AdoptAdminKubeconfig,
		},
	}
}

func (o *Builder) generateInstallerManifestsConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      o.getManifestsConfigMapName(),
			Namespace: o.Namespace,
		},
		BinaryData: o.InstallerManifests,
	}
}

func (o *Builder) generateAdoptedAdminPasswordSecret() *corev1.Secret {
	if o.AdoptAdminUsername == "" {
		return nil
	}
	adminPasswordSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      o.getAdoptAdminPasswordSecretName(),
			Namespace: o.Namespace,
		},
		StringData: map[string]string{
			"username": o.AdoptAdminUsername,
			"password": o.AdoptAdminPassword,
		},
	}
	return adminPasswordSecret
}

func (o *Builder) getManifestsConfigMapName() string {
	return fmt.Sprintf("%s-manifests", o.Name)
}
func (o *Builder) getAdoptAdminPasswordSecretName() string {
	return fmt.Sprintf("%s-adopted-admin-password", o.Name)
}

func (o *Builder) getServingCertSecretName() string {
	return fmt.Sprintf("%s-serving-cert", o.Name)
}

func (o *Builder) getAdoptAdminKubeconfigSecretName() string {
	return fmt.Sprintf("%s-adopted-admin-kubeconfig", o.Name)
}

// TODO: handle long cluster names.
func (o *Builder) getSSHPrivateKeySecretName() string {
	return fmt.Sprintf("%s-ssh-private-key", o.Name)
}

// TODO: handle long cluster names.
func (o *Builder) GetPullSecretSecretName() string {
	return fmt.Sprintf("%s-pull-secret", o.Name)
}

// CloudBuilder interface exposes the functions we will use to set cloud specific portions of the cluster's resources.
type CloudBuilder interface {
	addMachinePoolPlatform(o *Builder, mp *hivev1.MachinePool)
	addInstallConfigPlatform(o *Builder, ic *installertypes.InstallConfig)
	generateCloudCertificatesSecret(o *Builder) *corev1.Secret

	GetCloudPlatform(o *Builder) hivev1.Platform
	CredsSecretName(o *Builder) string
	GenerateCredentialsSecret(o *Builder) *corev1.Secret
}
