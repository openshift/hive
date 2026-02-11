package machinepoolresource

import (
	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

// PlatformFiller fills MachinePool.Spec.Platform from options defined in this package.
// All MachinePool platform fields are represented by option types in machinepoolresource
// (e.g. AWSOptions, AzureOptions) and applied via FillPlatform.
type PlatformFiller interface {
	FillPlatform(mp *hivev1.MachinePool)
}
