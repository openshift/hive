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

package controller

import (
	// TODO: Remove this blank import when re-enabling federation. For now, we need it here so
	// that the federation controller still compiles with the rest of the controllers and it
	// doesn't bit rot.
	_ "github.com/openshift/hive/pkg/controller/federation"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.

	// TODO: Use a better method for determining whether federation is installed before re-enabling
	// federation. The federation CRDs may be installed but federation may be limited to only certain
	// namespaces of the cluster. We may need to introduce a configuration setting either in the
	// operator config or per cluster deployment.
	// AddToManagerFuncs = append(AddToManagerFuncs, federation.Add)
}
