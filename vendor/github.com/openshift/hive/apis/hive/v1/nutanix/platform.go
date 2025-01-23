package nutanix

import (
	corev1 "k8s.io/api/core/v1"
)

// Platform stores any global configuration used for Nutanix platforms.
type Platform struct {
	// Endpoint is the URL of the Nutanix Prism Central instance.
	Endpoint string `json:"endpoint"`

	// Port is the port of the Nutanix Prism Central instance.
	Port int32 `json:"port"`

	// CredentialsSecretRef refers to a secret that contains the Nutanix account access
	// credentials.
	CredentialsSecretRef corev1.LocalObjectReference `json:"credentialsSecretRef"`

	// CertificatesSecretRef refers to a secret that contains the Nutanix Prism CA certificates
	// necessary for communicating with the Nutanix Prism Central.
	CertificatesSecretRef corev1.LocalObjectReference `json:"certificatesSecretRef"`

	// Cluster is the name of the Nutanix cluster to use for provisioning volumes.
	Cluster string `json:"cluster,omitempty"`

	// Subnet is the name of the subnet to use for provisioning volumes.
	Subnet string `json:"subnet,omitempty"`
}
