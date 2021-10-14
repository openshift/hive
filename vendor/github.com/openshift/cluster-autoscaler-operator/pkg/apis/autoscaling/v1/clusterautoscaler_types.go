package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	SchemeBuilder.Register(&ClusterAutoscaler{}, &ClusterAutoscalerList{})
}

// ClusterAutoscalerSpec defines the desired state of ClusterAutoscaler
type ClusterAutoscalerSpec struct {
	// Constraints of autoscaling resources
	ResourceLimits *ResourceLimits `json:"resourceLimits,omitempty"`

	// Configuration of scale down operation
	ScaleDown *ScaleDownConfig `json:"scaleDown,omitempty"`

	// Gives pods graceful termination time before scaling down
	MaxPodGracePeriod *int32 `json:"maxPodGracePeriod,omitempty"`

	// Maximum time CA waits for node to be provisioned
	// +kubebuilder:validation:Pattern=^([0-9]+(\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$
	MaxNodeProvisionTime string `json:"maxNodeProvisionTime,omitempty"`

	// To allow users to schedule "best-effort" pods, which shouldn't trigger
	// Cluster Autoscaler actions, but only run when there are spare resources available,
	// More info: https://github.com/kubernetes/autoscaler/blob/master/cluster-autoscaler/FAQ.md#how-does-cluster-autoscaler-work-with-pod-priority-and-preemption
	PodPriorityThreshold *int32 `json:"podPriorityThreshold,omitempty"`

	// BalanceSimilarNodeGroups enables/disables the
	// `--balance-similar-node-groups` cluster-autocaler feature.
	// This feature will automatically identify node groups with
	// the same instance type and the same set of labels and try
	// to keep the respective sizes of those node groups balanced.
	BalanceSimilarNodeGroups *bool `json:"balanceSimilarNodeGroups,omitempty"`

	// Enables/Disables `--ignore-daemonsets-utilization` CA feature flag. Should CA ignore DaemonSet pods when calculating resource utilization for scaling down. false by default
	IgnoreDaemonsetsUtilization *bool `json:"ignoreDaemonsetsUtilization,omitempty"`

	// Enables/Disables `--skip-nodes-with-local-storage` CA feature flag. If true cluster autoscaler will never delete nodes with pods with local storage, e.g. EmptyDir or HostPath. true by default at autoscaler
	SkipNodesWithLocalStorage *bool `json:"skipNodesWithLocalStorage,omitempty"`
}

// ClusterAutoscalerStatus defines the observed state of ClusterAutoscaler
type ClusterAutoscalerStatus struct {
	// TODO: Add status fields.
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterAutoscaler is the Schema for the clusterautoscalers API
// +k8s:openapi-gen=true
// +kubebuilder:resource:path=clusterautoscalers,shortName=ca,scope=Cluster
// +kubebuilder:subresource:status
// +genclient:nonNamespaced
type ClusterAutoscaler struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Desired state of ClusterAutoscaler resource
	Spec ClusterAutoscalerSpec `json:"spec,omitempty"`

	// Most recently observed status of ClusterAutoscaler resource
	Status ClusterAutoscalerStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterAutoscalerList contains a list of ClusterAutoscaler
type ClusterAutoscalerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterAutoscaler `json:"items"`
}

type ResourceLimits struct {
	// Maximum number of nodes in all node groups.
	// Cluster autoscaler will not grow the cluster beyond this number.
	// +kubebuilder:validation:Minimum=0
	MaxNodesTotal *int32 `json:"maxNodesTotal,omitempty"`

	// Minimum and maximum number of cores in cluster, in the format <min>:<max>.
	// Cluster autoscaler will not scale the cluster beyond these numbers.
	Cores *ResourceRange `json:"cores,omitempty"`

	// Minimum and maximum number of gigabytes of memory in cluster, in the format <min>:<max>.
	// Cluster autoscaler will not scale the cluster beyond these numbers.
	Memory *ResourceRange `json:"memory,omitempty"`

	// Minimum and maximum number of different GPUs in cluster, in the format <gpu_type>:<min>:<max>.
	// Cluster autoscaler will not scale the cluster beyond these numbers. Can be passed multiple times.
	GPUS []GPULimit `json:"gpus,omitempty"`
}

type GPULimit struct {
	// +kubebuilder:validation:MinLength=1
	Type string `json:"type"`

	// +kubebuilder:validation:Minimum=0
	Min int32 `json:"min"`
	// +kubebuilder:validation:Minimum=1
	Max int32 `json:"max"`
}

type ResourceRange struct {
	// +kubebuilder:validation:Minimum=0
	Min int32 `json:"min"`
	Max int32 `json:"max"`
}

type ScaleDownConfig struct {
	// Should CA scale down the cluster
	Enabled bool `json:"enabled"`

	// How long after scale up that scale down evaluation resumes
	// +kubebuilder:validation:Pattern=([0-9]*(\.[0-9]*)?[a-z]+)+
	DelayAfterAdd *string `json:"delayAfterAdd,omitempty"`

	// How long after node deletion that scale down evaluation resumes, defaults to scan-interval
	// +kubebuilder:validation:Pattern=([0-9]*(\.[0-9]*)?[a-z]+)+
	DelayAfterDelete *string `json:"delayAfterDelete,omitempty"`

	// How long after scale down failure that scale down evaluation resumes
	// +kubebuilder:validation:Pattern=([0-9]*(\.[0-9]*)?[a-z]+)+
	DelayAfterFailure *string `json:"delayAfterFailure,omitempty"`

	// How long a node should be unneeded before it is eligible for scale down
	// +kubebuilder:validation:Pattern=([0-9]*(\.[0-9]*)?[a-z]+)+
	UnneededTime *string `json:"unneededTime,omitempty"`
}
