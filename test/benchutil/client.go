package benchutil

import (
	"fmt"
	"testing"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/remoteclient"
	hivescheme "github.com/openshift/hive/pkg/util/scheme"
)

// BenchRemoteClient bundles dual local/remote round trip counting with builder creation.
type BenchRemoteClient struct {
	env            *BenchEnv
	controllerName hivev1.ControllerName
	localRTs       *RTCounter
	localClient    client.Client
	remoteRTs      *RTCounter
}

// NewBenchRemoteClient creates dual counters and a counting local client.
func NewBenchRemoteClient(b *testing.B, env *BenchEnv, controllerName string) *BenchRemoteClient {
	b.Helper()
	localRTs := TrackRoundTrips(env.Cfg)
	localClient, err := newClient(localRTs.Cfg)
	if err != nil {
		b.Fatalf("failed to create local client: %v", err)
	}
	return &BenchRemoteClient{
		env:            env,
		controllerName: hivev1.ControllerName(controllerName),
		localRTs:       localRTs,
		localClient:    localClient,
		remoteRTs:      NewRTCounter(),
	}
}

// NewBuilder creates a Builder with the remote counter wired in.
func (brc *BenchRemoteClient) NewBuilder() remoteclient.Builder {
	return remoteclient.NewBuilder(brc.localClient, brc.env.CD, brc.controllerName,
		remoteclient.WithTransportWrapper(brc.remoteRTs.WrapTransport))
}

// ResetAll zeros both counters and resets the benchmark timer.
func (brc *BenchRemoteClient) ResetAll(b *testing.B) {
	brc.localRTs.reset()
	brc.remoteRTs.reset()
	b.ResetTimer()
}

// Report emits local and remote metrics.
func (brc *BenchRemoteClient) Report(b *testing.B) {
	brc.localRTs.ReportAs(b, "local")
	brc.remoteRTs.ReportAs(b, "remote")
}

func newClient(cfg *rest.Config) (client.Client, error) {
	httpClient, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP client: %w", err)
	}
	mapper, err := apiutil.NewDynamicRESTMapper(cfg, httpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create REST mapper: %w", err)
	}
	return client.New(cfg, client.Options{
		Scheme: hivescheme.GetScheme(),
		Mapper: mapper,
	})
}
