package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	NETWORKS_LAST_LEASE_UPDATE_ANNOTATION = "vspherecapacitymanager.splat.io/last-network-update"
	NetworkFinalizer                      = "vsphere-capacity-manager.splat-team.io/network-finalizer"
	NetworkKind                           = "Network"
	NetworkTypeLabel                      = "vsphere-capacity-manager.splat-team.io/network-type"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Network defines a pool of resources defined available for a given vCenter, cluster, and datacenter
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:scope=Namespaced
// +kubebuilder:printcolumn:name="Port Group",type=string,JSONPath=`.spec.portGroupName`
// +kubebuilder:printcolumn:name="Pod",type=string,JSONPath=`.spec.podName`
type Network struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec NetworkSpec `json:"spec"`
	// +optional
	Status NetworkStatus `json:"status"`
}

// NetworkSpec defines the specification for a pool
type NetworkSpec struct {
	// PortGroupName is the non-pathed network (port group) name
	PortGroupName string `json:"portGroupName"`

	VlanId string `json:"vlanId"`

	// The PodName is the pod that this VLAN is associated with.
	PodName *string `json:"podName,omitempty"`
	// DatacenterName is foo

	// The DatacenterName is the datacenter that the firewall resides in.
	DatacenterName *string `json:"datacenterName"`

	// The Classless Inter-Domain Routing prefix of this subnet, which specifies the range of spanned IP addresses.
	//
	// [Classless_Inter-Domain_Routing at Wikipedia](http://en.wikipedia.org/wiki/Classless_Inter-Domain_Routing)
	Cidr *int `json:"cidr,omitempty"`

	// The IP address of this subnet reserved for use on the router as a gateway address and which is unavailable for other use.
	Gateway *string `json:"gateway,omitempty"`

	// A count of the IP address records belonging to this subnet.
	IpAddressCount *uint `json:"ipAddressCount,omitempty"`

	// The bitmask in dotted-quad format for this subnet, which specifies the range of spanned IP addresses.
	Netmask *string `json:"netmask,omitempty"`

	SubnetType *string `json:"subnetType,omitempty"`

	// MachineNetworkCidr represents the machine network CIDR.
	// +optional
	MachineNetworkCidr string `json:"machineNetworkCidr"`

	// The IP address records belonging to this subnet.
	// +optional
	IpAddresses []string `json:"ipAddresses"`

	// CidrIPv6 represents the IPv6 network mask.
	// +optional
	CidrIPv6 int `json:"cidrIPv6"`
	// GatewayIPv6 represents the IPv6 gateway IP address.
	// +optional
	GatewayIPv6 string `json:"gatewayipv6"`

	// Ipv6prefix represents the IPv6 prefix.
	// +optional
	IpV6prefix string `json:"ipv6prefix"`

	// StartIPv6Address represents the start IPv6 address for DHCP.
	// +optional
	StartIPv6Address string `json:"startIPv6Address"`

	// PrimaryRouterHostname hostname of the primary router.
	// +optional
	PrimaryRouterHostname string `json:"primaryRouterHostname"`
}

// NetworkStatus defines the status for a pool
type NetworkStatus struct {
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NetworkList is a list of pools
type NetworkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Network `json:"items"`
}

type Networks []*Network
