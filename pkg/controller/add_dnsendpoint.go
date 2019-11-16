package controller

import (
	"github.com/openshift/hive/pkg/controller/dnsendpoint"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, dnsendpoint.Add)
}
