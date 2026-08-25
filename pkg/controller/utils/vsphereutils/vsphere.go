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
	// ConvertInstallConfig copies legacy names onto the installer Platform and
	// leaves them there. infrastructure.folder is CRD-validated as a full
	// inventory path (^/.*?/vm/.*?); a non-pathed leftover fails KAS
	// (OCPBUGS-105520). Drop all deprecated fields — topology already holds
	// the upconverted values.
	infra := platform.Infrastructure
	infra.DeprecatedVCenter = ""
	infra.DeprecatedUsername = ""
	infra.DeprecatedPassword = ""
	infra.DeprecatedDatacenter = ""
	infra.DeprecatedDefaultDatastore = ""
	infra.DeprecatedFolder = ""
	infra.DeprecatedCluster = ""
	infra.DeprecatedResourcePool = ""
	infra.DeprecatedAPIVIP = ""
	infra.DeprecatedIngressVIP = ""
	infra.DeprecatedNetwork = ""
	return nil
}
