package controller

import (
	"github.com/openshift/hive/pkg/controller/remotemachineset"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, remotemachineset.Add)
}
