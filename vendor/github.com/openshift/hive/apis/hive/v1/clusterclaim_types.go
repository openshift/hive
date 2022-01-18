package v1

import (
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterClaimSpec defines the desired state of the ClusterClaim.
type ClusterClaimSpec struct {
	// ClusterPoolName is the name of the cluster pool from which to claim a cluster.
	ClusterPoolName string `json:"clusterPoolName"`

	// Subjects hold references to which to authorize access to the claimed cluster.
	// +optional
	Subjects []rbacv1.Subject `json:"subjects,omitempty"`

	// Namespace is the namespace containing the ClusterDeployment (name will match the namespace) of the claimed cluster.
	// This field will be set as soon as a suitable cluster can be found.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Lifetime is the maximum lifetime of the claim after it is assigned a cluster. If the claim still exists
	// when the lifetime has elapsed, the claim will be deleted by Hive.
	// This is a Duration value; see https://pkg.go.dev/time#ParseDuration for accepted formats.
	// +optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Format=duration
	Lifetime *metav1.Duration `json:"lifetime,omitempty"`
}

// ClusterClaimStatus defines the observed state of ClusterClaim.
type ClusterClaimStatus struct {
	// Conditions includes more detailed status for the cluster pool.
	// +optional
	Conditions []ClusterClaimCondition `json:"conditions,omitempty"`

	// Lifetime is the maximum lifetime of the claim after it is assigned a cluster. If the claim still exists
	// when the lifetime has elapsed, the claim will be deleted by Hive.
	// +optional
	Lifetime *metav1.Duration `json:"lifetime,omitempty"`

	// ClusterState indicates the status of the cluster assigned to this ClusterClaim.
	ClusterState ClusterClaimClusterState `json:"clusterState,omitempty"`
}

type ClusterClaimClusterState string

var (
	// ClusterStateNoCluster is set in two cases:
	// 1) No cluster has yet been assigned to this ClusterClaim.
	// 2) The cluster assigned to this claim does not exist (usually because it has been deleted).
	ClusterStateNoCluster ClusterClaimClusterState = "NoCluster"
	// ClusterStateProvisioned indicates that the assigned cluster exists and is not deleting.
	// (NB: it is not necessarily Running -- see ClusterDeployment.Status.PowerState for that
	// level of detail.)
	ClusterStateProvisioned ClusterClaimClusterState = "Provisioned"
	// ClusterStateDeleting indicates that the claimed cluster is in the process of being deleted.
	// Usually this means it is being deprovisioned; but see the ClusterDeployment's Provisioned
	// condition for details.
	ClusterStateDeleting ClusterClaimClusterState = "Deleting"
)

// ClusterClaimCondition contains details for the current condition of a cluster claim.
type ClusterClaimCondition struct {
	// Type is the type of the condition.
	Type ClusterClaimConditionType `json:"type"`
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

// ClusterClaimConditionType is a valid value for ClusterClaimCondition.Type.
type ClusterClaimConditionType string

const (
	// ClusterClaimPendingCondition is set when a cluster has not yet been assigned and made ready to the claim.
	ClusterClaimPendingCondition ClusterClaimConditionType = "Pending"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterClaim represents a claim to a cluster from a cluster pool.
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=clusterclaims
// +kubebuilder:printcolumn:name="Pool",type="string",JSONPath=".spec.clusterPoolName"
// +kubebuilder:printcolumn:name="Pending",type="string",JSONPath=".status.conditions[?(@.type=='Pending')].reason"
// +kubebuilder:printcolumn:name="ClusterState",type="string",JSONPath=".status.clusterState"
// +kubebuilder:printcolumn:name="ClusterNamespace",type="string",JSONPath=".spec.namespace"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type ClusterClaim struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterClaimSpec   `json:"spec"`
	Status ClusterClaimStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterClaimList contains a list of ClusterClaims.
type ClusterClaimList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterClaim `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterClaim{}, &ClusterClaimList{})
}
