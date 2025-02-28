package constants

const (
	// NutanixUsernameEnvVar is the environment variable specifying the Nutanix Prism Central username.
	NutanixUsernameEnvVar = "NUTANIX_USERNAME"

	// NutanixPasswordEnvVar is the environment variable specifying the Nutanix Prism Central password.
	NutanixPasswordEnvVar = "NUTANIX_PASSWORD"

	// NutanixPrismCentralEndpointEnvVar is the environment variable specifying the Nutanix Prism Central endpoint.
	NutanixPrismCentralEndpointEnvVar = "NUTANIX_PC_ADDRESS"

	// NutanixPrismCentralPortEnvVar is the environment variable specifying the Nutanix Prism Central port.
	NutanixPrismCentralPortEnvVar = "NUTANIX_PC_PORT"

	// NutanixPrismElementEndpointEnvVar is the environment variable specifying the Nutanix Prism Element endpoint.
	NutanixPrismElementEndpointEnvVar = "NUTANIX_PE_ADDRESS"

	// NutanixPrismElementPortEnvVar is the environment variable specifying the Nutanix Prism Element port.
	NutanixPrismElementPortEnvVar = "NUTANIX_PE_PORT"

	// CliNutanixPcAddressCMD defines the hiveutil variable name used to specify the Nutanix Prism Central address.
	CliNutanixPcAddressCMD = "nutanix-pc-address"

	// CliNutanixPcPortCMD defines the hiveutil variable name used to specify the Nutanix Prism Central port.
	CliNutanixPcPortCMD = "nutanix-pc-port"

	// CliNutanixPeAddressCMD defines the hiveutil variable name used to specify the Nutanix Prism Element address.
	CliNutanixPeAddressCMD = "nutanix-pe-address"

	// CliNutanixPePortCMD defines the hiveutil variable name used to specify the Nutanix Prism Element port.
	CliNutanixPePortCMD = "nutanix-pe-port"

	// CliNutanixPeUUIDCMD defines the hiveutil variable name used to specify the Nutanix Prism Element UUID.
	CliNutanixPeUUIDCMD = "nutanix-pe-uuid"

	// CliNutanixPeNameCMD defines the hiveutil variable name used to specify the Nutanix Prism Element name.
	CliNutanixPeNameCMD = "nutanix-pe-name"

	// CliNutanixAzNameCMD defines the hiveutil variable name used to specify the Nutanix Availability Zone name.
	CliNutanixAzNameCMD = "nutanix-az-name"

	// CliNutanixApiVipCMD defines the hiveutil variable name used to specify the Virtual IP address for the api endpoint.
	CliNutanixApiVipCMD = "nutanix-api-vip"

	// CliNutanixIngressVipCMD defines the hiveutil variable name used to specify the Virtual IP address for ingress application routing.
	CliNutanixIngressVipCMD = "nutanix-ingress-vip"

	// CliNutanixSubnetUUIDCmd defines the hiveutil variable name used to specify the a list of network subnets uuids to be used by the cluster.
	CliNutanixSubnetUUIDCmd = "nutanix-subnetUUIDs"
)
