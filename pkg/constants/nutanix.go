package constants

const (
	// NutanixUsernameEnvVar is the environment variable specifying the Nutanix Prism Central username.
	NutanixUsernameEnvVar = "NUTANIX_USERNAME"

	// NutanixPasswordEnvVar is the environment variable specifying the Nutanix Prism Central password.
	NutanixPasswordEnvVar = "NUTANIX_PASSWORD"

	// CliNutanixPcAddressOpt defines the hiveutil variable name used to specify the Nutanix Prism Central address.
	CliNutanixPcAddressOpt = "nutanix-pc-address"

	// CliNutanixPcPortOpt defines the hiveutil variable name used to specify the Nutanix Prism Central port.
	CliNutanixPcPortOpt = "nutanix-pc-port"

	// CliNutanixPeAddressOpt defines the hiveutil variable name used to specify the Nutanix Prism Element address.
	CliNutanixPeAddressOpt = "nutanix-pe-address"

	// CliNutanixPePortOpt defines the hiveutil variable name used to specify the Nutanix Prism Element port.
	CliNutanixPePortOpt = "nutanix-pe-port"

	// CliNutanixPeUUIDOpt defines the hiveutil variable name used to specify the Nutanix Prism Element UUID.
	CliNutanixPeUUIDOpt = "nutanix-pe-uuid"

	// CliNutanixPeNameOpt defines the hiveutil variable name used to specify the Nutanix Prism Element name.
	CliNutanixPeNameOpt = "nutanix-pe-name"

	// CliNutanixAzNameOpt defines the hiveutil variable name used to specify the Nutanix Availability Zone name.
	CliNutanixAzNameOpt = "nutanix-az-name"

	// CliNutanixApiVipOpt defines the hiveutil variable name used to specify the Virtual IP address for the api endpoint.
	CliNutanixApiVipOpt = "nutanix-api-vip"

	// CliNutanixIngressVipOpt defines the hiveutil variable name used to specify the Virtual IP address for ingress application routing.
	CliNutanixIngressVipOpt = "nutanix-ingress-vip"

	// CliNutanixSubnetUUIDOpt defines the hiveutil variable name used to specify the a list of network subnets uuids to be used by the cluster.
	CliNutanixSubnetUUIDOpt = "nutanix-subnetUUIDs"

	// CliNutanixCACertsOpt defines the hiveutil variable name used to specify the CA certificates for connecting to Nutanix endpoints.
	CliNutanixCACertsOpt = "nutanix-ca-certs"

	// NutanixCertificatesDir is the directory containing Prism Central certificate files.
	NutanixCertificatesDir = "/nutanix-certificates"
)
