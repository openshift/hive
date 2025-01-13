package vsphere

import (
	"github.com/openshift/installer/pkg/types/vsphere"
	corev1 "k8s.io/api/core/v1"
)

// Platform stores any global configuration used for vSphere platforms.
type Platform struct {
	// Infrastructure is the desired state of the vSphere infrastructure provider.
	Infrastructure *vsphere.Platform `json:"infrastructure,omitempty"`

	// VCenter is the domain name or IP address of the vCenter.
	// Deprecated: Please use Platform.Infrastructure instead
	// See also: Platform.ConvertDeprecatedFields
	// +optional
	DeprecatedVCenter string `json:"vCenter,omitempty"`

	// CredentialsSecretRef refers to a secret that contains the vSphere account access
	// credentials: GOVC_USERNAME, GOVC_PASSWORD fields.
	CredentialsSecretRef corev1.LocalObjectReference `json:"credentialsSecretRef"`

	// CertificatesSecretRef refers to a secret that contains the vSphere CA certificates
	// necessary for communicating with the VCenter.
	CertificatesSecretRef corev1.LocalObjectReference `json:"certificatesSecretRef"`

	// Datacenter is the name of the datacenter to use in the vCenter.
	// Deprecated: Please use Platform.Infrastructure instead
	// See also: Platform.ConvertDeprecatedFields
	// +optional
	DeprecatedDatacenter string `json:"datacenter,omitempty"`

	// DefaultDatastore is the default datastore to use for provisioning volumes.
	// Deprecated: Please use Platform.Infrastructure instead
	// See also: Platform.ConvertDeprecatedFields
	// +optional
	DeprecatedDefaultDatastore string `json:"defaultDatastore,omitempty"`

	// Folder is the name of the folder that will be used and/or created for
	// virtual machines.
	// Deprecated: Please use Platform.Infrastructure instead
	// See also: Platform.ConvertDeprecatedFields
	// +optional
	DeprecatedFolder string `json:"folder,omitempty"`

	// Cluster is the name of the cluster virtual machines will be cloned into.
	// Deprecated: Please use Platform.Infrastructure instead
	// See also: Platform.ConvertDeprecatedFields
	// +optional
	DeprecatedCluster string `json:"cluster,omitempty"`

	// Network specifies the name of the network to be used by the cluster.
	// Deprecated: Please use Platform.Infrastructure instead
	// See also: Platform.ConvertDeprecatedFields
	// +optional
	DeprecatedNetwork string `json:"network,omitempty"`
}
