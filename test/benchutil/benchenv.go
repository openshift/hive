package benchutil

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
)

// BenchEnv bundles config, client, ClusterDeployment, and namespace for a benchmark.
type BenchEnv struct {
	// Cfg is the base REST config for the benchmark cluster (envtest, or the
	// cluster referenced by KubeconfigEnvVar).
	Cfg *rest.Config

	// SeedClient is a shared controller-runtime client to the benchmark
	// cluster. It should be used to seed data necessary for the benchmarks. It
	// should NOT be used in the actual benchmarks: its round trips are not
	// counted, which makes it ideal for out-of-band setup and cleanup.
	SeedClient client.Client

	// CD is an in-memory ClusterDeployment with a kubeconfig Secret pointing
	// at the benchmark cluster. Only the Secret is created.
	CD *hivev1.ClusterDeployment

	// Namespace is a unique namespace for benchmark isolation.
	Namespace string
}

// NewBenchEnv creates a fully initialized benchmark environment.
func NewBenchEnv(b *testing.B) *BenchEnv {
	b.Helper()
	cfg, kubeconfigBytes := EnsureCluster(b)
	ns := BenchNamespace(b, cfg)

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
