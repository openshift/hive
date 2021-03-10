package aws

import (
	corev1 "k8s.io/api/core/v1"
)

// Platform stores all the global configuration that
// all machinesets use.
type Platform struct {
	// CredentialsSecretRef refers to a secret that contains the AWS account access
	// credentials.
	CredentialsSecretRef corev1.LocalObjectReference `json:"credentialsSecretRef"`

	// Region specifies the AWS region where the cluster will be created.
	Region string `json:"region"`

	// UserTags specifies additional tags for AWS resources created for the cluster.
	// +optional
	UserTags map[string]string `json:"userTags,omitempty"`

	// STS specifies configuration for deploying a cluster that uses the AWS Security Token Service, instead of long lived credentials.
	// +optional
	STS *STS `json:"sts,omitempty"`
}

// STS specifies configuration for deploying a cluster that uses the AWS Security Token Service, instead of long lived credentials.
type STS struct {
	// ServiceAccountIssuerKeySecretRef refers to a secret that contains a 'bound-service-account-signing-key.key' data key pointing to the private key that will be used to sign ServiceAccount objects.
	ServiceAccountIssuerKeySecretRef corev1.LocalObjectReference `json:"serviceAccountIssuerKeySecretRef"`

	CredentialsRequestRoles []CredentialsRequestRoles `json:"credentialsRequestRoles"`
}

// CredentialsRequestRoles maps CredentailsRequests (by namespace/name) that are in the release image the cluster will install,
// to preconfigured Role ARNs. These will be used to generate appropriate Secrets to inject into the install procedure.
// Specify a credentials request of "*" to use the given role for any CredentialsRequest which is not explicitly mapped to.
type CredentialsRequestRoles struct {
	SecretNamespaceName string `json:"secretNamespaceName"`
	RoleARN             string `json:"roleARN"`
}
