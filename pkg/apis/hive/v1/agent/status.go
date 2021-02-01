package agent

// InstallStrategyStatus defines the observed state of the Agent install strategy for this cluster.
type InstallStrategyStatus struct {

	// AgentNetworks provides an aggregate view of all network CIDRs detected across all Agents.
	// It can be used to help select a valid machine network in spec if one is not known beforehand.
	AgentNetworks []AgentNetwork `json:"agentNetworks,omitempty"`

	// ControlPlaneAgentsDiscovered is the number of Agents currently linked to this ClusterDeployment.
	// +optional
	ControlPlaneAgentsDiscovered int `json:"controlPlaneAgentsDiscovered,omitempty"`
	// ControlPlaneAgentsDiscovered is the number of Agents currently linked to this ClusterDeployment that are ready for use.
	// +optional
	ControlPlaneAgentsReady int `json:"controlPlaneAgentsReady,omitempty"`
	// WorkerAgentsDiscovered is the number of worker Agents currently linked to this ClusterDeployment.
	// +optional
	WorkerAgentsDiscovered int `json:"workerAgentsDiscovered,omitempty"`
	// WorkerAgentsDiscovered is the number of worker Agents currently linked to this ClusterDeployment that are ready for use.
	// +optional
	WorkerAgentsReady int `json:"workerAgentsReady,omitempty"`

	ConnectivityMajorityGroups string `json:"connectivityMajorityGroups,omitempty"`
}

// AgentNetwork contains infromation about a detected host network on the agents for this cluster.
type AgentNetwork struct {

	// MachineNetwork contains the agent network CIDR.
	MachineNetwork MachineNetworkEntry `json:"machineNetwork"`

	// AgentRefs contains the namespace and name of each Agent resource associated with the cluster.
	AgentRefs []AgentRef `json:"agentRefs"`
}

// AgentRef defines the namespace and name of an Agent resource.
type AgentRef struct {

	// Namespace of the Agent resource.
	Namespace string `json:"namespace"`

	// Name of the Agent resource.
	Name string `json:"name"`
}
