package nutanix

import corev1 "k8s.io/api/core/v1"

// CredentialsSecretName is the default nutanix credentials secret name.
//
//nolint:gosec
const CredentialsSecretName = "nutanix-credentials"

// Platform stores any global configuration used for Nutanix platforms.
type Platform struct {
	// PrismCentral is the endpoint (address and port) and credentials to connect to the Prism Central.
	// This serves as the default Prism-Central.
	PrismCentral PrismCentral `json:"prismCentral"`

	// PrismElements holds a list of Prism Elements (clusters). A Prism Element encompasses all Nutanix resources (VMs, subnets, etc.)
	// used to host the OpenShift cluster. Currently only a single Prism Element may be defined.
	// This serves as the default Prism-Element.
	PrismElements []PrismElement `json:"prismElements"`

	// CredentialsSecretRef refers to a secret that contains the Nutanix account access
	// credentials.
	CredentialsSecretRef corev1.LocalObjectReference `json:"credentialsSecretRef"`

	// CertificatesSecretRef refers to a secret that contains the Prism Central CA certificates
	// necessary for communicating with the Prism Central.
	CertificatesSecretRef corev1.LocalObjectReference `json:"certificatesSecretRef"`

	// ClusterOSImage overrides the url provided in rhcos.json to download the RHCOS Image.
	//
	// +optional
	ClusterOSImage string `json:"clusterOSImage,omitempty"`

	// DefaultMachinePlatform is the default configuration used when
	// installing on Nutanix for machine pools which do not define their own
	// platform configuration.
	// +optional
	DefaultMachinePlatform *MachinePool `json:"defaultMachinePlatform,omitempty"`

	// SubnetUUIDs identifies the network subnets to be used by the cluster.
	// Currently, we only support one subnet for an OpenShift cluster.
	SubnetUUIDs []string `json:"subnetUUIDs"`
}

// PrismCentral holds the endpoint and credentials data used to connect to the Prism Central
type PrismCentral struct {
	// Endpoint holds the address and port of the Prism Central
	Endpoint PrismEndpoint `json:"endpoint"`
}

// PrismElement holds the uuid, endpoint of the Prism Element (cluster)
type PrismElement struct {
	// UUID is the UUID of the Prism Element (cluster)
	UUID string `json:"uuid"`

	// Endpoint holds the address and port of the Prism Element
	// +optional
	Endpoint PrismEndpoint `json:"endpoint,omitempty"`

	// Name is prism endpoint Name
	Name string `json:"name,omitempty"`
}

// PrismEndpoint holds the endpoint address and port to access the Nutanix Prism Central or Element (cluster)
type PrismEndpoint struct {
	// address is the endpoint address (DNS name or IP address) of the Nutanix Prism Central or Element (cluster)
	Address string `json:"address"`

	// port is the port number to access the Nutanix Prism Central or Element (cluster)
	Port int32 `json:"port"`
}

// FailureDomain configures failure domain information for the Nutanix platform.
type FailureDomain struct {
	// Name defines the unique name of a failure domain.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=64
	// +kubebuilder:validation:Pattern=`^[0-9A-Za-z_.-@/]+$`
	Name string `json:"name"`

	// prismElement holds the identification (name, uuid) and the optional endpoint address and port of the Nutanix Prism Element.
	// When a cluster-wide proxy is installed, by default, this endpoint will be accessed via the proxy.
	// Should you wish for communication with this endpoint not to be proxied, please add the endpoint to the
	// proxy spec.noProxy list.
	// +kubebuilder:validation:Required
	PrismElement PrismElement `json:"prismElement"`

	// SubnetUUIDs identifies the network subnets of the Prism Element.
	// Currently we only support one subnet for a failure domain.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	// +listType=set
	SubnetUUIDs []string `json:"subnetUUIDs"`

	// StorageContainers identifies the storage containers in the Prism Element.
	// +optional
	StorageContainers []StorageResourceReference `json:"storageContainers,omitempty"`

	// DataSourceImages identifies the datasource images in the Prism Element.
	// +optional
	DataSourceImages []StorageResourceReference `json:"dataSourceImages,omitempty"`
}

// StorageResourceReference holds reference information of a storage resource (storage container, data source image, etc.)
type StorageResourceReference struct {
	// ReferenceName is the identifier of the storage resource configured in the FailureDomain.
	// +optional
	ReferenceName string `json:"referenceName,omitempty"`

	// UUID is the UUID of the storage container resource in the Prism Element.
	// +kubebuilder:validation:Required
	UUID string `json:"uuid"`

	// Name is the name of the storage container resource in the Prism Element.
	// +optional
	Name string `json:"name,omitempty"`
}
