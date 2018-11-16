package controller

import (
	"github.com/openshift/cluster-network-operator/pkg/controller/networkconfig"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, networkconfig.Add)
}
