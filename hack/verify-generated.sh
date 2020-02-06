#!/bin/bash

set -e

tempdir="$(mktemp -d)"

SRC_DIR="$(git rev-parse --show-toplevel)"

cp "${SRC_DIR}/pkg/apis/hive/v1/zz_generated.deepcopy.go" "${tempdir}"
cp "${SRC_DIR}/pkg/operator/assets/bindata.go" "${tempdir}"
mkdir "${tempdir}/crds"
cp ${SRC_DIR}/config/crds/* "${tempdir}/crds/"

cd "${SRC_DIR}"
make generate

if ! diff -Naup "./pkg/apis/hive/v1/zz_generated.deepcopy.go" "${tempdir}/zz_generated.deepcopy.go"; then
	echo "Generated deepcopy file is different. Run 'make generate'"
	exit 1
fi
if ! diff -Naup "./pkg/operator/assets/bindata.go" "${tempdir}/bindata.go"; then
	echo "Generated bindata file is different. Run 'make generate'"
	exit 1
fi

make manifests
for filename in ${SRC_DIR}/config/crds/*; do
	if ! diff -Naup "${filename}" "${tempdir}/crds/$(basename -- ${filename})"; then
		"Generated crd file is different. Run 'make manifests'"
	fi
done
