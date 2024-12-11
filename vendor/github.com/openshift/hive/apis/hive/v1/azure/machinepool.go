package azure

// MachinePool stores the configuration for a machine pool installed
// on Azure.
type MachinePool struct {
	// Zones is list of availability zones that can be used.
	// eg. ["1", "2", "3"]
	Zones []string `json:"zones,omitempty"`

	// InstanceType defines the azure instance type.
	// eg. Standard_DS_V2
	InstanceType string `json:"type"`

	// OSDisk defines the storage for instance.
	OSDisk `json:"osDisk"`

	// OSImage defines the image to use for the OS.
	// +optional
	OSImage *OSImage `json:"osImage,omitempty"`

	// NetworkResourceGroupName specifies the network resource group that contains an existing VNet.
	// Ignored unless VirtualNetwork is also specified.
	// +optional
	NetworkResourceGroupName string `json:"networkResourceGroupName,omitempty"`

	// ComputeSubnet specifies an existing subnet for use by compute nodes.
	// If omitted, the default (${infraID}-worker-subnet) will be used.
	// +optional
	ComputeSubnet string `json:"computeSubnet,omitempty"`

	// VirtualNetwork specifies the name of an existing VNet for the Machines to use
	// If omitted, the default (${infraID}-vnet) will be used.
	// +optional
	VirtualNetwork string `json:"virtualNetwork,omitempty"`

	// VMNetworkingType specifies whether to enable accelerated networking.
	// Accelerated networking enables single root I/O virtualization (SR-IOV) to a VM, greatly improving its
	// networking performance.
	// eg. values: "Accelerated", "Basic"
	//
	// +kubebuilder:validation:Enum="Accelerated"; "Basic"
	// +optional
	VMNetworkingType string `json:"vmNetworkingType,omitempty"`

	// OutboundType is a strategy for how egress from cluster is achieved. When not specified default is "Loadbalancer".
	// +optional
	OutboundType string `json:"outboundType"`
}

// OSImage is the image to use for the OS of a machine.
type OSImage struct {
	// Publisher is the publisher of the image.
	Publisher string `json:"publisher"`
	// Offer is the offer of the image.
	Offer string `json:"offer"`
	// SKU is the SKU of the image.
	SKU string `json:"sku"`
	// Version is the version of the image.
	Version string `json:"version"`
}
