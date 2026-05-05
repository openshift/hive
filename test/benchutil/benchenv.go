package benchutil

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
)

// BenchEnv bundles config, client, ClusterDeployment, and namespace for a benchmark.
type BenchEnv struct {
	// Cfg is the base envtest REST config.
	Cfg *rest.Config

	// SeedClient is a shared controller-runtime client to the envtest cluster.
	// It should be used to seed data necessary for the benchmarks. It should NOT
	// be used in the actual benchmarks.
	SeedClient client.Client

	// CD is an in-memory ClusterDeployment with a kubeconfig Secret
	// pointing at the envtest cluster. Only the Secret is created.
	CD *hivev1.ClusterDeployment

	// Namespace is a unique namespace for benchmark isolation.
	Namespace string
}

// NewBenchEnv creates a fully initialized benchmark environment.
func NewBenchEnv(b *testing.B) *BenchEnv {
	b.Helper()
	cfg := EnsureEnvTest(b)
	ns := BenchNamespace(b, cfg)

	// Build a kubeconfig Secret pointing at the envtest API server.
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

	secretName := "admin-kubeconfig"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: ns},
		Data: map[string][]byte{
			constants.KubeconfigSecretKey: kubeconfigBytes,
		},
	}
	if err := seedClient.Create(context.Background(), secret); err != nil {
		b.Fatalf("failed to create kubeconfig secret: %v", err)
	}

	cd := &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bench-cluster",
			Namespace: ns,
		},
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterMetadata: &hivev1.ClusterMetadata{
				AdminKubeconfigSecretRef: corev1.LocalObjectReference{
					Name: secretName,
				},
			},
		},
	}

	return &BenchEnv{
		Cfg:        cfg,
		SeedClient: seedClient,
		CD:         cd,
		Namespace:  ns,
	}
}
