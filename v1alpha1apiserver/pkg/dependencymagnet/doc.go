// +build tools

// Package dependencymagnet is a package for setting dependencies used by extra-build tooling.
// go mod won't pull in code that isn't depended upon, but we have some code we don't depend on from code that must be
// included for our build or testing to work.
package dependencymagnet

import (
	// Used for Makefile
	_ "github.com/openshift/build-machinery-go"
)
