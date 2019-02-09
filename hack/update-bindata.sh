#!/bin/sh

set -e

SRC_DIR="$(git rev-parse --show-toplevel)"

OUTPUT_FILE="${OUTPUT_FILE:-./pkg/controller/assets/bindata.go}"

# ensure go-bindata
cd "${SRC_DIR}"
go build -o ./bin/go-bindata ./vendor/github.com/jteeuwen/go-bindata/go-bindata

"./bin/go-bindata" \
        -nocompress \
        -pkg "assets" \
        -o "${OUTPUT_FILE}" \
        -ignore "OWNERS" \
        -ignore ".*\.sw.?" \
        ./config/samples/... && \
gofmt -s -w "${OUTPUT_FILE}"
