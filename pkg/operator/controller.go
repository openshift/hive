package operator

import (
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// AddToOperatorFuncs is a list of functions to add all Controllers to the Manager
var AddToOperatorFuncs []func(manager.Manager) error

// AddToOperator adds all Controllers to the Operator manager
func AddToOperator(m manager.Manager) error {
	for _, f := range AddToOperatorFuncs {
		if err := f(m); err != nil {
			return err
		}
	}
	return nil
}
