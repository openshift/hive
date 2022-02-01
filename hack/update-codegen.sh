#!/bin/bash

# set the passed in directory as a usable GOPATH
# that deepcopy-gen can operate in
ensure-temp-gopath() {
	fake_gopath=$1

	# set up symlink pointing to our repo root
	fake_repopath=$fake_gopath/src/github.com/openshift/hive
	mkdir -p "$(dirname "${fake_repopath}")"
	ln -s "$REPO_FULL_PATH" "${fake_repopath}"
}

SCRIPT_ROOT=$(dirname ${BASH_SOURCE})/..
REPO_FULL_PATH=$(realpath ${SCRIPT_ROOT})
CODEGEN_PKG=${CODEGEN_PKG:-$(cd ${SCRIPT_ROOT}; ls -d -1 ./vendor/k8s.io/code-generator 2>/dev/null || echo ../../../k8s.io/code-generator)}

verify="${VERIFY:-}"

valid_gopath=$(realpath $REPO_FULL_PATH/../../../..)
if [[ "$(realpath ${valid_gopath}/src/github.com/openshift/hive)" == "${REPO_FULL_PATH}" ]]; then
	temp_gopath=${valid_gopath}
else
	TMP_DIR=$(mktemp -d -t hive-codegen.XXXX)
	function finish {
		chmod -R +w ${TMP_DIR}
		# ok b/c we will symlink to the original repo
		rm -r ${TMP_DIR}
	}
	trap finish EXIT

	ensure-temp-gopath ${TMP_DIR}

	temp_gopath=${TMP_DIR}
fi

GOPATH="${temp_gopath}" GOFLAGS="" bash ${CODEGEN_PKG}/generate-groups.sh "all" \
  github.com/openshift/hive/pkg/client \
  github.com/openshift/hive/apis \
  "hive:v1 hiveinternal:v1alpha1" \
  --go-header-file ${SCRIPT_ROOT}/hack/boilerplate.go.txt \
  ${verify}

# Generate deepcopy for platform-specific types.
GOPATH="${temp_gopath}" GOFLAGS="" bash ${CODEGEN_PKG}/generate-groups.sh "deepcopy" \
  github.com/openshift/hive/pkg/client \
  github.com/openshift/hive/apis \
  "hive:v1/agent hive:v1/aws hive:v1/azure hive:v1/baremetal hive:v1/gcp hive:v1/openstack hive:v1/ovirt hive:v1/vsphere hivecontracts:v1alpha1" \
  --go-header-file ${SCRIPT_ROOT}/hack/boilerplate.go.txt \
  ${verify}

# deepcopy generators place the generated files in vendor directory, so move them back
(cd ./vendor/github.com/openshift/hive/apis/; find . -name 'zz_generated.deepcopy.go' -exec cp --parents {} ../../../../../apis/ \;)
