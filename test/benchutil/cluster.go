package benchutil

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	log "github.com/sirupsen/logrus"
)

// KubeconfigEnvVar names the environment variable that, when set, runs the
// benchmarks against an existing cluster instead of a local envtest instance.
// Its value must be the path to a kubeconfig file for that cluster.
const KubeconfigEnvVar = "BENCHMARK_KUBECONFIG"

// RealClusterMode reports whether the benchmarks run against an existing
// cluster (KubeconfigEnvVar set) rather than a local envtest instance.
func RealClusterMode() bool {
	return os.Getenv(KubeconfigEnvVar) != ""
}

var (
	clusterOnce    sync.Once
	clusterCfg     *rest.Config
	clusterKubeCfg []byte        // kubeconfig bytes to embed in the CD's admin-kubeconfig Secret
	seedClient     client.Client // initialized in clusterOnce; see newClient in client.go
)

// EnsureCluster returns the REST config for the benchmark cluster, starting
// envtest on first use. The cluster is a real kube-apiserver + etcd (envtest)
// unless KubeconfigEnvVar is set, in which case the referenced cluster is used
// and envtest is never started.
//
// The second return value is the kubeconfig (in bytes) that the benchmark
// environment embeds in the ClusterDeployment's admin-kubeconfig Secret, so
// remoteclient builders connect to the same cluster the seed client uses.
func EnsureCluster(b *testing.B) (*rest.Config, []byte) {
	b.Helper()
	clusterOnce.Do(func() {
		log.SetLevel(log.ErrorLevel)
		if RealClusterMode() {
			clusterCfg, clusterKubeCfg, seedClient = loadRealCluster(b, os.Getenv(KubeconfigEnvVar))
			return
		}
		clusterCfg, clusterKubeCfg, seedClient = startEnvtest(b)
	})
	// A failed start exits its benchmark via b.Fatalf, but sync.Once is still
	// marked done -- every subsequent benchmark would otherwise see a nil
	// config and panic. Fail them cleanly instead.
	if clusterCfg == nil {
		b.Fatalf("benchmark cluster was never started (see the first failure above)")
	}
	return clusterCfg, clusterKubeCfg
}

// loadRealCluster connects to the cluster referenced by the kubeconfig at path.
// CRDs are NOT installed: benchmarks that need Hive or OpenShift CRDs require
// a cluster that already serves them (e.g. an OpenShift cluster with Hive
// installed). See test/benchmark/README.md.
func loadRealCluster(b *testing.B, path string) (*rest.Config, []byte, client.Client) {
	b.Helper()
	kubeconfigBytes, err := os.ReadFile(path)
	if err != nil {
		b.Fatalf("failed to read %s=%s: %v", KubeconfigEnvVar, path, err)
	}
	cfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfigBytes)
	if err != nil {
		b.Fatalf("failed to parse kubeconfig %s: %v", path, err)
	}
	seed, err := newClient(cfg)
	if err != nil {
		b.Fatalf("failed to create seed client for %s: %v", path, err)
	}
	fmt.Fprintf(os.Stderr, "benchutil: using real cluster from %s (KUBEBUILDER_ASSETS ignored)\n", path)
	return cfg, kubeconfigBytes, seed
}
