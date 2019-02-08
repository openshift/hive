/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
