package v1alpha1

import (
	"github.com/openshift/hive/pkg/apis/hive/v1alpha1/aws"
	"github.com/openshift/hive/pkg/apis/hive/v1alpha1/azure"
	"github.com/openshift/hive/pkg/apis/hive/v1alpha1/gcp"
	corev1 "k8s.io/api/core/v1"
)

// MachinePool is a pool of machines to be installed.
type MachinePool struct {
	// Name is the name of the machine pool.
	Name string `json:"name"`

	// Replicas is the count of machines for this machine pool.
	// Default is 1.
	Replicas *int64 `json:"replicas"`

	// Platform is configuration for machine pool specific to the platform.
	Platform MachinePoolPlatform `json:"platform"`

	// Map of label string keys and values that will be applied to the created MachineSet's
	// MachineSpec. This list will overwrite any modifications made to Node labels on an
	// ongoing basis.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// List of taints that will be applied to the created MachineSet's MachineSpec.
	// This list will overwrite any modifications made to Node taints on an ongoing basis.
	// +optional
	Taints []corev1.Taint `json:"taints,omitempty"`
}

// MachinePoolPlatform is the platform-specific configuration for a machine
// pool. Only one of the platforms should be set.
type MachinePoolPlatform struct {
	// AWS is the configuration used when installing on AWS.
	AWS *aws.MachinePoolPlatform `json:"aws,omitempty"`
	// Azure is the configuration used when installing on Azure.
	Azure *azure.MachinePool `json:"azure,omitempty"`
	// GCP is the configuration used when installing on GCP.
	GCP *gcp.MachinePool `json:"gcp,omitempty"`
}
