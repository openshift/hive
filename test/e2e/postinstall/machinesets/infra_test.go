package machinesets

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/openshift/hive/test/e2e/common"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	machinev1 "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/pkg/apis/hive/v1/aws"
)

func TestManageMachineSets(t *testing.T) {
	cd := common.MustGetInstalledClusterDeployment()

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
	cfg := common.MustGetClusterDeploymentClientConfig()
	err = common.WaitForMachines(cfg, func(machines []*machinev1.Machine) bool {
		count := 0
		for _, m := range machines {
			if strings.HasPrefix(m.Name, fmt.Sprintf("%s-%s", cd.Spec.ClusterMetadata.InfraID, "infra")) {
				count++
			}
		}
		return count >= 3
	}, 5*time.Minute)

	if err != nil {
		t.Errorf("timed out waiting for machines to be created")
	}

	t.Logf("Waiting for nodes to be created")
	err = common.WaitForNodes(cfg, func(nodes []*corev1.Node) bool {
		infraNodes := []*corev1.Node{}
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
			if strings.HasPrefix(machineName, fmt.Sprintf("%s-%s", cd.Spec.ClusterMetadata.InfraID, "infra")) {
				infraNodes = append(infraNodes, n)
			}
		}
		if len(infraNodes) < 3 {
			return false
		}

		// Ensure that labels and taints were applied to the nodes
		for _, node := range infraNodes {
			if node.Labels == nil {
				return false
			}
			if machineType := node.Labels["openshift.io/machine-type"]; machineType != "infra" {
				t.Logf("Did not find expected label in node")
				return false
			}
			found := false
			for _, taint := range node.Spec.Taints {
				if taint.Key == "openshift.io/compute" && taint.Value == "true" {
					found = true
				}
			}
			if !found {
				t.Logf("Did not find expected taint in node")
				return false
			}
		}
		return true
	}, 10*time.Minute)

	if err != nil {
		t.Errorf("timed out waiting for nodes to be created")
	}

	// Now reduce the number of machines to 1 in the machine pool and wait for there to be only one machine
	infraMachinePool = common.GetMachinePool(cd, "infra")
	if infraMachinePool == nil {
		t.Fatal("could not find infra machine pool")
	}
	infraMachinePool.Spec.Replicas = pointer.Int64Ptr(1)
	if err := c.Update(context.TODO(), infraMachinePool); err != nil {
		t.Fatalf("cannot update infra machine pool to reduce replicas: %v", err)
	}

	common.WaitForMachines(cfg, func(machines []*machinev1.Machine) bool {
		count := 0
		for _, m := range machines {
			if strings.HasPrefix(m.Name, fmt.Sprintf("%s-%s", cd.Spec.ClusterMetadata.InfraID, "infra")) {
				count++
			}
		}
		return count == 1
	}, 5*time.Minute)

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
			if strings.HasPrefix(ms.Name, fmt.Sprintf("%s-%s", cd.Spec.ClusterMetadata.InfraID, "infra")) {
				count++
			}
		}
		return count == 0
	}, 5*time.Minute)
}
