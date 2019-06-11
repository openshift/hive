package operator

import (
	"github.com/openshift/hive/pkg/operator/hive"
)

func init() {
	// AddToOperatorFuncs is a list of functions to create controllers and add them to an operator manager.
	AddToOperatorFuncs = append(AddToOperatorFuncs, hive.Add)
}
