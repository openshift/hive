package agent

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InstallStrategy is the install strategy configuration for provisioning a cluster with the
// Agent based assisted installer.
type InstallStrategy struct {
	// Networking is the configuration for the pod network provider in
	// the cluster.
	Networking Networking `json:"networking"`

	// SSHPublicKey will be added to all cluster hosts for use in debugging.
	// +optional
	SSHPublicKey string `json:"sshPublicKey,omitempty"`

	// AgentSelector is a label selector used for associating relevant custom resources with this cluster.
	// (Agent, BareMetalHost, etc)
	AgentSelector metav1.LabelSelector `json:"agentSelector"`

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
