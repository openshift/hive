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
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func BenchmarkControllerReconcileClusterSync(b *testing.B) {
	b.Run("Small", func(b *testing.B) {
		benchutil.ControllerHarness[*benchutil.BenchRemoteClient]{
			NewObjects: func(_ *testing.B, ns string) []client.Object {
				return []client.Object{
					benchutil.CopyAndSetNamespace(benchutil.GenerateConfigMap("syncset-cm", 0), ns),
					benchutil.CopyAndSetNamespace(benchutil.GenerateSecret("syncset-secret", 0), ns),
					benchutil.CopyAndSetNamespace(benchutil.GenerateDeployment("syncset-deploy"), ns),
					benchutil.CopyAndSetNamespace(benchutil.GenerateServiceAccount("syncset-sa"), ns),
				}
			},
			Setup:       benchutil.SetupRemoteClient,
			Reconcile:   clusterSyncApply,
			SteadyState: true,
		}.Run(b)
	})

	b.Run("Large", func(b *testing.B) {
		benchutil.ControllerHarness[*benchutil.BenchRemoteClient]{
			NewObjects: func(_ *testing.B, ns string) []client.Object {
				// 5 ConfigMaps + 4 Secrets + 3 Deployments + 3 ServiceAccounts + 2 large ConfigMaps = 17
				objects := make([]client.Object, 0, 17)
				for i := 0; i < 5; i++ {
					objects = append(objects, benchutil.CopyAndSetNamespace(
						benchutil.GenerateConfigMap(fmt.Sprintf("config-%d", i), 1024), ns))
				}
				for i := 0; i < 4; i++ {
					objects = append(objects, benchutil.CopyAndSetNamespace(
						benchutil.GenerateSecret(fmt.Sprintf("secret-%d", i), 512), ns))
				}
				for i := 0; i < 3; i++ {
					objects = append(objects, benchutil.CopyAndSetNamespace(
						benchutil.GenerateDeployment(fmt.Sprintf("deploy-%d", i)), ns))
				}
				for i := 0; i < 3; i++ {
					objects = append(objects, benchutil.CopyAndSetNamespace(
						benchutil.GenerateServiceAccount(fmt.Sprintf("sa-%d", i)), ns))
				}
				objects = append(objects,
					benchutil.CopyAndSetNamespace(benchutil.GenerateConfigMap("ca-bundle", 10*1024), ns),
					benchutil.CopyAndSetNamespace(benchutil.GenerateConfigMap("install-script", 5*1024), ns),
				)
				return objects
			},
			Setup:       benchutil.SetupRemoteClient,
			Reconcile:   clusterSyncApply,
			SteadyState: true,
		}.Run(b)
	})

	b.Run("WithDelete", func(b *testing.B) {
		benchutil.ControllerHarness[*benchutil.BenchRemoteClient]{
			NewObjects: func(_ *testing.B, ns string) []client.Object {
				return []client.Object{
					benchutil.CopyAndSetNamespace(benchutil.GenerateConfigMap("active-cm", 0), ns),
					benchutil.CopyAndSetNamespace(benchutil.GenerateSecret("active-secret", 0), ns),
					benchutil.CopyAndSetNamespace(benchutil.GenerateServiceAccount("active-sa"), ns),
				}
			},
			Setup: func(b *testing.B, env *benchutil.BenchEnv) *benchutil.BenchRemoteClient {
				for i := 0; i < b.N; i++ {
					cm := benchutil.CopyAndSetNamespace(
						benchutil.GenerateConfigMap(fmt.Sprintf("stale-config-%d", i), 0), env.Namespace)
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
		}.Run(b)
	})

	// WithStaticPatch: unchanging patch data (idempotent after first iteration).
	b.Run("WithStaticPatch", func(b *testing.B) {
		benchutil.ControllerHarness[*benchutil.BenchRemoteClient]{
			NewObjects: func(_ *testing.B, ns string) []client.Object {
				return []client.Object{
					benchutil.CopyAndSetNamespace(benchutil.GenerateConfigMap("active-cm", 0), ns),
					benchutil.CopyAndSetNamespace(benchutil.GenerateSecret("active-secret", 0), ns),
				}
			},
			Setup: func(b *testing.B, env *benchutil.BenchEnv) *benchutil.BenchRemoteClient {
				target := benchutil.CopyAndSetNamespace(benchutil.GenerateConfigMap("patch-target", 0), env.Namespace)
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
		}.Run(b)
	})

	// WithDynamicPatch: patch data varies per iteration, forcing real writes.
	b.Run("WithDynamicPatch", func(b *testing.B) {
		benchutil.ControllerHarness[*benchutil.BenchRemoteClient]{
			NewObjects: func(_ *testing.B, ns string) []client.Object {
				return []client.Object{
					benchutil.CopyAndSetNamespace(benchutil.GenerateConfigMap("active-cm", 0), ns),
					benchutil.CopyAndSetNamespace(benchutil.GenerateSecret("active-secret", 0), ns),
				}
			},
			Setup: func(b *testing.B, env *benchutil.BenchEnv) *benchutil.BenchRemoteClient {
				target := benchutil.CopyAndSetNamespace(benchutil.GenerateConfigMap("patch-target", 0), env.Namespace)
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
		}.Run(b)
	})
}

// BenchmarkControllerReconcileClusterSyncCreateOrUpdate is an A/B comparison with Apply.
func BenchmarkControllerReconcileClusterSyncCreateOrUpdate(b *testing.B) {
	b.Run("Small", func(b *testing.B) {
		benchutil.ControllerHarness[*benchutil.BenchRemoteClient]{
			NewObjects: func(_ *testing.B, ns string) []client.Object {
				return []client.Object{
					benchutil.CopyAndSetNamespace(benchutil.GenerateConfigMap("cou-cm", 0), ns),
					benchutil.CopyAndSetNamespace(benchutil.GenerateSecret("cou-secret", 0), ns),
					benchutil.CopyAndSetNamespace(benchutil.GenerateDeployment("cou-deploy"), ns),
					benchutil.CopyAndSetNamespace(benchutil.GenerateServiceAccount("cou-sa"), ns),
				}
			},
			Setup:       benchutil.SetupRemoteClient,
			Reconcile:   clusterSyncCreateOrUpdate,
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
	type firstApplyState struct {
		*benchutil.BenchRemoteClient
		ns string
	}
	b.Run("FirstApply", func(b *testing.B) {
		benchutil.ControllerHarness[*firstApplyState]{
			Setup: func(b *testing.B, env *benchutil.BenchEnv) *firstApplyState {
				return &firstApplyState{
					BenchRemoteClient: benchutil.SetupRemoteClient(b, env),
					ns:                env.Namespace,
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
					Namespace: s.ns, Name: workerName,
				}, ms); err != nil && !apierrors.IsNotFound(err) {
					b.Fatalf("Get MachineSet failed: %v", err)
				}

				machineList := &machinev1beta1.MachineList{}
				if err := remoteClient.List(context.Background(), machineList, client.InNamespace(s.ns)); err != nil {
					b.Fatalf("List Machines failed: %v", err)
				}

				machineSetList := &machinev1beta1.MachineSetList{}
				if err := remoteClient.List(context.Background(), machineSetList, client.InNamespace(s.ns)); err != nil {
					b.Fatalf("List MachineSets failed: %v", err)
				}

				replicas := int32(3)
				toCreate := &machinev1beta1.MachineSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      workerName,
						Namespace: s.ns,
					},
					Spec: machinev1beta1.MachineSetSpec{
						Replicas: &replicas,
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{"machine.openshift.io/cluster-api-machineset": workerName},
						},
					},
				}
				if err := remoteClient.Create(context.Background(), toCreate); err != nil {
					b.Fatalf("Create failed: %v", err)
				}
			},
		}.Run(b)
	})

	type steadyState struct {
		*benchutil.BenchRemoteClient
		ns       string
		existing *machinev1beta1.MachineSet
	}
	b.Run("SteadyState", func(b *testing.B) {
		benchutil.ControllerHarness[*steadyState]{
			Setup: func(b *testing.B, env *benchutil.BenchEnv) *steadyState {
				replicas := int32(3)
				ms := &machinev1beta1.MachineSet{
					ObjectMeta: metav1.ObjectMeta{Name: "worker", Namespace: env.Namespace},
					Spec: machinev1beta1.MachineSetSpec{
						Replicas: &replicas,
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{"machine.openshift.io/cluster-api-machineset": "worker"},
						},
					},
				}
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
			if _, err := s.Helper.ApplyRuntimeObject(objects[i], scheme.Scheme); err != nil {
				b.Fatalf("ApplyRuntimeObject failed: %v", err)
			}
		},
		SteadyState: true,
	}.Run(b)
}
