package agent

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

	// AgentSelector is a label selector used for associating relevant custom resources with this cluster.
	// (Agent, BareMetalHost, etc)
	AgentSelector metav1.LabelSelector `json:"agentSelector"`
}

// VIPDHCPAllocationType is a valid value for bareMetalPlatform.vipDHCPAllocation.
// +kubebuilder:validation:Enum="";"Enabled"
type VIPDHCPAllocationType string

const (
	VIPDHCPAllocationDisabled VIPDHCPAllocationType = ""
	VIPDHCPAllocationEnabled  VIPDHCPAllocationType = "Enabled"
)
