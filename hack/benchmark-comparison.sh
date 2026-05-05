#!/bin/bash
# Compare benchmark performance before and after working changes.
#
# Uses git worktrees for safe comparison without touching your working tree.
# Compares the current working tree against a base commit (default: HEAD~1).
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

set -eo pipefail

# Configurable benchmark parameters
BENCHTIME=${BENCHTIME:-5x}
COUNT=${COUNT:-6}
BENCH_PATTERN=${BENCH_PATTERN:-'^Benchmark'}
BENCH_PKGS=${BENCH_PKGS:-./test/benchmark/}
BASE_COMMIT=${1:-HEAD~1}

# Ensure we're in a git repository
if ! git rev-parse --git-dir > /dev/null 2>&1; then
    echo "Error: Not in a git repository"
    exit 1
fi

# Ensure KUBEBUILDER_ASSETS is set
if [ -z "$KUBEBUILDER_ASSETS" ]; then
    echo "Error: KUBEBUILDER_ASSETS environment variable not set"
    echo ""
    echo "Run: make setup-envtest"
    echo "Then: export KUBEBUILDER_ASSETS=\"\$(hack/setup-envtest.sh | tail -1 | cut -d'=' -f2 | tr -d '\"')\""
    exit 1
fi

# Verify base commit exists
if ! git rev-parse "$BASE_COMMIT" > /dev/null 2>&1; then
    echo "Error: Base commit '$BASE_COMMIT' not found"
    exit 1
fi

TEMP_DIR=$(mktemp -d)
WORKTREE_DIR=$(mktemp -d -t hive-benchmark-worktree.XXXXXX)

cleanup() {
    if [ -d "$WORKTREE_DIR" ]; then
        echo ""
        echo "Cleaning up worktree..."
        git worktree remove "$WORKTREE_DIR" --force 2>/dev/null || true
        rm -rf "$WORKTREE_DIR" 2>/dev/null || true
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

# Run benchmarks in the current working tree
echo "-> Running benchmarks in current working tree..."
go test -bench="${BENCH_PATTERN}" -benchmem -benchtime="${BENCHTIME}" -count="${COUNT}" ${BENCH_PKGS} 2>&1 | tee "${TEMP_DIR}/new.txt"

# Create worktree at base commit
echo ""
echo "-> Creating worktree at ${BASE_COMMIT}..."
git worktree add --detach "$WORKTREE_DIR" "$BASE_COMMIT" --quiet

# Run benchmarks at base commit (in worktree)
echo "-> Running benchmarks at base commit..."
(cd "$WORKTREE_DIR" && \
    export KUBEBUILDER_ASSETS="$KUBEBUILDER_ASSETS" && \
    go test -bench="${BENCH_PATTERN}" -benchmem -benchtime="${BENCHTIME}" -count="${COUNT}" ${BENCH_PKGS}) 2>&1 | tee "${TEMP_DIR}/old.txt"

# Cleanup worktree early
echo ""
git worktree remove "$WORKTREE_DIR" --force
rm -rf "$WORKTREE_DIR"

# Compare results
echo ""
echo "==========================================================="
echo "BENCHMARK COMPARISON"
echo "==========================================================="
echo ""

if ! command -v benchstat &> /dev/null; then
    echo "Warning: benchstat not installed. Install with:"
    echo "  go install golang.org/x/perf/cmd/benchstat@latest"
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
