/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"net"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openshiftapiv1 "github.com/openshift/api/config/v1"
	netopv1 "github.com/openshift/cluster-network-operator/pkg/apis/networkoperator/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
// Important: Run "make" to regenerate code after modifying this file

const (
	// FinalizerDeprovision is used on ClusterDeployments to ensure we run a successful deprovision
	// job before cleaning up the API object.
	FinalizerDeprovision string = "hive.openshift.io/deprovision"

	// FinalizerFederation is used on ClusterDeployments to ensure that federation-related artifacts are cleaned up from
	// the host cluster before a ClusterDeployment is deleted.
	FinalizerFederation string = "hive.openshift.io/federation"
)

// ClusterDeploymentSpec defines the desired state of ClusterDeployment
type ClusterDeploymentSpec struct {

	// ClusterName is the friendly name of the cluster. It is used for subdomains,
	// some resource tagging, and other instances where a friendly name for the
	// cluster is useful.
	ClusterName string `json:"clusterName"`

	// SSHKey is the reference to the secret that contains a public key to use for access to compute instances.
	SSHKey *corev1.LocalObjectReference `json:"sshKey,omitempty"`

	// BaseDomain is the base domain to which the cluster should belong.
	BaseDomain string `json:"baseDomain"`

	// Networking defines the pod network provider in the cluster.
	Networking `json:"networking"`

	// ControlPlane is the MachinePool containing control plane nodes that need to be installed.
	ControlPlane MachinePool `json:"controlPlane"`

	// Compute is the list of MachinePools containing compute nodes that need to be installed.
	Compute []MachinePool `json:"compute"`

	// Platform is the configuration for the specific platform upon which to
	// perform the installation.
	Platform `json:"platform"`

	// PullSecret is the reference to the secret to use when pulling images.
	PullSecret corev1.LocalObjectReference `json:"pullSecret"`
	// PlatformSecrets contains credentials and secrets for the cluster infrastructure.
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
}

// ProvisionImages allows overriding the default images used to provision a cluster.
type ProvisionImages struct {
	// InstallerImage is the image containing the openshift-install binary that will be used to install.
	InstallerImage string `json:"installerImage,omitempty"`
	// InstallerImagePullPolicy is the pull policy for the installer image.
	InstallerImagePullPolicy corev1.PullPolicy `json:"installerImagePullPolicy,omitempty"`
	// HiveImage is the image used in the sidecar container to manage execution of openshift-install.
	HiveImage string `json:"hiveImage,omitempty"`
	// HiveImagePullPolicy is the pull policy for the installer image.
	HiveImagePullPolicy corev1.PullPolicy `json:"hiveImagePullPolicy,omitempty"`

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
	AWS *AWSPlatformSecrets `json:"aws,omitempty"`
}

// AWSPlatformSecrets contains secrets for clusters on the AWS platform.
type AWSPlatformSecrets struct {
	// SSH refers to a secret that contains the ssh private key to access
	// EC2 instances in this cluster.
	//SSH corev1.LocalObjectReference `json:"ssh"`

	// Credentials refers to a secret that contains the AWS account access
	// credentials.
	Credentials corev1.LocalObjectReference `json:"credentials"`
}

// ClusterDeploymentStatus defines the observed state of ClusterDeployment
type ClusterDeploymentStatus struct {

	// ClusterID is a globally unique identifier for this cluster generated during installation. Used for reporting metrics among other places.
	ClusterID string `json:"clusterID,omitempty"`

	// InfraID is an identifier for this cluster generated during installation and used for tagging/naming resources in cloud providers.
	InfraID string `json:"infraID,omitempty"`

	// Installed is true if the installer job has successfully completed for this cluster.
	Installed bool `json:"installed"`

	// Federated is true if the cluster deployment has been federated with the host cluster.
	Federated bool `json:"federated,omitempty"`

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

	// SyncSetStatus is the list of status for SyncSets which apply to the cluster deployment.
	// +optional
	SyncSetStatus []SyncSetObjectStatus `json:"syncSetStatus,omitempty"`

	// SelectorSyncSetStatus is the list of status for SelectorSyncSets which apply to the cluster deployment.
	// +optional
	SelectorSyncSetStatus []SyncSetObjectStatus `json:"selectorSyncSetStatus,omitempty"`

	// InstallerImage is the name of the installer image to use when installing the target cluster
	// +optional
	InstallerImage *string `json:"installerImage,omitempty"`

	// Conditions includes more detailed status for the cluster deployment
	// +optional
	Conditions []ClusterDeploymentCondition `json:"conditions,omitempty"`

	// CertificateBundles contains of the status of the certificate bundles associated with this cluster deployment.
	// +optional
	CertificateBundles []CertificateBundleStatus `json:"certificateBundles,omitempty"`
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
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterDeployment is the Schema for the clusterdeployments API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="BaseDomain",type="string",JSONPath=".spec.baseDomain"
// +kubebuilder:printcolumn:name="Installed",type="boolean",JSONPath=".status.installed"
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
	AWS *AWSPlatform `json:"aws,omitempty"`
	// Libvirt is the configuration used when installing on libvirt.
	Libvirt *LibvirtPlatform `json:"libvirt,omitempty"`
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

// AWSPlatform stores all the global configuration that
// all machinesets use.
type AWSPlatform struct {
	// Region specifies the AWS region where the cluster will be created.
	Region string `json:"region"`

	// UserTags specifies additional tags for AWS resources created for the cluster.
	UserTags map[string]string `json:"userTags,omitempty"`

	// DefaultMachinePlatform is the default configuration used when
	// installing on AWS for machine pools which do not define their own
	// platform configuration.
	DefaultMachinePlatform *AWSMachinePoolPlatform `json:"defaultMachinePlatform,omitempty"`
}

// LibvirtPlatform stores all the global configuration that
// all machinesets use.
type LibvirtPlatform struct {
	// URI is the identifier for the libvirtd connection.  It must be
	// reachable from both the host (where the installer is run) and the
	// cluster (where the cluster-API controller pod will be running).
	URI string `json:"URI"`

	// DefaultMachinePlatform is the default configuration used when
	// installing on AWS for machine pools which do not define their own
	// platform configuration.
	DefaultMachinePlatform *LibvirtMachinePoolPlatform `json:"defaultMachinePlatform,omitempty"`

	// Network
	Network LibvirtNetwork `json:"network"`

	// MasterIPs
	MasterIPs []net.IP `json:"masterIPs"`
}

// LibvirtNetwork is the configuration of the libvirt network.
type LibvirtNetwork struct {
	// Name is the name of the nework.
	Name string `json:"name"`
	// IfName is the name of the network interface.
	IfName string `json:"if"`
	// IPRange is the range of IPs to use.
	IPRange string `json:"ipRange"`
}

// ClusterIngress contains the configurable pieces for any ClusterIngress objects
// that should exist on the cluster.
type ClusterIngress struct {
	// Name of the ClusterIngress object to create.
	Name string `json:"name"`

	// Domain (sometimes refered to as shard) is the full DNS suffix that the resulting
	// IngressController object will service (eg abcd.mycluster.mydomain.com).
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
