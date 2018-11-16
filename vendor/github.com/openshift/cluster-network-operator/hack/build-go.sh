#!/usr/bin/env bash

set -eu

REPO=github.com/openshift/cluster-network-operator
CMDS=${CMDS:-cluster-network-operator cluster-network-renderer}
GOFLAGS=${GOFLAGS:-}
GLDFLAGS=${GLDFLAGS:-}

eval $(go env | grep -e "GOHOSTOS" -e "GOHOSTARCH")

GOOS=${GOOS:-${GOHOSTOS}}
GOARCH=${GOACH:-${GOHOSTARCH}}

# Go to the root of the repo
cd "$(git rev-parse --show-cdup)"

if [ -z ${VERSION+a} ]; then
	echo "Using version from git..."
	VERSION=$(git describe --abbrev=8 --dirty --always)
fi

GLDFLAGS+="-X ${REPO}/pkg/version.Raw=${VERSION}"

eval $(go env)

if [ -z ${BIN_PATH+a} ]; then
	export BIN_PATH=_output/${GOOS}/${GOARCH}
fi

mkdir -p ${BIN_PATH}

for cmd in ${CMDS}; do
	echo "Building ${REPO}/cmd/${cmd} (${VERSION})"
	CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build ${GOFLAGS} -ldflags "${GLDFLAGS}" -o ${BIN_PATH}/${cmd} ${REPO}/cmd/${cmd}
done
