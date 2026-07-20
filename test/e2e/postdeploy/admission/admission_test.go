package admission

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"

	"github.com/openshift/hive/pkg/util/scheme"
	webhook "github.com/openshift/hive/pkg/validating-webhooks/hive/v1"
	"github.com/openshift/hive/test/e2e/common"
)

const (
	hiveAdmissionDeployment = "hiveadmission"
	hiveAdmissionAPIService = "v1.admission.hive.openshift.io"
)

func waitForAdmissionDeployment(t *testing.T) bool {
	client := common.MustGetKubernetesClient()
	err := common.WaitForDeploymentReady(client, common.GetHiveNamespaceOrDie(), hiveAdmissionDeployment, 10*time.Minute)
	if err != nil {
		t.Errorf("Failed waiting for hive admission deployment: %v", err)
		return false
	}
	return true
}

func waitForAdmissionAPIService(t *testing.T) bool {
	client := common.MustGetAPIRegistrationClient()
	err := common.WaitForAPIServiceAvailable(client, hiveAdmissionAPIService, 10*time.Minute)
	if err != nil {
		t.Errorf("Failed waiting for admission API service to be ready: %v", err)
		return false
	}
	return true
}

func waitForAdmission(t *testing.T) bool {
	return waitForAdmissionDeployment(t) && waitForAdmissionAPIService(t)
}

// waitForAdmissionRouting sends a smoke request to the aggregated API endpoint
// in a poll loop, working around a race where kube-apiserver reports the
// APIService as Available but has not yet fully configured request routing.
// Until routing is ready the request hits native RBAC and is rejected with
// Forbidden for system:anonymous.
func waitForAdmissionRouting(t *testing.T, c dynamic.Interface, gvr schema.GroupVersionResource) bool {
	// Build a minimal AdmissionReview that the webhook will accept.
	probe := &unstructured.Unstructured{}
	probe.SetUnstructuredContent(map[string]interface{}{
		"apiVersion": "admission.k8s.io/v1",
		"kind":       "AdmissionReview",
		"request": map[string]interface{}{
			"kind": map[string]interface{}{
				"group":   "hive.openshift.io",
				"version": "v1",
				"kind":    "DNSZone",
			},
			"resource": map[string]interface{}{
				"group":    "hive.openshift.io",
				"version":  "v1",
				"resource": "dnszones",
			},
			"operation": "CREATE",
			"object": map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "routing-probe",
				},
				"spec": map[string]interface{}{
					"zone": "probe.example.com",
				},
			},
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	err := wait.PollUntilContextTimeout(ctx, 1*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		_, err := c.Resource(gvr).Create(ctx, probe, metav1.CreateOptions{})
		if err != nil {
			// The Forbidden error for system:anonymous means kube-apiserver
			// has not yet wired up routing to the aggregated API server.
			if strings.Contains(err.Error(), "is forbidden") {
				t.Logf("Admission routing not ready yet: %v", err)
				return false, nil
			}
			return false, fmt.Errorf("unexpected error during admission routing probe: %w", err)
		}
		return true, nil
	})
	if err != nil {
		t.Errorf("Timed out waiting for admission routing to be ready: %v", err)
		return false
	}
	t.Log("Admission routing is ready")
	return true
}

func TestAdmission(t *testing.T) {
	c := common.MustGetDynamicClient()
	gvr, _ := (&webhook.DNSZoneValidatingAdmissionHook{}).ValidatingResource()
	if !waitForAdmission(t) {
		return
	}
	if !waitForAdmissionRouting(t, c, gvr) {
		return
	}

	tests := []struct {
		name          string
		file          string
		expectAllowed bool
	}{
		{
			name:          "review failure",
			file:          "hiveadmission-review-failure.json",
			expectAllowed: false,
		},
		{
			name:          "review success",
			file:          "hiveadmission-review-success.json",
			expectAllowed: true,
		},
		{
			name:          "review update failure",
			file:          "hiveadmission-review-update-failure.json",
			expectAllowed: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file := filepath.Join("testdata", test.file)
			b, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("Unexpected file read error: %v", err)
			}
			obj := &unstructured.Unstructured{}
			err = obj.UnmarshalJSON(b)
			if err != nil {
				t.Fatalf("Unexpected unmarshal error: %v", err)
			}
			result, err := c.Resource(gvr).Create(context.Background(), obj, metav1.CreateOptions{})
			if err != nil {
				t.Fatalf("Unexpected create error: %v", err)
			}
			reviewResult := &admissionv1.AdmissionReview{}
			scheme := scheme.GetScheme()
			err = scheme.Convert(result, reviewResult, nil)
			if err != nil {
				t.Fatalf("Unexpected conversion error: %v", err)
			}
			if reviewResult.Response.Allowed != test.expectAllowed {
				t.Errorf("Unexpected response: %v", reviewResult.Response.Allowed)
			}
		})
	}
}
