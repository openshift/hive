package controller

import (
	"github.com/openshift/hive/pkg/controller/clusterstate"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, clusterstate.Add)
}
