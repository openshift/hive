package resource_test

import (
	"context"
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/hive/pkg/resource"
	hivescheme "github.com/openshift/hive/pkg/util/scheme"
	"github.com/openshift/hive/test/benchutil"
)

// benchResourceTypes is the standard matrix of resource types applied to
// remote clusters. It includes an OpenShift custom resource (MachineSet) so
// the custom-resource code paths (CRD discovery, patch-type fallback) are
// exercised alongside the built-in types.
var benchResourceTypes = []struct {
	name string
	obj  client.Object
}{
	{"ConfigMap", benchutil.GenerateConfigMap("bench-cm", 0)},
	{"Secret", benchutil.GenerateSecret("bench-secret", 0)},
	{"Deployment", benchutil.GenerateDeployment("bench-deploy")},
	{"ServiceAccount", benchutil.GenerateServiceAccount("bench-sa")},
	{"MachineSet", benchutil.GenerateMachineSet("bench-ms")},
}

// benchSizes is the standard payload-size matrix.
var benchSizes = []struct {
	name string
	size int
}{
	{"100B", 100},
	{"1KB", 1024},
	{"10KB", 10 * 1024},
	{"100KB", 100 * 1024},
}

// benchByTypeAndSize runs the standard ByType and BySize sub-benchmark matrices
// for op. sized builds the object used in the BySize runs.
func benchByTypeAndSize(b *testing.B, sized func(int) client.Object, op func(*testing.B, resource.Helper, client.Object)) {
	b.Helper()
	b.Run("ByType", func(b *testing.B) {
		for _, tc := range benchResourceTypes {
			b.Run(tc.name, func(b *testing.B) {
				benchHelperOp(b, tc.obj, op)
			})
		}
	})
	b.Run("BySize", func(b *testing.B) {
		for _, sz := range benchSizes {
			b.Run(sz.name, func(b *testing.B) {
				benchHelperOp(b, sized(sz.size), op)
			})
		}
	})
}

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
	benchByTypeAndSize(b,
		func(size int) client.Object { return benchutil.GenerateConfigMap("apply-sized", size) },
		func(b *testing.B, helper resource.Helper, obj client.Object) {
			if _, err := helper.Apply(benchutil.MustSerialize(obj)); err != nil {
				b.Fatalf("apply failed: %v", err)
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
					cm := benchutil.CopyAndSetNamespace(env.Namespace,
						benchutil.GenerateConfigMap("patch-cm", 0))[0]
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
	for _, tc := range benchResourceTypes {
		b.Run(tc.name, func(b *testing.B) {
			benchHelperOp(b, tc.obj, func(b *testing.B, helper resource.Helper, obj client.Object) {
				if _, err := helper.ApplyRuntimeObject(obj, hivescheme.GetScheme()); err != nil {
					b.Fatalf("apply runtime object failed: %v", err)
				}
			})
		})
	}
}

// BenchmarkCreateOrUpdate benchmarks CreateOrUpdate by resource type and payload size.
func BenchmarkCreateOrUpdate(b *testing.B) {
	benchByTypeAndSize(b,
		func(size int) client.Object { return benchutil.GenerateConfigMap("cou-sized", size) },
		func(b *testing.B, helper resource.Helper, obj client.Object) {
			if _, err := helper.CreateOrUpdate(benchutil.MustSerialize(obj)); err != nil {
				b.Fatalf("createOrUpdate failed: %v", err)
			}
		})
}

// BenchmarkCreateOrUpdateRuntimeObject benchmarks CreateOrUpdateRuntimeObject.
func BenchmarkCreateOrUpdateRuntimeObject(b *testing.B) {
	for _, tc := range benchResourceTypes {
		b.Run(tc.name, func(b *testing.B) {
			benchHelperOp(b, tc.obj, func(b *testing.B, helper resource.Helper, obj client.Object) {
				if _, err := helper.CreateOrUpdateRuntimeObject(obj, hivescheme.GetScheme()); err != nil {
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
					objects[i] = benchutil.CopyAndSetNamespace(ns,
						benchutil.GenerateConfigMap(fmt.Sprintf("create-%d", i), 0))[0]
				}
				return objects
			},
			Setup: benchutil.SetupLocalHelper,
			Reconcile: func(b *testing.B, s *benchutil.HelperState, objects []client.Object, i int) {
				if _, err := s.Helper.Create(benchutil.MustSerialize(objects[i])); err != nil {
					b.Fatalf("create failed: %v", err)
				}
			},
		}.Run(b)
	})

	b.Run("AlreadyExists", func(b *testing.B) {
		benchutil.ControllerHarness[*benchutil.HelperState]{
			NewObjects: benchutil.SingleTemplate(benchutil.GenerateConfigMap("create-existing", 0)),
			Setup: func(b *testing.B, env *benchutil.BenchEnv) *benchutil.HelperState {
				obj := benchutil.CopyAndSetNamespace(env.Namespace,
					benchutil.GenerateConfigMap("create-existing", 0))[0]
				if err := env.SeedClient.Create(context.Background(), obj); err != nil {
					b.Fatalf("failed to seed resource: %v", err)
				}
				return benchutil.SetupLocalHelper(b, env)
			},
			Reconcile: func(b *testing.B, s *benchutil.HelperState, objects []client.Object, i int) {
				if _, err := s.Helper.Create(benchutil.MustSerialize(objects[i])); err != nil {
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
				cm := benchutil.CopyAndSetNamespace(ns,
					benchutil.GenerateConfigMap(fmt.Sprintf("delete-%d", i), 0))[0]
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
