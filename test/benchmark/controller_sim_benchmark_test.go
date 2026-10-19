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

	// NoOp: every syncset is up-to-date and the lease is not due for renewal,
	// so the real controller applies nothing -- but it still builds the helper
	// before deciding to skip (secret read + RESTConfig + NewHelper, whose
	// OpenAPI schema fetch pays a revalidation round trip against the remote
	// API server). This is the floor cost every clustersync reconcile pays,
	// including no-op ones; remote-roundtrips/op of ~1 here is that
	// revalidation, not an apply.
	b.Run("NoOp", func(b *testing.B) {
		benchutil.ControllerHarness[*benchutil.BenchRemoteClient]{
			Setup: benchutil.SetupRemoteClient,
			Reconcile: func(b *testing.B, brc *benchutil.BenchRemoteClient, _ []client.Object, _ int) {
				restCfg, err := brc.NewBuilder().RESTConfig()
				if err != nil {
					b.Fatalf("RESTConfig failed: %v", err)
				}
				_ = benchutil.BenchHelper(b, restCfg, "benchmark")
			},
		}.Run(b)
	})

	// WithDelete: models deleteFromTargetCluster -- resources in the
	// ClusterSync status that no longer match any syncset are deleted.
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

	// WithDynamicPatch: the patch value varies per iteration, forcing real
	// writes; the key stays fixed so the target object does not grow with b.N.
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
				}, "ConfigMap", "v1", []byte(fmt.Sprintf(`{"data":{"patchkey":"value%d"}}`, i)), "strategic"); err != nil {
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
			// Read a ClusterVersion (cluster-scoped singleton). It is seeded
			// if missing under envtest; on a real cluster it must already
			// exist, since the benchmarks never write cluster-scoped objects
			// (see test/benchmark/README.md).
			benchutil.EnsureClusterScopedObject(b, env, &configv1.ClusterVersion{
				ObjectMeta: metav1.ObjectMeta{Name: "version"},
				Spec: configv1.ClusterVersionSpec{
					ClusterID: "bench-cluster-id",
				},
			})
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

// Pattern 3: Build -> read -> sync MachineSets -- models machinepool.
//
// The real reconcile (ReconcileMachinePool.reconcile) builds the remote
// client, then reads: the master Machines (a label-selected List), all
// MachineSets, and the cluster-scoped Infrastructure object. It then syncs
// MachineSets (creating, updating or deleting as the generated sets dictate),
// and updatePoolStatusForMachineSets lists the Machines matching each
// MachineSet's selector whenever the MachineSet's replica count is not
// reflected in its status -- always the case right after a write here, since
// nothing in the benchmark cluster updates MachineSet status. The actuator
// (GenerateMachineSets) is a cloud-API seam and is not simulated; its output
// is represented by the generated MachineSet the sync step creates/updates.
//
// The real controller's Lists are cluster-wide; the sim scopes them to the
// benchmark namespace so response sizes stay deterministic (objects from
// earlier runs' namespaces linger in Terminating namespaces on a bare API
// server, which never finishes deleting them).
func BenchmarkControllerReconcileMachinePool(b *testing.B) {
	// masterMachineLabel mirrors machinepool_controller's unexported
	// masterMachineLabelSelector.
	const masterMachineLabel = "machine.openshift.io/cluster-api-machine-type=master"

	// mpState carries the env: BenchRemoteClient hides its own, and the sync
	// step needs the namespace for its reads and inter-iteration cleanup.
	type mpState struct {
		*benchutil.BenchRemoteClient
		env *benchutil.BenchEnv
	}

	// seedRemoteState seeds what the real reconcile's reads expect: master
	// Machines carrying the master label, and the cluster-scoped
	// Infrastructure object getInfrastructure reads (a missing object is a
	// hard error for the real controller).
	seedRemoteState := func(b *testing.B, env *benchutil.BenchEnv) {
		b.Helper()
		for i := 0; i < 3; i++ {
			machine := &machinev1beta1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("master-%d", i),
					Namespace: env.Namespace,
					Labels:    map[string]string{"machine.openshift.io/cluster-api-machine-type": "master"},
				},
			}
			if err := env.SeedClient.Create(context.Background(), machine); err != nil {
				b.Fatalf("failed to seed master Machine: %v", err)
			}
		}
		benchutil.EnsureClusterScopedObject(b, env, &configv1.Infrastructure{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		})
	}

	// remoteReads performs the reads the real reconcile makes before syncing:
	// getMasterMachine, getRemoteMachineSets and getInfrastructure. It returns
	// the listed MachineSets for the sync step to match against.
	remoteReads := func(b *testing.B, remoteClient client.Client, ns string) *machinev1beta1.MachineSetList {
		b.Helper()
		tm := metav1.TypeMeta{}
		tm.SetGroupVersionKind(machinev1beta1.SchemeGroupVersion.WithKind("Machine"))
		masterMachines := &machinev1beta1.MachineList{}
		if err := remoteClient.List(context.Background(), masterMachines,
			client.InNamespace(ns),
			&client.ListOptions{Raw: &metav1.ListOptions{TypeMeta: tm, LabelSelector: masterMachineLabel}}); err != nil {
			b.Fatalf("List master Machines failed: %v", err)
		}
		machineSetList := &machinev1beta1.MachineSetList{}
		if err := remoteClient.List(context.Background(), machineSetList, client.InNamespace(ns)); err != nil {
			b.Fatalf("List MachineSets failed: %v", err)
		}
		infrastructure := &configv1.Infrastructure{}
		if err := remoteClient.Get(context.Background(), client.ObjectKey{Name: "cluster"}, infrastructure); err != nil {
			b.Fatalf("Get Infrastructure failed: %v", err)
		}
		return machineSetList
	}

	// listMachinesForMachineSet mirrors summarizeMachinesError's List: the
	// Machines matching the MachineSet's selector, in its namespace.
	listMachinesForMachineSet := func(b *testing.B, remoteClient client.Client, ms *machinev1beta1.MachineSet) {
		b.Helper()
		sel, err := metav1.LabelSelectorAsSelector(&ms.Spec.Selector)
		if err != nil {
			b.Fatalf("failed to create label selector: %v", err)
		}
		list := &machinev1beta1.MachineList{}
		if err := remoteClient.List(context.TODO(), list,
			client.InNamespace(ms.GetNamespace()),
			client.MatchingLabelsSelector{Selector: sel}); err != nil {
			b.Fatalf("List Machines for MachineSet failed: %v", err)
		}
	}

	// FirstApply: the generated MachineSet matches nothing in the List, so
	// syncMachineSets creates it. The created MachineSet's status is empty,
	// so the status pass pays the summarize Machines List too (nothing in the
	// benchmark cluster matches the new selector; on a real cluster the
	// machine-api controller's Machines would). The MachineSet is removed
	// between iterations (uncounted, timer paused): otherwise the List
	// responses of later iterations would grow.
	b.Run("FirstApply", func(b *testing.B) {
		benchutil.ControllerHarness[*mpState]{
			Setup: func(b *testing.B, env *benchutil.BenchEnv) *mpState {
				seedRemoteState(b, env)
				return &mpState{
					BenchRemoteClient: benchutil.SetupRemoteClient(b, env),
					env:               env,
				}
			},
			Reconcile: func(b *testing.B, s *mpState, _ []client.Object, i int) {
				remoteClient, err := s.NewBuilder().Build()
				if err != nil {
					b.Fatalf("Build failed: %v", err)
				}
				machineSetList := remoteReads(b, remoteClient, s.env.Namespace)
				if len(machineSetList.Items) != 0 {
					b.Fatalf("expected no existing MachineSets, got %d", len(machineSetList.Items))
				}
				toCreate := benchutil.GenerateMachineSet(fmt.Sprintf("worker-%d", i))
				toCreate.Namespace = s.env.Namespace
				if err := remoteClient.Create(context.Background(), toCreate); err != nil {
					b.Fatalf("Create failed: %v", err)
				}
				listMachinesForMachineSet(b, remoteClient, toCreate)

				b.StopTimer()
				err = s.env.SeedClient.Delete(context.Background(), toCreate)
				b.StartTimer()
				if err != nil && !apierrors.IsNotFound(err) {
					b.Fatalf("inter-iteration MachineSet cleanup failed: %v", err)
				}
			},
		}.Run(b)
	})

	// SteadyState: the remote MachineSet is healthy (ReadyReplicas matches
	// Replicas) and matches the generated set, so the real reconcile performs
	// no remote write and no summarize List: reads only. This is the cost
	// every poll-interval requeue pays when nothing changed.
	b.Run("SteadyState", func(b *testing.B) {
		benchutil.ControllerHarness[*mpState]{
			Setup: func(b *testing.B, env *benchutil.BenchEnv) *mpState {
				seedRemoteState(b, env)
				ms := benchutil.GenerateMachineSet("worker")
				ms.Namespace = env.Namespace
				ms.Status.ReadyReplicas = *ms.Spec.Replicas
				if err := env.SeedClient.Create(context.Background(), ms); err != nil {
					b.Fatalf("failed to seed MachineSet: %v", err)
				}
				return &mpState{
					BenchRemoteClient: benchutil.SetupRemoteClient(b, env),
					env:               env,
				}
			},
			Reconcile: func(b *testing.B, s *mpState, _ []client.Object, _ int) {
				remoteClient, err := s.NewBuilder().Build()
				if err != nil {
					b.Fatalf("Build failed: %v", err)
				}
				machineSetList := remoteReads(b, remoteClient, s.env.Namespace)
				if len(machineSetList.Items) != 1 {
					b.Fatalf("expected 1 existing MachineSet, got %d", len(machineSetList.Items))
				}
				// syncMachineSets matches the generated set against the listed
				// one in memory and finds nothing to modify: no remote write.
			},
		}.Run(b)
	})

	// Scaling: the desired replica count differs from the remote MachineSet's
	// each iteration, so syncMachineSets Updates it (a real write every
	// iteration; the object does not grow with b.N), and the status pass pays
	// the summarize Machines List (replicas no longer match readyReplicas
	// once updated). Worker Machines matching the MachineSet's selector are
	// seeded so that List returns realistic content.
	b.Run("Scaling", func(b *testing.B) {
		benchutil.ControllerHarness[*mpState]{
			Setup: func(b *testing.B, env *benchutil.BenchEnv) *mpState {
				seedRemoteState(b, env)
				ms := benchutil.GenerateMachineSet("worker")
				ms.Namespace = env.Namespace
				ms.Status.ReadyReplicas = *ms.Spec.Replicas
				if err := env.SeedClient.Create(context.Background(), ms); err != nil {
					b.Fatalf("failed to seed MachineSet: %v", err)
				}
				for i := 0; i < 3; i++ {
					machine := &machinev1beta1.Machine{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("worker-machine-%d", i),
							Namespace: env.Namespace,
							Labels:    map[string]string{"machine.openshift.io/cluster-api-machineset": "worker"},
						},
					}
					if err := env.SeedClient.Create(context.Background(), machine); err != nil {
						b.Fatalf("failed to seed worker Machine: %v", err)
					}
				}
				return &mpState{
					BenchRemoteClient: benchutil.SetupRemoteClient(b, env),
					env:               env,
				}
			},
			Reconcile: func(b *testing.B, s *mpState, _ []client.Object, i int) {
				remoteClient, err := s.NewBuilder().Build()
				if err != nil {
					b.Fatalf("Build failed: %v", err)
				}
				machineSetList := remoteReads(b, remoteClient, s.env.Namespace)
				if len(machineSetList.Items) != 1 {
					b.Fatalf("expected 1 existing MachineSet, got %d", len(machineSetList.Items))
				}
				// syncMachineSets: match in memory against the listed object,
				// then Update it with the new replica count.
				ms := &machineSetList.Items[0]
				newReplicas := int32(i + 1)
				ms.Spec.Replicas = &newReplicas
				if err := remoteClient.Update(context.Background(), ms); err != nil {
					b.Fatalf("Update failed: %v", err)
				}
				listMachinesForMachineSet(b, remoteClient, ms)
			},
		}.Run(b)
	})
}

// Pattern 4: Build + BuildKubeClient -- models hibernation.

func BenchmarkControllerReconcileHibernation(b *testing.B) {
	// hibState carries the env: BenchRemoteClient hides its own, and the
	// Machine List below needs the namespace for deterministic response sizes.
	type hibState struct {
		*benchutil.BenchRemoteClient
		env *benchutil.BenchEnv
	}

	benchutil.ControllerHarness[*hibState]{
		Setup: func(b *testing.B, env *benchutil.BenchEnv) *hibState {
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
			return &hibState{
				BenchRemoteClient: benchutil.SetupRemoteClient(b, env),
				env:               env,
			}
		},
		Reconcile: func(b *testing.B, s *hibState, _ []client.Object, _ int) {
			// First client: controller-runtime (Build)
			remoteClient, err := s.NewBuilder().Build()
			if err != nil {
				b.Fatalf("Build failed: %v", err)
			}

			// nodesReady: List Nodes. Nodes are cluster-scoped, so cluster-wide
			// is the only possible scope; nothing creates them under envtest,
			// so the response stays empty and deterministic. Real-cluster runs
			// see the real Nodes -- see the README's real-cluster caveats.
			nodeList := &corev1.NodeList{}
			if err := remoteClient.List(context.Background(), nodeList); err != nil {
				b.Fatalf("List Nodes failed: %v", err)
			}

			// checkCSRs (the nodes-not-ready phase of a resume): List Machines,
			// then the CSR check via the typed kubernetes client. The real
			// controller's List is cluster-wide; the sim scopes it to the
			// benchmark namespace for the same reason as the machinepool sim's
			// Lists (see BenchmarkControllerReconcileMachinePool): on a bare
			// API server without the namespace controller, objects linger in
			// Terminating namespaces, so a cluster-wide List would grow with
			// earlier benchmarks' leftovers and depend on execution order. The
			// sim unions this phase with the nodes-ready phase below
			// (operatorsReady) since both are hot in production; within one
			// real reconcile only one of the two runs.
			machineList := &machinev1beta1.MachineList{}
			if err := remoteClient.List(context.Background(), machineList,
				client.InNamespace(s.env.Namespace)); err != nil {
				b.Fatalf("List Machines failed: %v", err)
			}

			// Second client: typed kubernetes (BuildKubeClient)
			kubeClient, err := s.NewBuilder().BuildKubeClient()
			if err != nil {
				b.Fatalf("BuildKubeClient failed: %v", err)
			}

			if _, err := kubeClient.CertificatesV1().CertificateSigningRequests().List(context.Background(), metav1.ListOptions{}); err != nil {
				b.Fatalf("kube List CSRs failed: %v", err)
			}

			// operatorsReady (the nodes-ready phase): List ClusterOperators.
			coList := &configv1.ClusterOperatorList{}
			if err := remoteClient.List(context.Background(), coList); err != nil {
				b.Fatalf("List ClusterOperators failed: %v", err)
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
