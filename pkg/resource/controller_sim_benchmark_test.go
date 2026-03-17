package resource

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/types"
)

// BenchmarkControllerReconcileSyncSet simulates a SyncSet controller reconciliation
// This represents syncing 5-10 resources to a cluster
func BenchmarkControllerReconcileSyncSet(b *testing.B) {
	_, cfg := setupTestEnv(b)

	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	helper, err := NewHelper(logger, FromRESTConfig(cfg), WithControllerName("syncset"))
	if err != nil {
		b.Fatalf("failed to create helper: %v", err)
	}

	// Resources a SyncSet might sync
	resources := [][]byte{
		configMapYAML,
		secretYAML,
		deploymentYAML,
		serviceAccountYAML,
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// Simulate reconciling one SyncSet (apply all its resources)
		for _, resource := range resources {
			_, err := helper.Apply(resource)
			if err != nil {
				b.Fatalf("apply failed: %v", err)
			}
		}
	}
}

// BenchmarkControllerReconcileClusterInstall simulates cluster installation workflow
// This represents the install job creating cluster resources
func BenchmarkControllerReconcileClusterInstall(b *testing.B) {
	_, cfg := setupTestEnv(b)

	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	helper, err := NewHelper(logger, FromRESTConfig(cfg), WithControllerName("install"))
	if err != nil {
		b.Fatalf("failed to create helper: %v", err)
	}

	// Resources created during install
	installResources := [][]byte{
		[]byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: install-config
  namespace: default
data:
  install-config.yaml: |
    apiVersion: v1
    baseDomain: example.com`),
		[]byte(`apiVersion: v1
kind: Secret
metadata:
  name: admin-kubeconfig
  namespace: default
type: Opaque
data:
  kubeconfig: YWRtaW4ta3ViZWNvbmZpZw==`),
		[]byte(`apiVersion: v1
kind: Secret
metadata:
  name: admin-password
  namespace: default
type: Opaque
data:
  password: cGFzc3dvcmQxMjM=`),
		serviceAccountYAML,
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// Simulate one install workflow
		for _, resource := range installResources {
			_, err := helper.Apply(resource)
			if err != nil {
				b.Fatalf("apply failed: %v", err)
			}
		}

		// Patch data to simulate status update
		patch := []byte(`{"data":{"install-complete":"true"}}`)
		err := helper.Patch(
			namespacedName("default", "install-config"),
			"ConfigMap",
			"v1",
			patch,
			"strategic",
		)
		if err != nil {
			b.Fatalf("patch failed: %v", err)
		}
	}
}

// BenchmarkControllerReconcileClusterPool simulates ClusterPool provisioning
// This represents creating multiple cluster deployments from a pool
func BenchmarkControllerReconcileClusterPool(b *testing.B) {
	_, cfg := setupTestEnv(b)

	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	helper, err := NewHelper(logger, FromRESTConfig(cfg), WithControllerName("clusterpool"))
	if err != nil {
		b.Fatalf("failed to create helper: %v", err)
	}

	// Resources per cluster
	pullSecret := []byte(`apiVersion: v1
kind: Secret
metadata:
  name: pull-secret
  namespace: default
type: kubernetes.io/dockerconfigjson
data:
  .dockerconfigjson: eyJhdXRocyI6e319`)

	installConfig := []byte(`apiVersion: v1
kind: Secret
metadata:
  name: install-config
  namespace: default
type: Opaque
data:
  install-config.yaml: YXBpVmVyc2lvbjogdjE=`)

	testCases := []struct {
		name         string
		clusterCount int
	}{
		{"3Clusters", 3},
		{"5Clusters", 5},
		{"10Clusters", 10},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				// Simulate provisioning N clusters
				for cluster := 0; cluster < tc.clusterCount; cluster++ {
					// Each cluster needs these resources
					_, err := helper.Apply(pullSecret)
					if err != nil {
						b.Fatalf("apply pull secret failed: %v", err)
					}

					_, err = helper.Apply(installConfig)
					if err != nil {
						b.Fatalf("apply install config failed: %v", err)
					}
				}
			}
		})
	}
}

// BenchmarkControllerReconcileIdentityProvider simulates syncidentityprovider controller
// This represents creating/updating ConfigMaps for identity providers
func BenchmarkControllerReconcileIdentityProvider(b *testing.B) {
	_, cfg := setupTestEnv(b)

	logger := log.New()
	logger.SetLevel(log.ErrorLevel)

	helper, err := NewHelper(logger, FromRESTConfig(cfg), WithControllerName("syncidp"))
	if err != nil {
		b.Fatalf("failed to create helper: %v", err)
	}

	// ConfigMap representing identity provider config
	idpConfig := []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: cluster-idp
  namespace: default
data:
  oauth-config: |
    identityProviders:
    - name: github
      type: GitHub
      mappingMethod: claim`)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// Reconcile creates/updates the config
		_, err := helper.Apply(idpConfig)
		if err != nil {
			b.Fatalf("apply idp config failed: %v", err)
		}
	}
}

// Helper function
func namespacedName(namespace, name string) types.NamespacedName {
	return types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
}
