package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AgentClusterInstall represents a request to provision an agent based cluster.
//
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type AgentClusterInstall struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentClusterInstallSpec   `json:"spec"`
	Status AgentClusterInstallStatus `json:"status,omitempty"`
}

// AgentClusterInstallSpec defines the desired state of the AgentClusterInstall.
type AgentClusterInstallSpec struct {

	// ImageSetRef is a reference to a ClusterImageSet. The release image specified in the ClusterImageSet will be used
	// to install the cluster.
	ImageSetRef ClusterImageSetReference `json:"imageSetRef"`

	// ClusterDeploymentRef is a reference to the ClusterDeployment associated with this AgentClusterInstall.
	ClusterDeploymentRef corev1.LocalObjectReference `json:"clusterDeploymentRef"`

	// ClusterMetadata contains metadata information about the installed cluster. It should be populated once the cluster install is completed. (it can be populated sooner if desired, but Hive will not copy back to ClusterDeployment until the Installed condition goes True.
	ClusterMetadata *ClusterMetadata `json:"clusterMetadata,omitempty"`

	// Networking is the configuration for the pod network provider in
	// the cluster.
	Networking Networking `json:"networking"`

	// SSHPublicKey will be added to all cluster hosts for use in debugging.
	// +optional
	SSHPublicKey string `json:"sshPublicKey,omitempty"`

	// ProvisionRequirements defines configuration for when the installation is ready to be launched automatically.
	ProvisionRequirements ProvisionRequirements `json:"provisionRequirements"`
}

// ProvisionRequirements defines configuration for when the installation is ready to be launched automatically.
type ProvisionRequirements struct {

	// ControlPlaneAgents is the number of matching approved and ready Agents with the control plane role
	// required to launch the install. Must be either 1 or 3.
	ControlPlaneAgents int `json:"controlPlaneAgents"`

	// WorkerAgents is the minimum number of matching approved and ready Agents with the worker role
	// required to launch the install.
	// +kubebuilder:validation:Minimum=0
	// +optional
	WorkerAgents int `json:"workerAgents,omitempty"`
}

// Networking defines the pod network provider in the cluster.
type Networking struct {
	// MachineNetwork is the list of IP address pools for machines.
	// +optional
	MachineNetwork []MachineNetworkEntry `json:"machineNetwork,omitempty"`

	// ClusterNetwork is the list of IP address pools for pods.
	// Default is 10.128.0.0/14 and a host prefix of /23.
	//
	// +optional
	ClusterNetwork []ClusterNetworkEntry `json:"clusterNetwork,omitempty"`

	// ServiceNetwork is the list of IP address pools for services.
	// Default is 172.30.0.0/16.
	// NOTE: currently only one entry is supported.
	//
	// +kubebuilder:validation:MaxItems=1
	// +optional
	ServiceNetwork []string `json:"serviceNetwork,omitempty"`
}

// MachineNetworkEntry is a single IP address block for node IP blocks.
type MachineNetworkEntry struct {
	// CIDR is the IP block address pool for machines within the cluster.
	CIDR string `json:"cidr"`
}

// ClusterNetworkEntry is a single IP address block for pod IP blocks. IP blocks
// are allocated with size 2^HostSubnetLength.
type ClusterNetworkEntry struct {
	// CIDR is the IP block address pool.
	CIDR string `json:"cidr"`

	// HostPrefix is the prefix size to allocate to each node from the CIDR.
	// For example, 24 would allocate 2^8=256 adresses to each node. If this
	// field is not used by the plugin, it can be left unset.
	// +optional
	HostPrefix int32 `json:"hostPrefix,omitempty"`
}

// AgentClusterInstallStatus defines the observed state of the AgentClusterInstall.
type AgentClusterInstallStatus struct {
	// Conditions includes more detailed status for the cluster install.
	// +optional
	Conditions []ClusterInstallCondition `json:"conditions,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AgentClusterInstallList contains a list of AgentClusterInstalls
type AgentClusterInstallList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentClusterInstall `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AgentClusterInstall{}, &AgentClusterInstallList{})
}
