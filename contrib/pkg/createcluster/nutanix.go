package createcluster

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/openshift/hive/pkg/clusterresource"
	c "github.com/openshift/hive/pkg/constants"
	installernutanix "github.com/openshift/installer/pkg/types/nutanix"
)

func getNutanixPort(portEnvVar, portCmd string, optionPort int32) (int32, error) {
	// If CLI explicitly set (non-zero), use it
	if optionPort != 0 {
		return optionPort, nil
	}

	// Else fallback to env var
	val := os.Getenv(portEnvVar)
	if val == "" {
		return 0, fmt.Errorf("must provide --%s or set %s env var", portCmd, portEnvVar)
	}

	port, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("error converting '%s' port variable to int: %w", val, err)
	}

	if port == 0 {
		return 0, fmt.Errorf("must provide --%s or set %s env var", portCmd, portEnvVar)
	}

	return int32(port), nil
}

func getNutanixStrVal(envVar, valueCmd, optionValue string, optional bool) (string, error) {
	value := os.Getenv(envVar)
	if optionValue != "" {
		value = optionValue
	}

	if value == "" && !optional {
		if envVar == "" {
			return "", fmt.Errorf("must provide --%s", valueCmd)
		}
		return "", fmt.Errorf("must provide --%s or set %s env var", valueCmd, envVar)
	}

	return value, nil
}

func (o *Options) getNutanixCACert() ([]byte, error) {
	nutanixCACerts, err := getNutanixStrVal("", c.CliNutanixCACertsOpt, o.NutanixCACerts, true)
	if err != nil {
		return nil, err
	}

	var caCerts [][]byte
	for _, cert := range filepath.SplitList(nutanixCACerts) {
		caCert, err := os.ReadFile(cert)
		if err != nil {
			return nil, fmt.Errorf("error reading %s: %w", cert, err)
		}
		caCerts = append(caCerts, caCert)
	}

	return bytes.Join(caCerts, []byte("\n")), nil
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

	pcCACert, err := o.getNutanixCACert()
	if err != nil {
		return nil, err
	}

	prismCentralEndpoint, err := getNutanixStrVal("", c.CliNutanixPcAddressOpt, o.NutanixPrismCentralEndpoint, false)
	if err != nil {
		return nil, err
	}

	prismElementAddress, err := getNutanixStrVal("", c.CliNutanixPeAddressOpt, o.NutanixPrismElementAddress, false)
	if err != nil {
		return nil, err
	}

	prismElementName, err := getNutanixStrVal("", c.CliNutanixPeNameOpt, o.NutanixPrismElementName, false)
	if err != nil {
		return nil, err
	}

	prismElementUUID, err := getNutanixStrVal("", c.CliNutanixPeUUIDOpt, o.NutanixPrismElementUUID, false)
	if err != nil {
		return nil, err
	}

	prismCentralPort, err := getNutanixPort("", c.CliNutanixPcPortOpt, o.NutanixPrismCentralPort)
	if err != nil {
		return nil, err
	}

	prismElementPort, err := getNutanixPort("", c.CliNutanixPePortOpt, o.NutanixPrismElementPort)
	if err != nil {
		return nil, err
	}

	azName, err := getNutanixStrVal("", c.CliNutanixAzNameOpt, o.NutanixAzName, false)
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
		CACert:     pcCACert,
	}

	return nutanixBuilder, nil
}
