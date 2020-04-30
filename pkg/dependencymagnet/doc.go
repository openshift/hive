// +build tools

// Package dependencymagnet is a package for setting dependencies used by extra-build tooling.
// go mod won't pull in code that isn't depended upon, but we have some code we don't depend on from code that must be
// included for our build or testing to work.
package dependencymagnet

import (
	// Used for Makefile
	_ "github.com/openshift/build-machinery-go"
	// Used to generate code for custom resources
	_ "k8s.io/code-generator"
	// Used to generated CRDs
	_ "sigs.k8s.io/controller-tools/cmd/controller-gen"
	// Used to generate bindata
	_ "github.com/jteeuwen/go-bindata/go-bindata"
	// Used to generate mocks
	_ "github.com/golang/mock/mockgen"
	// Used to lint code
	_ "golang.org/x/lint/golint"
	// Used to lint code
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
)
