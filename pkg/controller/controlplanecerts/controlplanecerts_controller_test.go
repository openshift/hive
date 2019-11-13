package controlplanecerts

import (
	"context"
	"crypto/rand"
	"fmt"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	configv1 "github.com/openshift/api/config/v1"
	openshiftapiv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/resource"
)

const (
	fakeName      = "fake-cluster"
	fakeNamespace = "fake-namespace"
	fakeDomain    = "example.com"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestReconcileControlPlaneCerts(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	openshiftapiv1.Install(scheme.Scheme)

	tests := []struct {
		name     string
		existing []runtime.Object
		validate func(*testing.T, client.Client, []runtime.Object)
	}{
		{
			name: "no control plane certs",
			existing: []runtime.Object{
				fakeClusterDeployment().obj(),
			},
			validate: func(t *testing.T, c client.Client, applied []runtime.Object) {
				cd := getFakeClusterDeployment(t, c)
				assert.Empty(t, applied, "no syncset should be applied")
				assert.Empty(t, cd.Status.Conditions, "no conditions should be set")
			},
		},
		{
			name: "default control plane certs",
			existing: []runtime.Object{
				fakeClusterDeployment().defaultCert("default-cert", "default-secret").obj(),
				fakeCertSecret("default-secret"),
			},
			validate: func(t *testing.T, c client.Client, applied []runtime.Object) {
				cd := getFakeClusterDeployment(t, c)
				assert.Empty(t, cd.Status.Conditions, "no conditions should be set")
				validateAppliedSyncSet(t, applied, "", additionalCert(defaultControlPlaneDomain(cd), "default-secret"))
			},
		},
		{
			name: "additional certs only",
			existing: []runtime.Object{
				fakeClusterDeployment().
					namedCert("cert1", "foo.com", "secret1").
					namedCert("cert2", "bar.com", "secret2").obj(),
				fakeCertSecret("secret1"),
				fakeCertSecret("secret2"),
			},
			validate: func(t *testing.T, c client.Client, applied []runtime.Object) {
				cd := getFakeClusterDeployment(t, c)
				assert.Empty(t, cd.Status.Conditions, "no conditions should be set")
				validateAppliedSyncSet(t, applied, "", additionalCert("foo.com", "secret1"), additionalCert("bar.com", "secret2"))
			},
		},
		{
			name: "default and additional certs",
			existing: []runtime.Object{
				fakeClusterDeployment().
					defaultCert("default", "secret0").
					namedCert("cert1", "foo.com", "secret1").
					namedCert("cert2", "bar.com", "secret2").obj(),
				fakeCertSecret("secret0"),
				fakeCertSecret("secret1"),
				fakeCertSecret("secret2"),
			},
			validate: func(t *testing.T, c client.Client, applied []runtime.Object) {
				cd := getFakeClusterDeployment(t, c)
				assert.Empty(t, cd.Status.Conditions, "no conditions should be set")
				validateAppliedSyncSet(t, applied, "", additionalCert(defaultControlPlaneDomain(cd), "secret0"), additionalCert("foo.com", "secret1"), additionalCert("bar.com", "secret2"))
			},
		},
		{
			name: "missing secret",
			existing: []runtime.Object{
				fakeClusterDeployment().
					defaultCert("default", "secret0").
					namedCert("cert1", "foo.com", "secret1").
					namedCert("cert2", "bar.com", "secret2").obj(),
				fakeCertSecret("secret0"),
				fakeCertSecret("secret2"),
			},
			validate: func(t *testing.T, c client.Client, applied []runtime.Object) {
				assert.Empty(t, applied)
				cd := getFakeClusterDeployment(t, c)
				assert.Equal(t, 1, len(cd.Status.Conditions))
				notFoundCondition := controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.ControlPlaneCertificateNotFoundCondition)
				assert.NotNil(t, notFoundCondition)
				assert.Equal(t, notFoundCondition.Status, corev1.ConditionTrue)
			},
		},
		{
			name: "existing syncset, remove certs",
			existing: []runtime.Object{
				fakeClusterDeployment().obj(),
				fakeSyncSet(),
			},
			validate: func(t *testing.T, c client.Client, applied []runtime.Object) {
				validateAppliedSyncSet(t, applied, "")
			},
		},
		{
			name: "existing not found condition, change to false",
			existing: []runtime.Object{
				fakeClusterDeployment().defaultCert("defaut", "test-secret").withNotFoundCondition().obj(),
				fakeCertSecret("test-secret"),
			},
			validate: func(t *testing.T, c client.Client, applied []runtime.Object) {
				cd := getFakeClusterDeployment(t, c)
				assert.Equal(t, 1, len(cd.Status.Conditions))
				notFoundCondition := controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.ControlPlaneCertificateNotFoundCondition)
				assert.NotNil(t, notFoundCondition)
				assert.Equal(t, string(corev1.ConditionFalse), string(notFoundCondition.Status))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClient(test.existing...)
			applier := &fakeApplier{}
			r := &ReconcileControlPlaneCerts{
				Client:  fakeClient,
				scheme:  scheme.Scheme,
				applier: applier,
			}

			_, err := r.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      fakeName,
					Namespace: fakeNamespace,
				},
			})

			assert.Nil(t, err)

			if test.validate != nil {
				test.validate(t, fakeClient, applier.appliedObjects)
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

func (f *fakeClusterDeploymentWrapper) withNotFoundCondition() *fakeClusterDeploymentWrapper {
	f.cd.Status.Conditions = controllerutils.SetClusterDeploymentCondition(
		f.cd.Status.Conditions,
		hivev1.ControlPlaneCertificateNotFoundCondition,
		corev1.ConditionTrue,
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
		"tls.key": []byte("blah"),
		"tls.crt": []byte("blah"),
	}
	s.Type = corev1.SecretTypeTLS
	return s
}

type additionalCertSpec struct {
	domain string
	secret string
}

func additionalCert(domain, secret string) additionalCertSpec {
	return additionalCertSpec{
		domain: domain,
		secret: fakeName + "-" + secret,
	}
}

func validateAppliedSyncSet(t *testing.T, objs []runtime.Object, defaultSecret string, additional ...additionalCertSpec) {
	assert.Len(t, objs, 1, "single syncset expected")
	assert.IsType(t, &hivev1.SyncSet{}, objs[0], "syncset object expected")
	ss := objs[0].(*hivev1.SyncSet)
	secretReferences := ss.Spec.SecretReferences
	var apiServerConfig *configv1.APIServer
	for _, rr := range ss.Spec.Resources {
		if config, ok := rr.Object.(*configv1.APIServer); ok {
			apiServerConfig = config
			continue
		}
		assert.Fail(t, "unexpected resource type", "syncset resource type: %T", rr.Object)
	}
	// Ensure there are no duplicate names
	names := secretNames(secretReferences)
	assert.Equal(t, sets.NewString(names...).Len(), len(names))

	if defaultSecret != "" {
		s := findSecret(secretReferences, defaultSecret)
		assert.NotNil(t, s)
		assert.Equal(t, apiServerConfig.Spec.ServingCerts.DefaultServingCertificate.Name, s)
	} else {
		assert.Empty(t, apiServerConfig.Spec.ServingCerts.DefaultServingCertificate.Name)
	}

	for i, c := range additional {
		s := findSecret(secretReferences, c.secret)
		assert.NotNil(t, s)
		assert.Contains(t, apiServerConfig.Spec.ServingCerts.NamedCertificates[i].Names, c.domain)
		assert.Equal(t, apiServerConfig.Spec.ServingCerts.NamedCertificates[i].ServingCertificate.Name, c.secret)
	}
	if len(additional) == 0 {
		assert.Empty(t, apiServerConfig.Spec.ServingCerts.NamedCertificates)
	}

	// If not setting any secrets, ensure they're empty
	if defaultSecret == "" && len(additional) == 0 {
		assert.Empty(t, secretReferences)
	}
}

func fakeSyncSet() *hivev1.SyncSet {
	ss := &hivev1.SyncSet{}
	ss.Namespace = fakeNamespace
	ss.Name = controlPlaneCertsSyncSetName(fakeName)
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

func findSecret(ss []hivev1.SecretReference, name string) string {
	for _, s := range ss {
		if s.Target.Name == name {
			return name
		}
	}
	return ""
}

func secretNames(ss []hivev1.SecretReference) []string {
	names := []string{}
	for _, s := range ss {
		names = append(names, s.Target.Name)
	}
	return names
}
