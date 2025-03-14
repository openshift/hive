package nutanixutils

import (
	"reflect"
	"sort"
	"testing"

	"github.com/openshift/hive/apis/hive/v1/nutanix"
	nutanixinstaller "github.com/openshift/installer/pkg/types/nutanix"
)

func TestConvertHiveFailureDomains(t *testing.T) {
	tests := []struct {
		name                   string
		hiveFailureDomains     []nutanix.FailureDomain
		expectedFailureDomains []nutanixinstaller.FailureDomain
		expectedPrismElements  []nutanixinstaller.PrismElement
		expectedSubnetUUIDs    []string
	}{
		{
			name:                   "empty failure domains",
			hiveFailureDomains:     []nutanix.FailureDomain{},
			expectedFailureDomains: []nutanixinstaller.FailureDomain{},
			expectedPrismElements:  []nutanixinstaller.PrismElement{},
			expectedSubnetUUIDs:    []string{},
		},
		{
			name: "single failure domain",
			hiveFailureDomains: []nutanix.FailureDomain{
				{
					Name: "fd1",
					PrismElement: nutanix.PrismElement{
						UUID: "prism-uuid-1",
						Endpoint: nutanix.PrismEndpoint{
							Address: "10.0.0.1",
							Port:    9440,
						},
						Name: "prism1",
					},
					SubnetUUIDs: []string{"subnet-uuid-1", "subnet-uuid-2"},
					StorageContainers: []nutanix.StorageResourceReference{
						{
							ReferenceName: "sc1",
							UUID:          "sc-uuid-1",
							Name:          "storage-container-1",
						},
					},
					DataSourceImages: []nutanix.StorageResourceReference{
						{
							ReferenceName: "dsi1",
							UUID:          "dsi-uuid-1",
							Name:          "datasource-image-1",
						},
					},
				},
			},
			expectedFailureDomains: []nutanixinstaller.FailureDomain{
				{
					Name: "fd1",
					PrismElement: nutanixinstaller.PrismElement{
						UUID: "prism-uuid-1",
						Endpoint: nutanixinstaller.PrismEndpoint{
							Address: "10.0.0.1",
							Port:    9440,
						},
						Name: "prism1",
					},
					SubnetUUIDs: []string{"subnet-uuid-1", "subnet-uuid-2"},
					StorageContainers: []nutanixinstaller.StorageResourceReference{
						{
							ReferenceName: "sc1",
							UUID:          "sc-uuid-1",
							Name:          "storage-container-1",
						},
					},
					DataSourceImages: []nutanixinstaller.StorageResourceReference{
						{
							ReferenceName: "dsi1",
							UUID:          "dsi-uuid-1",
							Name:          "datasource-image-1",
						},
					},
				},
			},
			expectedPrismElements: []nutanixinstaller.PrismElement{
				{
					UUID: "prism-uuid-1",
					Endpoint: nutanixinstaller.PrismEndpoint{
						Address: "10.0.0.1",
						Port:    9440,
					},
					Name: "prism1",
				},
			},
			expectedSubnetUUIDs: []string{"subnet-uuid-1", "subnet-uuid-2"},
		},
		{
			name: "multiple failure domains",
			hiveFailureDomains: []nutanix.FailureDomain{
				{
					Name: "fd1",
					PrismElement: nutanix.PrismElement{
						UUID: "prism-uuid-1",
						Endpoint: nutanix.PrismEndpoint{
							Address: "10.0.0.1",
							Port:    9440,
						},
						Name: "prism1",
					},
					SubnetUUIDs: []string{"subnet-uuid-1"},
					StorageContainers: []nutanix.StorageResourceReference{
						{
							ReferenceName: "sc1",
							UUID:          "sc-uuid-1",
							Name:          "storage-container-1",
						},
					},
					DataSourceImages: []nutanix.StorageResourceReference{
						{
							ReferenceName: "dsi1",
							UUID:          "dsi-uuid-1",
							Name:          "datasource-image-1",
						},
					},
				},
				{
					Name: "fd2",
					PrismElement: nutanix.PrismElement{
						UUID: "prism-uuid-2",
						Endpoint: nutanix.PrismEndpoint{
							Address: "10.0.0.2",
							Port:    9440,
						},
						Name: "prism2",
					},
					SubnetUUIDs: []string{"subnet-uuid-2"},
					StorageContainers: []nutanix.StorageResourceReference{
						{
							ReferenceName: "sc2",
							UUID:          "sc-uuid-2",
							Name:          "storage-container-2",
						},
					},
					DataSourceImages: []nutanix.StorageResourceReference{
						{
							ReferenceName: "dsi2",
							UUID:          "dsi-uuid-2",
							Name:          "datasource-image-2",
						},
					},
				},
			},
			expectedFailureDomains: []nutanixinstaller.FailureDomain{
				{
					Name: "fd1",
					PrismElement: nutanixinstaller.PrismElement{
						UUID: "prism-uuid-1",
						Endpoint: nutanixinstaller.PrismEndpoint{
							Address: "10.0.0.1",
							Port:    9440,
						},
						Name: "prism1",
					},
					SubnetUUIDs: []string{"subnet-uuid-1"},
					StorageContainers: []nutanixinstaller.StorageResourceReference{
						{
							ReferenceName: "sc1",
							UUID:          "sc-uuid-1",
							Name:          "storage-container-1",
						},
					},
					DataSourceImages: []nutanixinstaller.StorageResourceReference{
						{
							ReferenceName: "dsi1",
							UUID:          "dsi-uuid-1",
							Name:          "datasource-image-1",
						},
					},
				},
				{
					Name: "fd2",
					PrismElement: nutanixinstaller.PrismElement{
						UUID: "prism-uuid-2",
						Endpoint: nutanixinstaller.PrismEndpoint{
							Address: "10.0.0.2",
							Port:    9440,
						},
						Name: "prism2",
					},
					SubnetUUIDs: []string{"subnet-uuid-2"},
					StorageContainers: []nutanixinstaller.StorageResourceReference{
						{
							ReferenceName: "sc2",
							UUID:          "sc-uuid-2",
							Name:          "storage-container-2",
						},
					},
					DataSourceImages: []nutanixinstaller.StorageResourceReference{
						{
							ReferenceName: "dsi2",
							UUID:          "dsi-uuid-2",
							Name:          "datasource-image-2",
						},
					},
				},
			},
			expectedPrismElements: []nutanixinstaller.PrismElement{
				{
					UUID: "prism-uuid-1",
					Endpoint: nutanixinstaller.PrismEndpoint{
						Address: "10.0.0.1",
						Port:    9440,
					},
					Name: "prism1",
				},
				{
					UUID: "prism-uuid-2",
					Endpoint: nutanixinstaller.PrismEndpoint{
						Address: "10.0.0.2",
						Port:    9440,
					},
					Name: "prism2",
				},
			},
			expectedSubnetUUIDs: []string{"subnet-uuid-1", "subnet-uuid-2"},
		},
		{
			name: "multiple failure domains with shared subnet UUIDs and PrismElements",
			hiveFailureDomains: []nutanix.FailureDomain{
				{
					Name: "fd1",
					PrismElement: nutanix.PrismElement{
						UUID: "prism-uuid-1",
						Endpoint: nutanix.PrismEndpoint{
							Address: "10.0.0.1",
							Port:    9440,
						},
						Name: "prism1",
					},
					SubnetUUIDs: []string{"subnet-uuid-1", "subnet-uuid-2"},
					StorageContainers: []nutanix.StorageResourceReference{
						{
							ReferenceName: "sc1",
							UUID:          "sc-uuid-1",
							Name:          "storage-container-1",
						},
					},
					DataSourceImages: []nutanix.StorageResourceReference{
						{
							ReferenceName: "dsi1",
							UUID:          "dsi-uuid-1",
							Name:          "datasource-image-1",
						},
					},
				},
				{
					Name: "fd2",
					PrismElement: nutanix.PrismElement{
						UUID: "prism-uuid-1", // Shared PrismElement UUID
						Endpoint: nutanix.PrismEndpoint{
							Address: "10.0.0.1", // Shared PrismElement Endpoint
							Port:    9440,
						},
						Name: "prism1", // Shared PrismElement Name
					},
					SubnetUUIDs: []string{"subnet-uuid-2", "subnet-uuid-3"}, // Shared subnet-uuid-2
					StorageContainers: []nutanix.StorageResourceReference{
						{
							ReferenceName: "sc2",
							UUID:          "sc-uuid-2",
							Name:          "storage-container-2",
						},
					},
					DataSourceImages: []nutanix.StorageResourceReference{
						{
							ReferenceName: "dsi2",
							UUID:          "dsi-uuid-2",
							Name:          "datasource-image-2",
						},
					},
				},
			},
			expectedFailureDomains: []nutanixinstaller.FailureDomain{
				{
					Name: "fd1",
					PrismElement: nutanixinstaller.PrismElement{
						UUID: "prism-uuid-1",
						Endpoint: nutanixinstaller.PrismEndpoint{
							Address: "10.0.0.1",
							Port:    9440,
						},
						Name: "prism1",
					},
					SubnetUUIDs: []string{"subnet-uuid-1", "subnet-uuid-2"},
					StorageContainers: []nutanixinstaller.StorageResourceReference{
						{
							ReferenceName: "sc1",
							UUID:          "sc-uuid-1",
							Name:          "storage-container-1",
						},
					},
					DataSourceImages: []nutanixinstaller.StorageResourceReference{
						{
							ReferenceName: "dsi1",
							UUID:          "dsi-uuid-1",
							Name:          "datasource-image-1",
						},
					},
				},
				{
					Name: "fd2",
					PrismElement: nutanixinstaller.PrismElement{
						UUID: "prism-uuid-1",
						Endpoint: nutanixinstaller.PrismEndpoint{
							Address: "10.0.0.1",
							Port:    9440,
						},
						Name: "prism1",
					},
					SubnetUUIDs: []string{"subnet-uuid-2", "subnet-uuid-3"},
					StorageContainers: []nutanixinstaller.StorageResourceReference{
						{
							ReferenceName: "sc2",
							UUID:          "sc-uuid-2",
							Name:          "storage-container-2",
						},
					},
					DataSourceImages: []nutanixinstaller.StorageResourceReference{
						{
							ReferenceName: "dsi2",
							UUID:          "dsi-uuid-2",
							Name:          "datasource-image-2",
						},
					},
				},
			},
			expectedPrismElements: []nutanixinstaller.PrismElement{
				{
					UUID: "prism-uuid-1",
					Endpoint: nutanixinstaller.PrismEndpoint{
						Address: "10.0.0.1",
						Port:    9440,
					},
					Name: "prism1",
				},
			},
			expectedSubnetUUIDs: []string{"subnet-uuid-1", "subnet-uuid-2", "subnet-uuid-3"},
		},
		{
			name: "failure domain with nil PrismElement", // Negative test case
			hiveFailureDomains: []nutanix.FailureDomain{
				{
					Name:         "fd-nil-prism",
					PrismElement: nutanix.PrismElement{}, // Intentionally Zero-Valued PrismElement (simulating nil in terms of expected output after handling)
					SubnetUUIDs:  []string{"subnet-uuid-4"},
					StorageContainers: []nutanix.StorageResourceReference{
						{
							ReferenceName: "sc3",
							UUID:          "sc-uuid-3",
							Name:          "storage-container-3",
						},
					},
					DataSourceImages: []nutanix.StorageResourceReference{
						{
							ReferenceName: "dsi3",
							UUID:          "dsi-uuid-3",
							Name:          "datasource-image-3",
						},
					},
				},
			},
			expectedFailureDomains: []nutanixinstaller.FailureDomain{
				{
					Name:         "fd-nil-prism",
					PrismElement: nutanixinstaller.PrismElement{}, // Expect Zero-Valued PrismElement in output
					SubnetUUIDs:  []string{"subnet-uuid-4"},
					StorageContainers: []nutanixinstaller.StorageResourceReference{
						{
							ReferenceName: "sc3",
							UUID:          "sc-uuid-3",
							Name:          "storage-container-3",
						},
					},
					DataSourceImages: []nutanixinstaller.StorageResourceReference{
						{
							ReferenceName: "dsi3",
							UUID:          "dsi-uuid-3",
							Name:          "datasource-image-3",
						},
					},
				},
			},
			expectedPrismElements: []nutanixinstaller.PrismElement{}, // Not expecting any PrismElement to be added to unique list for nil input (or zero-valued - depends on logic)
			expectedSubnetUUIDs:   []string{"subnet-uuid-4"},         // Subnet UUIDs should still be processed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualFailureDomains, actualPrismElements, actualSubnetUUIDs := ConvertHiveFailureDomains(tt.hiveFailureDomains)

			// Sort Subnet UUIDs for consistent comparison
			sort.Strings(actualSubnetUUIDs)
			sort.Strings(tt.expectedSubnetUUIDs)

			if !reflect.DeepEqual(actualFailureDomains, tt.expectedFailureDomains) {
				t.Errorf("ConvertHiveFailureDomains() FailureDomains got = %v, want %v", actualFailureDomains, tt.expectedFailureDomains)
			}

			if !reflect.DeepEqual(actualPrismElements, tt.expectedPrismElements) {
				t.Errorf("ConvertHiveFailureDomains() PrismElements got = %v, want %v", actualPrismElements, tt.expectedPrismElements)
			}

			if !reflect.DeepEqual(actualSubnetUUIDs, tt.expectedSubnetUUIDs) {
				t.Errorf("ConvertHiveFailureDomains() SubnetUUIDs got = %v, want %v", actualSubnetUUIDs, tt.expectedSubnetUUIDs)
			}
		})
	}
}

func TestConvertInstallerFailureDomains(t *testing.T) {
	tests := []struct {
		name                    string
		installerFailureDomains []nutanixinstaller.FailureDomain
		expectedFailureDomains  []nutanix.FailureDomain
		expectedPrismElements   []nutanix.PrismElement
		expectedSubnetUUIDs     []string
	}{
		{
			name:                    "empty failure domains",
			installerFailureDomains: []nutanixinstaller.FailureDomain{},
			expectedFailureDomains:  []nutanix.FailureDomain{},
			expectedPrismElements:   []nutanix.PrismElement{},
			expectedSubnetUUIDs:     []string{},
		},
		{
			name: "single failure domain",
			installerFailureDomains: []nutanixinstaller.FailureDomain{
				{
					Name: "fd1",
					PrismElement: nutanixinstaller.PrismElement{
						UUID: "installer-prism-uuid-1",
						Endpoint: nutanixinstaller.PrismEndpoint{
							Address: "10.0.0.1",
							Port:    9440,
						},
						Name: "installer-prism1",
					},
					SubnetUUIDs: []string{"subnet-uuid-1", "subnet-uuid-2"},
					StorageContainers: []nutanixinstaller.StorageResourceReference{
						{
							ReferenceName: "sc1",
							UUID:          "sc-uuid-1",
							Name:          "storage-container-1",
						},
					},
					DataSourceImages: []nutanixinstaller.StorageResourceReference{
						{
							ReferenceName: "dsi1",
							UUID:          "dsi-uuid-1",
							Name:          "datasource-image-1",
						},
					},
				},
			},
			expectedFailureDomains: []nutanix.FailureDomain{
				{
					Name: "fd1",
					PrismElement: nutanix.PrismElement{
						UUID: "installer-prism-uuid-1",
						Endpoint: nutanix.PrismEndpoint{
							Address: "10.0.0.1",
							Port:    9440,
						},
						Name: "installer-prism1",
					},
					SubnetUUIDs: []string{"subnet-uuid-1", "subnet-uuid-2"},
					StorageContainers: []nutanix.StorageResourceReference{
						{
							ReferenceName: "sc1",
							UUID:          "sc-uuid-1",
							Name:          "storage-container-1",
						},
					},
					DataSourceImages: []nutanix.StorageResourceReference{
						{
							ReferenceName: "dsi1",
							UUID:          "dsi-uuid-1",
							Name:          "datasource-image-1",
						},
					},
				},
			},
			expectedPrismElements: []nutanix.PrismElement{
				{
					UUID: "installer-prism-uuid-1",
					Endpoint: nutanix.PrismEndpoint{
						Address: "10.0.0.1",
						Port:    9440,
					},
					Name: "installer-prism1",
				},
			},
			expectedSubnetUUIDs: []string{"subnet-uuid-1", "subnet-uuid-2"},
		},
		{
			name: "multiple failure domains",
			installerFailureDomains: []nutanixinstaller.FailureDomain{
				{
					Name: "fd1",
					PrismElement: nutanixinstaller.PrismElement{
						UUID: "installer-prism-uuid-1",
						Endpoint: nutanixinstaller.PrismEndpoint{
							Address: "10.0.0.1",
							Port:    9440,
						},
						Name: "installer-prism1",
					},
					SubnetUUIDs: []string{"subnet-uuid-1"},
					StorageContainers: []nutanixinstaller.StorageResourceReference{
						{
							ReferenceName: "sc1",
							UUID:          "sc-uuid-1",
							Name:          "storage-container-1",
						},
					},
					DataSourceImages: []nutanixinstaller.StorageResourceReference{
						{
							ReferenceName: "dsi1",
							UUID:          "dsi-uuid-1",
							Name:          "datasource-image-1",
						},
					},
				},
				{
					Name: "fd2",
					PrismElement: nutanixinstaller.PrismElement{
						UUID: "installer-prism-uuid-2",
						Endpoint: nutanixinstaller.PrismEndpoint{
							Address: "10.0.0.2",
							Port:    9440,
						},
						Name: "installer-prism2",
					},
					SubnetUUIDs: []string{"subnet-uuid-2"},
					StorageContainers: []nutanixinstaller.StorageResourceReference{
						{
							ReferenceName: "sc2",
							UUID:          "sc-uuid-2",
							Name:          "storage-container-2",
						},
					},
					DataSourceImages: []nutanixinstaller.StorageResourceReference{
						{
							ReferenceName: "dsi2",
							UUID:          "dsi-uuid-2",
							Name:          "datasource-image-2",
						},
					},
				},
			},
			expectedFailureDomains: []nutanix.FailureDomain{
				{
					Name: "fd1",
					PrismElement: nutanix.PrismElement{
						UUID: "installer-prism-uuid-1",
						Endpoint: nutanix.PrismEndpoint{
							Address: "10.0.0.1",
							Port:    9440,
						},
						Name: "installer-prism1",
					},
					SubnetUUIDs: []string{"subnet-uuid-1"},
					StorageContainers: []nutanix.StorageResourceReference{
						{
							ReferenceName: "sc1",
							UUID:          "sc-uuid-1",
							Name:          "storage-container-1",
						},
					},
					DataSourceImages: []nutanix.StorageResourceReference{
						{
							ReferenceName: "dsi1",
							UUID:          "dsi-uuid-1",
							Name:          "datasource-image-1",
						},
					},
				},
				{
					Name: "fd2",
					PrismElement: nutanix.PrismElement{
						UUID: "installer-prism-uuid-2",
						Endpoint: nutanix.PrismEndpoint{
							Address: "10.0.0.2",
							Port:    9440,
						},
						Name: "installer-prism2",
					},
					SubnetUUIDs: []string{"subnet-uuid-2"},
					StorageContainers: []nutanix.StorageResourceReference{
						{
							ReferenceName: "sc2",
							UUID:          "sc-uuid-2",
							Name:          "storage-container-2",
						},
					},
					DataSourceImages: []nutanix.StorageResourceReference{
						{
							ReferenceName: "dsi2",
							UUID:          "dsi-uuid-2",
							Name:          "datasource-image-2",
						},
					},
				},
			},
			expectedPrismElements: []nutanix.PrismElement{
				{
					UUID: "installer-prism-uuid-1",
					Endpoint: nutanix.PrismEndpoint{
						Address: "10.0.0.1",
						Port:    9440,
					},
					Name: "installer-prism1",
				},
				{
					UUID: "installer-prism-uuid-2",
					Endpoint: nutanix.PrismEndpoint{
						Address: "10.0.0.2",
						Port:    9440,
					},
					Name: "installer-prism2",
				},
			},
			expectedSubnetUUIDs: []string{"subnet-uuid-1", "subnet-uuid-2"},
		},
		{
			name: "multiple installer failure domains with shared subnet UUIDs and PrismElements",
			installerFailureDomains: []nutanixinstaller.FailureDomain{
				{
					Name: "fd1",
					PrismElement: nutanixinstaller.PrismElement{
						UUID: "installer-prism-uuid-1",
						Endpoint: nutanixinstaller.PrismEndpoint{
							Address: "10.0.0.1",
							Port:    9440,
						},
						Name: "installer-prism1",
					},
					SubnetUUIDs: []string{"subnet-uuid-1", "subnet-uuid-2"},
					StorageContainers: []nutanixinstaller.StorageResourceReference{
						{
							ReferenceName: "sc1",
							UUID:          "sc-uuid-1",
							Name:          "storage-container-1",
						},
					},
					DataSourceImages: []nutanixinstaller.StorageResourceReference{
						{
							ReferenceName: "dsi1",
							UUID:          "dsi-uuid-1",
							Name:          "datasource-image-1",
						},
					},
				},
				{
					Name: "fd2",
					PrismElement: nutanixinstaller.PrismElement{
						UUID: "installer-prism-uuid-1", // Shared PrismElement UUID
						Endpoint: nutanixinstaller.PrismEndpoint{
							Address: "10.0.0.1", // Shared PrismElement Endpoint
							Port:    9440,
						},
						Name: "installer-prism1", // Shared PrismElement Name
					},
					SubnetUUIDs: []string{"subnet-uuid-2", "subnet-uuid-3"}, // Shared subnet-uuid-2
					StorageContainers: []nutanixinstaller.StorageResourceReference{
						{
							ReferenceName: "sc2",
							UUID:          "sc-uuid-2",
							Name:          "storage-container-2",
						},
					},
					DataSourceImages: []nutanixinstaller.StorageResourceReference{
						{
							ReferenceName: "dsi2",
							UUID:          "dsi-uuid-2",
							Name:          "datasource-image-2",
						},
					},
				},
			},
			expectedFailureDomains: []nutanix.FailureDomain{
				{
					Name: "fd1",
					PrismElement: nutanix.PrismElement{
						UUID: "installer-prism-uuid-1",
						Endpoint: nutanix.PrismEndpoint{
							Address: "10.0.0.1",
							Port:    9440,
						},
						Name: "installer-prism1",
					},
					SubnetUUIDs: []string{"subnet-uuid-1", "subnet-uuid-2"},
					StorageContainers: []nutanix.StorageResourceReference{
						{
							ReferenceName: "sc1",
							UUID:          "sc-uuid-1",
							Name:          "storage-container-1",
						},
					},
					DataSourceImages: []nutanix.StorageResourceReference{
						{
							ReferenceName: "dsi1",
							UUID:          "dsi-uuid-1",
							Name:          "datasource-image-1",
						},
					},
				},
				{
					Name: "fd2",
					PrismElement: nutanix.PrismElement{
						UUID: "installer-prism-uuid-1",
						Endpoint: nutanix.PrismEndpoint{
							Address: "10.0.0.1",
							Port:    9440,
						},
						Name: "installer-prism1",
					},
					SubnetUUIDs: []string{"subnet-uuid-2", "subnet-uuid-3"},
					StorageContainers: []nutanix.StorageResourceReference{
						{
							ReferenceName: "sc2",
							UUID:          "sc-uuid-2",
							Name:          "storage-container-2",
						},
					},
					DataSourceImages: []nutanix.StorageResourceReference{
						{
							ReferenceName: "dsi2",
							UUID:          "dsi-uuid-2",
							Name:          "datasource-image-2",
						},
					},
				},
			},
			expectedPrismElements: []nutanix.PrismElement{
				{
					UUID: "installer-prism-uuid-1",
					Endpoint: nutanix.PrismEndpoint{
						Address: "10.0.0.1",
						Port:    9440,
					},
					Name: "installer-prism1",
				},
			},
			expectedSubnetUUIDs: []string{"subnet-uuid-1", "subnet-uuid-2", "subnet-uuid-3"},
		},
		{
			name: "failure domain with nil PrismElement", // Negative test case
			installerFailureDomains: []nutanixinstaller.FailureDomain{
				{
					Name:         "fd-nil-prism",
					PrismElement: nutanixinstaller.PrismElement{}, // Intentionally Zero-Valued PrismElement (simulating nil in terms of expected output after handling)
					SubnetUUIDs:  []string{"subnet-uuid-4"},
					StorageContainers: []nutanixinstaller.StorageResourceReference{
						{
							ReferenceName: "sc3",
							UUID:          "sc-uuid-3",
							Name:          "storage-container-3",
						},
					},
					DataSourceImages: []nutanixinstaller.StorageResourceReference{
						{
							ReferenceName: "dsi3",
							UUID:          "dsi-uuid-3",
							Name:          "datasource-image-3",
						},
					},
				},
			},
			expectedFailureDomains: []nutanix.FailureDomain{
				{
					Name:         "fd-nil-prism",
					PrismElement: nutanix.PrismElement{}, // Expect Zero-Valued PrismElement in output
					SubnetUUIDs:  []string{"subnet-uuid-4"},
					StorageContainers: []nutanix.StorageResourceReference{
						{
							ReferenceName: "sc3",
							UUID:          "sc-uuid-3",
							Name:          "storage-container-3",
						},
					},
					DataSourceImages: []nutanix.StorageResourceReference{
						{
							ReferenceName: "dsi3",
							UUID:          "dsi-uuid-3",
							Name:          "datasource-image-3",
						},
					},
				},
			},
			expectedPrismElements: []nutanix.PrismElement{},  // Not expecting any PrismElement to be added to unique list for nil input (or zero-valued)
			expectedSubnetUUIDs:   []string{"subnet-uuid-4"}, // Subnet UUIDs should still be processed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualFailureDomains, actualPrismElements, actualSubnetUUIDs := ConvertInstallerFailureDomains(tt.installerFailureDomains)

			// Sort Subnet UUIDs for consistent comparison
			sort.Strings(actualSubnetUUIDs)
			sort.Strings(tt.expectedSubnetUUIDs)

			if !reflect.DeepEqual(actualFailureDomains, tt.expectedFailureDomains) {
				t.Errorf("ConvertInstallerFailureDomains() FailureDomains got = %v, want %v", actualFailureDomains, tt.expectedFailureDomains)
			}

			if !reflect.DeepEqual(actualPrismElements, tt.expectedPrismElements) {
				t.Errorf("ConvertInstallerFailureDomains() PrismElements got = %v, want %v", actualPrismElements, tt.expectedPrismElements)
			}

			if !reflect.DeepEqual(actualSubnetUUIDs, tt.expectedSubnetUUIDs) {
				t.Errorf("ConvertInstallerFailureDomains() SubnetUUIDs got = %v, want %v", actualSubnetUUIDs, tt.expectedSubnetUUIDs)
			}
		})
	}
}
