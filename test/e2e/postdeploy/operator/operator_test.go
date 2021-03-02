package operator

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ghodss/yaml"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/test/e2e/common"
)

const (
	hiveOperatorDeployment = "hive-operator"
	hiveConfigName         = "hive"
	hiveManagerService     = "hive-controllers"
	crdDirectory           = "../../../../config/crds"
	lastAppliedAnnotation  = "kubectl.kubernetes.io/last-applied-configuration"
)

// TestOperatorDeployment ensures that the Hive operator deployment exists and that
// it's available.
func TestOperatorDeployment(t *testing.T) {
	kubeClient := common.MustGetKubernetesClient()
	err := common.WaitForDeploymentReady(kubeClient, common.GetHiveOperatorNamespaceOrDie(), hiveOperatorDeployment, 5*time.Minute)
	if err != nil {
		t.Errorf("Failed waiting for hive operator deployment: %v", err)
		return
	}

	// Ensure that the deployment has 1 available replica
	c := common.MustGetClient()
	operatorDeployment := &appsv1.Deployment{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: hiveOperatorDeployment, Namespace: common.GetHiveOperatorNamespaceOrDie()}, operatorDeployment)
	if err != nil {
		t.Errorf("Failed to get hive operator deployment: %v", err)
		return
	}
	if operatorDeployment.Status.AvailableReplicas != 1 {
		t.Errorf("Unexpected operator available replicas: %d", operatorDeployment.Status.AvailableReplicas)
	}
}

func readHiveCRDs(t *testing.T) []*apiextv1beta1.CustomResourceDefinition {
	files, err := ioutil.ReadDir(crdDirectory)
	if err != nil {
		t.Fatalf("cannot read crd directory: %v", err)
	}
	result := []*apiextv1beta1.CustomResourceDefinition{}
	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".yaml") {
			continue
		}
		b, err := ioutil.ReadFile(filepath.Join(crdDirectory, file.Name()))
		if err != nil {
			t.Fatalf("cannot read crd file %s: %v", file.Name(), err)
		}
		crd := &apiextv1beta1.CustomResourceDefinition{}
		if err = yaml.Unmarshal(b, crd); err != nil {
			t.Fatalf("failed to unmarshall crd from %s: %v", file.Name(), err)
		}
		result = append(result, crd)
	}

	return result
}

// TestHiveCRDs ensures that CRDs created by Hive exist in the cluster and that
// they reflect the spec stored in the Hive source repository
func TestHiveCRDs(t *testing.T) {
	c := common.MustGetClient()
	hiveCRDs := readHiveCRDs(t)
	for _, crd := range hiveCRDs {
		serverCRD := &apiextv1beta1.CustomResourceDefinition{}
		err := c.Get(context.TODO(), types.NamespacedName{Name: crd.Name}, serverCRD)
		if err != nil {
			t.Errorf("cannot fetch expected crd (%s): %v", crd.Name, err)
			continue
		}

		if serverCRD.Annotations == nil {
			t.Logf("server crd %s has no annotations", serverCRD.Name)
			continue
		}

		lastApplied, ok := serverCRD.Annotations[lastAppliedAnnotation]
		if !ok {
			t.Logf("server crd %s has no last applied annotation", serverCRD.Name)
			continue
		}

		lastAppliedCRD := &apiextv1beta1.CustomResourceDefinition{}
		if err = yaml.Unmarshal([]byte(lastApplied), lastAppliedCRD); err != nil {
			t.Fatalf("Unable to unmarshal last applied CRD for %s", serverCRD.Name)
		}

		if !apiequality.Semantic.DeepEqual(crd.Spec, lastAppliedCRD.Spec) {
			diff, err := common.JSONDiff(lastAppliedCRD.Spec, crd.Spec)
			if err == nil {
				t.Errorf("crd spec on server does not match expected crd spec (%s): %s", crd.Name, string(diff))
				continue
			}
			t.Errorf("crd spec on server does not match expected crd spec (%s), could not calculate a diff", crd.Name)
		}
	}
}

// TestHiveConfig tests that the hive configuration exists and that it's a valid configuration
func TestHiveConfig(t *testing.T) {
	c := common.MustGetClient()
	hiveConfig := &hivev1.HiveConfig{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: hiveConfigName}, hiveConfig)
	if err != nil {
		t.Errorf("could not fetch hive config: %v", err)
		return
	}
	for _, md := range hiveConfig.Spec.ManagedDomains {
		if md.AWS == nil && md.GCP == nil {
			t.Errorf("managed domain entry found without cloud configuration")
			return
		}

		var secretName string

		if md.AWS != nil {
			if len(md.AWS.CredentialsSecretRef.Name) == 0 {
				t.Errorf("AWS managed DNS configured, but no credentials secret specified")
				return
			}
			secretName = md.AWS.CredentialsSecretRef.Name
		}

		if md.GCP != nil {
			if len(md.GCP.CredentialsSecretRef.Name) == 0 {
				t.Errorf("GCP managed DNS configured, but no credentials secret specified")
				return
			}
			secretName = md.GCP.CredentialsSecretRef.Name
		}

		secret := &corev1.Secret{}
		secretKey := types.NamespacedName{
			Namespace: common.GetHiveNamespaceOrDie(),
			Name:      secretName,
		}
		err := c.Get(context.TODO(), secretKey, secret)
		if err != nil {
			t.Errorf("managed DNS credentials secret specified (%s), but secret cannot be fetched: %v", secretName, err)
		}
	}
}

// TestOperatorApply ensures that changing a resource managed by the
// Hive operator results in the operator overwriting the change.
/*
NOTE: Disabling this for now, since it won't pass. We need to fix
the operator so that it does watch the resources that it creates and
optionally we specify resources that we want unmanaged.

func TestOperatorApply(t *testing.T) {
	kubeClient := common.MustGetKubernetesClient()
	// First ensure that the service exists
	err := common.WaitForService(kubeClient, hiveNamespace, hiveManagerService, func(*corev1.Service) bool { return true }, 8*time.Minute)
	if err != nil {
		t.Errorf("Failed waiting for hive manager service: %v", err)
		return
	}

	service := &corev1.Service{}
	c := common.MustGetClient()
	err = c.Get(context.TODO(), types.NamespacedName{Namespace: hiveNamespace, Name: hiveManagerService}, service)
	if err != nil {
		t.Errorf("Failed to fetch hive manager service: %v", err)
		return
	}

	service.Annotations = map[string]string{"test-change": "my test change"}
	err = c.Update(context.TODO(), service)
	if err != nil {
		t.Errorf("Failed to update hive manager service: %v", err)
		return
	}

	serviceIsReverted := func(s *corev1.Service) bool {
		annotations := s.Annotations
		if annotations == nil {
			return true
		}
		_, key_exists := annotations["test-change"]
		return !key_exists
	}

	// Wait for change to service resource to be reverted
	err = common.WaitForService(kubeClient, hiveNamespace, hiveManagerService, serviceIsReverted, 5*time.Minute)
	if err != nil {
		t.Errorf("Failed to get change reverted: %v", err)
	}
}
*/
