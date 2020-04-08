#!/bin/sh

set -e

SRC_DIR="$(git rev-parse --show-toplevel)"

OUTPUT_FILE="${OUTPUT_FILE:-./pkg/operator/assets/bindata.go}"

# go-bindata generates code assets from the yaml we want to deploy by the operator.
go-bindata \
        -nocompress \
	-nometadata \
        -pkg "assets" \
        -o "${OUTPUT_FILE}" \
        -ignore "OWNERS" \
        -ignore ".*\.sw.?" \
        ./config/apiserver/... ./config/hiveadmission/... ./config/controllers/... ./config/rbac/... ./config/crds/... ./config/configmaps/... && \
gofmt -s -w "${OUTPUT_FILE}"
