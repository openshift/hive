package nutanix

import corev1 "k8s.io/api/core/v1"

// Platform stores any global configuration used for Nutanix platform
type Platform struct {
	// PrismCentral is the endpoint (address and port) to connect to the Prism Central.
	// This serves as the default Prism-Central.
	PrismCentral PrismEndpoint `json:"prismCentral"`

	// CredentialsSecretRef refers to a secret that contains the Nutanix account access
	// credentials.
	CredentialsSecretRef corev1.LocalObjectReference `json:"credentialsSecretRef"`
}

// PrismEndpoint holds the endpoint address and port to access the Nutanix Prism Central or Element (cluster)
type PrismEndpoint struct {
	// address is the endpoint address (DNS name or IP address) of the Nutanix Prism Central or Element (cluster)
	Address string `json:"address"`

	// port is the port number to access the Nutanix Prism Central or Element (cluster)
	Port int32 `json:"port"`
}
