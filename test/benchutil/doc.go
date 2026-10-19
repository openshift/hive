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
// # The benchmark cluster: envtest or real
//
// Benchmarks run against a real Kubernetes API server (etcd + kube-apiserver)
// via controller-runtime's envtest. Apply, Patch, and Delete go through real
// API server validation, storage, and admission -- not mocks. There is no
// kubelet or controller manager; only the API server and etcd are running.
//
// Setting the [KubeconfigEnvVar] environment variable (a path to a kubeconfig
// file) switches the whole suite to an existing cluster instead; envtest never
// starts. That mode measures real network latency, but the cluster must
// already serve the CRDs the benchmarks touch (see the requirements table in
// test/benchmark/README.md), and nothing is installed on your behalf. The
// generated objects satisfy the real CRD schemas (verified against the
// machine-api-operator and openshift/api CRD manifests), though full OpenShift
// clusters may still reject synthetic Machine/MachineSet objects at the
// machine-api admission webhooks, which schema validation cannot speak to.
// Writes stay confined to the per-benchmark namespaces.
//
// Either way, [EnsureCluster] lazily initializes the shared cluster (via
// sync.Once) and returns its REST config plus the kubeconfig bytes that
// [NewBenchEnv] embeds in the ClusterDeployment's admin-kubeconfig Secret --
// so the "management" cluster (where Secrets live) and the "remote" cluster
// (where resources are applied) are one and the same. Envtest is torn down in
// that package's TestMain via [StopEnvTest] (a no-op in real-cluster mode).
//
// In envtest mode, Hive CRDs are loaded from config/crds/. OpenShift platform
// CRDs (ClusterVersion, ClusterOperator, Infrastructure, Machine, MachineSet)
// are built at
// runtime as minimal CustomResourceDefinition objects with
// x-kubernetes-preserve-unknown-fields -- no checked-in YAML or controller-gen
// dependency needed. See [openshiftCRDs] in crds.go. Setting [CRDEnvVar] to a
// directory of real OpenShift CRD manifests installs those instead, which
// exercises the real schemas (and the OpenAPI-driven patch computation that
// depends on them) without needing a cluster.
//
// Each benchmark package gets its own envtest instance. Within a package, a
// single instance is shared across all benchmarks. This means:
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
//   - FixedSet -- marks NewObjects as returning a fixed set applied every
//     iteration, rather than b.N uniquely-named objects. In FirstApply runs the
//     harness deletes the set between iterations (via the uncounted SeedClient,
//     with the timer paused) so every iteration measures a true first apply.
//     Without it, only iteration 0 is a first apply and the remaining iterations
//     dilute the result toward steady state.
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
// [NewBenchEnv] creates a fully initialized environment in one call: starts the
// benchmark cluster (if not already running), creates an isolated namespace
// (bench-<prefix>-1, bench-<prefix>-2, ... via a process-unique prefix and an
// atomic counter, so names never collide with earlier runs' namespaces that may
// still be Terminating; deleted via b.Cleanup when the benchmark completes),
// builds a shared controller-runtime client (for resource
// seeding), and sets up a kubeconfig Secret + in-memory ClusterDeployment for
// remoteclient use.
//
// The ClusterDeployment and its associated kubeconfig Secret point back at the
// benchmark cluster, so the same server acts as both the "management" cluster
// (where Secrets live) and the "remote" cluster (where resources are applied).
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
// output. The local counter also sees the local client's occasional REST mapper
// re-discovery, so local-roundtrips/op can exceed the single Secret read a reconcile
// implies.
//
// Traffic through the SeedClient and the namespace-management clientset is
// deliberately NOT counted: they exist for out-of-band setup and cleanup, and
// the harness relies on that for inter-iteration cleanup (see FixedSet above).
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
