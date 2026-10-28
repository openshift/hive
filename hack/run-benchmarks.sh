#!/usr/bin/env bash

# Runs the Hive benchmark suites. Used by 'make benchmark'.
#
# By default the benchmarks run against a local envtest instance (real
# kube-apiserver + etcd). Set BENCHMARK_KUBECONFIG=/path/to/kubeconfig to run
# them against an existing cluster instead -- see test/benchmark/README.md
# for the requirements and caveats of real-cluster mode.
#
# Environment variables:
#   BENCH_PKGS  packages to benchmark
#               (default: ./pkg/remoteclient/ ./pkg/resource/ ./test/benchmark/)
#   BENCH_ARGS  go test flags, replacing the defaults entirely
#               (default: -bench=. -benchmem -benchtime=5x)
#   BENCHMARK_KUBECONFIG  path to a kubeconfig for real-cluster mode
#   BENCH_CRD_DIR         directory of real OpenShift CRD manifests to install
#                         in envtest mode instead of the minimal synthesized
#                         ones (see test/benchmark/README.md)
#   KUBEBUILDER_ASSETS   envtest binaries path; resolved automatically if unset
#
# Package test binaries run sequentially (-p 1): each suite starts its own
# envtest (kube-apiserver + etcd) and is CPU-bound, so concurrent suites
# contaminate wall-time measurements. Put -p N in BENCH_ARGS to override
# (later flags win).

set -o errexit
set -o nounset
set -o pipefail

# Run from the repo root regardless of where the script is invoked from.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}/.."

BENCH_PKGS=${BENCH_PKGS:-./pkg/remoteclient/ ./pkg/resource/ ./test/benchmark/}
BENCH_ARGS=${BENCH_ARGS:--bench=. -benchmem -benchtime=5x}

if [[ -n "${BENCHMARK_KUBECONFIG:-}" ]]; then
    echo "BENCHMARK_KUBECONFIG is set; running against that cluster (envtest will not be used)"
else
    if [[ -z "${KUBEBUILDER_ASSETS:-}" ]]; then
        if ! command -v setup-envtest &> /dev/null; then
            echo "Error: setup-envtest not found and KUBEBUILDER_ASSETS is not set." >&2
            echo "Run 'make install-tools' first (it installs to \$(go env GOPATH)/bin --" >&2
            echo "is that on your PATH?), or set BENCHMARK_KUBECONFIG to use a real cluster." >&2
            exit 1
        fi
        # NB: assign separately from the export so a failing setup-envtest.sh
        # trips errexit instead of being silently swallowed (SC2155).
        KUBEBUILDER_ASSETS="$(hack/setup-envtest.sh -q)"
        export KUBEBUILDER_ASSETS
        echo "using envtest assets from $KUBEBUILDER_ASSETS"
    fi
fi

echo "Tip: hack/benchmark-comparison.sh A/B-compares the working tree against a base commit."

# shellcheck disable=SC2086
exec go test -p 1 -run '^$' ${BENCH_ARGS} ${BENCH_PKGS}
