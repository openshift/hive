package gcp

import (
	corev1 "k8s.io/api/core/v1"
)

// Platform stores all the global configuration that all machinesets
// use.
type Platform struct {
	// ProjectID is the the project that will be used for the cluster.
	ProjectID string `json:"projectID"`

	// Region specifies the GCP region where the cluster will be created.
	Region string `json:"region"`

	// DefaultMachinePlatform is the default configuration used when
	// installing on GCP for machine pools which do not define their own
	// platform configuration.
	// +optional
	DefaultMachinePlatform *MachinePool `json:"defaultMachinePlatform,omitempty"`
}

// PlatformSecrets contains secrets for clusters on the GCP platform.
type PlatformSecrets struct {
	// Credentials refers to a secret that contains the GCP account access
	// credentials.
	Credentials corev1.LocalObjectReference `json:"credentials"`
}
