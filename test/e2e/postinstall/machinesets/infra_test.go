package machinesets

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/openshift/hive/test/e2e/common"
	corev1 "k8s.io/api/core/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"

	machinev1 "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/pkg/apis/hive/v1/aws"
)

func TestScaleMachinePool(t *testing.T) {
	cd := common.MustGetInstalledClusterDeployment()
	cfg := common.MustGetClusterDeploymentClientConfig()

	switch p := cd.Spec.Platform; {
	case p.AWS == nil:
	case p.GCP == nil:
	default:
		t.Log("Scaling the machine pool is only implemented for AWS and GCP")
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

	if cd.Spec.Platform.AWS == nil {
		t.Log("Remote machineset management is only implemented for AWS")
		return
	}

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
			Platform: hivev1.MachinePoolPlatform{
				AWS: &hivev1aws.MachinePoolPlatform{
					InstanceType: "m4.large",
					EC2RootVolume: hivev1aws.EC2RootVolume{
						IOPS: 100,
						Size: 22,
						Type: "gp2",
					},
				},
			},
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

	common.WaitForMachineSets(cfg, func(machineSets []*machinev1.MachineSet) bool {
		count := 0
		for _, ms := range machineSets {
			if strings.HasPrefix(ms.Name, machineNamePrefix(cd, "infra")) {
				count++
			}
		}
		return count == 0
	}, 5*time.Minute)
}

func TestAutoscalingMachinePool(t *testing.T) {
	cd := common.MustGetInstalledClusterDeployment()
	cfg := common.MustGetClusterDeploymentClientConfig()

	switch p := cd.Spec.Platform; {
	case p.AWS == nil:
	case p.GCP == nil:
	default:
		t.Log("Scaling the machine pool is only implemented for AWS and GCP")
	}

	c := common.MustGetClient()

	// Scale up
	pool := common.GetMachinePool(cd, "worker")
	if pool == nil {
		t.Fatal("worker machine pool does not exist")
	}
	pool.Spec.Replicas = nil
	pool.Spec.Autoscaling = &hivev1.MachinePoolAutoscaling{
		MinReplicas: 10,
		MaxReplicas: 12,
	}
	if err := c.Update(context.TODO(), pool); err != nil {
		t.Fatalf("cannot update worker machine pool to reduce replicas: %v", err)
	}

	busyboxDeployment := &extensionsv1beta1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "busybox",
		},
		Spec: extensionsv1beta1.DeploymentSpec{
			Replicas: pointer.Int32Ptr(50),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:    "sleep",
						Image:   "busybox",
						Command: []string{"sleep", "3600"},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU: func() resource.Quantity {
									q, err := resource.ParseQuantity("2")
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
	if err := c.Create(context.TODO(), busyboxDeployment); err != nil {
		t.Fatalf("cannot create busybox deployment: %v", err)
	}

	if err := waitForMachines(cfg, cd, "worker", 12); err != nil {
		t.Errorf("timed out waiting for machines to be created")
	}
	if err := waitForNodes(cfg, cd, "worker", 12); err != nil {
		t.Errorf("timed out waiting for nodes to be created")
	}

	// Scale down
	if err := c.Get(context.TODO(), client.ObjectKey{Name: "busybox"}, busyboxDeployment); err != nil {
		t.Fatalf("could not get busybox deployment: %v", err)
	}
	busyboxDeployment.Spec.Replicas = pointer.Int32Ptr(0)
	if err := c.Update(context.TODO(), busyboxDeployment); err != nil {
		t.Fatalf("could not update busybox deployment to scale down replicas: %v", err)
	}
	if err := waitForMachines(cfg, cd, "worker", 10); err != nil {
		t.Errorf("timed out waiting for machines to be created")
	}
	if err := waitForNodes(cfg, cd, "worker", 10); err != nil {
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
	}, 5*time.Minute)
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
