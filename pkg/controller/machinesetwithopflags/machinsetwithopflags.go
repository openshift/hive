package machinesetwithopflags

import (
	machineapi "github.com/openshift/api/machine/v1beta1"
)

type MachineSetWithOpFlags struct {
	MS          *machineapi.MachineSet
	NeedsUpdate bool
	NeedsDelete bool
}
