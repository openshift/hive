package resource_test

import (
	"context"
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/hive/pkg/resource"
	"github.com/openshift/hive/test/benchutil"
)

func benchHelperOp(b *testing.B, template client.Object, fn func(*testing.B, resource.Helper, client.Object)) {
	benchutil.ControllerHarness[*benchutil.HelperState]{
		NewObjects: benchutil.SingleTemplate(template),
		Setup:      benchutil.SetupLocalHelper,
		Reconcile: func(b *testing.B, s *benchutil.HelperState, objects []client.Object, i int) {
			fn(b, s.Helper, objects[i])
		},
		SteadyState: true,
	}.Run(b)
}

// BenchmarkNewHelper measures Helper construction cost.
func BenchmarkNewHelper(b *testing.B) {
	benchutil.ControllerHarness[*benchutil.HelperState]{
		Setup: benchutil.SetupLocalHelper,
		Reconcile: func(b *testing.B, s *benchutil.HelperState, _ []client.Object, _ int) {
			benchutil.BenchHelper(b, s.RTCounter.Cfg, "benchmark")
		},
	}.Run(b)
}

// BenchmarkApply benchmarks Apply by resource type and payload size.
func BenchmarkApply(b *testing.B) {
	applyFn := func(b *testing.B, helper resource.Helper, obj client.Object) {
		serialized := benchutil.MustSerialize(obj)
		if _, err := helper.Apply(serialized); err != nil {
			b.Fatalf("apply failed: %v", err)
		}
	}

	b.Run("ByType", func(b *testing.B) {
		resources := []struct {
			name string
			obj  client.Object
		}{
			{"ConfigMap", benchutil.GenerateConfigMap("apply-cm", 0)},
			{"Secret", benchutil.GenerateSecret("apply-secret", 0)},
			{"Deployment", benchutil.GenerateDeployment("apply-deploy")},
			{"ServiceAccount", benchutil.GenerateServiceAccount("apply-sa")},
		}
		for _, tc := range resources {
			b.Run(tc.name, func(b *testing.B) {
				benchHelperOp(b, tc.obj, applyFn)
			})
		}
	})

	b.Run("BySize", func(b *testing.B) {
		sizes := []struct {
			name string
			size int
		}{
			{"100B", 100},
			{"1KB", 1024},
			{"10KB", 10 * 1024},
			{"100KB", 100 * 1024},
		}
		for _, sz := range sizes {
			b.Run(sz.name, func(b *testing.B) {
				benchHelperOp(b, benchutil.GenerateConfigMap("apply-sized", sz.size), applyFn)
			})
		}
	})
}

// BenchmarkPatch benchmarks all patch types (StrategicMerge, JSON, Merge).
func BenchmarkPatch(b *testing.B) {
	patches := []struct {
		name      string
		patchData string
		patchType string
	}{
		{"StrategicMerge", `{"data":{"newkey%d":"newvalue"}}`, "strategic"},
		{"JSONPatch", `[{"op":"add","path":"/data/newkey%d","value":"newvalue"}]`, "json"},
		{"MergePatch", `{"data":{"newkey%d":"newvalue"}}`, "merge"},
	}

	for _, tc := range patches {
		b.Run(tc.name, func(b *testing.B) {
			var target types.NamespacedName
			benchutil.ControllerHarness[*benchutil.HelperState]{
				Setup: func(b *testing.B, env *benchutil.BenchEnv) *benchutil.HelperState {
					cm := benchutil.CopyAndSetNamespace(benchutil.GenerateConfigMap("patch-cm", 0), env.Namespace)
					if err := env.SeedClient.Create(context.Background(), cm); err != nil {
						b.Fatalf("failed to seed configmap: %v", err)
					}
					target = types.NamespacedName{Name: "patch-cm", Namespace: env.Namespace}
					return benchutil.SetupLocalHelper(b, env)
				},
				Reconcile: func(b *testing.B, s *benchutil.HelperState, _ []client.Object, i int) {
					patchData := []byte(fmt.Sprintf(tc.patchData, i))
					if err := s.Helper.Patch(target, "ConfigMap", "v1", patchData, tc.patchType); err != nil {
						b.Fatalf("patch failed: %v", err)
					}
				},
			}.Run(b)
		})
	}
}

// BenchmarkApplyRuntimeObject benchmarks ApplyRuntimeObject.
func BenchmarkApplyRuntimeObject(b *testing.B) {
	objects := []struct {
		name string
		obj  client.Object
	}{
		{"ConfigMap", benchutil.GenerateConfigMap("rt-cm", 0)},
		{"Secret", benchutil.GenerateSecret("rt-secret", 0)},
		{"ServiceAccount", benchutil.GenerateServiceAccount("rt-sa")},
	}

	for _, tc := range objects {
		b.Run(tc.name, func(b *testing.B) {
			benchHelperOp(b, tc.obj, func(b *testing.B, helper resource.Helper, obj client.Object) {
				if _, err := helper.ApplyRuntimeObject(obj, scheme.Scheme); err != nil {
					b.Fatalf("apply runtime object failed: %v", err)
				}
			})
		})
	}
}

// BenchmarkCreateOrUpdate benchmarks CreateOrUpdate by resource type and payload size.
func BenchmarkCreateOrUpdate(b *testing.B) {
	createOrUpdateFn := func(b *testing.B, helper resource.Helper, obj client.Object) {
		serialized := benchutil.MustSerialize(obj)
		if _, err := helper.CreateOrUpdate(serialized); err != nil {
			b.Fatalf("createOrUpdate failed: %v", err)
		}
	}

	b.Run("ByType", func(b *testing.B) {
		resources := []struct {
			name string
			obj  client.Object
		}{
			{"ConfigMap", benchutil.GenerateConfigMap("cou-cm", 0)},
			{"Secret", benchutil.GenerateSecret("cou-secret", 0)},
			{"Deployment", benchutil.GenerateDeployment("cou-deploy")},
			{"ServiceAccount", benchutil.GenerateServiceAccount("cou-sa")},
		}
		for _, tc := range resources {
			b.Run(tc.name, func(b *testing.B) {
				benchHelperOp(b, tc.obj, createOrUpdateFn)
			})
		}
	})

	b.Run("BySize", func(b *testing.B) {
		sizes := []struct {
			name string
			size int
		}{
			{"100B", 100},
			{"1KB", 1024},
			{"10KB", 10 * 1024},
			{"100KB", 100 * 1024},
		}
		for _, sz := range sizes {
			b.Run(sz.name, func(b *testing.B) {
				benchHelperOp(b, benchutil.GenerateConfigMap("cou-sized", sz.size), createOrUpdateFn)
			})
		}
	})
}

// BenchmarkCreateOrUpdateRuntimeObject benchmarks CreateOrUpdateRuntimeObject.
func BenchmarkCreateOrUpdateRuntimeObject(b *testing.B) {
	objects := []struct {
		name string
		obj  client.Object
	}{
		{"ConfigMap", benchutil.GenerateConfigMap("cou-rt-cm", 0)},
		{"Secret", benchutil.GenerateSecret("cou-rt-secret", 0)},
		{"ServiceAccount", benchutil.GenerateServiceAccount("cou-rt-sa")},
	}

	for _, tc := range objects {
		b.Run(tc.name, func(b *testing.B) {
			benchHelperOp(b, tc.obj, func(b *testing.B, helper resource.Helper, obj client.Object) {
				if _, err := helper.CreateOrUpdateRuntimeObject(obj, scheme.Scheme); err != nil {
					b.Fatalf("createOrUpdate runtime object failed: %v", err)
				}
			})
		})
	}
}

// BenchmarkCreate benchmarks Create (FirstCreate and AlreadyExists paths).
func BenchmarkCreate(b *testing.B) {
	b.Run("FirstCreate", func(b *testing.B) {
		benchutil.ControllerHarness[*benchutil.HelperState]{
			NewObjects: func(b *testing.B, ns string) []client.Object {
				objects := make([]client.Object, b.N)
				for i := range objects {
					objects[i] = benchutil.CopyAndSetNamespace(
						benchutil.GenerateConfigMap(fmt.Sprintf("create-%d", i), 0), ns)
				}
				return objects
			},
			Setup: benchutil.SetupLocalHelper,
			Reconcile: func(b *testing.B, s *benchutil.HelperState, objects []client.Object, i int) {
				serialized := benchutil.MustSerialize(objects[i])
				if _, err := s.Helper.Create(serialized); err != nil {
					b.Fatalf("create failed: %v", err)
				}
			},
		}.Run(b)
	})

	b.Run("AlreadyExists", func(b *testing.B) {
		benchutil.ControllerHarness[*benchutil.HelperState]{
			NewObjects: benchutil.SingleTemplate(benchutil.GenerateConfigMap("create-existing", 0)),
			Setup: func(b *testing.B, env *benchutil.BenchEnv) *benchutil.HelperState {
				obj := benchutil.CopyAndSetNamespace(benchutil.GenerateConfigMap("create-existing", 0), env.Namespace)
				if err := env.SeedClient.Create(context.Background(), obj); err != nil {
					b.Fatalf("failed to seed resource: %v", err)
				}
				return benchutil.SetupLocalHelper(b, env)
			},
			Reconcile: func(b *testing.B, s *benchutil.HelperState, objects []client.Object, i int) {
				serialized := benchutil.MustSerialize(objects[i])
				if _, err := s.Helper.Create(serialized); err != nil {
					b.Fatalf("create failed: %v", err)
				}
			},
		}.Run(b)
	})
}

// BenchmarkDelete benchmarks Delete with pre-seeded resources.
func BenchmarkDelete(b *testing.B) {
	var ns string
	benchutil.ControllerHarness[*benchutil.HelperState]{
		Setup: func(b *testing.B, env *benchutil.BenchEnv) *benchutil.HelperState {
			ns = env.Namespace
			for i := 0; i < b.N; i++ {
				cm := benchutil.CopyAndSetNamespace(
					benchutil.GenerateConfigMap(fmt.Sprintf("delete-%d", i), 0), ns)
				if err := env.SeedClient.Create(context.Background(), cm); err != nil {
					b.Fatalf("failed to seed resource: %v", err)
				}
			}
			return benchutil.SetupLocalHelper(b, env)
		},
		Reconcile: func(b *testing.B, s *benchutil.HelperState, _ []client.Object, i int) {
			if err := s.Helper.Delete("v1", "ConfigMap", ns, fmt.Sprintf("delete-%d", i)); err != nil {
				b.Fatalf("delete failed: %v", err)
			}
		},
	}.Run(b)
}
