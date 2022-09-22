package machinepool

import (
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"

	machineapi "github.com/openshift/api/machine/v1beta1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

//go:generate mockgen -source=./actuator.go -destination=./mock/actuator_generated.go -package=mock

// Actuator is the interface that must be implemented to standardize generating
// and returning the list of MachineSets to by synced to the remote cluster.
type Actuator interface {

	// GenerateMachineSets returns the desired set of MachineSets in the target cluster for a given MachinePool.
	// Returns the list of generated machine sets, a boolean indicating if the controller should proceed with reconcile
	// or not, and an error. The boolean may be set in situations where we have not encountered an error, but still need
	// to wait before we can proceed with reconciling. (e.g. obtaining a pool name lease)
	GenerateMachineSets(*hivev1.ClusterDeployment, *hivev1.MachinePool, log.FieldLogger) (msets []*machineapi.MachineSet, proceed bool, genError error)

	MachineProviderSpecEqual(*runtime.RawExtension, *runtime.RawExtension, log.FieldLogger) (bool, error)
}
