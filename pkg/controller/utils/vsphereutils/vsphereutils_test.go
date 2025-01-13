package vsphereutils

import (
	"testing"

	"github.com/go-test/deep"
	hivevsphere "github.com/openshift/hive/apis/hive/v1/vsphere"
	installervsphere "github.com/openshift/installer/pkg/types/vsphere"
	"github.com/stretchr/testify/require"
)

func TestConvertDeprecatedFields(t *testing.T) {
	tests := []struct {
		Name                       string
		DeprecatedVCenter          string
		DeprecatedDatacenter       string
		DeprecatedDefaultDatastore string
		DeprecatedFolder           string
		DeprecatedCluster          string
		DeprecatedNetwork          string
		Expected                   *installervsphere.Platform
	}{
		{
			Name:                       "Basic",
			DeprecatedVCenter:          "my_vcenter",
			DeprecatedDatacenter:       "my_datacenter",
			DeprecatedDefaultDatastore: "my_datastore",
			DeprecatedFolder:           "my_folder",
			DeprecatedCluster:          "my_cluster",
			DeprecatedNetwork:          "my_network",
			Expected: &installervsphere.Platform{
				VCenters: []installervsphere.VCenter{
					{
						Server:      "my_vcenter",
						Port:        443,
						Datacenters: []string{"my_datacenter"},
					},
				},
				FailureDomains: []installervsphere.FailureDomain{
					{
						Name:   "generated-failure-domain",
						Region: "generated-region",
						Zone:   "generated-zone",
						Server: "my_vcenter",
						Topology: installervsphere.Topology{
							Datacenter:     "my_datacenter",
							ComputeCluster: "/my_datacenter/host/my_cluster",
							Networks:       []string{"my_network"},
							Datastore:      "/my_datacenter/datastore/my_datastore",
							Folder:         "/my_datacenter/vm/my_folder",
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			platform := &hivevsphere.Platform{
				DeprecatedVCenter:          test.DeprecatedVCenter,
				DeprecatedDatacenter:       test.DeprecatedDatacenter,
				DeprecatedDefaultDatastore: test.DeprecatedDefaultDatastore,
				DeprecatedFolder:           test.DeprecatedFolder,
				DeprecatedCluster:          test.DeprecatedCluster,
				DeprecatedNetwork:          test.DeprecatedNetwork,
			}

			test.Expected.DeprecatedVCenter = test.DeprecatedVCenter
			test.Expected.DeprecatedDatacenter = test.DeprecatedDatacenter
			test.Expected.DeprecatedDefaultDatastore = test.DeprecatedDefaultDatastore
			test.Expected.DeprecatedFolder = test.DeprecatedFolder
			test.Expected.DeprecatedCluster = test.DeprecatedCluster
			test.Expected.DeprecatedNetwork = test.DeprecatedNetwork

			err := ConvertDeprecatedFields(platform)
			require.NoError(t, err)

			if diff := deep.Equal(platform.Infrastructure, test.Expected); diff != nil {
				t.Error(diff)
			}
		})
	}
}
