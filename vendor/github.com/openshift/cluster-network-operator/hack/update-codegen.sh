#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT=$(dirname ${BASH_SOURCE})/..

${SCRIPT_ROOT}/vendor/k8s.io/code-generator/generate-groups.sh deepcopy \
  github.com/openshift/cluster-network-operator/pkg/generated github.com/openshift/cluster-network-operator/pkg/apis \
  "networkoperator:v1" \
  --go-header-file ${SCRIPT_ROOT}/hack/custom-boilerplate.go.txt
