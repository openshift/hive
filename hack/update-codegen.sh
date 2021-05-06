#!/bin/bash

SCRIPT_ROOT=$(dirname ${BASH_SOURCE})/..
CODEGEN_PKG=${CODEGEN_PKG:-$(cd ${SCRIPT_ROOT}; ls -d -1 ./vendor/k8s.io/code-generator 2>/dev/null || echo ../../../k8s.io/code-generator)}

verify="${VERIFY:-}"

GOFLAGS="" bash ${CODEGEN_PKG}/generate-groups.sh "all" \
  github.com/openshift/hive/pkg/client \
  github.com/openshift/hive/apis \
  "hive:v1 hiveinternal:v1alpha1" \
  --go-header-file ${SCRIPT_ROOT}/hack/boilerplate.go.txt \
  ${verify}

# Generate deepcopy for platform-specific types.
GOFLAGS="" bash ${CODEGEN_PKG}/generate-groups.sh "deepcopy" \
  github.com/openshift/hive/pkg/client \
  github.com/openshift/hive/apis \
  "hive:v1/agent hive:v1/aws hive:v1/azure hive:v1/baremetal hive:v1/gcp hive:v1/openstack hive:v1/ovirt hive:v1/vsphere hivecontracts:v1alpha1" \
  --go-header-file ${SCRIPT_ROOT}/hack/boilerplate.go.txt \
  ${verify}

# deepcopy generators place the generated files in vendor directory, so move them back
(cd ./vendor/github.com/openshift/hive/apis/; find . -name 'zz_generated.deepcopy.go' -exec cp --parents {} ../../../../../apis/ \;)
