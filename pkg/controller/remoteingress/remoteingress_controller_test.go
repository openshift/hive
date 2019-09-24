package remoteingress

import (
	"context"
	"fmt"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ingresscontroller "github.com/openshift/api/operator/v1"
	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	hivev1aws "github.com/openshift/hive/pkg/apis/hive/v1alpha1/aws"
	"github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/resource"
)

const (
	testClusterName                      = "foo"
	testNamespace                        = "default"
	testDefaultIngressName               = "default"
	testIngressDomain                    = "testapps.example.com"
	testDefaultIngressServingCertificate = "test-bundle"
	testDefaultCertBundleSecret          = "test-bundle-secret"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

type SyncSetIngressEntry struct {
	name               string
	domain             string
	routeSelector      *metav1.LabelSelector
	namespaceSelector  *metav1.LabelSelector
	defaultCertificate string
}

func TestRemoteClusterIngressReconcile(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	ingresscontroller.AddToScheme(scheme.Scheme)

	tests := []struct {
		name                          string
		localObjects                  []runtime.Object
		expectedSyncSetIngressEntries []SyncSetIngressEntry
		expectedSecretEntries         []string
	}{
		{
			name: "Test no ingress defined",
			localObjects: []runtime.Object{
				testClusterDeploymentWithoutIngress(),
			},
		},
		{
			name: "Test single ingress (only default)",
			localObjects: []runtime.Object{
				testClusterDeployment(),
			},
			expectedSyncSetIngressEntries: []SyncSetIngressEntry{
				{
					name:   testDefaultIngressName,
					domain: testIngressDomain,
				},
			},
		},

		{
			name: "Test multiple ingress",
			localObjects: []runtime.Object{
				addIngressToClusterDeployment(testClusterDeployment(), "secondingress", "moreingress.example.com", nil, nil, ""),
			},
			expectedSyncSetIngressEntries: []SyncSetIngressEntry{
				{
					name:   testDefaultIngressName,
					domain: testIngressDomain,
				},
				{
					name:   "secondingress",
					domain: "moreingress.example.com",
				},
			},
		},
		{
			name: "Test updating existing syncset",
			localObjects: []runtime.Object{
				addIngressToClusterDeployment(testClusterDeployment(), "secondingress", "moreingress.example.com", nil, nil, ""),
				syncSetFromClusterDeployment(testClusterDeployment()),
			},
			expectedSyncSetIngressEntries: []SyncSetIngressEntry{
				{
					name:   testDefaultIngressName,
					domain: testIngressDomain,
				},
				{
					name:   "secondingress",
					domain: "moreingress.example.com",
				},
			},
		},
		{
			name: "Test removing an ingress from existing syncset",
			localObjects: func() []runtime.Object {
				objects := []runtime.Object{}
				// create a temp cluster deployment with extra ingress
				cd := testClusterDeployment()
				cd.Spec.Ingress = append(cd.Spec.Ingress, hivev1.ClusterIngress{
					Name:   "secondingress",
					Domain: "moreingress.example.com",
				})

				// create a syncset that has the extra ingress
				ss := syncSetFromClusterDeployment(cd)
				objects = append(objects, ss)

				// create the current cluster deployment with only a single ingress
				cd = testClusterDeployment()
				objects = append(objects, cd)

				return objects
			}(),
			expectedSyncSetIngressEntries: []SyncSetIngressEntry{
				{
					name:   testDefaultIngressName,
					domain: testIngressDomain,
				},
			},
		},
		{
			name: "Test setting routeSelector",
			localObjects: []runtime.Object{
				addIngressToClusterDeployment(testClusterDeployment(), "secondingress", "moreingress.example.com", testRouteSelector(), nil, ""),
			},
			expectedSyncSetIngressEntries: []SyncSetIngressEntry{
				{
					name:   testDefaultIngressName,
					domain: testIngressDomain,
				},
				{
					name:          "secondingress",
					domain:        "moreingress.example.com",
					routeSelector: testRouteSelector(),
				},
			},
		},
		{
			name: "Test setting namespaceSelector",
			localObjects: []runtime.Object{
				addIngressToClusterDeployment(testClusterDeployment(), "secondingress", "moreingress.example.com", nil, testNamespaceSelector(), ""),
			},
			expectedSyncSetIngressEntries: []SyncSetIngressEntry{
				{
					name:   testDefaultIngressName,
					domain: testIngressDomain,
				},
				{
					name:              "secondingress",
					domain:            "moreingress.example.com",
					namespaceSelector: testNamespaceSelector(),
				},
			},
		},
		{
			name: "Test bringing your own custom certificate",
			localObjects: func() []runtime.Object {
				objects := []runtime.Object{}
				cd := testClusterDeploymentWithManualCertificate()
				objects = append(objects, cd)

				// put the expected secrets to satisfy the list of certificateBundles
				secrets := testSecretsForClusterDeployment(cd)
				for i := range secrets {
					objects = append(objects, &secrets[i])
				}

				return objects
			}(),
			expectedSyncSetIngressEntries: []SyncSetIngressEntry{
				{
					name:               testDefaultIngressName,
					domain:             testIngressDomain,
					defaultCertificate: fmt.Sprintf("%s-%s", testClusterName, testDefaultCertBundleSecret),
				},
			},
			expectedSecretEntries: []string{
				fmt.Sprintf("%s-%s", testClusterName, testDefaultCertBundleSecret),
			},
		},
		{
			name: "Test adding an ingress with custom certificate",
			localObjects: func() []runtime.Object {
				objects := []runtime.Object{}
				// syncset object with single ingress
				cd := testClusterDeploymentWithManualCertificate()
				ss := syncSetFromClusterDeployment(cd)
				objects = append(objects, ss)

				// now add the extra ingress entry (use same certBundle)
				addIngressToClusterDeployment(cd, "secondingress", "moreingress.example.com", nil, nil, testDefaultIngressServingCertificate)
				cd.Spec.CertificateBundles = addCertificateBundlesForIngressList(cd)
				objects = append(objects, cd)

				// secrets for the clusterDeployment
				secrets := testSecretsForClusterDeployment(cd)
				for i := range secrets {
					objects = append(objects, &secrets[i])
				}
				return objects
			}(),
			expectedSyncSetIngressEntries: []SyncSetIngressEntry{
				{
					name:               testDefaultIngressName,
					domain:             testIngressDomain,
					defaultCertificate: fmt.Sprintf("%s-%s", testClusterName, testDefaultCertBundleSecret),
				},
				{
					name:               "secondingress",
					domain:             "moreingress.example.com",
					defaultCertificate: fmt.Sprintf("%s-%s", testClusterName, testDefaultCertBundleSecret),
				},
			},
			expectedSecretEntries: []string{
				// still just one secret since they are the same certBundle
				testClusterName + "-" + testDefaultCertBundleSecret,
			},
		},
		{
			name: "Test removing an ingress with custom certificate",
			localObjects: func() []runtime.Object {
				objects := []runtime.Object{}
				// syncset object with extra ingress (pointing to same certBundle)
				cdExtraIngress := testClusterDeploymentWithManualCertificate()
				addIngressToClusterDeployment(cdExtraIngress, "secondingress", "moreingress.example.com", nil, nil, testDefaultIngressServingCertificate)
				cdExtraIngress.Spec.CertificateBundles = addCertificateBundlesForIngressList(cdExtraIngress)
				ss := syncSetFromClusterDeployment(cdExtraIngress)
				objects = append(objects, ss)

				// clusterDeployment with only one ingress
				cd := testClusterDeploymentWithManualCertificate()
				objects = append(objects, cd)

				// secrets for the clusterDeployment
				secrets := testSecretsForClusterDeployment(cd)
				for i := range secrets {
					objects = append(objects, &secrets[i])
				}

				return objects
			}(),
			expectedSyncSetIngressEntries: []SyncSetIngressEntry{
				{
					name:               testDefaultIngressName,
					domain:             testIngressDomain,
					defaultCertificate: fmt.Sprintf("%s-%s", testClusterName, testDefaultCertBundleSecret),
				},
			},
			expectedSecretEntries: []string{
				fmt.Sprintf("%s-%s", testClusterName, testDefaultCertBundleSecret),
			},
		},
		{
			name: "Test two certbundles",
			localObjects: func() []runtime.Object {
				objects := []runtime.Object{}
				// cluster with two ingress to two different certbundles
				cd := testClusterDeploymentWithManualCertificate()
				addIngressToClusterDeployment(cd, "secondingress", "moreingress.example.com", nil, nil, "secondCertBundle")
				cd.Spec.CertificateBundles = addCertificateBundlesForIngressList(cd)
				objects = append(objects, cd)

				// add the secrets for the certbundles
				secrets := testSecretsForClusterDeployment(cd)
				for i := range secrets {
					objects = append(objects, &secrets[i])
				}

				return objects
			}(),
			expectedSyncSetIngressEntries: []SyncSetIngressEntry{
				{
					name:               testDefaultIngressName,
					domain:             testIngressDomain,
					defaultCertificate: fmt.Sprintf("%s-%s", testClusterName, testDefaultCertBundleSecret),
				},
				{
					name:               "secondingress",
					domain:             "moreingress.example.com",
					defaultCertificate: fmt.Sprintf("%s-secondCertBundle-secret", testClusterName),
				},
			},
			expectedSecretEntries: []string{
				fmt.Sprintf("%s-%s", testClusterName, testDefaultCertBundleSecret),
				fmt.Sprintf("%s-secondCertBundle-secret", testClusterName),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClient(test.localObjects...)

			helper := &fakeKubeCLI{
				t: t,
			}

			rcd := &ReconcileRemoteClusterIngress{
				Client:  fakeClient,
				scheme:  scheme.Scheme,
				logger:  log.WithField("controller", controllerName),
				kubeCLI: helper,
			}
			_, err := rcd.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testClusterName,
					Namespace: testNamespace,
				},
			})

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			validateSyncSet(t, helper.createdSyncSet, test.expectedSecretEntries, test.expectedSyncSetIngressEntries)
		})
	}
}

func TestRemoteClusterIngressReconcileConditions(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	ingresscontroller.AddToScheme(scheme.Scheme)

	tests := []struct {
		name                    string
		localObjects            []runtime.Object
		expectClusterCondition  bool
		expectedConditionReason string
		expectedConditionStatus corev1.ConditionStatus
	}{
		{
			name: "Test no issue no condition",
			localObjects: func() []runtime.Object {
				objects := []runtime.Object{}
				cd := testClusterDeploymentWithManualCertificate()
				objects = append(objects, cd)

				//create the secret for the certbundle
				secret := testSecretForCertificateBundle(cd.Spec.CertificateBundles[0])
				objects = append(objects, &secret)

				return objects
			}(),
			expectClusterCondition: false,
		},
		{
			name: "Test certbundle missing",
			localObjects: func() []runtime.Object {
				cd := testClusterDeploymentWithManualCertificate()
				cd.Spec.CertificateBundles = []hivev1.CertificateBundleSpec{}

				return []runtime.Object{cd}
			}(),
			expectClusterCondition:  true,
			expectedConditionReason: ingressCertificateNotFoundReason,
			expectedConditionStatus: corev1.ConditionTrue,
		},
		{
			name: "Test secret missing",
			localObjects: []runtime.Object{
				testClusterDeploymentWithManualCertificate(),
			},
			expectedConditionReason: ingressCertificateNotFoundReason,
			expectedConditionStatus: corev1.ConditionTrue,
		},
		{
			name: "Test clear previous condition",
			localObjects: func() []runtime.Object {
				objects := []runtime.Object{}
				cd := testClusterDeploymentWithManualCertificate()
				conditions := utils.SetClusterDeploymentCondition(cd.Status.Conditions,
					hivev1.IngressCertificateNotFoundCondition, corev1.ConditionTrue, ingressCertificateNotFoundReason, "TEST MISSING SECRET MESSAGE",
					utils.UpdateConditionIfReasonOrMessageChange)
				cd.Status.Conditions = conditions
				objects = append(objects, cd)

				secret := testSecretForCertificateBundle(cd.Spec.CertificateBundles[0])
				objects = append(objects, &secret)

				return objects
			}(),
			expectClusterCondition:  true,
			expectedConditionReason: ingressCertificateFoundReason,
			expectedConditionStatus: corev1.ConditionFalse,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClient(test.localObjects...)

			helper := &fakeKubeCLI{
				t: t,
			}

			rcd := &ReconcileRemoteClusterIngress{
				Client:  fakeClient,
				scheme:  scheme.Scheme,
				logger:  log.WithField("controller", controllerName),
				kubeCLI: helper,
			}
			_, err := rcd.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testClusterName,
					Namespace: testNamespace,
				},
			})

			assert.NoError(t, err, "unexpected error returned from Reconcile()")

			if test.expectClusterCondition {
				cd := hivev1.ClusterDeployment{}
				searchKey := types.NamespacedName{Name: testClusterName, Namespace: testNamespace}
				assert.NoError(t, fakeClient.Get(context.TODO(), searchKey, &cd), "error fetching resulting clusterDeployment")

				condition := utils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.IngressCertificateNotFoundCondition)
				assert.NotNil(t, condition, "didn't find expected condition")

				assert.Equal(t, test.expectedConditionReason, condition.Reason)
				assert.Equal(t, test.expectedConditionStatus, condition.Status)
			}
		})
	}

}
func testNamespaceSelector() *metav1.LabelSelector {
	return testRouteSelector()
}

func testRouteSelector() *metav1.LabelSelector {
	selector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"shard": "secretshard",
		},
	}
	return selector
}

func addIngressToClusterDeployment(cd *hivev1.ClusterDeployment, ingressName, ingressDomain string, routeSelector, namespaceSelector *metav1.LabelSelector, servingCertificate string) *hivev1.ClusterDeployment {
	cd.Spec.Ingress = append(cd.Spec.Ingress, hivev1.ClusterIngress{
		Name:               ingressName,
		Domain:             ingressDomain,
		RouteSelector:      routeSelector,
		NamespaceSelector:  namespaceSelector,
		ServingCertificate: servingCertificate,
	})

	return cd
}

func syncSetFromClusterDeployment(cd *hivev1.ClusterDeployment) *hivev1.SyncSet {
	rContext := reconcileContext{
		clusterDeployment: cd,
		certBundleSecrets: fakeSecretListForCertBundles(cd),
	}
	rawExtensions := rawExtensionsFromClusterDeployment(&rContext)
	srList := secretRefrenceFromClusterDeployment(&rContext)
	ssSpec := newSyncSetSpec(cd, rawExtensions, srList)
	return &hivev1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cd.Name + "clusteringress",
			Namespace: cd.Namespace,
		},
		Spec: *ssSpec,
	}
}

func fakeSecretListForCertBundles(cd *hivev1.ClusterDeployment) []*corev1.Secret {
	secrets := []*corev1.Secret{}

	for _, cb := range cd.Spec.CertificateBundles {
		secret := testSecretForCertificateBundle(cb)
		secrets = append(secrets, &secret)
	}

	return secrets
}

func testClusterDeploymentWithManualCertificate() *hivev1.ClusterDeployment {
	cd := testClusterDeploymentWithoutIngress()

	cd = addIngressToClusterDeployment(cd, testDefaultIngressName, testIngressDomain, nil, nil, testDefaultIngressServingCertificate)

	cd.Spec.CertificateBundles = addCertificateBundlesForIngressList(cd)

	return cd
}

func testClusterDeployment() *hivev1.ClusterDeployment {
	cd := testClusterDeploymentWithoutIngress()

	cd = addIngressToClusterDeployment(cd, testDefaultIngressName, testIngressDomain, nil, nil, "")

	return cd
}

func testClusterDeploymentWithoutIngress() *hivev1.ClusterDeployment {
	return &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testClusterName,
			Namespace: testNamespace,
		},
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterName: testClusterName,
			PlatformSecrets: hivev1.PlatformSecrets{
				AWS: &hivev1aws.PlatformSecrets{
					Credentials: corev1.LocalObjectReference{
						Name: "aws-credentials",
					},
				},
			},
		},
	}
}

func testSecretsForClusterDeployment(cd *hivev1.ClusterDeployment) []corev1.Secret {
	secrets := []corev1.Secret{}

	for _, certBundle := range cd.Spec.CertificateBundles {
		secret := testSecretForCertificateBundle(certBundle)
		secrets = append(secrets, secret)
	}

	return secrets
}

func testSecretForCertificateBundle(cb hivev1.CertificateBundleSpec) corev1.Secret {
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cb.SecretRef.Name,
			Namespace: testNamespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "Secret",
		},
		Data: map[string][]byte{
			"tls.crt": []byte("SOME_FAKE_CERTIFICATE_DATA"),
			"tls.key": []byte("SOME_FAKE_CERTIFICATE_KEY_DATA"),
		},
	}
	return secret
}

func addCertificateBundlesForIngressList(cd *hivev1.ClusterDeployment) []hivev1.CertificateBundleSpec {
	certBundles := []hivev1.CertificateBundleSpec{}
	certBundleAlreadyProcessed := map[string]bool{}

	for _, ingress := range cd.Spec.Ingress {
		if certBundleAlreadyProcessed[ingress.ServingCertificate] {
			continue
		}
		cb := hivev1.CertificateBundleSpec{
			Name: ingress.ServingCertificate,
			SecretRef: corev1.LocalObjectReference{
				Name: fmt.Sprintf("%s-secret", ingress.ServingCertificate),
			},
		}

		certBundles = append(certBundles, cb)
		// no need to make multiple certbundle entries for the same certbundle
		certBundleAlreadyProcessed[ingress.ServingCertificate] = true
	}

	return certBundles
}

type createdResourceInfo struct {
	name               string
	namespace          string
	kind               string
	domain             string
	namespaceSelector  *metav1.LabelSelector
	routeSelector      *metav1.LabelSelector
	defaultCertificate string
}
type createdSyncSetInfo struct {
	name      string
	namespace string
	resources []createdResourceInfo
}

type fakeKubeCLI struct {
	t              *testing.T
	createdSyncSet createdSyncSetInfo
}

func (f *fakeKubeCLI) ApplyRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (resource.ApplyResult, error) {
	ss := obj.(*hivev1.SyncSet)
	created := createdSyncSetInfo{
		name:      ss.Name,
		namespace: ss.Namespace,
	}

	for _, raw := range ss.Spec.Resources {
		sec, ok := raw.Object.(*corev1.Secret)
		if ok {
			cr := createdResourceInfo{
				name:      sec.Name,
				namespace: sec.Namespace,
				kind:      sec.Kind,
			}
			created.resources = append(created.resources, cr)
			continue
		}

		ic, ok := raw.Object.(*ingresscontroller.IngressController)
		if ok {
			cr := createdResourceInfo{
				name:              ic.Name,
				namespace:         ic.Namespace,
				kind:              ic.Kind,
				domain:            ic.Spec.Domain,
				namespaceSelector: ic.Spec.NamespaceSelector,
				routeSelector:     ic.Spec.RouteSelector,
			}
			if ic.Spec.DefaultCertificate != nil {
				cr.defaultCertificate = ic.Spec.DefaultCertificate.Name
			}
			created.resources = append(created.resources, cr)
			continue
		}
	}

	f.createdSyncSet = created

	return "", nil
}

func validateSyncSet(t *testing.T, existingSyncSet createdSyncSetInfo, expectedSecrets []string, expectedIngressControllers []SyncSetIngressEntry) {
	for _, secret := range expectedSecrets {
		found := false
		for _, resObj := range existingSyncSet.resources {
			if resObj.kind == "Secret" && resObj.name == secret {
				found = true
				break
			}
		}
		assert.True(t, found, "didn't find expected secret: %v", secret)
	}

	for _, ic := range expectedIngressControllers {
		found := false
		for _, resObj := range existingSyncSet.resources {
			if resObj.kind == "IngressController" && resObj.name == ic.name {
				found = true

				assert.Equal(t, ic.domain, resObj.domain, "unexpected domain for ingressController %v", ic.name)
				assert.Equal(t, ic.namespaceSelector, resObj.namespaceSelector, "unexpected namespaceSelector on ingressController: %v", ic.name)
				assert.Equal(t, ic.routeSelector, resObj.routeSelector, "unexpected routeSelector on ingressController: %v", ic.name)
				assert.Equal(t, ic.defaultCertificate, resObj.defaultCertificate, "unexpected DefaultCertificate on ingressController: %v", ic.name)
			}
		}
		assert.True(t, found, "didn't find expected ingressController: %v", ic.name)
	}
	return
}

func TestSecretHash(t *testing.T) {
	secret1 := &corev1.Secret{
		Data: map[string][]byte{
			"abc": []byte("12345"),
			"xyz": []byte("67890"),
		},
	}

	secret2 := &corev1.Secret{
		Data: map[string][]byte{
			"xyz": []byte("67890"),
			"abc": []byte("12345"),
		},
	}

	hash1 := secretHash(secret1)
	t.Logf("hash of secret1 is %s\n", hash1)
	hash2 := secretHash(secret2)
	t.Logf("hash of secret2 is %s\n", hash2)
	hash3 := secretHash(nil)
	t.Logf("hash of nil is %s\n", hash3)

	if len(hash1) == 0 || len(hash2) == 0 {
		t.Errorf("hash not expected to be empty string")
	}

	if len(hash3) != 0 {
		t.Errorf("hash expected to be empty string")
	}

	if hash1 != hash2 {
		t.Errorf("hashes expected to be equal")
	}
}
