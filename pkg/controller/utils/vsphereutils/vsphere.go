package vsphereutils

import (
	hivevsphere "github.com/openshift/hive/apis/hive/v1/vsphere"
	installertypes "github.com/openshift/installer/pkg/types"
	installervsphere "github.com/openshift/installer/pkg/types/vsphere"
	"github.com/openshift/installer/pkg/types/vsphere/conversion"
)

func ConvertDeprecatedFields(platform *hivevsphere.Platform) error {
	if platform.Infrastructure != nil {
		return nil
	}

	dummyInstallConfig := &installertypes.InstallConfig{
		Platform: installertypes.Platform{
			VSphere: &installervsphere.Platform{
				DeprecatedVCenter:          platform.DeprecatedVCenter,
				DeprecatedDatacenter:       platform.DeprecatedDatacenter,
				DeprecatedDefaultDatastore: platform.DeprecatedDefaultDatastore,
				DeprecatedFolder:           platform.DeprecatedFolder,
				DeprecatedCluster:          platform.DeprecatedCluster,
				DeprecatedNetwork:          platform.DeprecatedNetwork,
			},
		},
	}

	if err := conversion.ConvertInstallConfig(dummyInstallConfig); err != nil {
		return err
	}

	platform.Infrastructure = dummyInstallConfig.Platform.VSphere
	return nil
}
