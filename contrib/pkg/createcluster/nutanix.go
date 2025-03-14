package createcluster

import (
	"fmt"
	"os"
	"strconv"

	"github.com/openshift/hive/pkg/clusterresource"
	c "github.com/openshift/hive/pkg/constants"
	installernutanix "github.com/openshift/installer/pkg/types/nutanix"
)

func getNutanixPort(portEnvVar, portCmd string, optionPort int32) (int32, error) {
	var nutanixPort int32
	nutanixPortVal := os.Getenv(portEnvVar)
	if optionPort != 0 {
		nutanixPort = optionPort
	}
	if nutanixPort == 0 {
		port, err := strconv.Atoi(nutanixPortVal)
		if err != nil {
			return 0, fmt.Errorf("error converting '%s' port variable to int: %w", nutanixPortVal, err)
		}
		nutanixPort = int32(port)
	}

	if nutanixPort == 0 {
		return 0, fmt.Errorf("must provide --%s or set %s env var", portCmd, portEnvVar)
	}

	return nutanixPort, nil
}

func getNutanixStrVal(envVar, valueCmd, optionValue string) (string, error) {
	value := os.Getenv(envVar)
	if optionValue != "" {
		value = optionValue
	}
	if value == "" {
		if envVar != "" {
			return "", fmt.Errorf("must provide %s env var", envVar)
		} else {
			return "", fmt.Errorf("must provide --%s or set %s env var", valueCmd, envVar)
		}
	}

	return value, nil
}

func (o *Options) getNutanixCloudBuilder() (*clusterresource.NutanixCloudBuilder, error) {
	username := os.Getenv(c.NutanixUsernameEnvVar)
	if username == "" {
		return nil, fmt.Errorf("no %s env var set, cannot proceed", c.NutanixUsernameEnvVar)
	}
	password := os.Getenv(c.NutanixPasswordEnvVar)
	if password == "" {
		return nil, fmt.Errorf("no %s env var set, cannot proceed", c.NutanixPasswordEnvVar)
	}

	prismCentralEndpoint, err := getNutanixStrVal(c.NutanixPrismCentralEndpointEnvVar, c.CliNutanixPcAddressOpt, o.NutanixPrismCentralEndpoint)
	if err != nil {
		return nil, err
	}

	prismElementAddress, err := getNutanixStrVal(c.NutanixPrismElementEndpointEnvVar, c.CliNutanixPeAddressOpt, o.NutanixPrismElementAddress)
	if err != nil {
		return nil, err
	}

	prismElementName, err := getNutanixStrVal("", c.CliNutanixPeNameOpt, o.NutanixPrismElementName)
	if err != nil {
		return nil, err
	}

	prismElementUUID, err := getNutanixStrVal("", c.CliNutanixPeUUIDOpt, o.NutanixPrismElementUUID)
	if err != nil {
		return nil, err
	}

	prismCentralPort, err := getNutanixPort(c.NutanixPrismCentralPortEnvVar, c.CliNutanixPcPortOpt, o.NutanixPrismCentralPort)
	if err != nil {
		return nil, err
	}

	prismElementPort, err := getNutanixPort(c.NutanixPrismElementPortEnvVar, c.CliNutanixPePortOpt, o.NutanixPrismElementPort)
	if err != nil {
		return nil, err
	}

	azName, err := getNutanixStrVal("", c.CliNutanixAzNameOpt, o.NutanixAzName)
	if err != nil {
		return nil, err
	}

	subnetUUIds := o.NutanixSubnetUUIDs
	if len(subnetUUIds) == 0 {
		return nil, fmt.Errorf("must provide --%s", c.CliNutanixSubnetUUIDOpt)
	}

	nutanixBuilder := &clusterresource.NutanixCloudBuilder{
		PrismCentral: installernutanix.PrismCentral{
			Endpoint: installernutanix.PrismEndpoint{
				Address: prismCentralEndpoint,
				Port:    prismCentralPort,
			},
			Username: username,
			Password: password,
		},
		FailureDomains: []installernutanix.FailureDomain{
			{
				PrismElement: installernutanix.PrismElement{
					Endpoint: installernutanix.PrismEndpoint{
						Address: prismElementAddress,
						Port:    prismElementPort,
					},
					UUID: prismElementUUID,
					Name: prismElementName,
				},
				Name:              azName,
				SubnetUUIDs:       subnetUUIds,
				StorageContainers: nil,
				DataSourceImages:  nil,
			},
		},
		APIVIP:     o.NutanixAPIVIP,
		IngressVIP: o.NutanixIngressVIP,
	}

	return nutanixBuilder, nil
}
