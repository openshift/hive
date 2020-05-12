#!/bin/bash

set -e

tempdir="$(mktemp -d)"

SRC_DIR="$(git rev-parse --show-toplevel)"

mkdir "${tempdir}/crds"
cp ${SRC_DIR}/config/crds/* "${tempdir}/crds/"

make manifests
for filename in ${SRC_DIR}/config/crds/*; do
	if ! diff -Naup "${filename}" "${tempdir}/crds/$(basename -- ${filename})"; then
		"Generated crd file is different. Run 'make manifests'"
	fi
done
