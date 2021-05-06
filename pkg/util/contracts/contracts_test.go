package contracts

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSupportedImplementations(t *testing.T) {
	config := SupportedContractImplementationsList{{
		Name: "a",
		Supported: []ContractImplementation{{
			Group:   "g1",
			Version: "v1",
			Kind:    "kind1",
		}, {
			Group:   "g1",
			Version: "v1",
			Kind:    "kind2",
		}, {
			Group:   "g2",
			Version: "v1",
			Kind:    "kind3",
		}},
	}, {
		Name: "b",
		Supported: []ContractImplementation{{
			Group:   "g1",
			Version: "v1",
			Kind:    "kind4",
		}, {
			Group:   "g2",
			Version: "v1",
			Kind:    "kind5",
		}},
	}}

	tests := []struct {
		contract string
		expected []string
	}{{
		contract: "a",
		expected: []string{
			"g1/v1, Kind=kind1",
			"g1/v1, Kind=kind2",
			"g2/v1, Kind=kind3",
		},
	}, {
		contract: "b",
		expected: []string{
			"g1/v1, Kind=kind4",
			"g2/v1, Kind=kind5",
		},
	}, {
		contract: "c",
	}}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			supported := config.SupportedImplementations(tt.contract)
			assert.Equal(t, tt.expected, supported)
		})
	}
}

func TestIsSupported(t *testing.T) {
	config := SupportedContractImplementationsList{{
		Name: "a",
		Supported: []ContractImplementation{{
			Group:   "g1",
			Version: "v1",
			Kind:    "kind1",
		}, {
			Group:   "g1",
			Version: "v1",
			Kind:    "kind2",
		}, {
			Group:   "g2",
			Version: "v1",
			Kind:    "kind3",
		}},
	}, {
		Name: "b",
		Supported: []ContractImplementation{{
			Group:   "g1",
			Version: "v1",
			Kind:    "kind4",
		}, {
			Group:   "g2",
			Version: "v1",
			Kind:    "kind5",
		}},
	}}

	tests := []struct {
		contract string
		resource ContractImplementation

		expected bool
	}{{
		contract: "a",
		resource: ContractImplementation{
			Group:   "g1",
			Version: "v1",
			Kind:    "kind1",
		},
		expected: true,
	}, {
		contract: "b",
		resource: ContractImplementation{
			Group:   "g2",
			Version: "v1",
			Kind:    "kind5",
		},
		expected: true,
	}, {
		contract: "a",
		resource: ContractImplementation{
			Group:   "g1",
			Version: "v1",
			Kind:    "kind3",
		},
		expected: false,
	}, {
		contract: "c",
		resource: ContractImplementation{
			Group:   "g1",
			Version: "v1",
			Kind:    "kind1",
		},
		expected: false,
	}}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			supported := config.IsSupported(tt.contract, tt.resource)
			assert.Equal(t, tt.expected, supported)
		})
	}
}
