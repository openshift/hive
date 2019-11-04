package aws

import (
	corev1 "k8s.io/api/core/v1"
)

// Platform stores all the global configuration that
// all machinesets use.
type Platform struct {
	// Region specifies the AWS region where the cluster will be created.
	Region string `json:"region"`

	// UserTags specifies additional tags for AWS resources created for the cluster.
	// +optional
	UserTags map[string]string `json:"userTags,omitempty"`

	// DefaultMachinePlatform is the default configuration used when
	// installing on AWS for machine pools which do not define their own
	// platform configuration.
	DefaultMachinePlatform *MachinePoolPlatform `json:"defaultMachinePlatform,omitempty"`
}

// PlatformSecrets contains secrets for clusters on the AWS platform.
type PlatformSecrets struct {
	// SSH refers to a secret that contains the ssh private key to access
	// EC2 instances in this cluster.
	//SSH corev1.LocalObjectReference `json:"ssh"`

	// Credentials refers to a secret that contains the AWS account access
	// credentials.
	Credentials corev1.LocalObjectReference `json:"credentials"`
}
