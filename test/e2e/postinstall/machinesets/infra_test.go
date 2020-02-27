package machinesets

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	machinev1 "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	autoscalingv1 "github.com/openshift/cluster-autoscaler-operator/pkg/apis/autoscaling/v1"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/pkg/apis/hive/v1/aws"
	hivev1azure "github.com/openshift/hive/pkg/apis/hive/v1/azure"
	"github.com/openshift/hive/test/e2e/common"
)

func TestScaleMachinePool(t *testing.T) {
	cd := common.MustGetInstalledClusterDeployment()
	cfg := common.MustGetClusterDeploymentClientConfig()

	switch p := cd.Spec.Platform; {
	case p.AWS != nil:
	case p.Azure != nil:
	case p.GCP != nil:
	default:
		t.Log("Scaling the machine pool is only implemented for AWS, Azure, and GCP")
		return
	}

	c := common.MustGetClient()

	// Scale down
	pool := common.GetMachinePool(cd, "worker")
	if pool == nil {
		t.Fatal("worker machine pool does not exist")
	}
	pool.Spec.Replicas = pointer.Int64Ptr(1)
	if err := c.Update(context.TODO(), pool); err != nil {
		t.Fatalf("cannot update worker machine pool to reduce replicas: %v", err)
	}
	if err := waitForMachines(cfg, cd, "worker", 1); err != nil {
		t.Errorf("timed out waiting for machines to be created")
	}
	if err := waitForNodes(cfg, cd, "worker", 1); err != nil {
		t.Errorf("timed out waiting for nodes to be created")
	}

	// Scale up
	pool = common.GetMachinePool(cd, "worker")
	if pool == nil {
		t.Fatal("worker machine pool does not exist")
	}
	pool.Spec.Replicas = pointer.Int64Ptr(3)
	if err := c.Update(context.TODO(), pool); err != nil {
		t.Fatalf("cannot update worker machine pool to increase replicas: %v", err)
	}
	if err := waitForMachines(cfg, cd, "worker", 3); err != nil {
		t.Errorf("timed out waiting for machines to be created")
	}
	if err := waitForNodes(cfg, cd, "worker", 3); err != nil {
		t.Errorf("timed out waiting for nodes to be created")
	}
}

func TestNewMachinePool(t *testing.T) {
	cd := common.MustGetInstalledClusterDeployment()
	cfg := common.MustGetClusterDeploymentClientConfig()

	c := common.MustGetClient()

	if pool := common.GetMachinePool(cd, "infra"); pool != nil {
		t.Fatal("infra machine pool already exists")
	}

	infraMachinePool := &hivev1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cd.Namespace,
			Name:      fmt.Sprintf("%s-%s", cd.Name, "infra"),
		},
		Spec: hivev1.MachinePoolSpec{
			ClusterDeploymentRef: corev1.LocalObjectReference{Name: cd.Name},
			Name:                 "infra",
			Replicas:             pointer.Int64Ptr(3),
			Labels: map[string]string{
				"openshift.io/machine-type": "infra",
			},
			Taints: []corev1.Taint{
				{
					Key:    "openshift.io/compute",
					Value:  "true",
					Effect: corev1.TaintEffectPreferNoSchedule,
				},
			},
		},
	}

	switch p := cd.Spec.Platform; {
	case p.AWS != nil:
	infraMachinePool.Spec.Platform = hivev1.MachinePoolPlatform{
		AWS: &hivev1aws.MachinePoolPlatform{
			InstanceType: "m4.large",
			EC2RootVolume: hivev1aws.EC2RootVolume{
				IOPS: 100,
				Size: 22,
				Type: "gp2",
			},
		},
	}
	case p.Azure != nil:
		infraMachinePool.Spec.Platform = hivev1.MachinePoolPlatform{
			Azure: &hivev1azure.MachinePool{
				InstanceType: "Standard_D2s_v3",
				OSDisk: hivev1azure.OSDisk{
					DiskSizeGB: 128,
				},
			},
		}
	default:
		t.Log("Adding machine pools is only implemented for AWS and Azure")
		return
	}

	err := c.Create(context.TODO(), infraMachinePool)
	if err != nil {
		t.Fatalf("cannot create infra machine pool: %v", err)
	}

	// Wait for machines to be created
	t.Logf("Waiting for 3 infra machines to be created")
	if err := waitForMachines(cfg, cd, "infra", 3); err != nil {
		t.Errorf("timed out waiting for machines to be created")
	}

	if err := waitForNodes(cfg, cd, "infra", 3,
		// Ensure that labels were applied to the nodes
		func(node *corev1.Node) bool {
			if machineType := node.Labels["openshift.io/machine-type"]; machineType != "infra" {
				t.Logf("Did not find expected label in node")
				return false
			}
			return true
		},
		// Ensure that taints were applied to the nodes
		func(node *corev1.Node) bool {
			for _, taint := range node.Spec.Taints {
				if taint.Key == "openshift.io/compute" && taint.Value == "true" {
					return true
				}
			}
			t.Logf("Did not find expected taint in node")
			return false
		},
	); err != nil {
		t.Errorf("timed out waiting for nodes to be created")
	}

	// Now remove the infra machinepool and make sure that any machinesets associated
	// with it are removed
	infraMachinePool = common.GetMachinePool(cd, "infra")
	if infraMachinePool == nil {
		t.Fatalf("could not find infra machine pool")
	}
	if err := c.Delete(context.TODO(), infraMachinePool); err != nil {
		t.Fatalf("cannot delete infra machine pool: %v", err)
	}

	if err := common.WaitForMachineSets(
		cfg,
		func(machineSets []*machinev1.MachineSet) bool {
			for _, ms := range machineSets {
				if strings.HasPrefix(ms.Name, machineNamePrefix(cd, "infra")) {
					return false
				}
			}
			return true
		},
		5*time.Minute,
	); err != nil {
		t.Fatalf("timed out waiting for machinesets: %v", err)
	}
}

// TestAutoscalineMachinePool tests the features of an auto-scaling machine pool.
// 1) The test changes the default worker pool to be a auto-scaling.
// 2) The test creates a busybox deployment for pods that do not do anything
//    but that make large CPU requests. The replica count of the deployment
//    is set to a very large number to ensure that the pods will overwhlem
//    the CPU available for the worker pool even when scaled out to the max
//    replicas of the pool.
// 3) The test verifies that machines (and correspondingly nodes) have been
//    created to reach the max replicas for the pool.
// 4) The test deletes the busybox deployment. This will relieve the CPU pressure
//    on the worker pool. The autoscaler will scale the pool back down to the
//    min replicas.
// 5) The test verifies that machines and nodes have been deleted to reach
//    the min replicas of the pool.
// 6) The test changes the default worker pool back to use an explicit replica
//    count.
// 7) The test verifies that machines and nodes have been deleted to reach
//    the explicit replica count.
func TestAutoscalingMachinePool(t *testing.T) {
	// minReplicas is the minimum number of replicas for the machine pool.
	// It is set to 10 to ensure that it is at least as large as the number
	// of zones in whichever platform and region the test is running on. If
	// the min replicas is less than the number of zones, then the machine pool
	// controller will reject the machine pool.
	const minReplicas = 10
	const maxReplicas = 12

	cd := common.MustGetInstalledClusterDeployment()
	cfg := common.MustGetClusterDeploymentClientConfig()

	switch p := cd.Spec.Platform; {
	case p.AWS != nil:
	case p.Azure != nil:
	case p.GCP != nil:
	default:
		t.Log("Scaling the machine pool is only implemented for AWS, Azure, and GCP")
	}

	c := common.MustGetClient()
	rc := common.MustGetClientFromConfig(cfg)

	// Scale up
	pool := common.GetMachinePool(cd, "worker")
	if pool == nil {
		t.Fatal("worker machine pool does not exist")
	}
	pool.Spec.Replicas = nil
	pool.Spec.Autoscaling = &hivev1.MachinePoolAutoscaling{
		MinReplicas: minReplicas,
		MaxReplicas: maxReplicas,
	}
	if err := c.Update(context.TODO(), pool); err != nil {
		t.Fatalf("cannot update worker machine pool to reduce replicas: %v", err)
	}

	// Lower autoscaler delays so that scaling down happens faster
	clusterAutoscaler := &autoscalingv1.ClusterAutoscaler{}
	for i := 0; i < 10; i++ {
		switch err := rc.Get(context.Background(), client.ObjectKey{Name: "default"}, clusterAutoscaler); {
		case apierrors.IsNotFound(err):
			t.Log("waiting for Hive to create cluster autoscaler")
			time.Sleep(10 * time.Second)
		case err != nil:
			t.Fatalf("could not get the cluster autoscaler: %v", err)
		}
	}
	if clusterAutoscaler.Name == "" {
		t.Fatalf("timed out waiting for cluster autoscaler")
	}
	if clusterAutoscaler.Spec.ScaleDown == nil {
		clusterAutoscaler.Spec.ScaleDown = &autoscalingv1.ScaleDownConfig{}
	}
	clusterAutoscaler.Spec.ScaleDown.DelayAfterAdd = pointer.StringPtr("10s")
	clusterAutoscaler.Spec.ScaleDown.DelayAfterDelete = pointer.StringPtr("10s")
	clusterAutoscaler.Spec.ScaleDown.DelayAfterFailure = pointer.StringPtr("10s")
	clusterAutoscaler.Spec.ScaleDown.UnneededTime = pointer.StringPtr("10s")
	if err := rc.Update(context.Background(), clusterAutoscaler); err != nil {
		t.Fatalf("could not update the cluster autoscaler: %v", err)
	}

	// busyboxDeployment creates a large number of pods to place CPU pressure
	// on the machine pool. With 100 replicas and a CPU request for each pod of
	// 1, the total CPU request from the deployment is 100. For AWS using m4.xlarge,
	// each machine has a CPU limit of 4. For the max replicas of 12, the total
	// CPU limit is 48.
	busyboxDeployment := &extensionsv1beta1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "busybox",
		},
		Spec: extensionsv1beta1.DeploymentSpec{
			Replicas: pointer.Int32Ptr(100),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"scaling-app": "busybox",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"scaling-app": "busybox",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:    "sleep",
						Image:   "busybox",
						Command: []string{"sleep", "3600"},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU: func() resource.Quantity {
									q, err := resource.ParseQuantity("1")
									if err != nil {
										t.Fatalf("could not parse quantity")
									}
									return q
								}(),
							},
						},
					}},
				},
			},
		},
	}
	if err := rc.Create(context.TODO(), busyboxDeployment); err != nil {
		t.Fatalf("cannot create busybox deployment: %v", err)
	}

	if err := waitForMachines(cfg, cd, "worker", maxReplicas); err != nil {
		t.Errorf("timed out waiting for machines to be created")
	}
	if err := waitForNodes(cfg, cd, "worker", maxReplicas); err != nil {
		t.Errorf("timed out waiting for nodes to be created")
	}

	// Scale down
	if err := rc.Get(context.TODO(), client.ObjectKey{Namespace: "default", Name: "busybox"}, busyboxDeployment); err != nil {
		t.Fatalf("could not get busybox deployment: %v", err)
	}
	busyboxDeployment.Spec.Replicas = pointer.Int32Ptr(0)
	if err := rc.Update(context.TODO(), busyboxDeployment); err != nil {
		t.Fatalf("could not update busybox deployment to scale down replicas: %v", err)
	}
	if err := waitForMachines(cfg, cd, "worker", minReplicas); err != nil {
		t.Errorf("timed out waiting for machines to be created")
	}
	if err := waitForNodes(cfg, cd, "worker", minReplicas); err != nil {
		t.Errorf("timed out waiting for nodes to be created")
	}

	// Disable auto-scaling
	pool = common.GetMachinePool(cd, "worker")
	if pool == nil {
		t.Fatal("worker machine pool does not exist")
	}
	pool.Spec.Replicas = pointer.Int64Ptr(3)
	pool.Spec.Autoscaling = nil
	if err := c.Update(context.TODO(), pool); err != nil {
		t.Fatalf("cannot update worker machine pool to turn off auto-scaling: %v", err)
	}
	if err := waitForMachines(cfg, cd, "worker", 3); err != nil {
		t.Errorf("timed out waiting for machines to be created")
	}
	if err := waitForNodes(cfg, cd, "worker", 3); err != nil {
		t.Errorf("timed out waiting for nodes to be created")
	}
}

func waitForMachines(cfg *rest.Config, cd *hivev1.ClusterDeployment, poolName string, expectedReplicas int) error {
	return common.WaitForMachines(cfg, func(machines []*machinev1.Machine) bool {
		count := 0
		for _, m := range machines {
			if strings.HasPrefix(m.Name, machineNamePrefix(cd, poolName)) {
				count++
			}
		}
		return count == expectedReplicas
	}, 10*time.Minute)
}

func waitForNodes(cfg *rest.Config, cd *hivev1.ClusterDeployment, poolName string, expectedReplicas int, extraChecks ...func(node *corev1.Node) bool) error {
	return common.WaitForNodes(cfg, func(nodes []*corev1.Node) bool {
		poolNodes := []*corev1.Node{}
		for _, n := range nodes {
			if n.Annotations == nil {
				continue
			}
			machineAnnotation := n.Annotations["machine.openshift.io/machine"]
			name := strings.Split(machineAnnotation, "/")
			if len(name) < 2 {
				continue
			}
			machineName := name[1]
			if strings.HasPrefix(machineName, machineNamePrefix(cd, poolName)) {
				poolNodes = append(poolNodes, n)
			}
		}
		if len(poolNodes) != expectedReplicas {
			return false
		}

		for _, c := range extraChecks {
			for _, n := range poolNodes {
				if !c(n) {
					return false
				}
			}
		}

		return true
	}, 10*time.Minute)
}

func machineNamePrefix(cd *hivev1.ClusterDeployment, poolName string) string {
	switch p := cd.Spec.Platform; {
	case p.GCP != nil:
		return fmt.Sprintf("%s-%s-", cd.Spec.ClusterMetadata.InfraID, poolName[:1])
	default:
		return fmt.Sprintf("%s-%s-", cd.Spec.ClusterMetadata.InfraID, poolName)
	}
}
