package openstack

import (
	corev1 "k8s.io/api/core/v1"
)

// Platform stores all the global OpenStack configuration
type Platform struct {
	// CredentialsSecretRef refers to a secret that contains the OpenStack account access
	// credentials.
	CredentialsSecretRef corev1.LocalObjectReference `json:"credentialsSecretRef"`

	// Cloud will be used to indicate the OS_CLOUD value to use the right section
	// from the cloud.yaml in the CredentialsSecretRef.
	Cloud string `json:"cloud"`

	// TrunkSupport indicates whether or not to use trunk ports in your OpenShift cluster.
	// +optional
	TrunkSupport bool `json:"trunkSupport,omitempty"`
}
