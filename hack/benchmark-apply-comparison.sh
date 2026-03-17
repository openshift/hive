#!/bin/bash
# Compare Apply/Patch benchmark performance before and after buffer pool optimization
#
# Usage:
#   ./hack/benchmark-apply-comparison.sh                    # Default: 10x iterations, 5 runs (~2 min)
#   BENCHTIME=100x COUNT=10 ./hack/benchmark-apply-comparison.sh  # Thorough: (~13 min)
#   BENCHTIME=2s COUNT=10 ./hack/benchmark-apply-comparison.sh    # Time-based: (~20 min)
#   BENCHTIME=3x COUNT=3 ./hack/benchmark-apply-comparison.sh     # Ultra-fast: (~1 min)

set -e

# Configurable benchmark parameters
BENCHTIME=${BENCHTIME:-3x}
COUNT=${COUNT:-6}

TEMP_DIR=$(mktemp -d)
STASHED=0

# Ensure git stash is restored on exit
cleanup() {
    if [ "$STASHED" -eq 1 ]; then
        echo ""
        echo "Restoring stashed changes..."
        git stash pop -q || echo "Warning: failed to restore stash"
    fi
}
trap cleanup EXIT

echo "Benchmark settings: benchtime=${BENCHTIME}, count=${COUNT}"
echo "Results will be saved to: ${TEMP_DIR}"
echo ""

echo "Running benchmarks WITH buffer pool..."
go test -bench='^Benchmark(Apply|Patch|Controller)' -benchmem -benchtime=${BENCHTIME} -count=${COUNT} ./pkg/resource/ > "${TEMP_DIR}/with.txt" 2>&1

echo "Stashing buffer pool changes..."
git stash push -q pkg/resource/bufferpool.go pkg/resource/apply.go pkg/resource/patch.go
STASHED=1

echo "Running benchmarks WITHOUT buffer pool..."
go test -bench='^Benchmark(Apply|Patch|Controller)' -benchmem -benchtime=${BENCHTIME} -count=${COUNT} ./pkg/resource/ > "${TEMP_DIR}/without.txt" 2>&1

echo "Restoring changes..."
git stash pop -q
STASHED=0

echo ""
echo "=== COMPARISON ==="
benchstat "${TEMP_DIR}/without.txt" "${TEMP_DIR}/with.txt"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Results saved to: ${TEMP_DIR}"
echo "  - ${TEMP_DIR}/without.txt"
echo "  - ${TEMP_DIR}/with.txt"
echo ""
echo "To view again: benchstat ${TEMP_DIR}/without.txt ${TEMP_DIR}/with.txt"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
