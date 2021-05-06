package contracts

import (
	"encoding/json"
	"io/ioutil"
	"os"

	"github.com/openshift/hive/pkg/constants"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// SupportedContractImplementations defines a list of resources that implement
// a contract
type SupportedContractImplementations struct {
	Name      string                   `json:"name"`
	Supported []ContractImplementation `json:"supported"`
}

// ContractImplementation is a resources that implements some contract
type ContractImplementation struct {
	Group   string `json:"group"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
}

// SupportedContractImplementationsList is a list of contracts and their supported
// implementations
type SupportedContractImplementationsList []SupportedContractImplementations

func (l SupportedContractImplementationsList) SupportedImplementations(contract string) []string {
	var s []string
	for _, c := range l {
		if c.Name == contract {
			for _, i := range c.Supported {
				gvk := schema.GroupVersionKind{
					Group:   i.Group,
					Version: i.Version,
					Kind:    i.Kind,
				}
				s = append(s, gvk.String())
			}
		}
	}
	return s
}

func (l SupportedContractImplementationsList) IsSupported(contract string, impl ContractImplementation) bool {
	for _, c := range l {
		if c.Name == contract {
			for _, i := range c.Supported {
				if i == impl {
					return true
				}
			}
		}
	}
	return false
}

// ReadSupportContractsFile reads the configuration file and returns a
// a list of contracts and their supported implementations
func ReadSupportContractsFile() (SupportedContractImplementationsList, error) {
	fpath := os.Getenv(constants.SupportedContractImplementationsFileEnvVar)
	if len(fpath) == 0 {
		return nil, nil
	}

	var supportedList SupportedContractImplementationsList

	fileBytes, err := ioutil.ReadFile(fpath)
	if os.IsNotExist(err) {
		return supportedList, nil
	}
	if err != nil {
		return supportedList, err
	}
	if err := json.Unmarshal(fileBytes, &supportedList); err != nil {
		return supportedList, err
	}

	return supportedList, nil
}
