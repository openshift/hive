package benchutil

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/go-logr/logr"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	testEnv        *envtest.Environment
	envtestStarted bool
)

// CRDEnvVar names the environment variable that, when set to a directory of
// real OpenShift CRD manifests, installs those instead of the minimal
// synthesized OpenShift CRDs in envtest mode. No effect in real-cluster mode
// (KubeconfigEnvVar), where nothing is installed.
const CRDEnvVar = "BENCH_CRD_DIR"

// startEnvtest launches a shared envtest environment (real kube-apiserver +
// etcd) and returns its REST config plus kubeconfig bytes referencing it.
// Torn down via StopEnvTest.
func startEnvtest(b *testing.B) (*rest.Config, []byte, client.Client) {
	b.Helper()

	// Verify KUBEBUILDER_ASSETS is set before starting envtest
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		b.Fatalf(`KUBEBUILDER_ASSETS environment variable not set

Benchmarks require envtest binaries (kube-apiserver, etcd).

To set up:
  $ make setup-envtest

Then export the path shown, or simply run the suites via:
  $ make benchmark

Alternatively, run against an existing cluster:
  $ BENCHMARK_KUBECONFIG=/path/to/kubeconfig make benchmark

See test/benchmark/README.md for details.`)
	}

	logf.SetLogger(logr.New(logr.Discard().GetSink()))
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: hiveCRDPaths(b),
		CRDs:              openshiftCRDs(),
	}
	// Optionally swap the minimal synthesized OpenShift CRDs for real ones:
	// the real schemas change what the Helper's OpenAPI-driven patch
	// computation sees, so runs with them are closer to production behavior.
	// The directory must provide Machine, MachineSet, ClusterVersion,
	// ClusterOperator and Infrastructure CRDs -- see test/benchmark/README.md
	// for sources.
	if dir := os.Getenv(CRDEnvVar); dir != "" {
		if _, err := os.Stat(dir); err != nil {
			b.Fatalf("%s=%s not usable: %v", CRDEnvVar, dir, err)
		}
		testEnv.CRDDirectoryPaths = append(testEnv.CRDDirectoryPaths, dir)
		testEnv.CRDs = nil // the real CRDs replace the minimal synthesized ones
	}
	cfg, err := testEnv.Start()
	if err != nil {
		b.Fatalf("failed to start test environment: %v", err)
	}
	envtestStarted = true

	seed, err := newClient(cfg)
	if err != nil {
		b.Fatalf("failed to create seed client: %v", err)
	}

	// Build a kubeconfig referencing the envtest API server, for the CD's
	// admin-kubeconfig Secret.
	kubeconfig := clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"envtest": {
				Server:                   cfg.Host,
				CertificateAuthorityData: cfg.CAData,
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"envtest": {
				ClientCertificateData: cfg.CertData,
				ClientKeyData:         cfg.KeyData,
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"envtest": {Cluster: "envtest", AuthInfo: "envtest"},
		},
		CurrentContext: "envtest",
	}
	kubeconfigBytes, err := clientcmd.Write(kubeconfig)
	if err != nil {
		b.Fatalf("failed to serialize kubeconfig: %v", err)
	}
	return cfg, kubeconfigBytes, seed
}

// StopEnvTest tears down the shared envtest environment. No-op when the
// benchmarks run against a real cluster (KubeconfigEnvVar) or when envtest
// never started.
func StopEnvTest() {
	if testEnv != nil && envtestStarted {
		if err := testEnv.Stop(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to stop test environment: %v\n", err)
		}
	}
}

// hiveCRDPaths returns the path to Hive's CRD manifests, located relative to
// the repository root (this package lives in test/benchutil).
//
// NB: runtime.Caller yields the compile-time source path, which plain `go
// test` preserves but -trimpath builds rewrite; the benchmarks are not
// expected to run trimpathed.
func hiveCRDPaths(b *testing.B) []string {
	b.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		b.Fatalf("failed to determine source path for CRD lookup")
	}
	crdDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "config", "crds")
	if _, err := os.Stat(crdDir); err != nil {
		b.Fatalf("CRD directory not found at %s: %v", crdDir, err)
	}
	return []string{crdDir}
}
