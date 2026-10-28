package benchutil

import (
	"context"
	"fmt"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	// NewObjects is an optional factory that creates the objects to reconcile.
	// Receives *testing.B (for b.N access when creating per-iteration unique
	// objects) and the isolated namespace. When nil, Reconcile receives a nil
	// slice.
	NewObjects func(b *testing.B, ns string) []client.Object

	// FixedSet marks NewObjects as returning a fixed set of objects that is
	// applied on every iteration, rather than b.N uniquely-named objects (see
	// SingleTemplate for the latter). In FirstApply runs the harness deletes
	// the set between iterations -- via the uncounted SeedClient, outside the
	// measured time -- so every iteration measures a true first apply.
	// Without this, only iteration 0 is a first apply and the remaining
	// iterations dilute the result toward steady state.
	FixedSet bool

	// Setup creates per-benchmark state. Must return a Resettable (satisfied by
	// RTCounter, BenchRemoteClient, and HelperState). Custom seeding goes here.
	Setup func(b *testing.B, env *BenchEnv) S

	// Reconcile runs one iteration. objects is the slice returned by NewObjects (nil if NewObjects is nil).
	// When NewObjects returns b.N uniquely-named objects (e.g. via SingleTemplate), use objects[i].
	// When it returns a fixed set to apply every iteration, iterate over the whole slice and ignore i.
	Reconcile func(b *testing.B, state S, objects []client.Object, i int)

	// SteadyState, when true, additionally runs the benchmark with all objects
	// pre-seeded, under a SteadyState sub-benchmark.
	SteadyState bool
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
		for _, obj := range CopyAndSetNamespace(env.Namespace, objects...) {
			if err := env.SeedClient.Create(context.Background(), obj); err != nil {
				b.Fatalf("seed failed: %v", err)
			}
		}
	}
	state := h.Setup(b, env)

	b.ReportAllocs()
	state.ResetAll(b)
	for i := 0; i < b.N; i++ {
		h.Reconcile(b, state, objects, i)
		if !seed && h.FixedSet {
			h.deleteObjects(b, env, objects)
		}
	}
	state.Report(b)
}

// deleteObjects removes a fixed object set between FirstApply iterations so
// the next iteration is a true first apply. Uses the uncounted SeedClient and
// pauses the benchmark timer, so neither the round trips nor the wall time of
// the cleanup show up in the reported metrics.
func (h ControllerHarness[S]) deleteObjects(b *testing.B, env *BenchEnv, objects []client.Object) {
	b.Helper()
	b.StopTimer()
	defer b.StartTimer()
	for _, obj := range CopyAndSetNamespace(env.Namespace, objects...) {
		if err := env.SeedClient.Delete(context.Background(), obj); err != nil && !apierrors.IsNotFound(err) {
			b.Fatalf("inter-iteration cleanup failed for %s (%s): %v", obj.GetName(), obj.GetObjectKind().GroupVersionKind(), err)
		}
	}
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
			obj := CopyAndSetNamespace(ns, template)[0]
			obj.SetName(fmt.Sprintf("%s-%d", template.GetName(), i))
			objects[i] = obj
		}
		return objects
	}
}
