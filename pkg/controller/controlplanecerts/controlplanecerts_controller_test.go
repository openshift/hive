package controlplanecerts

import (
	"context"
	"crypto/rand"
	"fmt"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/resource"
	testfake "github.com/openshift/hive/pkg/test/fake"
	testsecret "github.com/openshift/hive/pkg/test/secret"
	"github.com/openshift/hive/pkg/util/scheme"
)

const (
	fakeName             = "fake-cluster"
	fakeNamespace        = "fake-namespace"
	fakeDomain           = "example.com"
	fakeAPIURL           = "https://test-api-url:6443"
	fakeAPIURLDomain     = "test-api-url"
	kubeconfigSecretName = "test-kubeconfig"
	adminKubeconfig      = `clusters:
- cluster:
    server: https://test-api-url:6443
  name: bar
contexts:
- context:
    cluster: bar
  name: admin
current-context: admin
`
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestReconcileControlPlaneCerts(t *testing.T) {

	tests := []struct {
		name     string
		existing []runtime.Object

		expectNoSyncSet        bool
		expectedPatch          string
		expectedSecrets        []string
		expectedNotFoundStatus corev1.ConditionStatus
	}{
		{
			name: "initialize conditions",
			existing: []runtime.Object{
				fakeClusterDeployment().obj(),
			},
			expectNoSyncSet:        true,
			expectedNotFoundStatus: corev1.ConditionUnknown,
		},
		{
			name: "no control plane certs",
			existing: []runtime.Object{
				fakeClusterDeployment().withNotFoundCondition(corev1.ConditionUnknown).obj(),
			},
			expectNoSyncSet: true,
		},
		{
			name: "default control plane certs",
			existing: []runtime.Object{
				fakeClusterDeployment().
					defaultCert("default-cert", "default-secret").
					withNotFoundCondition(corev1.ConditionFalse).obj(),
				fakeCertSecret("default-secret"),
			},
			expectedPatch:   `[ { "op": "add", "path": "/spec/servingCerts", "value": {} }, { "op": "add", "path": "/spec/servingCerts/namedCertificates", "value": [  ] }, { "op": "replace", "path": "/spec/servingCerts/namedCertificates", "value": [  { "names": [ "test-api-url" ], "servingCertificate": { "name": "fake-cluster-default-secret" } } ] } ]`,
			expectedSecrets: []string{"default-secret"},
		},
		{
			name: "additional certs only",
			existing: []runtime.Object{
				fakeClusterDeployment().
					namedCert("cert1", "foo.com", "secret1").
					namedCert("cert2", "bar.com", "secret2").
					withNotFoundCondition(corev1.ConditionFalse).obj(),
				fakeCertSecret("secret1"),
				fakeCertSecret("secret2"),
			},
			expectedPatch:   `[ { "op": "add", "path": "/spec/servingCerts", "value": {} }, { "op": "add", "path": "/spec/servingCerts/namedCertificates", "value": [  ] }, { "op": "replace", "path": "/spec/servingCerts/namedCertificates", "value": [  { "names": [ "foo.com" ], "servingCertificate": { "name": "fake-cluster-secret1" } }, { "names": [ "bar.com" ], "servingCertificate": { "name": "fake-cluster-secret2" } } ] } ]`,
			expectedSecrets: []string{"secret1", "secret2"},
		},
		{
			name: "default and additional certs",
			existing: []runtime.Object{
				fakeClusterDeployment().
					defaultCert("default", "secret0").
					namedCert("cert1", "foo.com", "secret1").
					namedCert("cert2", "bar.com", "secret2").
					withNotFoundCondition(corev1.ConditionFalse).
					obj(),
				fakeCertSecret("secret0"),
				fakeCertSecret("secret1"),
				fakeCertSecret("secret2"),
			},
			expectedPatch:   `[ { "op": "add", "path": "/spec/servingCerts", "value": {} }, { "op": "add", "path": "/spec/servingCerts/namedCertificates", "value": [  ] }, { "op": "replace", "path": "/spec/servingCerts/namedCertificates", "value": [  { "names": [ "test-api-url" ], "servingCertificate": { "name": "fake-cluster-secret0" } }, { "names": [ "foo.com" ], "servingCertificate": { "name": "fake-cluster-secret1" } }, { "names": [ "bar.com" ], "servingCertificate": { "name": "fake-cluster-secret2" } } ] } ]`,
			expectedSecrets: []string{"secret0", "secret1", "secret2"},
		},
		{
			name: "missing secret",
			existing: []runtime.Object{
				fakeClusterDeployment().
					withNotFoundCondition(corev1.ConditionUnknown).
					defaultCert("default", "secret0").
					namedCert("cert1", "foo.com", "secret1").
					namedCert("cert2", "bar.com", "secret2").obj(),
				fakeCertSecret("secret0"),
				fakeCertSecret("secret2"),
			},
			expectNoSyncSet:        true,
			expectedNotFoundStatus: corev1.ConditionTrue,
		},
		{
			name: "existing syncset remove certs",
			existing: []runtime.Object{
				fakeClusterDeployment().
					withNotFoundCondition(corev1.ConditionFalse).obj(),
				fakeSyncSet(),
			},
			expectedPatch: `[ { "op": "add", "path": "/spec/servingCerts", "value": {} }, { "op": "add", "path": "/spec/servingCerts/namedCertificates", "value": [  ] }, { "op": "replace", "path": "/spec/servingCerts/namedCertificates", "value": [  ] } ]`,
		},
		{
			name: "existing not found condition changed to false",
			existing: []runtime.Object{
				fakeClusterDeployment().defaultCert("default", "test-secret").
					withNotFoundCondition(corev1.ConditionTrue).obj(),
				fakeCertSecret("test-secret"),
			},
			// no apply expected because we update the condition and return immediately, next reconcile would apply
			expectNoSyncSet:        true,
			expectedNotFoundStatus: corev1.ConditionFalse,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.existing = append(test.existing,
				testsecret.Build(
					testsecret.WithName(kubeconfigSecretName),
					testsecret.WithNamespace(fakeNamespace),
					testsecret.WithDataKeyValue(constants.KubeconfigSecretKey, []byte(adminKubeconfig)),
				),
			)
			scheme := scheme.GetScheme()
			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(test.existing...).Build()

			applier := &fakeApplier{}
			r := &ReconcileControlPlaneCerts{
				Client:  fakeClient,
				scheme:  scheme,
				applier: applier,
			}

			_, err := r.Reconcile(context.TODO(), reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      fakeName,
					Namespace: fakeNamespace,
				},
			})

			assert.Nil(t, err)

			cd := getFakeClusterDeployment(t, fakeClient)

			if test.expectNoSyncSet {
				assert.Len(t, applier.appliedObjects, 0, "unexpected syncset apply")
			} else {
				require.Len(t, applier.appliedObjects, 1, "single apply expected")
				require.IsType(t, &hivev1.SyncSet{}, applier.appliedObjects[0], "syncset apply expected")
				ss := applier.appliedObjects[0].(*hivev1.SyncSet)

				// Resources should have been removed from the SyncSet
				assert.Equal(t, 0, len(ss.Spec.Resources))

				// Expect our patch, plus one to force the kubeapiserver redeploy:
				require.Equal(t, 2, len(ss.Spec.Patches))
				assert.Equal(t, test.expectedPatch, ss.Spec.Patches[0].Patch)

				secretMappings := ss.Spec.Secrets
				require.Equal(t, len(test.expectedSecrets), len(secretMappings))

				// Ensure there are no duplicate Secret names
				names := secretNames(secretMappings)
				assert.Equal(t, sets.NewString(names...).Len(), len(names), "duplicate secret names present")
				for i, sn := range test.expectedSecrets {
					assert.Equal(t, hivev1.SecretReference{Name: sn, Namespace: fakeNamespace}, ss.Spec.Secrets[i].SourceRef)
				}

				labels := ss.GetLabels()

				assert.Equal(t, fakeClusterDeployment().obj().Name, labels[constants.ClusterDeploymentNameLabel], "incorrect cluster deployment name label")
				assert.Equal(t, constants.SyncSetTypeControlPlaneCerts, labels[constants.SyncSetTypeLabel], "incorrect syncset type label")
			}

			notFoundCondition := controllerutils.FindCondition(cd.Status.Conditions, hivev1.ControlPlaneCertificateNotFoundCondition)
			if test.expectedNotFoundStatus != "" {
				assert.NotNil(t, notFoundCondition, "expected a NotFound condition")
				assert.Equal(t, test.expectedNotFoundStatus, notFoundCondition.Status, "unexpected NotFound status")
			} else {
				assert.Equal(t, corev1.ConditionFalse, notFoundCondition.Status)
			}

		})
	}
}

func TestGetControlPlaneSecretNames(t *testing.T) {
	tests := []struct {
		name  string
		cd    *hivev1.ClusterDeployment
		names []string
	}{
		{
			name:  "only default",
			cd:    fakeClusterDeployment().defaultCert("foo", "foo-secret").obj(),
			names: []string{"foo-secret"},
		},
		{
			name: "only additional",
			cd: fakeClusterDeployment().
				namedCert("bar", "example.com", "bar-secret").
				namedCert("baz", "another.example.com", "baz-secret").obj(),
			names: []string{"bar-secret", "baz-secret"},
		},
		{
			name: "default + additional",
			cd: fakeClusterDeployment().defaultCert("aaa", "aaa-secret").
				namedCert("bar", "example.com", "bbb-secret").
				namedCert("baz", "example2.com", "ccc-secret").obj(),
			names: []string{"aaa-secret", "bbb-secret", "ccc-secret"},
		},
		{
			name: "default + additional, same secret",
			cd: fakeClusterDeployment().defaultCert("aaa", "aaa-secret").
				namedCert("bar", "example.com", "aaa-secret").
				namedCert("baz", "example2.com", "aaa-secret").obj(),
			names: []string{"aaa-secret"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := getControlPlaneSecretNames(test.cd, log.WithField("test", test.name))
			assert.Nil(t, err)
			assert.Equal(t, test.names, actual)
		})
	}
}

func TestSecretsHash(t *testing.T) {
	s1 := testSecret("secret1")
	s2 := testSecret("secret2")
	s3 := testSecret("secret3")

	secrets := []*corev1.Secret{s1, s2, s3}
	currentHash := ""
	for i := 0; i < 50; i++ {
		secretsForHash := []*corev1.Secret{secrets[i%3], secrets[(i+1)%3], secrets[(i+2)%3]}
		hash := secretsHash(secretsForHash)
		if currentHash == "" {
			currentHash = hash
			continue
		}
		if hash != currentHash {
			t.Fatalf("got inconsistent hash")
		}
	}
}

func testSecret(name string) *corev1.Secret {
	s := &corev1.Secret{}
	s.Name = name
	s.Namespace = "namespace"
	s.Data = map[string][]byte{}
	for i := 0; i < 10; i++ {
		b := make([]byte, 100)
		rand.Read(b)
		s.Data[fmt.Sprintf("key_%d", i)] = b
	}
	return s
}

func getFakeClusterDeployment(t *testing.T, c client.Client) *hivev1.ClusterDeployment {
	cd := &hivev1.ClusterDeployment{}
	err := c.Get(context.TODO(), types.NamespacedName{Namespace: fakeNamespace, Name: fakeName}, cd)
	assert.Nil(t, err)
	return cd
}

type fakeApplier struct {
	appliedObjects []runtime.Object
}

func (a *fakeApplier) ApplyRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (resource.ApplyResult, error) {
	a.appliedObjects = append(a.appliedObjects, obj)
	return "", nil
}

type fakeClusterDeploymentWrapper struct {
	cd *hivev1.ClusterDeployment
}

func fakeClusterDeployment() *fakeClusterDeploymentWrapper {
	cd := &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fakeName,
			Namespace: fakeNamespace,
		},
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterName: fakeName,
			BaseDomain:  fakeDomain,
			Installed:   true,
			ClusterMetadata: &hivev1.ClusterMetadata{
				AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: kubeconfigSecretName},
			},
		},
		Status: hivev1.ClusterDeploymentStatus{
			APIURL: fakeAPIURL,
			Conditions: []hivev1.ClusterDeploymentCondition{{
				Type:   hivev1.UnreachableCondition,
				Status: corev1.ConditionFalse,
			}},
		},
	}
	return &fakeClusterDeploymentWrapper{cd: cd}
}

func (f *fakeClusterDeploymentWrapper) obj() *hivev1.ClusterDeployment {
	return f.cd
}

func (f *fakeClusterDeploymentWrapper) defaultCert(name, secretName string) *fakeClusterDeploymentWrapper {
	f.cd.Spec.ControlPlaneConfig.ServingCertificates.Default = name
	f.cd.Spec.CertificateBundles = append(f.cd.Spec.CertificateBundles, hivev1.CertificateBundleSpec{
		Name: name,
		CertificateSecretRef: corev1.LocalObjectReference{
			Name: secretName,
		},
	})
	return f
}

func (f *fakeClusterDeploymentWrapper) namedCert(name, domain, secretName string) *fakeClusterDeploymentWrapper {
	f.cd.Spec.ControlPlaneConfig.ServingCertificates.Additional = append(f.cd.Spec.ControlPlaneConfig.ServingCertificates.Additional, hivev1.ControlPlaneAdditionalCertificate{
		Domain: domain,
		Name:   name,
	})
	f.cd.Spec.CertificateBundles = append(f.cd.Spec.CertificateBundles, hivev1.CertificateBundleSpec{
		Name: name,
		CertificateSecretRef: corev1.LocalObjectReference{
			Name: secretName,
		},
	})
	return f
}

func (f *fakeClusterDeploymentWrapper) withNotFoundCondition(status corev1.ConditionStatus) *fakeClusterDeploymentWrapper {
	f.cd.Status.Conditions = controllerutils.SetClusterDeploymentCondition(
		f.cd.Status.Conditions,
		hivev1.ControlPlaneCertificateNotFoundCondition,
		status,
		"",
		"",
		controllerutils.UpdateConditionNever,
	)
	return f
}

func fakeCertSecret(name string) *corev1.Secret {
	s := &corev1.Secret{}
	s.Name = name
	s.Namespace = fakeNamespace
	s.Data = map[string][]byte{
		constants.TLSKeySecretKey: []byte("blah"),
		constants.TLSCrtSecretKey: []byte("blah"),
	}
	s.Type = corev1.SecretTypeTLS
	return s
}

func fakeSyncSet() *hivev1.SyncSet {
	ss := &hivev1.SyncSet{}
	ss.Namespace = fakeNamespace
	ss.Name = GenerateControlPlaneCertsSyncSetName(fakeName)
	// Simulated legacy resources, should be migrated to a patch after reconcile.
	ss.Spec.Resources = []runtime.RawExtension{
		{
			Object: fakeCertSecret("foo1"),
		},
		{
			Object: fakeCertSecret("foo2"),
		},
	}
	return ss
}

func secretNames(ss []hivev1.SecretMapping) []string {
	names := []string{}
	for _, s := range ss {
		names = append(names, s.TargetRef.Name)
	}
	return names
}
