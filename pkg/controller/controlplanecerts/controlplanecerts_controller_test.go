/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controlplanecerts

import (
	"context"
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
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	fakeName      = "fake-cluster"
	fakeNamespace = "fake-namespace"
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
				validateAppliedSyncSet(t, applied, defaultCert("default-secret"))
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
				validateAppliedSyncSet(t, applied, defaultCert("secret0"), additionalCert("foo.com", "secret1"), additionalCert("bar.com", "secret2"))
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

func getFakeClusterDeployment(t *testing.T, c client.Client) *hivev1.ClusterDeployment {
	cd := &hivev1.ClusterDeployment{}
	err := c.Get(context.TODO(), types.NamespacedName{Namespace: fakeNamespace, Name: fakeName}, cd)
	assert.Nil(t, err)
	return cd
}

type fakeApplier struct {
	appliedObjects []runtime.Object
}

func (a *fakeApplier) ApplyRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) error {
	a.appliedObjects = append(a.appliedObjects, obj)
	return nil
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
		Spec: hivev1.ClusterDeploymentSpec{},
	}
	controllerutils.FixupEmptyClusterVersionFields(&cd.Status.ClusterVersionStatus)
	return &fakeClusterDeploymentWrapper{cd: cd}
}

func (f *fakeClusterDeploymentWrapper) obj() *hivev1.ClusterDeployment {
	return f.cd
}

func (f *fakeClusterDeploymentWrapper) defaultCert(name, secretName string) *fakeClusterDeploymentWrapper {
	f.cd.Spec.ControlPlaneConfig.ServingCertificates.Default = name
	f.cd.Spec.CertificateBundles = append(f.cd.Spec.CertificateBundles, hivev1.CertificateBundleSpec{
		Name: name,
		SecretRef: corev1.LocalObjectReference{
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
		SecretRef: corev1.LocalObjectReference{
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

func defaultCert(secret string) string {
	return fakeName + "-" + secret
}

func validateAppliedSyncSet(t *testing.T, objs []runtime.Object, defaultSecret string, additional ...additionalCertSpec) {
	assert.Len(t, objs, 1, "single syncset expected")
	assert.IsType(t, &hivev1.SyncSet{}, objs[0], "syncset object expected")
	ss := objs[0].(*hivev1.SyncSet)
	secrets := []*corev1.Secret{}
	var apiServerConfig *configv1.APIServer
	for _, rr := range ss.Spec.Resources {
		if secret, ok := rr.Object.(*corev1.Secret); ok {
			secrets = append(secrets, secret)
			continue
		}
		if config, ok := rr.Object.(*configv1.APIServer); ok {
			apiServerConfig = config
			continue
		}
		assert.Fail(t, "unexpected resource type", "syncset resource type: %T", rr.Object)
	}
	// Ensure there are no duplicate names
	names := secretNames(secrets)
	assert.Equal(t, sets.NewString(names...).Len(), len(names))

	if defaultSecret != "" {
		s := findSecret(secrets, defaultSecret)
		assert.NotNil(t, s)
		assert.Equal(t, apiServerConfig.Spec.ServingCerts.DefaultServingCertificate.Name, s.Name)
	} else {
		assert.Empty(t, apiServerConfig.Spec.ServingCerts.DefaultServingCertificate.Name)
	}

	for i, c := range additional {
		s := findSecret(secrets, c.secret)
		assert.NotNil(t, s)
		assert.Contains(t, apiServerConfig.Spec.ServingCerts.NamedCertificates[i].Names, c.domain)
		assert.Equal(t, apiServerConfig.Spec.ServingCerts.NamedCertificates[i].ServingCertificate.Name, c.secret)
	}
	if len(additional) == 0 {
		assert.Empty(t, apiServerConfig.Spec.ServingCerts.NamedCertificates)
	}

	// If not setting any secrets, ensure they're empty
	if defaultSecret == "" && len(additional) == 0 {
		assert.Empty(t, secrets)
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

func findSecret(ss []*corev1.Secret, name string) *corev1.Secret {
	for _, s := range ss {
		if s.Name == name {
			return s
		}
	}
	return nil
}

func secretNames(ss []*corev1.Secret) []string {
	names := []string{}
	for _, s := range ss {
		names = append(names, s.Name)
	}
	return names
}
