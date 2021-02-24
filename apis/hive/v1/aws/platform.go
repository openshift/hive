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

	// PrivateLink allows uses to enable access to the cluster's API server using AWS
	// PrivateLink. AWS PrivateLink includes a pair of VPC Endpoint Service and VPC
	// Endpoint accross AWS accounts and allows clients to connect to services using AWS's
	// internal networking instead of the Internet.
	PrivateLink *PrivateLinkAccess `json:"privateLink,omitempty"`
}

// PlatformStatus contains the observed state on AWS platform.
type PlatformStatus struct {
	PrivateLink *PrivateLinkAccessStatus `json:"privateLink,omitempty"`
}

// PrivateLinkAccess configures access to the cluster API using AWS PrivateLink
type PrivateLinkAccess struct {
	Enabled bool `json:"enabled"`
}

// PrivateLinkAccessStatus contains the observed state for PrivateLinkAccess resources.
type PrivateLinkAccessStatus struct {
	// +optional
	VPCEndpointService VPCEndpointService `json:"vpcEndpointService,omitempty"`
	// +optional
	VPCEndpointID string `json:"vpcEndpointID,omitempty"`
	// +optional
	HostedZoneID string `json:"hostedZoneID,omitempty"`
}

type VPCEndpointService struct {
	Name string `json:"name,omitempty"`
	ID   string `json:"id,omitempty"`
}
