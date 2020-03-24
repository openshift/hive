package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openshiftapiv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/hive/pkg/apis/hive/v1/aws"
	"github.com/openshift/hive/pkg/apis/hive/v1/azure"
	"github.com/openshift/hive/pkg/apis/hive/v1/baremetal"
	"github.com/openshift/hive/pkg/apis/hive/v1/gcp"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// Important: Run "make" to regenerate code after modifying this file

const (
	// FinalizerDeprovision is used on ClusterDeployments to ensure we run a successful deprovision
	// job before cleaning up the API object.
	FinalizerDeprovision string = "hive.openshift.io/deprovision"

	// HiveClusterTypeLabel is an optional label that can be applied to ClusterDeployments. It is
	// shown in short output, usable in searching, and adds metrics vectors which can be used to
	// alert on cluster types differently.
	HiveClusterTypeLabel = "hive.openshift.io/cluster-type"

	// DefaultClusterType will be used when the above HiveClusterTypeLabel is unset. This
	// value will not be added as a label, only used for metrics vectors.
	DefaultClusterType = "unspecified"

	// HiveInstallLogLabel is used on ConfigMaps uploaded by the install manager which contain an install log.
	HiveInstallLogLabel = "hive.openshift.io/install-log"

	// HiveClusterPlatformLabel is a label that is applied to ClusterDeployments
	// to denote which platform the cluster was created on. This can be used in
	// searching and filtering clusters, as well as in SelectorSyncSets to only
	// target specific cloud platforms.
	HiveClusterPlatformLabel = "hive.openshift.io/cluster-platform"

	// HiveClusterRegionLabel is a label that is applied to ClusterDeployments
	// to denote which region the cluster was created in. This can be used in
	// searching and filtering clusters, as well as in SelectorSyncSets to only
	// target specific regions of the cluster-platform.
	HiveClusterRegionLabel = "hive.openshift.io/cluster-region"
)

// ClusterDeploymentSpec defines the desired state of ClusterDeployment
type ClusterDeploymentSpec struct {

	// ClusterName is the friendly name of the cluster. It is used for subdomains,
	// some resource tagging, and other instances where a friendly name for the
	// cluster is useful.
	// +required
	ClusterName string `json:"clusterName"`

	// BaseDomain is the base domain to which the cluster should belong.
	// +required
	BaseDomain string `json:"baseDomain"`

	// Platform is the configuration for the specific platform upon which to
	// perform the installation.
	// +required
	Platform Platform `json:"platform"`

	// PullSecretRef is the reference to the secret to use when pulling images.
	// +optional
	PullSecretRef *corev1.LocalObjectReference `json:"pullSecretRef,omitempty"`

	// PreserveOnDelete allows the user to disconnect a cluster from Hive without deprovisioning it
	PreserveOnDelete bool `json:"preserveOnDelete,omitempty"`

	// ControlPlaneConfig contains additional configuration for the target cluster's control plane
	// +optional
	ControlPlaneConfig ControlPlaneConfigSpec `json:"controlPlaneConfig,omitempty"`

	// Ingress allows defining desired clusteringress/shards to be configured on the cluster.
	// +optional
	Ingress []ClusterIngress `json:"ingress,omitempty"`

	// CertificateBundles is a list of certificate bundles associated with this cluster
	// +optional
	CertificateBundles []CertificateBundleSpec `json:"certificateBundles,omitempty"`

	// ManageDNS specifies whether a DNSZone should be created and managed automatically
	// for this ClusterDeployment
	// +optional
	ManageDNS bool `json:"manageDNS,omitempty"`

	// ClusterMetadata contains metadata information about the installed cluster.
	ClusterMetadata *ClusterMetadata `json:"clusterMetadata,omitempty"`

	// Installed is true if the cluster has been installed
	Installed bool `json:"installed"`

	// Provisioning contains settings used only for initial cluster provisioning.
	// May be unset in the case of adopted clusters.
	Provisioning *Provisioning `json:"provisioning,omitempty"`
}

// Provisioning contains settings used only for initial cluster provisioning.
type Provisioning struct {
	// InstallConfigSecretRef is the reference to a secret that contains an openshift-install
	// InstallConfig. This file will be passed through directly to the installer.
	// Any version of InstallConfig can be used, provided it can be parsed by the openshift-install
	// version for the release you are provisioning.
	InstallConfigSecretRef corev1.LocalObjectReference `json:"installConfigSecretRef"`

	// ReleaseImage is the image containing metadata for all components that run in the cluster, and
	// is the primary and best way to specify what specific version of OpenShift you wish to install.
	ReleaseImage string `json:"releaseImage,omitempty"`

	// ImageSetRef is a reference to a ClusterImageSet. If a value is specified for ReleaseImage,
	// that will take precedence over the one from the ClusterImageSet.
	ImageSetRef *ClusterImageSetReference `json:"imageSetRef,omitempty"`

	// ManifestsConfigMapRef is a reference to user-provided manifests to
	// add to or replace manifests that are generated by the installer.
	ManifestsConfigMapRef *corev1.LocalObjectReference `json:"manifestsConfigMapRef,omitempty"`

	// SSHPrivateKeySecretRef is the reference to the secret that contains the private SSH key to use
	// for access to compute instances. This private key should correspond to the public key included
	// in the InstallConfig. The private key is used by Hive to gather logs on the target cluster if
	// there are install failures.
	// The SSH private key is expected to be in the secret data under the "ssh-privatekey" key.
	// +optional
	SSHPrivateKeySecretRef *corev1.LocalObjectReference `json:"sshPrivateKeySecretRef,omitempty"`

	// SSHKnownHosts are known hosts to be configured in the hive install manager pod to avoid ssh prompts.
	// Use of ssh in the install pod is somewhat limited today (failure log gathering from cluster, some bare metal
	// provisioning scenarios), so this setting is often not needed.
	SSHKnownHosts []string `json:"sshKnownHosts,omitempty"`

	// InstallerEnv are extra environment variables to pass through to the installer. This may be used to enable
	// additional features of the installer.
	// +optional
	InstallerEnv []corev1.EnvVar `json:"installerEnv,omitempty"`
}

// ClusterImageSetReference is a reference to a ClusterImageSet
type ClusterImageSetReference struct {
	// Name is the name of the ClusterImageSet that this refers to
	Name string `json:"name"`
}

// ClusterMetadata contains metadata information about the installed cluster.
type ClusterMetadata struct {

	// ClusterID is a globally unique identifier for this cluster generated during installation. Used for reporting metrics among other places.
	ClusterID string `json:"clusterID"`

	// InfraID is an identifier for this cluster generated during installation and used for tagging/naming resources in cloud providers.
	InfraID string `json:"infraID"`

	// AdminKubeconfigSecretRef references the secret containing the admin kubeconfig for this cluster.
	AdminKubeconfigSecretRef corev1.LocalObjectReference `json:"adminKubeconfigSecretRef"`

	// AdminPasswordSecretRef references the secret containing the admin username/password which can be used to login to this cluster.
	AdminPasswordSecretRef corev1.LocalObjectReference `json:"adminPasswordSecretRef"`
}

// ClusterDeploymentStatus defines the observed state of ClusterDeployment
type ClusterDeploymentStatus struct {

	// InstallRestarts is the total count of container restarts on the clusters install job.
	InstallRestarts int `json:"installRestarts,omitempty"`

	// ClusterVersionStatus will hold a copy of the remote cluster's ClusterVersion.Status
	ClusterVersionStatus openshiftapiv1.ClusterVersionStatus `json:"clusterVersionStatus,omitempty"`

	// APIURL is the URL where the cluster's API can be accessed.
	APIURL string `json:"apiURL,omitempty"`

	// WebConsoleURL is the URL for the cluster's web console UI.
	WebConsoleURL string `json:"webConsoleURL,omitempty"`

	// InstallerImage is the name of the installer image to use when installing the target cluster
	// +optional
	InstallerImage *string `json:"installerImage,omitempty"`

	// CLIImage is the name of the oc cli image to use when installing the target cluster
	// +optional
	CLIImage *string `json:"cliImage,omitempty"`

	// Conditions includes more detailed status for the cluster deployment
	// +optional
	Conditions []ClusterDeploymentCondition `json:"conditions,omitempty"`

	// CertificateBundles contains of the status of the certificate bundles associated with this cluster deployment.
	// +optional
	CertificateBundles []CertificateBundleStatus `json:"certificateBundles,omitempty"`

	// InstalledTimestamp is the time we first detected that the cluster has been successfully installed.
	InstalledTimestamp *metav1.Time `json:"installedTimestamp,omitempty"`

	// ProvisionRef is a reference to the last ClusterProvision created for the deployment
	// +optional
	ProvisionRef *corev1.LocalObjectReference `json:"provisionRef,omitempty"`
}

// ClusterDeploymentCondition contains details for the current condition of a cluster deployment
type ClusterDeploymentCondition struct {
	// Type is the type of the condition.
	Type ClusterDeploymentConditionType `json:"type"`
	// Status is the status of the condition.
	Status corev1.ConditionStatus `json:"status"`
	// LastProbeTime is the last time we probed the condition.
	// +optional
	LastProbeTime metav1.Time `json:"lastProbeTime,omitempty"`
	// LastTransitionTime is the last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// Reason is a unique, one-word, CamelCase reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`
	// Message is a human-readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty"`
}

// ClusterDeploymentConditionType is a valid value for ClusterDeploymentCondition.Type
type ClusterDeploymentConditionType string

const (
	// ClusterImageSetNotFoundCondition is set when the ClusterImageSet referenced by the
	// ClusterDeployment is not found.
	ClusterImageSetNotFoundCondition ClusterDeploymentConditionType = "ClusterImageSetNotFound"

	// InstallerImageResolutionFailedCondition is a condition that indicates whether the job
	// to determine the installer image based on a release image was successful.
	InstallerImageResolutionFailedCondition ClusterDeploymentConditionType = "InstallerImageResolutionFailed"

	// ControlPlaneCertificateNotFoundCondition is set when a control plane certificate bundle
	// is not available, preventing the target cluster's control plane from being configured with
	// certificates.
	ControlPlaneCertificateNotFoundCondition ClusterDeploymentConditionType = "ControlPlaneCertificateNotFound"

	// IngressCertificateNotFoundCondition is a condition indicating that one of the CertificateBundle
	// secrets required by an Ingress is not available.
	IngressCertificateNotFoundCondition ClusterDeploymentConditionType = "IngressCertificateNotFound"

	// UnreachableCondition indicates that Hive is unable to establish an API connection to the remote cluster.
	UnreachableCondition ClusterDeploymentConditionType = "Unreachable"

	// ActiveAPIURLOverrideCondition indicates that Hive is communicating with the remote cluster using the
	// API URL override.
	ActiveAPIURLOverrideCondition ClusterDeploymentConditionType = "ActiveAPIURLOverride"

	// InstallFailingCondition indicates that a failure has been detected and we will attempt to offer some
	// information as to why in the reason.
	InstallFailingCondition ClusterDeploymentConditionType = "InstallFailing"

	// DNSNotReadyCondition indicates that the the DNSZone object created for the clusterDeployment
	// (ie managedDNS==true) has not yet indicated that the DNS zone is successfully responding to queries.
	DNSNotReadyCondition ClusterDeploymentConditionType = "DNSNotReady"

	// ProvisionFailedCondition indicates that a provision failed
	ProvisionFailedCondition ClusterDeploymentConditionType = "ProvisionFailed"

	// SyncSetFailedCondition indicates if any syncset for a cluster deployment failed
	SyncSetFailedCondition ClusterDeploymentConditionType = "SyncSetFailed"
)

// AllClusterDeploymentConditions is a slice containing all condition types. This can be used for dealing with
// cluster deployment conditions dynamically.
var AllClusterDeploymentConditions = []ClusterDeploymentConditionType{
	ClusterImageSetNotFoundCondition,
	InstallerImageResolutionFailedCondition,
	ControlPlaneCertificateNotFoundCondition,
	IngressCertificateNotFoundCondition,
	UnreachableCondition,
	ActiveAPIURLOverrideCondition,
	InstallFailingCondition,
	DNSNotReadyCondition,
	ProvisionFailedCondition,
	SyncSetFailedCondition,
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterDeployment is the Schema for the clusterdeployments API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ClusterName",type="string",JSONPath=".spec.clusterName"
// +kubebuilder:printcolumn:name="ClusterType",type="string",JSONPath=".metadata.labels.hive\.openshift\.io/cluster-type"
// +kubebuilder:printcolumn:name="BaseDomain",type="string",JSONPath=".spec.baseDomain"
// +kubebuilder:printcolumn:name="Installed",type="boolean",JSONPath=".spec.installed"
// +kubebuilder:printcolumn:name="InfraID",type="string",JSONPath=".spec.clusterMetadata.infraID"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:path=clusterdeployments,shortName=cd
type ClusterDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterDeploymentSpec   `json:"spec,omitempty"`
	Status ClusterDeploymentStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterDeploymentList contains a list of ClusterDeployment
type ClusterDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterDeployment `json:"items"`
}

// Platform is the configuration for the specific platform upon which to perform
// the installation. Only one of the platform configuration should be set.
type Platform struct {
	// AWS is the configuration used when installing on AWS.
	AWS *aws.Platform `json:"aws,omitempty"`

	// Azure is the configuration used when installing on Azure.
	// +optional
	Azure *azure.Platform `json:"azure,omitempty"`

	// GCP is the configuration used when installing on Google Cloud Platform.
	// +optional
	GCP *gcp.Platform `json:"gcp,omitempty"`

	// BareMetal is the configuration used when installing on bare metal.
	BareMetal *baremetal.Platform `json:"baremetal,omitempty"`
}

// ClusterIngress contains the configurable pieces for any ClusterIngress objects
// that should exist on the cluster.
type ClusterIngress struct {
	// Name of the ClusterIngress object to create.
	// +required
	Name string `json:"name"`

	// Domain (sometimes referred to as shard) is the full DNS suffix that the resulting
	// IngressController object will service (eg abcd.mycluster.mydomain.com).
	// +required
	Domain string `json:"domain"`

	// NamespaceSelector allows filtering the list of namespaces serviced by the
	// ingress controller.
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`

	// RouteSelector allows filtering the set of Routes serviced by the ingress controller
	// +optional
	RouteSelector *metav1.LabelSelector `json:"routeSelector,omitempty"`

	// ServingCertificate references a CertificateBundle in the ClusterDeployment.Spec that
	// should be used for this Ingress
	// +optional
	ServingCertificate string `json:"servingCertificate,omitempty"`
}

// ControlPlaneConfigSpec contains additional configuration settings for a target
// cluster's control plane.
type ControlPlaneConfigSpec struct {
	// ServingCertificates specifies serving certificates for the control plane
	// +optional
	ServingCertificates ControlPlaneServingCertificateSpec `json:"servingCertificates,omitempty"`

	// APIURLOverride is the optional URL override to which Hive will transition for communication with the API
	// server of the remote cluster. When a remote cluster is created, Hive will initially communicate using the
	// API URL established during installation. If an API URL Override is specified, Hive will periodically attempt
	// to connect to the remote cluster using the override URL. Once Hive has determined that the override URL is
	// active, Hive will use the override URL for further communications with the API server of the remote cluster.
	// +optional
	APIURLOverride string `json:"apiURLOverride,omitempty"`
}

// ControlPlaneServingCertificateSpec specifies serving certificate settings for
// the control plane of the target cluster.
type ControlPlaneServingCertificateSpec struct {
	// Default references the name of a CertificateBundle in the ClusterDeployment that should be
	// used for the control plane's default endpoint.
	// +optional
	Default string `json:"default,omitempty"`

	// Additional is a list of additional domains and certificates that are also associated with
	// the control plane's api endpoint.
	// +optional
	Additional []ControlPlaneAdditionalCertificate `json:"additional,omitempty"`
}

// ControlPlaneAdditionalCertificate defines an additional serving certificate for a control plane
type ControlPlaneAdditionalCertificate struct {
	// Name references a CertificateBundle in the ClusterDeployment.Spec that should be
	// used for this additional certificate.
	Name string `json:"name"`

	// Domain is the domain of the additional control plane certificate
	Domain string `json:"domain"`
}

// CertificateBundleSpec specifies a certificate bundle associated with a cluster deployment
type CertificateBundleSpec struct {
	// Name is an identifier that must be unique within the bundle and must be referenced by
	// an ingress or by the control plane serving certs
	// +required
	Name string `json:"name"`

	// Generate indicates whether this bundle should have real certificates generated for it.
	// +optional
	Generate bool `json:"generate,omitempty"`

	// CertificateSecretRef is the reference to the secret that contains the certificate bundle. If
	// the certificate bundle is to be generated, it will be generated with the name in this
	// reference. Otherwise, it is expected that the secret should exist in the same namespace
	// as the ClusterDeployment
	CertificateSecretRef corev1.LocalObjectReference `json:"certificateSecretRef"`
}

// CertificateBundleStatus specifies whether a certificate bundle was generated for this
// cluster deployment.
type CertificateBundleStatus struct {
	// Name of the certificate bundle
	Name string `json:"name"`

	// Generated indicates whether the certificate bundle was generated
	Generated bool `json:"generated"`
}

func init() {
	SchemeBuilder.Register(&ClusterDeployment{}, &ClusterDeploymentList{})
}
