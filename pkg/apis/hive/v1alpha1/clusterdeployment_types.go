package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openshiftapiv1 "github.com/openshift/api/config/v1"
	netopv1 "github.com/openshift/cluster-network-operator/pkg/apis/networkoperator/v1"
	"github.com/openshift/hive/pkg/apis/hive/v1alpha1/aws"
	"github.com/openshift/hive/pkg/apis/hive/v1alpha1/azure"
	"github.com/openshift/hive/pkg/apis/hive/v1alpha1/gcp"
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

	// HiveClusterDeploymentNameLabel is used on various objects created by Hive to link to their associated
	// ClusterDeployment
	HiveClusterDeploymentNameLabel = "hive.openshift.io/cluster-deployment-name"

	// HiveInstallLogLabel is used on ConfigMaps uploaded by the install manager which contain an install log.
	HiveInstallLogLabel = "hive.openshift.io/install-log"

	// HiveClusterPlatformLabel is a label that is applied to ClusterDeployments
	// to denote which platform the cluster was created on. This can be used in
	// searching and filtering clusters, as well as in SelectorSyncSets to only
	// target specific cloud platforms.
	HiveClusterPlatformLabel = "hive.openshift.io/cluster-platform"
)

// ClusterDeploymentSpec defines the desired state of ClusterDeployment
type ClusterDeploymentSpec struct {

	// ClusterName is the friendly name of the cluster. It is used for subdomains,
	// some resource tagging, and other instances where a friendly name for the
	// cluster is useful.
	// +required
	ClusterName string `json:"clusterName"`

	// SSHKey is the reference to the secret that contains a public key to use for access to compute instances.
	// +required
	SSHKey corev1.LocalObjectReference `json:"sshKey"`

	// BaseDomain is the base domain to which the cluster should belong.
	// +required
	BaseDomain string `json:"baseDomain"`

	// Networking defines the pod network provider in the cluster.
	// +required
	Networking `json:"networking"`

	// ControlPlane is the MachinePool containing control plane nodes that need to be installed.
	// +required
	ControlPlane MachinePool `json:"controlPlane"`

	// Compute is the list of MachinePools containing compute nodes that need to be installed.
	// +required
	Compute []MachinePool `json:"compute"`

	// Platform is the configuration for the specific platform upon which to
	// perform the installation.
	// +required
	Platform `json:"platform"`

	// PullSecret is the reference to the secret to use when pulling images.
	// +optional
	PullSecret *corev1.LocalObjectReference `json:"pullSecret,omitempty"`

	// TODO: Should PlatformSecrets be moved within each Platform for v1?

	// PlatformSecrets contains credentials and secrets for the cluster infrastructure.
	// +required
	PlatformSecrets PlatformSecrets `json:"platformSecrets"`

	// Images allows overriding the default images used to provision and manage the cluster.
	Images ProvisionImages `json:"images,omitempty"`

	// ImageSet is a reference to a ClusterImageSet. If values are specified for Images,
	// those will take precedence over the ones from the ClusterImageSet.
	ImageSet *ClusterImageSetReference `json:"imageSet,omitempty"`

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

	// Installed is true if the cluster has been installed
	Installed bool `json:"installed"`
}

// ProvisionImages allows overriding the default images used to provision a cluster.
type ProvisionImages struct {
	// InstallerImage is the image containing the openshift-install binary that will be used to install.
	InstallerImage string `json:"installerImage,omitempty"`
	// InstallerImagePullPolicy is the pull policy for the installer image.
	InstallerImagePullPolicy corev1.PullPolicy `json:"installerImagePullPolicy,omitempty"`

	// ReleaseImage is the image containing metadata for all components that run in the cluster, and
	// is the primary and best way to specify what specific version of OpenShift you wish to install.
	ReleaseImage string `json:"releaseImage,omitempty"`
}

// ClusterImageSetReference is a reference to a ClusterImageSet
type ClusterImageSetReference struct {
	// Name is the name of the ClusterImageSet that this refers to
	Name string `json:"name"`
}

// PlatformSecrets defines the secrets to be used by various clouds.
type PlatformSecrets struct {
	// +optional
	AWS *aws.PlatformSecrets `json:"aws,omitempty"`
	// +optional
	Azure *azure.PlatformSecrets `json:"azure,omitempty"`
	// +optional
	GCP *gcp.PlatformSecrets `json:"gcp,omitempty"`
}

// ClusterDeploymentStatus defines the observed state of ClusterDeployment
type ClusterDeploymentStatus struct {

	// ClusterID is a globally unique identifier for this cluster generated during installation. Used for reporting metrics among other places.
	ClusterID string `json:"clusterID,omitempty"`

	// InfraID is an identifier for this cluster generated during installation and used for tagging/naming resources in cloud providers.
	InfraID string `json:"infraID,omitempty"`

	// Installed is true if the installer job has successfully completed for this cluster.
	// Deprecated.
	Installed bool `json:"installed"`

	// Federated is true if the cluster deployment has been federated with the host cluster.
	Federated bool `json:"federated,omitempty"`

	// InstallRestarts is the total count of container restarts on the clusters install job.
	InstallRestarts int `json:"installRestarts,omitempty"`

	// FederatedClusterRef is the reference to the federated cluster resource associated with
	// this ClusterDeployment.
	FederatedClusterRef *corev1.ObjectReference `json:"federatedClusterRef,omitempty"`

	// AdminKubeconfigSecret references the secret containing the admin kubeconfig for this cluster.
	AdminKubeconfigSecret corev1.LocalObjectReference `json:"adminKubeconfigSecret,omitempty"`

	// AdminPasswordSecret references the secret containing the admin username/password which can be used to login to this cluster.
	AdminPasswordSecret corev1.LocalObjectReference `json:"adminPasswordSecret,omitempty"`

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

	// Provision is a reference to the last ClusterProvision created for the deployment
	// +optional
	Provision *corev1.LocalObjectReference `json:"provision,omitempty"`
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

	// UnreachableCondition indicates that are unable to establish an API connection to the remote cluster.
	UnreachableCondition ClusterDeploymentConditionType = "Unreachable"

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
// +kubebuilder:printcolumn:name="InfraID",type="string",JSONPath=".status.infraID"
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
}

// Networking defines the pod network provider in the cluster.
type Networking struct {
	// MachineCIDR is the IP address space from which to assign machine IPs.
	MachineCIDR string `json:"machineCIDR"`

	// Type is the network type to install
	Type NetworkType `json:"type"`

	// ServiceCIDR is the IP address space from which to assign service IPs.
	ServiceCIDR string `json:"serviceCIDR"`

	// ClusterNetworks is the IP address space from which to assign pod IPs.
	ClusterNetworks []netopv1.ClusterNetwork `json:"clusterNetworks,omitempty"`
}

// NetworkType defines the pod network provider in the cluster.
type NetworkType string

const (
	// NetworkTypeOpenshiftSDN is used to install with SDN.
	NetworkTypeOpenshiftSDN NetworkType = "OpenShiftSDN"
	// NetworkTypeOpenshiftOVN is used to install with OVN.
	NetworkTypeOpenshiftOVN NetworkType = "OVNKubernetes"
)

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

	// SecretRef is the reference to the secret that contains the certificate bundle. If
	// the certificate bundle is to be generated, it will be generated with the name in this
	// reference. Otherwise, it is expected that the secret should exist in the same namespace
	// as the ClusterDeployment
	SecretRef corev1.LocalObjectReference `json:"secretRef"`
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
