# Hive Resource/RemoteClient Benchmarks

Performance benchmarks for measuring the impact of optimizations to Hive's
legacy `pkg/resource` (`Helper`) and `pkg/remoteclient` packages.

## Context

The clustersync controller and others that depend on these packages have been
repeatedly flagged by downstream users for slow reconciliation, memory leaks,
and excessive API server load. These packages are legacy code with behavioral
dependencies that prevent rewrites.

These benchmarks enable targeted optimizations by:
1. Identifying hot paths through round-trip and allocation measurement
2. Proving performance improvements via A/B comparison
3. Detecting behavioral changes - unexpected round-trip count changes indicate
   logic changes, not just performance changes

## Quick Start

```bash
# One-time tool setup
make install-tools

# Run all benchmarks (envtest binaries are resolved automatically)
make benchmark

# Or customize:
BENCH_ARGS="-bench=BenchmarkApply -benchtime=10x" make benchmark

# Compare before/after a change
./hack/benchmark-comparison.sh [base-commit]
```

The manual equivalent of `make benchmark`, if you prefer to manage the envtest
binaries yourself:

```bash
make setup-envtest
export KUBEBUILDER_ASSETS="$(hack/setup-envtest.sh -q)"
go test -p 1 -run '^$' -bench=. -benchmem -benchtime=5x \
    ./pkg/remoteclient/ ./pkg/resource/ ./test/benchmark/
```

## Automation Status

Nothing runs these suites automatically: there is no CI job or cron for
them, they are not part of `make test`, and no results are collected over
time. They are manual tooling for the
[measure → optimize → compare](#validating-optimizations) loop.

CI does cover one slice incidentally: `make test` compiles the benchmark
files in `pkg/resource/` and `pkg/remoteclient/` (they are plain `_test.go`
files in unit-tested packages), so breakage there fails the build -- and
envtest starts lazily on first benchmark execution, so unit-test runs are
unaffected. The `test/benchmark/` package, however, is outside `make
test`'s package list entirely: a refactor that breaks it, or that changes
what the benchmarks measure, will not be caught by CI. Run `make benchmark`
after touching the harness or the `pkg/resource`/`pkg/remoteclient` code it
exercises.

## Running Against a Real Cluster

By default the benchmarks run against a local envtest instance. To run them
against an existing cluster instead:

```bash
BENCHMARK_KUBECONFIG=/path/to/kubeconfig make benchmark
```

Everything else works the same: the harness creates (and deletes) `bench-*`
namespaces for isolation, and the ClusterDeployment's admin-kubeconfig Secret
is populated from the referenced kubeconfig.

### CRD requirements

CRDs are **not** installed for you. The benchmark's generated objects are
validated against the real CRD schemas -- the Machine/MachineSet CRDs from
[machine-api-operator](https://github.com/openshift/machine-api-operator/tree/master/install)
and the ClusterVersion/ClusterOperator CRDs from
[openshift/api](https://github.com/openshift/api/tree/master/config/v1/zz_generated.featuregated-crd-manifests)
(the Hive types use the checked-in CRDs from `config/crds/` even under
envtest) -- so passing schema validation is not a concern; the cluster just
has to serve the types:

| Suite               | CRDs required                                                       |
|---------------------|---------------------------------------------------------------------|
| `pkg/remoteclient/` | none (Secrets only)                                                  |
| `pkg/resource/`     | MachineSet (`machine.openshift.io`)                                  |
| `test/benchmark/`   | SyncSet (Hive), Machine + MachineSet, ClusterVersion + ClusterOperator, Infrastructure (`config.openshift.io`) |

Caveats:
- Schema-valid does not mean webhook-valid: on a full OpenShift cluster, the
  machine-api admission webhooks may still reject the synthetic MachineSets
  (they carry no provider config). If a benchmark fails with an admission
  error, filter it out of real-cluster runs via `BENCH_ARGS`
  (e.g. `-bench='BenchmarkApply/ByType/(ConfigMap|Secret|Deployment|ServiceAccount)'`).
- Benchmarks that list cluster-scoped resources (Nodes, ClusterOperators,
  CSRs) see the real objects on the cluster. They are read-only, but the
  results scale with cluster size.
- The Machine Lists in the hibernation and machinepool simulations are scoped
  to the benchmark namespace so response sizes stay deterministic under
  envtest (where namespace deletion never completes); on a real cluster they
  therefore list only the seeded synthetic Machines, not the cluster's real
  ones.
- No cluster-scoped objects are created in real-cluster mode. The
  clusterversion simulation reads the `version` ClusterVersion and the
  machinepool simulation reads the `cluster` Infrastructure; both must
  already exist on a real cluster (envtest seeds them). Writes are confined
  to the `bench-*` namespaces, which are deleted afterwards. Do not point
  this at a production cluster.

### Real CRD schemas under envtest

Without a cluster at hand you can still run envtest with the **real** OpenShift
CRD schemas instead of the minimal synthesized ones -- worth doing when you
care about patch-computation fidelity, since the Helper's strategic-merge
logic is driven by the OpenAPI schema the server serves:

```bash
mkdir -p real-crds && cd real-crds
curl -sLO https://raw.githubusercontent.com/openshift/machine-api-operator/master/install/0000_30_machine-api-operator_02_machine.Default.crd.yaml
curl -sLO https://raw.githubusercontent.com/openshift/machine-api-operator/master/install/0000_30_machine-api-operator_03_machineset.Default.crd.yaml
# The config.openshift.io manifests carry a templated feature-gate annotation
# with an empty value, which CRD validation rejects -- strip it on the way in.
curl -sL https://raw.githubusercontent.com/openshift/api/master/config/v1/zz_generated.featuregated-crd-manifests/clusterversions.config.openshift.io/AAA_ungated.yaml \
  | grep -v 'feature-gate.release.openshift.io' > clusterversion-crd.yaml
curl -sL https://raw.githubusercontent.com/openshift/api/master/config/v1/zz_generated.featuregated-crd-manifests/clusteroperators.config.openshift.io/AAA_ungated.yaml \
  | grep -v 'feature-gate.release.openshift.io' > clusteroperator-crd.yaml
cd ..
BENCH_CRD_DIR=real-crds make benchmark
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

### Measurement accuracy

The API server is real (a real kube-apiserver + etcd via envtest, or your
target cluster), so round-trip **counts** and **byte** figures are exact. But
envtest is local: wall-clock times include no meaningful network latency. Use
envtest results to compare relative costs (allocations, round trips, local
compute); use real-cluster mode when you need realistic latency in the
picture.

The suites also run sequentially (`go test -p 1`): each package starts its
own envtest instance (an extra kube-apiserver + etcd per suite), and running
them concurrently would contaminate wall-time numbers with CPU contention.

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

The comparison script uses [benchstat](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat)
(installed by `make install-tools`) to compare the runs. benchstat reports
per-metric statistics and flags significant deltas: look for changes in the
`roundtrips/op` column first -- those indicate behavioral changes -- then at
allocations and time. Without benchstat the raw results are still saved for
manual comparison.

NB: the base commit must itself contain the benchmark suites; comparing
against a commit that predates them fails fast with an error.

## Architecture

Benchmarks run against a real kube-apiserver + etcd (via controller-runtime's
envtest) by default, or an existing cluster in real-cluster mode. Each
benchmark uses `benchutil.ControllerHarness` -- a generic primitive that
handles environment setup, object seeding, counter management, and metric
reporting.

The harness touches production code in exactly one place: the optional
`RESTConfigOption` plumbing on `remoteclient.NewBuilder` and
`NewBuilderFromKubeconfig`. It is variadic and inert when no options are
passed -- existing callers are unaffected -- and exists so the benchmarks
can inject round-trip counting into the real builder path instead of
measuring a reimplementation of it.

### Simulations, not the real controllers

The `test/benchmark/` benchmarks simulate the *patterns* the controllers
follow (client construction, apply loops, reads and writes) rather than
invoking the controllers' `Reconcile()` functions. Real reconcile loops drag
in manager wiring, metrics, and event recorders that are irrelevant to what
these benchmarks measure -- the `pkg/resource` and `pkg/remoteclient` hot
paths -- but the simulations are maintained by hand, so they can drift from
the controllers they model. Driving the real `Reconcile()` methods against
the benchmark cluster (in a loop, until the reconciled state settles) is a
possible future extension of the harness.

For detailed internals (cluster lifecycle, harness field semantics,
round-trip counting, namespace isolation, the full benchmark catalog, and
measurement caveats) see:

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
export KUBEBUILDER_ASSETS="$(hack/setup-envtest.sh -q)"
```
...or just use `make benchmark`, which resolves the assets for you.

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
