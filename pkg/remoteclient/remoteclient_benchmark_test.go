package remoteclient_test

import (
	"testing"

	"github.com/openshift/hive/test/benchutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func benchBuilderOp(b *testing.B, fn func(*testing.B, *benchutil.BenchRemoteClient)) {
	benchutil.ControllerHarness[*benchutil.BenchRemoteClient]{
		Setup: benchutil.SetupRemoteClient,
		Reconcile: func(b *testing.B, brc *benchutil.BenchRemoteClient, _ []client.Object, _ int) {
			fn(b, brc)
		},
	}.Run(b)
}

// BenchmarkBuilderBuild measures Build: secret read + parse + discovery + client creation.
func BenchmarkBuilderBuild(b *testing.B) {
	benchBuilderOp(b, func(b *testing.B, brc *benchutil.BenchRemoteClient) {
		if _, err := brc.NewBuilder().Build(); err != nil {
			b.Fatalf("Build failed: %v", err)
		}
	})
}

// BenchmarkBuilderRESTConfig measures secret read + parse (no remote calls).
func BenchmarkBuilderRESTConfig(b *testing.B) {
	benchBuilderOp(b, func(b *testing.B, brc *benchutil.BenchRemoteClient) {
		if _, err := brc.NewBuilder().RESTConfig(); err != nil {
			b.Fatalf("RESTConfig failed: %v", err)
		}
	})
}

// BenchmarkBuilderBuildDynamic measures BuildDynamic (no discovery).
func BenchmarkBuilderBuildDynamic(b *testing.B) {
	benchBuilderOp(b, func(b *testing.B, brc *benchutil.BenchRemoteClient) {
		if _, err := brc.NewBuilder().BuildDynamic(); err != nil {
			b.Fatalf("BuildDynamic failed: %v", err)
		}
	})
}

// BenchmarkBuilderBuildKubeClient measures BuildKubeClient (no discovery).
func BenchmarkBuilderBuildKubeClient(b *testing.B) {
	benchBuilderOp(b, func(b *testing.B, brc *benchutil.BenchRemoteClient) {
		if _, err := brc.NewBuilder().BuildKubeClient(); err != nil {
			b.Fatalf("BuildKubeClient failed: %v", err)
		}
	})
}
