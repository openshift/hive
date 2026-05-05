// Package benchutil provides shared infrastructure for Hive's benchmark suites.
//
// Three packages use this infrastructure:
//
//   - pkg/remoteclient/  -- Builder operations (RESTConfig, Build, BuildDynamic, BuildKubeClient)
//   - pkg/resource/      -- Isolated Helper operations (Apply, CreateOrUpdate, Create, Delete, Patch, NewHelper)
//   - test/benchmark/    -- Full-stack controller simulations (remoteclient + Helper)
//
// See test/benchmark/README.md for quick-start instructions and the user-facing guide.
// This doc covers the internals.
//
// # envtest
//
// Benchmarks run against a real Kubernetes API server (etcd + kube-apiserver) via
// controller-runtime's envtest. Apply, Patch, and Delete go through real API server
// validation, storage, and admission -- not mocks. There is no kubelet or controller
// manager; only the API server and etcd are running.
//
// Hive CRDs are loaded from config/crds/. OpenShift platform CRDs (ClusterVersion,
// ClusterOperator, Machine, MachineSet) are built at runtime as minimal
// CustomResourceDefinition objects with x-kubernetes-preserve-unknown-fields -- no
// checked-in YAML or controller-gen dependency needed. See [openshiftCRDs] in envtest.go.
//
// Each benchmark package gets its own envtest instance. Within a package, a single
// instance is shared across all benchmarks. It is started lazily on the first call to
// [EnsureEnvTest] (via sync.Once) and torn down in that package's TestMain. This means:
//
//   - Running only unit tests (e.g. go test -run TestFoo ./pkg/resource/) never starts envtest.
//   - Running any benchmark starts envtest exactly once per package, regardless of how many benchmarks execute.
//   - API server state accumulates across benchmarks -- namespace isolation (below) prevents interference.
//
// # ControllerHarness
//
// [ControllerHarness] is the central benchmark primitive. Every benchmark across all
// three packages uses it. It handles environment creation, object seeding, counter
// management, b.ReportAllocs(), timer reset, the benchmark loop, and metric reporting --
// leaving each benchmark to define only what varies.
//
// Fields:
//
//   - NewObjects -- optional factory that creates the objects to reconcile. Receives
//     *testing.B (for b.N access when creating per-iteration unique objects) and the
//     isolated namespace. When nil, Reconcile receives a nil slice.
//
//   - Setup -- creates per-sub-benchmark state. Must return a [Resettable] (satisfied by
//     [RTCounter], [BenchRemoteClient], and [HelperState]). Custom seeding goes here.
//
//   - Reconcile -- runs one iteration. Receives typed state, the full object list, and
//     the iteration index. When NewObjects returns b.N uniquely-named objects (e.g. via
//     [SingleTemplate]), use objects[i]. When it returns a fixed set to apply every
//     iteration, iterate over the whole slice and ignore i.
//
//   - SteadyState -- when false (default), Run executes at the current benchmark level
//     with FirstApply semantics (objects do not exist). When true, Run creates FirstApply
//     and SteadyState sub-benchmarks, where SteadyState pre-seeds all objects via
//     SeedClient.Create before measurement.
//
// Common Setup functions:
//
//   - [SetupLocalHelper]  -- shared Helper + single RTCounter (local-only benchmarks)
//   - [SetupRemoteClient] -- dual counters for remoteclient benchmarks
//   - [SingleTemplate]    -- NewObjects factory producing b.N uniquely-named copies of a template
//
// # BenchEnv and namespace isolation
//
// [NewBenchEnv] creates a fully initialized environment in one call: starts envtest (if
// not already running), creates an isolated namespace (bench-1, bench-2, ... via atomic
// counter), builds a shared controller-runtime client (for resource seeding), and sets up
// a kubeconfig Secret + in-memory ClusterDeployment for remoteclient use.
//
// The ClusterDeployment and its associated kubeconfig Secret point back at the envtest
// API server, so the same server acts as both the "management" cluster (where Secrets
// live) and the "remote" cluster (where resources are applied).
//
// # Round-trip and byte counting
//
// Every benchmark reports custom metrics tracking HTTP round trips and bytes transferred
// via [RTCounter]. Both RTCounter and [BenchRemoteClient] satisfy the [Resettable]
// interface used by the harness.
//
// BenchRemoteClient wraps two RTCounter instances -- one for management cluster API calls
// (kubeconfig Secret reads) and one for remote cluster calls (discovery, Apply, Patch,
// Delete). These appear as local-roundtrips/op, remote-roundtrips/op, etc. in benchmark
// output.
//
// # What the benchmarks don't measure
//
// Prometheus metrics transport: In production, some controllers create their
// resource.Helper with resource.WithMetrics(), which wraps the HTTP transport with a
// Prometheus counter. The benchmarks omit this wrapper, but the overhead is negligible
// (an atomic counter increment per round trip).
//
// Controller metrics transport: remoteclient.Builder.RESTConfig() injects a controller
// metrics transport via AddControllerMetricsTransportWrapper. This IS present in the
// benchmarks (the real remoteclient code runs), so remote round trips include this
// overhead.
//
// Discovery disk cache: resource.Helper uses a disk-cached discovery client with a
// 10-minute TTL. The first iteration of a benchmark warms this cache; subsequent
// iterations benefit from it. This matches production behavior (the cache persists across
// reconciles), but the reported roundtrips/op is an amortized average -- the first
// iteration has more HTTP round trips than later ones. For small b.N values, per-op
// costs will skew higher.
package benchutil
