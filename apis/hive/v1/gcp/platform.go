package gcp

import (
	corev1 "k8s.io/api/core/v1"
)

// Platform stores all the global configuration that all machinesets use.
type Platform struct {
	// CredentialsSecretRef refers to a secret that contains the GCP account access credentials.
	// +optional
	CredentialsSecretRef corev1.LocalObjectReference `json:"credentialsSecretRef"`

	// Region specifies the GCP region where the cluster will be created.
	Region string `json:"region"`

	// PrivateSericeConnect allows users to enable access to the cluster's API server using GCP
	// Private Service Connect. It includes a forwarding rule paired with a Service Attachment
	// across GCP accounts and allows clients to connect to services using GCP internal networking
	// of using public load balancers.
	// +optional
	PrivateServiceConnect *PrivateServiceConnect `json:"privateServiceConnect,omitempty"`
}

// PrivateServiceConnectAccess configures access to the cluster API using GCP Private Service Connect
type PrivateServiceConnect struct {
	// Enabled specifies if Private Service Connect is to be enabled on the cluster.
	Enabled bool `json:"enabled"`

	// ServiceAttachment configures the service attachment to be used by the cluster.
	// +optional
	ServiceAttachment *ServiceAttachment `json:"serviceAttachment,omitempty"`
}

// ServiceAttachment configures the service attachment to be used by the cluster
type ServiceAttachment struct {
	// Subnet configures the subnetwork that contains the service attachment.
	// +optional
	Subnet *ServiceAttachmentSubnet `json:"subnet,omitempty"`
}

// ServiceAttachmentSubnet configures the subnetwork used by the service attachment
type ServiceAttachmentSubnet struct {
	// Cidr configures the network cidr of the subnetwork that contains the service attachment.
	// +optional
	Cidr string `json:"cidr,omitempty"`
}

// PlatformStatus contains the observed state on GCP platform.
type PlatformStatus struct {
	// PrivateServiceConnect contains the private service connect resource references
	// +optional
	PrivateServiceConnect *PrivateServiceConnectStatus `json:"privateServiceConnect,omitempty"`
}

// PrivateServiceConnectStatus contains the observed state for PrivateServiceConnect resources.
type PrivateServiceConnectStatus struct {
	// Endpoint is the selfLink of the endpoint created for the cluster.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// EndpointAddress is the selfLink of the address created for the cluster endpoint.
	// +optional
	EndpointAddress string `json:"endpointAddress,omitempty"`

	// ServiceAttachment is the selfLink of the service attachment created for the clsuter.
	// +optional
	ServiceAttachment string `json:"serviceAttachment,omitempty"`

	// ServiceAttachmentFirewall is the selfLink of the firewall that allows traffic between
	// the service attachment and the cluster's internal api load balancer.
	// +optional
	ServiceAttachmentFirewall string `json:"serviceAttachmentFirewall,omitempty"`

	// ServiceAttachmentSubnet is the selfLink of the subnet that will contain the service attachment.
	// +optional
	ServiceAttachmentSubnet string `json:"serviceAttachmentSubnet,omitempty"`
}
