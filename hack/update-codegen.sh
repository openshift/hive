#!/bin/bash

SCRIPT_ROOT=$(dirname ${BASH_SOURCE})/..
CODEGEN_PKG=${CODEGEN_PKG:-$(cd ${SCRIPT_ROOT}; ls -d -1 ./vendor/k8s.io/code-generator 2>/dev/null || echo ../../../k8s.io/code-generator)}

verify="${VERIFY:-}"

GOFLAGS="" bash ${CODEGEN_PKG}/generate-groups.sh "all" \
  github.com/openshift/hive/pkg/client \
  github.com/openshift/hive/pkg/apis \
  "hive:v1" \
  --go-header-file ${SCRIPT_ROOT}/hack/boilerplate.go.txt \
  ${verify}

