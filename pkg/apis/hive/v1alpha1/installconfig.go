package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"

	"net"
)

// InstallConfig is the configuration for an OpenShift install.
type InstallConfig struct {
	// ClusterID is the ID of the cluster.
	ClusterID string `json:"clusterID"`

	// SSHKey is the reference to the secret that contains a public key to use for access to compute instances.
	SSHKey *corev1.LocalObjectReference `json:"sshKey,omitempty"`

	// BaseDomain is the base domain to which the cluster should belong.
	BaseDomain string `json:"baseDomain"`

	// Networking defines the pod network provider in the cluster.
	Networking `json:"networking"`

	// Machines is the list of MachinePools that need to be installed.
	Machines []MachinePool `json:"machines"`

	// Platform is the configuration for the specific platform upon which to
	// perform the installation.
	Platform `json:"platform"`

	// PullSecret is the reference to the secret to use when pulling images.
	PullSecret corev1.LocalObjectReference `json:"pullSecret"`
}

// Platform is the configuration for the specific platform upon which to perform
// the installation. Only one of the platform configuration should be set.
type Platform struct {
	// AWS is the configuration used when installing on AWS.
	AWS *AWSPlatform `json:"aws,omitempty"`
	// Libvirt is the configuration used when installing on libvirt.
	Libvirt *LibvirtPlatform `json:"libvirt,omitempty"`
}

// Networking defines the pod network provider in the cluster.
type Networking struct {
	Type        NetworkType `json:"type"`
	ServiceCIDR string      `json:"serviceCIDR"`
	PodCIDR     string      `json:"podCIDR"`
}

// NetworkType defines the pod network provider in the cluster.
type NetworkType string

const (
	// NetworkTypeOpenshiftSDN is used to install with SDN.
	NetworkTypeOpenshiftSDN NetworkType = "OpenshiftSDN"
	// NetworkTypeOpenshiftOVN is used to install with OVN.
	NetworkTypeOpenshiftOVN NetworkType = "OVNKubernetes"
)

// AWSPlatform stores all the global configuration that
// all machinesets use.
type AWSPlatform struct {
	// Region specifies the AWS region where the cluster will be created.
	Region string `json:"region"`

	// UserTags specifies additional tags for AWS resources created for the cluster.
	UserTags map[string]string `json:"userTags,omitempty"`

	// DefaultMachinePlatform is the default configuration used when
	// installing on AWS for machine pools which do not define their own
	// platform configuration.
	DefaultMachinePlatform *AWSMachinePoolPlatform `json:"defaultMachinePlatform,omitempty"`

	// VPCID specifies the vpc to associate with the cluster.
	// If empty, new vpc will be created.
	// +optional
	VPCID string `json:"vpcID"`

	// VPCCIDRBlock
	// +optional
	VPCCIDRBlock string `json:"vpcCIDRBlock"`
}

// LibvirtPlatform stores all the global configuration that
// all machinesets use.
type LibvirtPlatform struct {
	// URI is the identifier for the libvirtd connection.  It must be
	// reachable from both the host (where the installer is run) and the
	// cluster (where the cluster-API controller pod will be running).
	URI string `json:"URI"`

	// DefaultMachinePlatform is the default configuration used when
	// installing on AWS for machine pools which do not define their own
	// platform configuration.
	DefaultMachinePlatform *LibvirtMachinePoolPlatform `json:"defaultMachinePlatform,omitempty"`

	// Network
	Network LibvirtNetwork `json:"network"`

	// MasterIPs
	MasterIPs []net.IP `json:"masterIPs"`
}

// LibvirtNetwork is the configuration of the libvirt network.
type LibvirtNetwork struct {
	// Name is the name of the nework.
	Name string `json:"name"`
	// IfName is the name of the network interface.
	IfName string `json:"if"`
	// IPRange is the range of IPs to use.
	IPRange string `json:"ipRange"`
}
