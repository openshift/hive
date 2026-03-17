# Hive Resource/RemoteClient Benchmarks

Performance benchmarks for validating safe optimizations to Hive's legacy
`pkg/resource` (`Helper`) and `pkg/remoteclient` packages.

## Context

The clustersync controller and others that depend on these packages have been
repeatedly flagged by downstream users for slow reconciliation, memory leaks,
and excessive API server load. These packages are legacy code with behavioral
dependencies that prevent rewrites.

These benchmarks enable targeted optimizations by:
1. Identifying hot paths through round-trip and allocation measurement
2. Proving performance improvements via A/B comparison
3. Detecting behavioral regressions - unexpected metric changes indicate logic changes

## Quick Start

```bash
# One-time setup
make install-tools
make setup-envtest
export KUBEBUILDER_ASSETS="$(hack/setup-envtest.sh | tail -1 | cut -d'=' -f2 | tr -d '"')"

# Run all benchmarks
go test -bench=. -benchmem -benchtime=5x -count=6 \
    ./pkg/remoteclient/ ./pkg/resource/ ./test/benchmark/

# Compare before/after a change
./hack/benchmark-comparison.sh [base-commit]
```

## What's Measured

Each benchmark reports:
- Wall time - Total operation duration
- Allocations - Memory allocations per operation (MB/op, allocs/op)
- Round trips - HTTP calls to API server (custom metric)
- Bytes transferred - Network traffic (custom metric)

Key insight: Round-trip count changes indicate behavioral changes.
If your optimization changes round-trip count, it's not just a performance
improvement -- it's a logic change that needs careful review.

### Benchmark Coverage

| Package             | What it measures                                                           |
|---------------------|----------------------------------------------------------------------------|
| `pkg/remoteclient/` | RESTConfig parsing, client creation, discovery                             |
| `pkg/resource/`     | Apply, Patch, Delete, CreateOrUpdate operations                            |
| `test/benchmark/`   | Full reconciliation patterns (clustersync, machinepool, hibernation, etc.) |

## Validating Optimizations

```bash
# 1. Make your optimization
vim pkg/resource/apply.go

# 2. Run comparison against base (main branch or HEAD~1)
./hack/benchmark-comparison.sh main

# Or specify custom settings
BENCHTIME=10x COUNT=8 ./hack/benchmark-comparison.sh
```

## Architecture

Benchmarks run against a real kube-apiserver + etcd (via controller-runtime's envtest),
not mocks, giving accurate round-trip and byte measurements. Each benchmark uses
`benchutil.ControllerHarness` -- a generic primitive that handles environment setup,
object seeding, counter management, and metric reporting.

For detailed internals (envtest lifecycle, harness field semantics, round-trip counting,
namespace isolation, the full benchmark catalog, and measurement caveats) see:

```bash
go doc github.com/openshift/hive/test/benchutil
```

## Adding Benchmarks

Most common patterns are already covered. To add a new one:

```go
// In pkg/resource/helper_benchmark_test.go
func BenchmarkMyOperation(b *testing.B) {
    benchutil.ControllerHarness[*benchutil.HelperState]{
        NewObjects: benchutil.SingleTemplate(benchutil.GenerateConfigMap("test", 0)),
        Setup:      benchutil.SetupLocalHelper,
        Reconcile: func(b *testing.B, s *benchutil.HelperState, objs []client.Object, i int) {
            if _, err := s.Helper.MyOperation(objs[i]); err != nil {
                b.Fatalf("MyOperation failed: %v", err)
            }
        },
        SteadyState: true,
    }.Run(b)
}
```

See `test/benchmark/controller_sim_benchmark_test.go` for full controller reconciliation examples.

## Troubleshooting

### `KUBEBUILDER_ASSETS` not set:
```bash
make setup-envtest
export KUBEBUILDER_ASSETS="$(hack/setup-envtest.sh | tail -1 | cut -d'=' -f2 | tr -d '"')"
```

### Benchmark variance too high:
- Increase iteration count: `-benchtime=10x`
- Increase run count: `-count=10`
- Close resource-intensive programs

### `envtest` startup failure:
```bash
# Verify setup-envtest installation
setup-envtest list
setup-envtest use -p path
```
