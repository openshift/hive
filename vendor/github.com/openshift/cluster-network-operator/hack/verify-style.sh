#!/usr/bin/env bash
#
# This script invokes tools that should be run prior to pushing
# a repo, such as linters. This is designed to prevent running
# CI on code that will have to be changed.

set -uo pipefail

if [[ ! $(which go) ]]; then
  echo "go not found on PATH. To install:"
  echo "https://golang.org/dl/"
  exit 1
fi
if [[ ! $(which golint) ]]; then
  echo "golint not found on PATH. To install:"
  echo "go get -u github.com/golang/lint/golint"
  exit 1
fi
if [[ ! $(which operator-sdk) ]]; then
  echo "operator-sdk not found on PATH. To install:"
  echo "go get -u github.com/operator-framework/operator-sdk/commands/operator-sdk"
fi

rc=0
trap 'rc=$?' ERR

# Go to the root of the repo
cd "$(git rev-parse --show-cdup)"

GOFILES=$(find . -path ./vendor -prune -o -name '*.go' | grep -v vendor | grep -v pkg/operator/assets)
GOPKGS=$(go list ./... | grep -v '/vendor/' | grep -v '/generated/' | grep -v pkg/operator/assets)

echo "Running gofmt..."
gofmt -s -d $GOFILES

echo "Running go vet..."
go vet $GOPKGS

# Temporariy disabled because it breaks openshift-ci
#echo "Running verify code-generators"
#(cd hack && ./verify-codegen.sh)

echo "Done!"
exit ${rc}
