package agent

// BareMetalPlatform defines agent based install configuration specific to bare metal clusters.
// Can only be used with spec.installStrategy.agent.
type BareMetalPlatform struct {
	// APIVIP is the virtual IP used to reach the OpenShift cluster's API.
	APIVIP string `json:"apiVIP"`

	// APIVIPDNSName is the domain name used to reach the OpenShift cluster API.
	// +optional
	APIVIPDNSName string `json:"apiVIPDNSName,omitempty"`

	// IngressVIP is the virtual IP used for cluster ingress traffic.
	IngressVIP string `json:"ingressVIP"`

	// VIPDHCPAllocation indicates if virtual IP DHCP allocation mode is enabled.
	// +optional
	VIPDHCPAllocation VIPDHCPAllocationType `json:"vipDHCPAllocation"`
}

// VIPDHCPAllocationType is a valid value for bareMetalPlatform.vipDHCPAllocation.
// +kubebuilder:validation:Enum="";"Enabled"
type VIPDHCPAllocationType string

const (
	VIPDHCPAllocationDisabled VIPDHCPAllocationType = ""
	VIPDHCPAllocationEnabled  VIPDHCPAllocationType = "Enabled"
)
