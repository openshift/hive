//go:build tools
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
	_ "github.com/go-bindata/go-bindata/go-bindata"
	// Used to generate mocks
	_ "go.uber.org/mock/mockgen"
	// Used to lint code
	_ "golang.org/x/lint/golint"
	// Used to lint code
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"

	// TODO: remove this patch with kube bump to 1.34, which will carry the fix (https://github.com/kubernetes/kubernetes/pull/132378)
	// Work around for https://github.com/kubernetes/kubernetes/issues/132377
	_ "k8s.io/code-generator/cmd/validation-gen"
)
