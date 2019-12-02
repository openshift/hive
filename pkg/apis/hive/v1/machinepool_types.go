package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/hive/pkg/apis/hive/v1/aws"
	"github.com/openshift/hive/pkg/apis/hive/v1/azure"
	"github.com/openshift/hive/pkg/apis/hive/v1/baremetal"
	"github.com/openshift/hive/pkg/apis/hive/v1/gcp"
)

// MachinePoolSpec defines the desired state of MachinePool
type MachinePoolSpec struct {

	// ClusterDeploymentRef references the cluster deployment to which this
	// machine pool belongs.
	ClusterDeploymentRef corev1.LocalObjectReference `json:"clusterDeploymentRef"`

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
	// BareMetal is the configuration used when installing on bare metal.
	BareMetal *baremetal.MachinePool `json:"bareMetal,omitempty"`
}

// MachinePoolStatus defines the observed state of MachinePool
type MachinePoolStatus struct {

	// Conditions includes more detailed status for the cluster deployment
	// +optional
	Conditions []MachinePoolCondition `json:"conditions,omitempty"`
}

// MachinePoolCondition contains details for the current condition of a machine pool
type MachinePoolCondition struct {
	// Type is the type of the condition.
	Type MachinePoolConditionType `json:"type"`
	// Status is the status of the condition.
	Status corev1.ConditionStatus `json:"status"`
	// LastProbeTime is the last time we probed the condition.
	// +optional
	LastProbeTime metav1.Time `json:"lastProbeTime,omitempty"`
	// LastTransitionTime is the last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// Reason is a unique, one-word, CamelCase reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`
	// Message is a human-readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty"`
}

// MachinePoolConditionType is a valid value for MachinePoolCondition.Type
type MachinePoolConditionType string

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MachinePool is the Schema for the machinepools API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="PoolName",type="string",JSONPath=".spec.name"
// +kubebuilder:printcolumn:name="ClusterDeployment",type="string",JSONPath=".spec.clusterDeploymentRef.name"
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".spec.replicas"
// +kubebuilder:resource:path=machinepools
type MachinePool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MachinePoolSpec   `json:"spec,omitempty"`
	Status MachinePoolStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MachinePoolList contains a list of MachinePool
type MachinePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MachinePool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MachinePool{}, &MachinePoolList{})
}
