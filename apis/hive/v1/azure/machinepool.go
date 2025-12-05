package azure

import "github.com/openshift/installer/pkg/types/azure"

// MachinePool stores the configuration for a machine pool installed
// on Azure.
type MachinePool struct {
	azure.MachinePool `json:",inline"`

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

	// OutboundType is a strategy for how egress from cluster is achieved. When not specified default is "Loadbalancer".
	// +optional
	OutboundType azure.OutboundType `json:"outboundType"`
}

// OSImage is the image to use for the OS of a machine.
type OSImage = azure.OSImage
