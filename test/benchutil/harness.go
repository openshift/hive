package benchutil

import (
	"context"
	"fmt"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/hive/pkg/resource"
)

// Resettable is the counter interface for ControllerHarness.
type Resettable interface {
	ResetAll(b *testing.B)
	Report(b *testing.B)
}

// HelperState bundles an RTCounter and a shared Helper for local-only benchmarks.
type HelperState struct {
	*RTCounter
	Helper resource.Helper
}

// ControllerHarness is the central benchmark primitive. See README for details.
type ControllerHarness[S Resettable] struct {
	NewObjects  func(b *testing.B, ns string) []client.Object               // optional; creates objects to reconcile
	Setup       func(b *testing.B, env *BenchEnv) S                         // creates per-benchmark state
	// Reconcile runs one iteration. objects is the slice returned by NewObjects (nil if NewObjects is nil).
	// When NewObjects returns b.N uniquely-named objects (e.g. via SingleTemplate), use objects[i].
	// When NewObjects returns a fixed set to apply every iteration, iterate over the whole slice and ignore i.
	Reconcile   func(b *testing.B, state S, objects []client.Object, i int)
	SteadyState bool // when true, also run with pre-seeded objects
}

// Run executes the benchmark.
func (h ControllerHarness[S]) Run(b *testing.B) {
	b.Helper()
	if h.SteadyState {
		b.Run("FirstApply", func(b *testing.B) { h.run(b, false) })
		b.Run("SteadyState", func(b *testing.B) { h.run(b, true) })
	} else {
		h.run(b, false)
	}
}

func (h ControllerHarness[S]) run(b *testing.B, seed bool) {
	b.Helper()
	env := NewBenchEnv(b)
	var objects []client.Object
	if h.NewObjects != nil {
		objects = h.NewObjects(b, env.Namespace)
	}
	if seed {
		for _, obj := range objects {
			seedCopy := CopyAndSetNamespace(obj, env.Namespace)
			if err := env.SeedClient.Create(context.Background(), seedCopy); err != nil {
				b.Fatalf("seed failed: %v", err)
			}
		}
	}
	state := h.Setup(b, env)

	b.ReportAllocs()
	state.ResetAll(b)
	for i := 0; i < b.N; i++ {
		h.Reconcile(b, state, objects, i)
	}
	state.Report(b)
}

// SetupLocalHelper creates a shared Helper with a single RTCounter.
func SetupLocalHelper(b *testing.B, env *BenchEnv) *HelperState {
	rc := TrackRoundTrips(env.Cfg)
	return &HelperState{
		RTCounter: rc,
		Helper:    BenchHelper(b, rc.Cfg, "benchmark"),
	}
}

// SetupRemoteClient creates a BenchRemoteClient with dual counters.
func SetupRemoteClient(b *testing.B, env *BenchEnv) *BenchRemoteClient {
	return NewBenchRemoteClient(b, env, "benchmark")
}

// SingleTemplate returns a NewObjects function producing b.N uniquely-named copies.
func SingleTemplate(template client.Object) func(b *testing.B, ns string) []client.Object {
	return func(b *testing.B, ns string) []client.Object {
		objects := make([]client.Object, b.N)
		for i := range objects {
			obj := CopyAndSetNamespace(template, ns)
			obj.SetName(fmt.Sprintf("%s-%d", template.GetName(), i))
			objects[i] = obj
		}
		return objects
	}
}
