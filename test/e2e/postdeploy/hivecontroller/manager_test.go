package hivecontroller

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	// "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"

	"github.com/openshift/hive/test/e2e/common"
)

const (
	hiveNamespace             = "hive"
	hiveControllersDeployment = "hive-controllers"
	hiveControllersService    = "hive-controllers"
)

func waitForManager(t *testing.T) bool {
	kubeClient := common.MustGetKubernetesClient()
	err := common.WaitForDeploymentReady(kubeClient, hiveNamespace, hiveControllersDeployment, 10*time.Minute)
	if err != nil {
		t.Errorf("Failed waiting for hive controllers deployment: %v", err)
		return false
	}
	return true
}

func TestHiveControllersDeployment(t *testing.T) {
	if !waitForManager(t) {
		return
	}
	// Ensure that the deployment has 1 available replica
	c := common.MustGetClient()
	deployment := &appsv1.Deployment{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: hiveControllersDeployment, Namespace: hiveNamespace}, deployment)
	if err != nil {
		t.Errorf("Failed to get hive controllers deployment: %v", err)
		return
	}
	if deployment.Status.AvailableReplicas != 1 {
		t.Errorf("Unexpected controller manager available replicas: %d", deployment.Status.AvailableReplicas)
	}
}

func TestHiveControllersMetrics(t *testing.T) {
	if !waitForManager(t) {
		return
	}

	c := common.MustGetClient()
	service := &corev1.Service{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: hiveControllersService, Namespace: hiveNamespace}, service)
	if err != nil {
		t.Errorf("Failed to get hive controllers service: %v", err)
		return
	}

	metricsPort := 0
	for _, port := range service.Spec.Ports {
		if port.Name == "metrics" {
			metricsPort = int(port.Port)
		}
	}
	if metricsPort == 0 {
		t.Errorf("cannot find metrics port in hive controllers service")
		return
	}

	kubeClient := common.MustGetKubernetesClient()

	// Query the metrics port in the hive-controllers service using the apiserver proxy
	body, err := kubeClient.CoreV1().RESTClient().Get().Namespace(hiveNamespace).Name(fmt.Sprintf("%s:%d", hiveControllersService, metricsPort)).Resource("services").SubResource("proxy").Suffix("metrics").DoRaw()
	if err != nil {
		t.Errorf("failed to reach metrics endpoint: %v", err)
		return
	}
	if !strings.Contains(string(body), "hive_cluster_deployment_install_job_delay_seconds_bucket") {
		t.Errorf("metrics response does not contain expected metric name")
	}
	t.Logf("metrics response:\n%s\n", string(body))
}
