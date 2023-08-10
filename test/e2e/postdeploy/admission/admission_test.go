package admission

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

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

func TestAdmission(t *testing.T) {
	c := common.MustGetDynamicClient()
	gvr, _ := (&webhook.DNSZoneValidatingAdmissionHook{}).ValidatingResource()
	if !waitForAdmission(t) {
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
			reviewResult := &admissionv1beta1.AdmissionReview{}
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
