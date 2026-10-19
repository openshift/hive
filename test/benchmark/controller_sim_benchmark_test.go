package benchmark

import (
	"context"
	"fmt"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/hive/test/benchutil"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivescheme "github.com/openshift/hive/pkg/util/scheme"
)

// Common ClusterSync reconcile functions (RESTConfig + Helper per iteration).

func clusterSyncApply(b *testing.B, brc *benchutil.BenchRemoteClient, objects []client.Object, _ int) {
	restCfg, err := brc.NewBuilder().RESTConfig()
	if err != nil {
		b.Fatalf("RESTConfig failed: %v", err)
	}
	helper := benchutil.BenchHelper(b, restCfg, "benchmark")
	for _, obj := range objects {
		if _, err := helper.Apply(benchutil.MustSerialize(obj)); err != nil {
			b.Fatalf("apply failed: %v", err)
		}
	}
}

func clusterSyncCreateOrUpdate(b *testing.B, brc *benchutil.BenchRemoteClient, objects []client.Object, _ int) {
	restCfg, err := brc.NewBuilder().RESTConfig()
	if err != nil {
		b.Fatalf("RESTConfig failed: %v", err)
	}
	helper := benchutil.BenchHelper(b, restCfg, "benchmark")
	for _, obj := range objects {
		if _, err := helper.CreateOrUpdate(benchutil.MustSerialize(obj)); err != nil {
			b.Fatalf("createOrUpdate failed: %v", err)
		}
	}
}

// Pattern 1: RESTConfig -> resource.Helper -- models clustersync.
// FixedSet makes FirstApply runs delete the object set between iterations
// (uncounted) so every iteration is a true first apply.

func BenchmarkControllerReconcileClusterSync(b *testing.B) {
	b.Run("Small", func(b *testing.B) {
		benchutil.ControllerHarness[*benchutil.BenchRemoteClient]{
			NewObjects: func(_ *testing.B, ns string) []client.Object {
				return benchutil.CopyAndSetNamespace(ns,
					benchutil.GenerateConfigMap("syncset-cm", 0),
					benchutil.GenerateSecret("syncset-secret", 0),
					benchutil.GenerateDeployment("syncset-deploy"),
					benchutil.GenerateServiceAccount("syncset-sa"),
				)
			},
			Setup:       benchutil.SetupRemoteClient,
			Reconcile:   clusterSyncApply,
			FixedSet:    true,
			SteadyState: true,
		}.Run(b)
	})

	b.Run("Large", func(b *testing.B) {
		benchutil.ControllerHarness[*benchutil.BenchRemoteClient]{
			NewObjects: func(_ *testing.B, ns string) []client.Object {
				// 5 ConfigMaps + 4 Secrets + 3 Deployments + 3 ServiceAccounts + 2 large ConfigMaps = 17
				objects := make([]client.Object, 0, 17)
				for i := 0; i < 5; i++ {
					objects = append(objects, benchutil.CopyAndSetNamespace(ns,
						benchutil.GenerateConfigMap(fmt.Sprintf("config-%d", i), 1024))...)
				}
				for i := 0; i < 4; i++ {
					objects = append(objects, benchutil.CopyAndSetNamespace(ns,
						benchutil.GenerateSecret(fmt.Sprintf("secret-%d", i), 512))...)
				}
				for i := 0; i < 3; i++ {
					objects = append(objects, benchutil.CopyAndSetNamespace(ns,
						benchutil.GenerateDeployment(fmt.Sprintf("deploy-%d", i)))...)
				}
				for i := 0; i < 3; i++ {
					objects = append(objects, benchutil.CopyAndSetNamespace(ns,
						benchutil.GenerateServiceAccount(fmt.Sprintf("sa-%d", i)))...)
				}
				objects = append(objects, benchutil.CopyAndSetNamespace(ns,
					benchutil.GenerateConfigMap("ca-bundle", 10*1024),
					benchutil.GenerateConfigMap("install-script", 5*1024),
				)...)
				return objects
			},
			Setup:       benchutil.SetupRemoteClient,
			Reconcile:   clusterSyncApply,
			FixedSet:    true,
			SteadyState: true,
		}.Run(b)
	})

	b.Run("WithDelete", func(b *testing.B) {
		benchutil.ControllerHarness[*benchutil.BenchRemoteClient]{
			NewObjects: func(_ *testing.B, ns string) []client.Object {
				return benchutil.CopyAndSetNamespace(ns,
					benchutil.GenerateConfigMap("active-cm", 0),
					benchutil.GenerateSecret("active-secret", 0),
					benchutil.GenerateServiceAccount("active-sa"),
				)
			},
			Setup: func(b *testing.B, env *benchutil.BenchEnv) *benchutil.BenchRemoteClient {
				for i := 0; i < b.N; i++ {
					cm := benchutil.CopyAndSetNamespace(env.Namespace,
						benchutil.GenerateConfigMap(fmt.Sprintf("stale-config-%d", i), 0))[0]
					if err := env.SeedClient.Create(context.Background(), cm); err != nil {
						b.Fatalf("failed to seed stale resource: %v", err)
					}
				}
				return benchutil.SetupRemoteClient(b, env)
			},
			Reconcile: func(b *testing.B, brc *benchutil.BenchRemoteClient, objects []client.Object, i int) {
				restCfg, err := brc.NewBuilder().RESTConfig()
				if err != nil {
					b.Fatalf("RESTConfig failed: %v", err)
				}
				helper := benchutil.BenchHelper(b, restCfg, "benchmark")
				for _, obj := range objects {
					if _, err := helper.Apply(benchutil.MustSerialize(obj)); err != nil {
						b.Fatalf("apply failed: %v", err)
					}
				}
				if err := helper.Delete("v1", "ConfigMap", objects[0].GetNamespace(), fmt.Sprintf("stale-config-%d", i)); err != nil {
					b.Fatalf("delete failed: %v", err)
				}
			},
			FixedSet: true,
		}.Run(b)
	})

	// WithStaticPatch: unchanging patch data (idempotent after first iteration).
	b.Run("WithStaticPatch", func(b *testing.B) {
		benchutil.ControllerHarness[*benchutil.BenchRemoteClient]{
			NewObjects: func(_ *testing.B, ns string) []client.Object {
				return benchutil.CopyAndSetNamespace(ns,
					benchutil.GenerateConfigMap("active-cm", 0),
					benchutil.GenerateSecret("active-secret", 0),
				)
			},
			Setup: func(b *testing.B, env *benchutil.BenchEnv) *benchutil.BenchRemoteClient {
				target := benchutil.CopyAndSetNamespace(env.Namespace,
					benchutil.GenerateConfigMap("patch-target", 0))[0]
				if err := env.SeedClient.Create(context.Background(), target); err != nil {
					b.Fatalf("failed to seed patch target: %v", err)
				}
				return benchutil.SetupRemoteClient(b, env)
			},
			Reconcile: func(b *testing.B, brc *benchutil.BenchRemoteClient, objects []client.Object, _ int) {
				restCfg, err := brc.NewBuilder().RESTConfig()
				if err != nil {
					b.Fatalf("RESTConfig failed: %v", err)
				}
				helper := benchutil.BenchHelper(b, restCfg, "benchmark")
				for _, obj := range objects {
					if _, err := helper.Apply(benchutil.MustSerialize(obj)); err != nil {
						b.Fatalf("apply failed: %v", err)
					}
				}
				if err := helper.Patch(types.NamespacedName{
					Name: "patch-target", Namespace: objects[0].GetNamespace(),
				}, "ConfigMap", "v1", []byte(`{"data":{"patched":"true"}}`), "strategic"); err != nil {
					b.Fatalf("patch failed: %v", err)
				}
			},
			FixedSet: true,
		}.Run(b)
	})

	// WithDynamicPatch: patch data varies per iteration, forcing real writes.
	b.Run("WithDynamicPatch", func(b *testing.B) {
		benchutil.ControllerHarness[*benchutil.BenchRemoteClient]{
			NewObjects: func(_ *testing.B, ns string) []client.Object {
				return benchutil.CopyAndSetNamespace(ns,
					benchutil.GenerateConfigMap("active-cm", 0),
					benchutil.GenerateSecret("active-secret", 0),
				)
			},
			Setup: func(b *testing.B, env *benchutil.BenchEnv) *benchutil.BenchRemoteClient {
				target := benchutil.CopyAndSetNamespace(env.Namespace,
					benchutil.GenerateConfigMap("patch-target", 0))[0]
				if err := env.SeedClient.Create(context.Background(), target); err != nil {
					b.Fatalf("failed to seed patch target: %v", err)
				}
				return benchutil.SetupRemoteClient(b, env)
			},
			Reconcile: func(b *testing.B, brc *benchutil.BenchRemoteClient, objects []client.Object, i int) {
				restCfg, err := brc.NewBuilder().RESTConfig()
				if err != nil {
					b.Fatalf("RESTConfig failed: %v", err)
				}
				helper := benchutil.BenchHelper(b, restCfg, "benchmark")
				for _, obj := range objects {
					if _, err := helper.Apply(benchutil.MustSerialize(obj)); err != nil {
						b.Fatalf("apply failed: %v", err)
					}
				}
				if err := helper.Patch(types.NamespacedName{
					Name: "patch-target", Namespace: objects[0].GetNamespace(),
				}, "ConfigMap", "v1", []byte(fmt.Sprintf(`{"data":{"newkey%d":"value"}}`, i)), "strategic"); err != nil {
					b.Fatalf("patch failed: %v", err)
				}
			},
			FixedSet: true,
		}.Run(b)
	})
}

// BenchmarkControllerReconcileClusterSyncCreateOrUpdate is an A/B comparison with Apply.
func BenchmarkControllerReconcileClusterSyncCreateOrUpdate(b *testing.B) {
	b.Run("Small", func(b *testing.B) {
		benchutil.ControllerHarness[*benchutil.BenchRemoteClient]{
			NewObjects: func(_ *testing.B, ns string) []client.Object {
				return benchutil.CopyAndSetNamespace(ns,
					benchutil.GenerateConfigMap("cou-cm", 0),
					benchutil.GenerateSecret("cou-secret", 0),
					benchutil.GenerateDeployment("cou-deploy"),
					benchutil.GenerateServiceAccount("cou-sa"),
				)
			},
			Setup:       benchutil.SetupRemoteClient,
			Reconcile:   clusterSyncCreateOrUpdate,
			FixedSet:    true,
			SteadyState: true,
		}.Run(b)
	})
}

// Pattern 2: Build -> Get (read-only) -- models clusterversion, clusterstate.

func BenchmarkControllerReconcileClusterVersion(b *testing.B) {
	benchutil.ControllerHarness[*benchutil.BenchRemoteClient]{
		Setup: func(b *testing.B, env *benchutil.BenchEnv) *benchutil.BenchRemoteClient {
			// Seed a ClusterVersion to read (cluster-scoped singleton).
			cv := &configv1.ClusterVersion{
				ObjectMeta: metav1.ObjectMeta{Name: "version"},
				Spec: configv1.ClusterVersionSpec{
					ClusterID: "bench-cluster-id",
				},
			}
			if err := env.SeedClient.Get(context.Background(), client.ObjectKeyFromObject(cv), cv); apierrors.IsNotFound(err) {
				if err := env.SeedClient.Create(context.Background(), cv); err != nil {
					b.Fatalf("failed to seed ClusterVersion: %v", err)
				}
			} else if err != nil {
				b.Fatalf("failed to check for existing ClusterVersion: %v", err)
			}
			return benchutil.SetupRemoteClient(b, env)
		},
		Reconcile: func(b *testing.B, brc *benchutil.BenchRemoteClient, _ []client.Object, _ int) {
			remoteClient, err := brc.NewBuilder().Build()
			if err != nil {
				b.Fatalf("Build failed: %v", err)
			}
			got := &configv1.ClusterVersion{}
			if err := remoteClient.Get(context.Background(), client.ObjectKey{Name: "version"}, got); err != nil {
				b.Fatalf("Get failed: %v", err)
			}
		},
	}.Run(b)
}

// Pattern 3: Build -> CRUD -- models machinepool.
// Separate harness calls per mode because FirstApply creates and SteadyState updates.

func BenchmarkControllerReconcileMachinePool(b *testing.B) {
	// firstApplyState carries the env so the created MachineSet can be removed
	// between iterations; otherwise MachineSetList responses (and their cost)
	// would grow with every iteration. The cleanup goes through the uncounted
	// SeedClient with the timer paused, so it does not affect the metrics.
	type firstApplyState struct {
		*benchutil.BenchRemoteClient
		env *benchutil.BenchEnv
	}
	b.Run("FirstApply", func(b *testing.B) {
		benchutil.ControllerHarness[*firstApplyState]{
			Setup: func(b *testing.B, env *benchutil.BenchEnv) *firstApplyState {
				return &firstApplyState{
					BenchRemoteClient: benchutil.SetupRemoteClient(b, env),
					env:               env,
				}
			},
			Reconcile: func(b *testing.B, s *firstApplyState, _ []client.Object, i int) {
				remoteClient, err := s.NewBuilder().Build()
				if err != nil {
					b.Fatalf("Build failed: %v", err)
				}
				workerName := fmt.Sprintf("worker-%d", i)

				ms := &machinev1beta1.MachineSet{}
				if err := remoteClient.Get(context.Background(), client.ObjectKey{
					Namespace: s.env.Namespace, Name: workerName,
				}, ms); err != nil && !apierrors.IsNotFound(err) {
					b.Fatalf("Get MachineSet failed: %v", err)
				}

				machineList := &machinev1beta1.MachineList{}
				if err := remoteClient.List(context.Background(), machineList, client.InNamespace(s.env.Namespace)); err != nil {
					b.Fatalf("List Machines failed: %v", err)
				}

				machineSetList := &machinev1beta1.MachineSetList{}
				if err := remoteClient.List(context.Background(), machineSetList, client.InNamespace(s.env.Namespace)); err != nil {
					b.Fatalf("List MachineSets failed: %v", err)
				}

				toCreate := benchutil.CopyAndSetNamespace(s.env.Namespace,
					benchutil.GenerateMachineSet(workerName))[0]
				if err := remoteClient.Create(context.Background(), toCreate); err != nil {
					b.Fatalf("Create failed: %v", err)
				}

				b.StopTimer()
				err = s.env.SeedClient.Delete(context.Background(), toCreate)
				b.StartTimer()
				if err != nil && !apierrors.IsNotFound(err) {
					b.Fatalf("inter-iteration MachineSet cleanup failed: %v", err)
				}
			},
		}.Run(b)
	})

	type steadyState struct {
		*benchutil.BenchRemoteClient
		ns       string
		existing client.Object
	}
	b.Run("SteadyState", func(b *testing.B) {
		benchutil.ControllerHarness[*steadyState]{
			Setup: func(b *testing.B, env *benchutil.BenchEnv) *steadyState {
				ms := benchutil.CopyAndSetNamespace(env.Namespace,
					benchutil.GenerateMachineSet("worker"))[0]
				if err := env.SeedClient.Create(context.Background(), ms); err != nil {
					b.Fatalf("failed to seed MachineSet: %v", err)
				}
				return &steadyState{
					BenchRemoteClient: benchutil.SetupRemoteClient(b, env),
					ns:                env.Namespace,
					existing:          ms,
				}
			},
			Reconcile: func(b *testing.B, s *steadyState, _ []client.Object, i int) {
				remoteClient, err := s.NewBuilder().Build()
				if err != nil {
					b.Fatalf("Build failed: %v", err)
				}

				got := &machinev1beta1.MachineSet{}
				if err := remoteClient.Get(context.Background(), client.ObjectKeyFromObject(s.existing), got); err != nil {
					b.Fatalf("Get failed: %v", err)
				}

				machineList := &machinev1beta1.MachineList{}
				if err := remoteClient.List(context.Background(), machineList, client.InNamespace(s.ns)); err != nil {
					b.Fatalf("List Machines failed: %v", err)
				}

				machineSetList := &machinev1beta1.MachineSetList{}
				if err := remoteClient.List(context.Background(), machineSetList, client.InNamespace(s.ns)); err != nil {
					b.Fatalf("List MachineSets failed: %v", err)
				}

				newReplicas := int32(i + 1)
				got.Spec.Replicas = &newReplicas
				if err := remoteClient.Update(context.Background(), got); err != nil {
					b.Fatalf("Update failed: %v", err)
				}
			},
		}.Run(b)
	})
}

// Pattern 4: Build + BuildKubeClient -- models hibernation.

func BenchmarkControllerReconcileHibernation(b *testing.B) {
	benchutil.ControllerHarness[*benchutil.BenchRemoteClient]{
		Setup: func(b *testing.B, env *benchutil.BenchEnv) *benchutil.BenchRemoteClient {
			for i := 0; i < 3; i++ {
				machine := &machinev1beta1.Machine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("worker-%d", i),
						Namespace: env.Namespace,
					},
				}
				if err := env.SeedClient.Create(context.Background(), machine); err != nil {
					b.Fatalf("failed to seed Machine: %v", err)
				}
			}
			return benchutil.SetupRemoteClient(b, env)
		},
		Reconcile: func(b *testing.B, brc *benchutil.BenchRemoteClient, _ []client.Object, _ int) {
			// First client: controller-runtime (Build)
			remoteClient, err := brc.NewBuilder().Build()
			if err != nil {
				b.Fatalf("Build failed: %v", err)
			}

			machineList := &machinev1beta1.MachineList{}
			if err := remoteClient.List(context.Background(), machineList); err != nil {
				b.Fatalf("List Machines failed: %v", err)
			}

			nodeList := &corev1.NodeList{}
			if err := remoteClient.List(context.Background(), nodeList); err != nil {
				b.Fatalf("List Nodes failed: %v", err)
			}

			coList := &configv1.ClusterOperatorList{}
			if err := remoteClient.List(context.Background(), coList); err != nil {
				b.Fatalf("List ClusterOperators failed: %v", err)
			}

			// Second client: typed kubernetes (BuildKubeClient)
			kubeClient, err := brc.NewBuilder().BuildKubeClient()
			if err != nil {
				b.Fatalf("BuildKubeClient failed: %v", err)
			}

			if _, err := kubeClient.CertificatesV1().CertificateSigningRequests().List(context.Background(), metav1.ListOptions{}); err != nil {
				b.Fatalf("kube List CSRs failed: %v", err)
			}
		},
	}.Run(b)
}

// Pattern 5: UsePrimaryAPIURL().Build() -- models unreachable.
//
// The unreachable controller builds a client via the primary API URL and, on
// failure, sets the Unreachable condition on the ClusterDeployment. This
// benchmark measures that Build against a reachable endpoint: a truly
// unreachable one would fail only after connect timeouts, which is wall-clock
// dominated and not meaningfully repeatable.

func BenchmarkControllerReconcileUnreachable(b *testing.B) {
	benchutil.ControllerHarness[*benchutil.BenchRemoteClient]{
		Setup: benchutil.SetupRemoteClient,
		Reconcile: func(b *testing.B, brc *benchutil.BenchRemoteClient, _ []client.Object, _ int) {
			if _, err := brc.NewBuilder().UsePrimaryAPIURL().Build(); err != nil {
				b.Fatalf("Build failed: %v", err)
			}
		},
	}.Run(b)
}

// Pattern 6: Local-only Helper -- models controlplanecerts, remoteingress.

func BenchmarkControllerReconcileControlPlaneCerts(b *testing.B) {
	benchutil.ControllerHarness[*benchutil.HelperState]{
		NewObjects: benchutil.SingleTemplate(benchutil.GenerateSyncSet("controlplane-certs")),
		Setup:      benchutil.SetupLocalHelper,
		Reconcile: func(b *testing.B, s *benchutil.HelperState, objects []client.Object, i int) {
			if _, err := s.Helper.ApplyRuntimeObject(objects[i], hivescheme.GetScheme()); err != nil {
				b.Fatalf("ApplyRuntimeObject failed: %v", err)
			}
		},
		SteadyState: true,
	}.Run(b)
}
