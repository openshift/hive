#!/usr/bin/env bash

# Downloads envtest binaries (kube-apiserver, etcd) and prints the
# KUBEBUILDER_ASSETS export command. Run the printed export command (or add it
# to your shell rc) before running benchmarks, or skip this entirely by using
# 'make benchmark', which wires the assets in for you.
#
# Usage:
#   hack/setup-envtest.sh        # human-readable output
#   hack/setup-envtest.sh -q     # prints only the assets path (for scripting)
#
# Environment variables:
#   K8S_VERSION  k8s version to fetch binaries for
#                (default: derived from the k8s.io/api version in go.mod)

set -o errexit
set -o nounset
set -o pipefail

QUIET=false
case "${1:-}" in
    -q) QUIET=true ;;
    "") ;;
    *)
        echo "usage: $0 [-q]" >&2
        exit 2
        ;;
esac

# Run from the repo root regardless of where the script is invoked from;
# the version derivation below needs module context.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}/.."

if ! command -v setup-envtest &> /dev/null; then
    echo "run 'make install-tools' to install setup-envtest" >&2
    echo "(it installs to \$(go env GOPATH)/bin -- is that on your PATH?)" >&2
    exit 1
fi

K8S_VERSION=${K8S_VERSION:-$(go list -m -f "{{ .Version }}" k8s.io/api | awk -F'[v.]' '{printf "1.%d", $3}')}

if [[ "$QUIET" == "false" ]]; then
    echo "using k8s version ${K8S_VERSION}"
    echo "Fetching binaries (on first run this may take some time)"
fi

# NB: do not redirect stderr into the command substitution; setup-envtest
# occasionally writes progress output there, which would pollute the path.
ENVTEST_PATH=$(setup-envtest use "$K8S_VERSION" -p path)

if [[ "$QUIET" == "true" ]]; then
    echo "$ENVTEST_PATH"
    exit 0
fi

echo
echo "Run this now or add it to your shell rc:"
echo "  export KUBEBUILDER_ASSETS=\"$ENVTEST_PATH\""
