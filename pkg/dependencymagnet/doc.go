// +build tools

// Package dependencymagnet is a package for setting dependencies used by extra-build tooling.
// go mod won't pull in code that isn't depended upon, but we have some code we don't depend on from code that must be
// included for our build or testing to work.
package dependencymagnet

import (
	// Used to generate deep copy functions for custom resources
	_ "k8s.io/code-generator/cmd/deepcopy-gen"
	// Used to generate conversion functions for custom resources
	_ "k8s.io/code-generator/cmd/conversion-gen"
	// Used to generate defaulter functions for custom resources
	_ "k8s.io/code-generator/cmd/defaulter-gen"
	// Used to generate go client for custom resources
	_ "k8s.io/code-generator/cmd/client-gen"
	// Used to generated CRDs
	_ "sigs.k8s.io/controller-tools/cmd/controller-gen"
	// Used to generate bindata
	_ "github.com/jteeuwen/go-bindata/go-bindata"
	// Used to generate mocks
	_ "github.com/golang/mock/mockgen"
	// Used to ling code
	_ "golang.org/x/lint/golint"
	// Used to lint code
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
)
