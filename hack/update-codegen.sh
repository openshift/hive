#!/bin/bash -xe

HIVE_ROOT=$(dirname ${BASH_SOURCE})/..
pushd $HIVE_ROOT
HIVE_ROOT=$(pwd)
popd

CODEGEN_PKG=${CODEGEN_PKG:-$(cd ${HIVE_ROOT}; ls -d -1 ./vendor/k8s.io/code-generator 2>/dev/null || echo ../../../k8s.io/code-generator)}

###
# NOTE: Keep Makefile's `verify-codegen` in sync with the paths in these commands (the second and third arg)
###

source "${CODEGEN_PKG}/kube_codegen.sh"

pushd "${HIVE_ROOT}/apis"
kube::codegen::gen_helpers \
  --boilerplate "${HIVE_ROOT}/hack/boilerplate.go.txt" \
  "${HIVE_ROOT}/apis"
# the gen_helpers function clobbers the vendor folder, so we have to restore it
git restore "${HIVE_ROOT}/apis/vendor"
popd

# NOTE (28 June 2024): This is currently a no-op, as there is nothing to generate in pkg/client
pushd "${HIVE_ROOT}"
kube::codegen::gen_helpers \
  --boilerplate "${HIVE_ROOT}/hack/boilerplate.go.txt" \
  "${HIVE_ROOT}/pkg/client"
popd