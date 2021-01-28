package agent

// Platform defines agent based install configuration specific to bare metal clusters.
type Platform struct {
	// APIVIP is the virtual IP used to reach the OpenShift cluster's API.
	APIVIP string `json:"apiVIP"`
	// APIVIPDNSName is the domain name used to reach the OpenShift cluster API.
	APIVIPDNSName string `json:"apiVIPDNSName"`
	// IngressVIP is the virtual IP used for cluster ingress traffic.
	IngressVIP string `json:"ingressVIP"`
	// VIPDHCPAllocation indicates if virtual IP DHCP allocation mode is enabled.
	// +optional
	VIPDHCPAllocation bool `json:"vipDHCPAllocation"`
}
