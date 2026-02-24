package machinesets

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	machinev1 "github.com/openshift/api/machine/v1beta1"
	autoscalingv1 "github.com/openshift/cluster-autoscaler-operator/pkg/apis/autoscaling/v1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	hivev1azure "github.com/openshift/hive/apis/hive/v1/azure"
	hivev1gcp "github.com/openshift/hive/apis/hive/v1/gcp"
	"github.com/openshift/hive/pkg/clusterresource"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/test/e2e/common"
)

const (
	workerMachinePoolName = "worker"
	infraMachinePoolName  = "infra"
)

func TestScaleMachinePool(t *testing.T) {
	cd := common.MustGetInstalledClusterDeployment()
	cfg := common.MustGetClusterDeploymentClientConfig()
	logger := log.WithField("test", "TestScaleMachinePool")

	switch p := cd.Spec.Platform; {
	case p.AWS != nil:
	case p.Azure != nil:
	case p.GCP != nil:
	default:
		t.Log("Scaling the machine pool is only implemented for AWS, Azure, and GCP")
		return
	}

	c := common.MustGetClient()
	machinePrefix, err := machineNamePrefix(cd, workerMachinePoolName)
	require.NoError(t, err, "cannot determine machine name prefix")

	// Scale down
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		pool := common.GetMachinePool(c, cd, workerMachinePoolName)
		require.NotNilf(t, pool, "worker machine pool does not exist: %s", workerMachinePoolName)

		logger = logger.WithField("pool", pool.Name)
		logger.Infof("expected Machine name prefix: %s", machinePrefix)

		logger.Info("scaling pool to 1 replicas")
		pool.Spec.Replicas = ptr.To(int64(1))
		return c.Update(context.TODO(), pool)
	})
	require.NoError(t, err, "cannot update worker machine pool to reduce replicas")

	err = waitForMachines(logger, cfg, cd, machinePrefix, 1)
	require.NoError(t, err, "timed out waiting for machines to be scaled down")

	err = waitForNodes(logger, cfg, cd, machinePrefix, 1)
	require.NoError(t, err, "timed out waiting for nodes to be scaled down")

	// Scale up
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		pool := common.GetMachinePool(c, cd, workerMachinePoolName)
		require.NotNilf(t, pool, "worker machine pool does not exist: %s", workerMachinePoolName)

		logger.Info("scaling pool back to 3 replicas")
		pool.Spec.Replicas = ptr.To(int64(3))
		return c.Update(context.TODO(), pool)
	})
	require.NoError(t, err, "cannot update worker machine pool to increase replicas")

	err = waitForMachines(logger, cfg, cd, machinePrefix, 3)
	require.NoError(t, err, "timed out waiting for machines to be scaled up")

	err = waitForNodes(logger, cfg, cd, machinePrefix, 3)
	require.NoError(t, err, "timed out waiting for nodes to be scaled up")
}

func TestNewMachinePool(t *testing.T) {
	cd := common.MustGetInstalledClusterDeployment()
	cfg := common.MustGetClusterDeploymentClientConfig()
	logger := log.WithField("test", "TestNewMachinePool")

	c := common.MustGetClient()

	pool := common.GetMachinePool(c, cd, infraMachinePoolName)
	require.Nil(t, pool, "infra machine pool already exists")

	infraMachinePool := &hivev1.MachinePool{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cd.Namespace,
			Name:      fmt.Sprintf("%s-%s", cd.Name, infraMachinePoolName),
		},
		Spec: hivev1.MachinePoolSpec{
			ClusterDeploymentRef: corev1.LocalObjectReference{Name: cd.Name},
			Name:                 infraMachinePoolName,
			Replicas:             ptr.To(int64(3)),
			Labels: map[string]string{
				"openshift.io/machine-type": infraMachinePoolName,
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
				InstanceType: clusterresource.AWSInstanceTypeDefault,
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
				InstanceType: "Standard_D4s_v3",
				OSDisk: hivev1azure.OSDisk{
					DiskSizeGB: 128,
				},
			},
		}
	case p.GCP != nil:
		infraMachinePool.Spec.Platform = hivev1.MachinePoolPlatform{
			GCP: &hivev1gcp.MachinePool{
				InstanceType: "n1-standard-4",
			},
		}
	default:
		logger.Info("Adding machine pools is only implemented for AWS, Azure and GCP")
		return
	}

	logger = logger.WithField("pool", infraMachinePool.Name)
	logger.Info("Creating MachinePool")
	err := c.Create(context.TODO(), infraMachinePool)
	require.NoError(t, err, "cannot create infra machine pool")

	machinePrefix, err := machineNamePrefix(cd, infraMachinePoolName)
	require.NoError(t, err, "cannot find/calculate machine name prefix")
	logger.Infof("expected Machine name prefix: %s", machinePrefix)

	// Wait for machines to be created
	t.Logf("Waiting for 3 infra machines to be created")
	err = waitForMachines(logger, cfg, cd, machinePrefix, 3)
	require.NoError(t, err, "timed out waiting for machines to be created")

	err = waitForNodes(logger, cfg, cd, machinePrefix, 3,
		// Ensure that labels were applied to the nodes
		func(node *corev1.Node) bool {
			if machineType := node.Labels["openshift.io/machine-type"]; machineType != infraMachinePoolName {
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
	)
	require.NoError(t, err, "timed out waiting for nodes to be created")

	// Now remove the infra machinepool and make sure that any machinesets associated
	// with it are removed
	logger.Info("removing pool")
	infraMachinePool = common.GetMachinePool(c, cd, infraMachinePoolName)
	require.NotNil(t, infraMachinePool, "could not find infra machine pool")
	err = c.Delete(context.TODO(), infraMachinePool)
	require.NoError(t, err, "cannot delete infra machine pool")

	err = common.WaitForMachineSets(
		cfg,
		func(machineSets []*machinev1.MachineSet) bool {
			for _, ms := range machineSets {
				if strings.HasPrefix(ms.Name, machinePrefix) {
					return false
				}
			}
			return true
		},
		5*time.Minute,
	)
	require.NoError(t, err, "timed out waiting for machinesets to be removed")
}

// TestAutoscalineMachinePool tests the features of an auto-scaling machine pool.
//  1. The test changes the default worker pool to be a auto-scaling.
//  2. The test creates a busybox deployment for pods that do not do anything
//     but that make large CPU requests. The replica count of the deployment
//     is set to a very large number to ensure that the pods will overwhlem
//     the CPU available for the worker pool even when scaled out to the max
//     replicas of the pool.
//  3. The test verifies that machines (and correspondingly nodes) have been
//     created to reach the max replicas for the pool.
//  4. The test deletes the busybox deployment. This will relieve the CPU pressure
//     on the worker pool. The autoscaler will scale the pool back down to the
//     min replicas.
//  5. The test verifies that machines and nodes have been deleted to reach
//     the min replicas of the pool.
//  6. The test changes the default worker pool back to use an explicit replica
//     count.
//  7. The test verifies that machines and nodes have been deleted to reach
//     the explicit replica count.
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
	logger := log.WithField("test", "TestAutoscalingMachinePool")

	switch p := cd.Spec.Platform; {
	case p.AWS != nil:
	case p.Azure != nil:
	case p.GCP != nil:
	default:
		logger.Info("Scaling the machine pool is only implemented for AWS, Azure, and GCP")
		return
	}

	c := common.MustGetClient()
	rc := common.MustGetClientFromConfig(cfg)

	var pool *hivev1.MachinePool

	// Scale up
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		pool = common.GetMachinePool(c, cd, workerMachinePoolName)
		require.NotNil(t, pool, "worker machine pool does not exist")

		logger.Info("switching pool from replicas to autoscaling")
		pool.Spec.Replicas = nil
		pool.Spec.Autoscaling = &hivev1.MachinePoolAutoscaling{
			MinReplicas: minReplicas,
			MaxReplicas: maxReplicas,
		}
		return c.Update(context.TODO(), pool)
	})
	require.NoError(t, err, "cannot update worker machine pool to reduce replicas")
	logger = logger.WithField("pool", pool.Name)

	machinePrefix, err := machineNamePrefix(cd, workerMachinePoolName)
	require.NoError(t, err, "cannot find/calculate machine name prefix")

	logger.Info("lowering autoscaler delay so scaling down happens faster")
	clusterAutoscaler := &autoscalingv1.ClusterAutoscaler{}
poll:
	for range 10 {
		switch err := rc.Get(context.Background(), client.ObjectKey{Name: "default"}, clusterAutoscaler); {
		case apierrors.IsNotFound(err):
			t.Log("waiting for Hive to create cluster autoscaler")
			time.Sleep(10 * time.Second)
		case err != nil:
			t.Fatalf("could not get the cluster autoscaler: %v", err)
		default:
			t.Log("found cluster autoscaler")
			break poll
		}
	}
	machineSetList := &machinev1.MachineSetList{}
	rc.List(context.Background(), machineSetList)
	for _, machineSet := range machineSetList.Items {
		// Only check machinesets that belong to this worker pool
		poolLabel, hasPoolLabel := machineSet.Labels["hive.openshift.io/machine-pool"]
		if !hasPoolLabel || poolLabel != pool.Spec.Name {
			continue
		}
		// Check labels
		require.Equal(t, "true", machineSet.Labels[constants.HiveManagedLabel], "Incorrect hive managed label on machineset")
		require.Equal(t, pool.Spec.Name, machineSet.Labels["hive.openshift.io/machine-pool"], "Incorrect machine pool label on machineset")
	}

	if clusterAutoscaler.Name == "" {
		t.Fatalf("timed out waiting for cluster autoscaler. MP: %#v \n Autoscaler: %#v", pool, clusterAutoscaler)
	}
	if clusterAutoscaler.Spec.ScaleDown == nil {
		clusterAutoscaler.Spec.ScaleDown = &autoscalingv1.ScaleDownConfig{}
	}
	clusterAutoscaler.Spec.ScaleDown.DelayAfterAdd = ptr.To("10s")
	clusterAutoscaler.Spec.ScaleDown.DelayAfterDelete = ptr.To("10s")
	clusterAutoscaler.Spec.ScaleDown.DelayAfterFailure = ptr.To("10s")
	clusterAutoscaler.Spec.ScaleDown.UnneededTime = ptr.To("10s")
	clusterAutoscaler.Spec.LogVerbosity = ptr.To[int32](4)
	err = rc.Update(context.Background(), clusterAutoscaler)
	require.NoError(t, err, "could not update the cluster autoscaler")

	// busyboxDeployment creates a large number of pods to place CPU pressure
	// on the machine pool. With 100 replicas and a CPU request for each pod of
	// 1, the total CPU request from the deployment is 100. For AWS using m6a.xlarge,
	// each machine has a CPU limit of 4. For the max replicas of 12, the total
	// CPU limit is 48.
	busyboxDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "busybox",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(100)),
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
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: ptr.To(false),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
						},
					}},
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
				},
			},
		},
	}
	err = rc.Create(context.TODO(), busyboxDeployment)
	require.NoError(t, err, "cannot create busybox deployment")

	err = waitForMachines(logger, cfg, cd, machinePrefix, maxReplicas)
	require.NoError(t, err, "timed out waiting for machines to be created")
	err = waitForNodes(logger, cfg, cd, machinePrefix, maxReplicas)
	require.NoError(t, err, "timed out waiting for nodes to be created")

	// Scale down
	err = rc.Get(context.TODO(), client.ObjectKey{Namespace: "default", Name: "busybox"}, busyboxDeployment)
	require.NoError(t, err, "could not get busybox deployment")

	logger.Info("deleting busybox deployment to relieve cpu pressure and scale down machines")
	err = rc.Delete(context.TODO(), busyboxDeployment, client.PropagationPolicy(metav1.DeletePropagationForeground))
	require.NoError(t, err, "could not delete busybox deployment")
	err = waitForMachines(logger, cfg, cd, machinePrefix, minReplicas)
	require.NoError(t, err, "timed out waiting for machine count")
	err = waitForNodes(logger, cfg, cd, machinePrefix, minReplicas)
	require.NoError(t, err, "timed out waiting for nodes to be created")

	logger.Info("disabling autoscaling")
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		pool = common.GetMachinePool(c, cd, "worker")
		require.NotNil(t, pool, "worker machine pool does not exist")

		pool.Spec.Replicas = ptr.To(int64(3))
		pool.Spec.Autoscaling = nil
		return c.Update(context.TODO(), pool)
	})
	require.NoError(t, err, "cannot update worker machine pool to turn off auto-scaling")
	err = waitForMachines(logger, cfg, cd, machinePrefix, 3)
	require.NoError(t, err, "timed out waiting for machines to be created")
	err = waitForNodes(logger, cfg, cd, machinePrefix, 3)
	require.NoError(t, err, "timed out waiting for nodes to be created")
}

func waitForMachines(logger log.FieldLogger, cfg *rest.Config, cd *hivev1.ClusterDeployment, machinePrefix string, expectedReplicas int) error {
	logger.Infof("waiting for %d machines with prefix '%s'", expectedReplicas, machinePrefix)
	lastCount := 0
	return common.WaitForMachines(cfg, func(machines []*machinev1.Machine) bool {
		count := 0
		for _, m := range machines {
			if strings.HasPrefix(m.Name, machinePrefix) {
				count++
			}
		}
		if count != lastCount {
			logger.Infof("found %d machines with prefix '%s'", count, machinePrefix)
			lastCount = count
		}
		return count == expectedReplicas
	}, 20*time.Minute)
}

func waitForNodes(logger log.FieldLogger, cfg *rest.Config, cd *hivev1.ClusterDeployment, machinePrefix string, expectedReplicas int, extraChecks ...func(node *corev1.Node) bool) error {
	logger.Infof("waiting for %d nodes with machine annotation prefix '%s'", expectedReplicas, machinePrefix)
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
			if strings.HasPrefix(machineName, machinePrefix) {
				poolNodes = append(poolNodes, n)
			}
		}
		if len(poolNodes) != expectedReplicas {
			return false
		}

		for _, c := range extraChecks {
			for _, n := range poolNodes {
				if !c(n) {
					logger.Info("extra checks not yet passing")
					return false
				}
			}
		}

		return true
	}, 15*time.Minute)
}

func machineNamePrefix(cd *hivev1.ClusterDeployment, poolName string) (string, error) {
	return fmt.Sprintf("%s-%s-", cd.Spec.ClusterMetadata.InfraID, poolName), nil
}
