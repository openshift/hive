package agent

// InstallStrategyStatus defines the observed state of the Agent install strategy for this cluster.
type InstallStrategyStatus struct {

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
