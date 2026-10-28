#!/usr/bin/env bash
# Compare benchmark performance before and after working changes.
#
# Uses git worktrees for safe comparison without touching your working tree.
# Compares the current working tree (including uncommitted changes) against a
# base commit (default: HEAD~1).
#
# Usage:
#   ./hack/benchmark-comparison.sh [base-commit]
#
# Examples:
#   # Compare current working tree vs parent commit
#   ./hack/benchmark-comparison.sh
#
#   # Compare current working tree vs main branch
#   ./hack/benchmark-comparison.sh main
#
#   # Compare with custom benchmark settings
#   BENCHTIME=10x COUNT=8 ./hack/benchmark-comparison.sh
#
#   # Benchmark specific packages
#   BENCH_PKGS="./pkg/resource/" ./hack/benchmark-comparison.sh
#
# The benchmarks run against envtest by default. Set BENCHMARK_KUBECONFIG to
# compare against a real cluster instead (see test/benchmark/README.md).
#
# NB: the base commit must itself contain the benchmark suites. Comparing
# against a commit that predates the harness (e.g. main before it merges)
# fails fast with an explanation.

set -o errexit
set -o nounset
set -o pipefail

# Configurable benchmark parameters
BENCHTIME=${BENCHTIME:-5x}
COUNT=${COUNT:-6}
BENCH_PATTERN=${BENCH_PATTERN:-'^Benchmark'}
# Defaults to the controller simulations only, to keep a COUNT-run comparison
# quick; use BENCH_PKGS to widen (all three suites match 'make benchmark').
BENCH_PKGS=${BENCH_PKGS:-./test/benchmark/}
BASE_COMMIT=${1:-HEAD~1}

# Run from the repo root regardless of where the script is invoked from.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}/.."

# Ensure we're in a git repository
if ! git rev-parse --git-dir > /dev/null 2>&1; then
    echo "Error: Not in a git repository"
    exit 1
fi

# Ensure a benchmark cluster is configured: envtest assets, or a real cluster.
if [ -z "${KUBEBUILDER_ASSETS:-}" ] && [ -z "${BENCHMARK_KUBECONFIG:-}" ]; then
    if command -v setup-envtest &> /dev/null; then
        # NB: assign separately from the export so a failing setup-envtest.sh
        # trips errexit instead of being silently swallowed (SC2155).
        KUBEBUILDER_ASSETS="$(hack/setup-envtest.sh -q)"
        export KUBEBUILDER_ASSETS
    else
        echo "Error: KUBEBUILDER_ASSETS environment variable not set"
        echo ""
        echo "Run: make install-tools && make setup-envtest"
        echo "Then: export KUBEBUILDER_ASSETS=\"\$(hack/setup-envtest.sh -q)\""
        echo "Or set BENCHMARK_KUBECONFIG to compare against a real cluster."
        exit 1
    fi
fi

# Verify base commit exists
if ! git rev-parse "$BASE_COMMIT" > /dev/null 2>&1; then
    echo "Error: Base commit '$BASE_COMMIT' not found"
    exit 1
fi

TEMP_DIR=$(mktemp -d)
# NB: no -t flag; it is deprecated in GNU coreutils and behaves differently
# on BSD mktemp.
WORKTREE_DIR=$(mktemp -d "${TMPDIR:-/tmp}/hive-benchmark-worktree.XXXXXX")

# remove_worktree deletes the comparison worktree. When git's own removal
# fails but the directory is gone anyway, the administrative metadata under
# .git/worktrees would linger in 'git worktree list' until a manual prune,
# so prune it here.
remove_worktree() {
    if git worktree remove "$WORKTREE_DIR" --force 2>/dev/null; then
        return 0
    fi
    rm -rf "$WORKTREE_DIR" 2>/dev/null || true
    git worktree prune 2>/dev/null || true
}

cleanup() {
    if [ -d "$WORKTREE_DIR" ]; then
        echo ""
        echo "Cleaning up worktree..."
        remove_worktree
    fi
}
trap cleanup EXIT

echo "==========================================================="
echo "Benchmark Comparison"
echo "==========================================================="
echo "Base commit:     $BASE_COMMIT ($(git rev-parse --short "$BASE_COMMIT"))"
echo "Working tree:    $(git rev-parse --short HEAD)$(git diff --quiet && git diff --cached --quiet || echo ' (dirty)')"
echo "Benchmark time:  ${BENCHTIME}"
echo "Run count:       ${COUNT}"
echo "Pattern:         ${BENCH_PATTERN}"
echo "Packages:        ${BENCH_PKGS}"
echo "Results dir:     ${TEMP_DIR}"
echo "==========================================================="
echo ""

# Create the worktree and validate it BEFORE spending time on benchmarks, so
# a bad base commit (one that predates the benchmark harness, or a mistyped
# BENCH_PKGS path) fails fast instead of after the current-tree run.
echo "-> Creating worktree at ${BASE_COMMIT}..."
git worktree add --detach "$WORKTREE_DIR" "$BASE_COMMIT" --quiet

# Each benchmarked package must contain benchmark functions at the base
# commit. Searching the commit's tree (not just checking that the package
# directory exists) also catches packages that predate the harness but
# already existed without benchmarks.
# shellcheck disable=SC2086
for pkg in ${BENCH_PKGS}; do
    # Package patterns (./pkg/...) can't be checked this way; the go test run
    # below will fail loudly on its own if they match nothing.
    case "$pkg" in
        *...*) continue ;;
    esac
    if ! git grep -q 'func Benchmark' "$BASE_COMMIT" -- "$pkg"; then
        echo "Error: no benchmark functions in ${pkg} at ${BASE_COMMIT}." >&2
        echo "The base commit likely predates the benchmark harness. Pick a base" >&2
        echo "that contains it, or set BENCH_PKGS to packages with benchmarks on both sides." >&2
        exit 1
    fi
done

# check_results aborts unless the results file contains benchmark output:
# benchstat on empty results is confusing, and an empty side usually means
# the pattern matched nothing there.
check_results() {
    if ! grep -q '^Benchmark' "$2"; then
        echo "Error: no benchmark results on the $1 side." >&2
        echo "Does that side contain the benchmark suites, and does the" >&2
        echo "pattern ('${BENCH_PATTERN}') match anything there?" >&2
        echo "Raw output: $2" >&2
        exit 1
    fi
}

# Run benchmarks in the current working tree.
# -run '^$' skips unit tests: they only add wall time here, and a failing (or
# flaky) unit test at either commit would otherwise abort the comparison.
# -p 1 runs package test binaries sequentially: each suite starts its own
# envtest (kube-apiserver + etcd) and is CPU-bound, so concurrent suites
# contaminate wall-time measurements. Only matters when BENCH_PKGS is widened
# beyond the single-package default.
echo "-> Running benchmarks in current working tree..."
# shellcheck disable=SC2086
go test -p 1 -run '^$' -bench="${BENCH_PATTERN}" -benchmem -benchtime="${BENCHTIME}" -count="${COUNT}" ${BENCH_PKGS} 2>&1 | tee "${TEMP_DIR}/new.txt"
# Check before spending a second full benchmark run on the base side: a
# pattern that matches nothing should not cost more than one run.
check_results "working tree" "${TEMP_DIR}/new.txt"

# Run benchmarks at base commit (in worktree). The subshell inherits the
# environment (KUBEBUILDER_ASSETS, BENCHMARK_KUBECONFIG, BENCH_CRD_DIR, ...),
# so both sides run under identical conditions.
echo ""
echo "-> Running benchmarks at base commit..."
# shellcheck disable=SC2086
(cd "$WORKTREE_DIR" && go test -p 1 -run '^$' -bench="${BENCH_PATTERN}" -benchmem -benchtime="${BENCHTIME}" -count="${COUNT}" ${BENCH_PKGS}) 2>&1 | tee "${TEMP_DIR}/old.txt"
check_results "base commit" "${TEMP_DIR}/old.txt"

# Cleanup worktree early; tolerate failure -- losing the comparison to a
# cleanup error after both runs completed would be worse than a lingering
# worktree (which the EXIT trap retries anyway).
echo ""
remove_worktree

# Compare results
echo ""
echo "==========================================================="
echo "BENCHMARK COMPARISON"
echo "==========================================================="
echo ""

if ! command -v benchstat &> /dev/null; then
    echo "Warning: benchstat not installed. Install the version pinned in vendor with:"
    echo "  make install-tools"
    echo "(or: go install golang.org/x/perf/cmd/benchstat@latest)"
    echo ""
    echo "Raw results saved to:"
    echo "  Old: ${TEMP_DIR}/old.txt"
    echo "  New: ${TEMP_DIR}/new.txt"
else
    benchstat "${TEMP_DIR}/old.txt" "${TEMP_DIR}/new.txt"
fi

echo ""
echo "==========================================================="
echo "Results saved to: ${TEMP_DIR}"
echo "  Base ($BASE_COMMIT):  ${TEMP_DIR}/old.txt"
echo "  Working tree:        ${TEMP_DIR}/new.txt"
echo ""
if command -v benchstat &> /dev/null; then
    echo "To view again:"
    echo "  benchstat ${TEMP_DIR}/old.txt ${TEMP_DIR}/new.txt"
fi
echo "==========================================================="
