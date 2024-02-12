#!/bin/bash -xe

SCRIPT_ROOT=$(dirname ${BASH_SOURCE})/..
CODEGEN_PKG=${CODEGEN_PKG:-$(cd ${SCRIPT_ROOT}; ls -d -1 ./vendor/k8s.io/code-generator 2>/dev/null || echo ../../../k8s.io/code-generator)}

# use gsed for MAC env
SED_CMD=sed
if [[ `uname` == 'Darwin' ]]; then
  SED_CMD=gsed
fi

# HACK: For some reason this script is not executable.
${SED_CMD} -i 's,^exec \(.*/generate-internal-groups.sh\),bash \1,g' ${CODEGEN_PKG}/generate-groups.sh
# ...but we have to put it back, or `verify` will puke.
trap "git checkout ${CODEGEN_PKG}/generate-groups.sh" EXIT

cd "${SCRIPT_ROOT}"

###
# NOTE: Keep Makefile's `verify-codegen` in sync with the paths in these commands (the second and third arg)
###

GOFLAGS="" bash ${CODEGEN_PKG}/generate-groups.sh "applyconfiguration,client,deepcopy" \
  github.com/openshift/hive/pkg/client \
  github.com/openshift/hive/apis \
  "hive:v1 hiveinternal:v1alpha1" \
  --go-header-file ${SCRIPT_ROOT}/hack/boilerplate.go.txt \
  --trim-path-prefix github.com/openshift/hive

# Generate deepcopy for platform-specific types.
GOFLAGS="" bash ${CODEGEN_PKG}/generate-groups.sh "deepcopy" \
  github.com/openshift/hive/pkg/client \
  github.com/openshift/hive/apis \
  "hive:v1/agent hive:v1/aws hive:v1/azure hive:v1/baremetal hive:v1/gcp hive:v1/metricsconfig hive:v1/none hive:v1/openstack hive:v1/ovirt hive:v1/vsphere hive:v1/ibmcloud hivecontracts:v1alpha1" \
  --go-header-file ${SCRIPT_ROOT}/hack/boilerplate.go.txt \
  --trim-path-prefix github.com/openshift/hive
