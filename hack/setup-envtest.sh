#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

if ! command -v setup-envtest &> /dev/null; then
    echo "run 'make install-tools' to install setup-envtest"
    exit 1
fi

K8S_VERSION=${K8S_VERSION:-$(go list -m -f "{{ .Version }}" k8s.io/api | awk -F'[v.]' '{printf "1.%d", $3}')}

echo "using k8s version ${K8S_VERSION}"

echo "Fetching binaries (on first run this may take some time)"
ENVTEST_PATH=$(setup-envtest use "$K8S_VERSION" -p path 2>&1)

echo
echo "Run this now or add it to your shell rc:"
echo "  export KUBEBUILDER_ASSETS=\"$ENVTEST_PATH\""
