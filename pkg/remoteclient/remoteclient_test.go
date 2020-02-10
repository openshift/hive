package remoteclient

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/pointer"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
)

const (
	testNamespace            = "test-namespace"
	testKubeconfigSecretName = "test-kubeconfig"
	apiURL                   = "https://api.hive-cluster.example.com:6443"
)

func TestNewBuilder(t *testing.T) {
	cd := testClusterDeployment()
	c := fakeClient(cd)
	controllerName := "test-controller-name"
	expected := &builder{
		c:              c,
		cd:             cd,
		controllerName: controllerName,
		urlToUse:       activeURL,
	}
	actual := NewBuilder(c, cd, controllerName)
	assert.Equal(t, expected, actual, "unexpected builder")
}

func Test_builder_APIURL(t *testing.T) {
	cd := testClusterDeployment()
	kubeconfigSecret := testKubeconfigSecret(t)
	c := fakeClient(cd, kubeconfigSecret)
	builder := NewBuilder(c, cd, "test-controller-name")
	expected := apiURL
	actual, err := builder.APIURL()
	assert.NoError(t, err, "unexpected error getting API URL")
	assert.Equal(t, expected, actual, "unexpected API URL")
}

func Test_builder_RESTConfig(t *testing.T) {
	cases := []struct {
		name           string
		overrideURL    string
		overrideActive bool
		usePrimary     bool
		useSecondary   bool
		expectedHost   string
	}{
		{
			name:         "no override",
			expectedHost: apiURL,
		},
		{
			name:         "no override, use primary",
			usePrimary:   true,
			expectedHost: apiURL,
		},
		{
			name:         "no override, use secondary",
			useSecondary: true,
			expectedHost: apiURL,
		},
		{
			name:         "override inactive",
			overrideURL:  "url-override",
			expectedHost: apiURL,
		},
		{
			name:         "override inactive, use primary",
			overrideURL:  "url-override",
			usePrimary:   true,
			expectedHost: "url-override",
		},
		{
			name:         "override inactive, use secondary",
			overrideURL:  "url-override",
			useSecondary: true,
			expectedHost: apiURL,
		},
		{
			name:           "override active",
			overrideURL:    "url-override",
			overrideActive: true,
			expectedHost:   "url-override",
		},
		{
			name:           "override active, use primary",
			overrideURL:    "url-override",
			overrideActive: true,
			usePrimary:     true,
			expectedHost:   "url-override",
		},
		{
			name:           "override active, use secondary",
			overrideURL:    "url-override",
			overrideActive: true,
			useSecondary:   true,
			expectedHost:   apiURL,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cd := testClusterDeployment()
			setAPIURLOverride(cd, tc.overrideURL)
			if tc.overrideActive {
				setOverrideActive(cd)
			}
			kubeconfigSecret := testKubeconfigSecret(t)
			c := fakeClient(cd, kubeconfigSecret)
			builder := NewBuilder(c, cd, "test-controller-name")
			switch {
			case tc.usePrimary:
				builder.UsePrimaryAPIURL()
			case tc.useSecondary:
				builder.UseSecondaryAPIURL()
			}
			cfg, err := builder.RESTConfig()
			assert.NoError(t, err, "unexpected error getting REST config")
			assert.Equal(t, tc.expectedHost, cfg.Host, "unexpected host")
		})
	}
}

func Test_builder_Unreachable(t *testing.T) {
	cases := []struct {
		name        string
		unreachable *bool
		expected    bool
	}{
		{
			name:     "unreachable unset",
			expected: false,
		},
		{
			name:        "unreachable true",
			unreachable: pointer.BoolPtr(true),
			expected:    true,
		},
		{
			name:        "unreachable false",
			unreachable: pointer.BoolPtr(false),
			expected:    false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cd := testClusterDeployment()
			if tc.unreachable != nil {
				setUnreachable(cd, *tc.unreachable)
			}
			c := fakeClient(cd)
			builder := NewBuilder(c, cd, "test-controller-name")
			actual := builder.Unreachable()
			assert.Equal(t, tc.expected, actual, "unexpected unreachable")
		})
	}
}

func Test_builder_Build(t *testing.T) {
	cases := []struct {
		name         string
		dynamic      bool
		overrideURL  string
		expectedHost string
	}{
		{
			name:         "no override",
			expectedHost: apiURL,
		},
		{
			name:         "override active",
			overrideURL:  "url-override",
			expectedHost: "url-override",
		},
		{
			name:         "no override, dynamic",
			dynamic:      true,
			expectedHost: apiURL,
		},
		{
			name:         "override active, dynamic",
			dynamic:      true,
			overrideURL:  "url-override",
			expectedHost: "url-override",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cd := testClusterDeployment()
			setAPIURLOverride(cd, tc.overrideURL)
			setOverrideActive(cd)
			kubeconfigSecret := testKubeconfigSecret(t)
			c := fakeClient(cd, kubeconfigSecret)
			builder := NewBuilder(c, cd, "test-controller-name")
			var err error
			if !tc.dynamic {
				_, err = builder.Build()
			} else {
				rc, buildErr := builder.BuildDynamic()
				assert.NoError(t, buildErr, "unexpected error building dynamic client")
				_, err = rc.Resource(hivev1.Resource("ClusterDeployment").WithVersion("v1")).
					Get("bad-name", metav1.GetOptions{})
			}
			if assert.Error(t, err, "expected error building") {
				assert.Contains(t, err.Error(), tc.expectedHost, "expected to find host in error")
				assert.Contains(t, err.Error(), "no such host", "expected to find \"no such host\" in error")
			}
		})
	}
}

func fakeClient(objects ...runtime.Object) client.Client {
	scheme := runtime.NewScheme()
	hivev1.AddToScheme(scheme)
	corev1.AddToScheme(scheme)
	return fake.NewFakeClientWithScheme(scheme, objects...)
}

func testClusterDeployment() *hivev1.ClusterDeployment {
	return &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-deployment",
			Namespace: testNamespace,
		},
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterMetadata: &hivev1.ClusterMetadata{
				AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: testKubeconfigSecretName},
			},
		},
	}
}

func setAPIURLOverride(cd *hivev1.ClusterDeployment, urlOverride string) {
	cd.Spec.ControlPlaneConfig.APIURLOverride = urlOverride
}

func setOverrideActive(cd *hivev1.ClusterDeployment) {
	cd.Status.Conditions = append(cd.Status.Conditions,
		hivev1.ClusterDeploymentCondition{
			Type:   hivev1.ActiveAPIURLOverrideCondition,
			Status: corev1.ConditionTrue,
		},
	)
}

func setUnreachable(cd *hivev1.ClusterDeployment, unreachable bool) {
	status := corev1.ConditionFalse
	if unreachable {
		status = corev1.ConditionTrue
	}
	cd.Status.Conditions = append(cd.Status.Conditions,
		hivev1.ClusterDeploymentCondition{
			Type:   hivev1.UnreachableCondition,
			Status: status,
		},
	)
}

func testKubeconfigSecret(t *testing.T) *corev1.Secret {
	kubeconfigFile := filepath.Join("testdata", "kubeconfig.sample")
	kubeconfig, err := ioutil.ReadFile(kubeconfigFile)
	if err != nil {
		t.Fatal(err)
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      testKubeconfigSecretName,
		},
		Data: map[string][]byte{adminKubeconfigKey: kubeconfig},
	}
}
