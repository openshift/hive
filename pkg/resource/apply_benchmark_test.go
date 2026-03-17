package resource

import (
	"testing"

	"github.com/go-logr/logr"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

// Sample resources for benchmarking
var (
	configMapYAML = []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: test-configmap
  namespace: default
data:
  key1: value1
  key2: value2
`)

	secretYAML = []byte(`apiVersion: v1
kind: Secret
metadata:
  name: test-secret
  namespace: default
type: Opaque
data:
  username: YWRtaW4=
  password: MWYyZDFlMmU2N2Rm
`)

	deploymentYAML = []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
  namespace: default
spec:
  replicas: 3
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
        ports:
        - containerPort: 80
`)

	serviceAccountYAML = []byte(`apiVersion: v1
kind: ServiceAccount
metadata:
  name: test-sa
  namespace: default
`)
)

// setupTestEnv creates a test Kubernetes environment for benchmarking
func setupTestEnv(b *testing.B) (*envtest.Environment, *rest.Config) {
	b.Helper()

	// Initialize controller-runtime logger to suppress warnings during benchmarks
	logf.SetLogger(logr.New(logr.Discard().GetSink()))

	// Add Hive scheme
	if err := hivev1.AddToScheme(scheme.Scheme); err != nil {
		b.Fatalf("failed to add hive scheme: %v", err)
	}

	testEnv := &envtest.Environment{}
	cfg, err := testEnv.Start()
	if err != nil {
		b.Fatalf("failed to start test environment: %v", err)
	}

	b.Cleanup(func() {
		if err := testEnv.Stop(); err != nil {
			b.Logf("failed to stop test environment: %v", err)
		}
	})

	return testEnv, cfg
}

// BenchmarkApply benchmarks the Apply operation with real kubernetes client
func BenchmarkApply(b *testing.B) {
	_, cfg := setupTestEnv(b)

	logger := log.New()
	logger.SetLevel(log.ErrorLevel) // Reduce noise in benchmarks

	helper, err := NewHelper(logger, FromRESTConfig(cfg), WithControllerName("benchmark"))
	if err != nil {
		b.Fatalf("failed to create helper: %v", err)
	}

	resources := map[string][]byte{
		"ConfigMap":      configMapYAML,
		"Secret":         secretYAML,
		"Deployment":     deploymentYAML,
		"ServiceAccount": serviceAccountYAML,
	}

	for name, resource := range resources {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := helper.Apply(resource)
				if err != nil {
					b.Fatalf("apply failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkApplyMultipleResources benchmarks applying multiple resources
func BenchmarkApplyMultipleResources(b *testing.B) {
	_, cfg := setupTestEnv(b)

	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	helper, err := NewHelper(logger, FromRESTConfig(cfg), WithControllerName("benchmark"))
	if err != nil {
		b.Fatalf("failed to create helper: %v", err)
	}

	resources := [][]byte{
		configMapYAML,
		secretYAML,
		serviceAccountYAML,
	}

	testCases := []struct {
		name  string
		count int
	}{
		{"5Resources", 5},
		{"10Resources", 10},
		{"20Resources", 20},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				for j := 0; j < tc.count; j++ {
					resource := resources[j%len(resources)]
					_, err := helper.Apply(resource)
					if err != nil {
						b.Fatalf("apply failed: %v", err)
					}
				}
			}
		})
	}
}

// BenchmarkPatch benchmarks the Patch operation
func BenchmarkPatch(b *testing.B) {
	_, cfg := setupTestEnv(b)

	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	helper, err := NewHelper(logger, FromRESTConfig(cfg), WithControllerName("benchmark"))
	if err != nil {
		b.Fatalf("failed to create helper: %v", err)
	}

	// Create a configmap first
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
		Data: map[string]string{
			"key": "value",
		},
	}
	cmBytes, err := Serialize(cm, scheme.Scheme)
	if err != nil {
		b.Fatalf("failed to serialize configmap: %v", err)
	}
	if _, err := helper.Apply(cmBytes); err != nil {
		b.Fatalf("failed to create configmap: %v", err)
	}

	patches := map[string]struct {
		patch     []byte
		patchType string
	}{
		"StrategicMerge": {
			patch:     []byte(`{"data":{"newkey":"newvalue"}}`),
			patchType: "strategic",
		},
		"JSONPatch": {
			patch:     []byte(`[{"op":"add","path":"/data/newkey","value":"newvalue"}]`),
			patchType: "json",
		},
		"MergePatch": {
			patch:     []byte(`{"data":{"newkey":"newvalue"}}`),
			patchType: "merge",
		},
	}

	for name, tc := range patches {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				err := helper.Patch(
					types.NamespacedName{Name: "test-cm", Namespace: "default"},
					"ConfigMap",
					"v1",
					tc.patch,
					tc.patchType,
				)
				if err != nil {
					b.Fatalf("patch failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkApplyRuntimeObject benchmarks applying runtime objects
func BenchmarkApplyRuntimeObject(b *testing.B) {
	_, cfg := setupTestEnv(b)

	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	helper, err := NewHelper(logger, FromRESTConfig(cfg), WithControllerName("benchmark"))
	if err != nil {
		b.Fatalf("failed to create helper: %v", err)
	}

	objects := map[string]runtime.Object{
		"ConfigMap": &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bench-cm",
				Namespace: "default",
			},
			Data: map[string]string{"key": "value"},
		},
		"Secret": &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bench-secret",
				Namespace: "default",
			},
			StringData: map[string]string{"password": "secret"},
		},
		"ServiceAccount": &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bench-sa",
				Namespace: "default",
			},
		},
	}

	for name, obj := range objects {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := helper.ApplyRuntimeObject(obj, scheme.Scheme)
				if err != nil {
					b.Fatalf("apply runtime object failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkConcurrentApply benchmarks concurrent apply operations
func BenchmarkConcurrentApply(b *testing.B) {
	_, cfg := setupTestEnv(b)

	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	helper, err := NewHelper(logger, FromRESTConfig(cfg), WithControllerName("benchmark"))
	if err != nil {
		b.Fatalf("failed to create helper: %v", err)
	}

	b.RunParallel(func(pb *testing.PB) {
		resources := [][]byte{configMapYAML, secretYAML, serviceAccountYAML}
		i := 0
		for pb.Next() {
			resource := resources[i%len(resources)]
			_, err := helper.Apply(resource)
			if err != nil {
				b.Fatalf("apply failed: %v", err)
			}
			i++
		}
	})
}
