package remoteclient

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	testfake "github.com/openshift/hive/pkg/test/fake"
)

const (
	testNamespace                                  = "test-namespace"
	testKubeconfigSecretName                       = "test-kubeconfig"
	apiURL                                         = "https://api.hive-cluster.example.com:6443"
	testControllerName       hivev1.ControllerName = "test-controller-name"
)

func TestNewBuilder(t *testing.T) {
	cd := testClusterDeployment()
	c := fakeClient(cd)
	controllerName := testControllerName
	expected := &builder{
		c:              c,
		cd:             cd,
		controllerName: controllerName,
		urlToUse:       activeURL,
	}
	actual := NewBuilder(c, cd, controllerName)
	assert.Equal(t, expected, actual, "unexpected builder")
}

func Test_InitialURL(t *testing.T) {
	cd := testClusterDeployment()
	kubeconfigSecret := testKubeconfigSecret(t)
	c := fakeClient(cd, kubeconfigSecret)
	expected := apiURL
	actual, err := InitialURL(c, cd)
	assert.NoError(t, err, "unexpected error getting API URL")
	assert.Equal(t, expected, actual, "unexpected API URL")
}

func Test_builder_RESTConfig(t *testing.T) {
	cases := []struct {
		name                string
		overrideURL         string
		overrideActive      bool
		usePrimary          bool
		useSecondary        bool
		expectedHost        string
		apiServerIPOverride string
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
		{
			name:                "apiServerIPOverride is set",
			expectedHost:        apiURL,
			apiServerIPOverride: "10.0.4.6",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cd := testClusterDeployment()
			setAPIURLOverride(cd, tc.overrideURL)
			if tc.overrideActive {
				setOverrideActive(cd)
			}
			if tc.apiServerIPOverride != "" {
				cd.Spec.ControlPlaneConfig.APIServerIPOverride = tc.apiServerIPOverride
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

			if tc.apiServerIPOverride != "" {
				assert.NotEmpty(t, cfg.Dial, "unexpected absence of a custom dial")
			} else {
				assert.Empty(t, cfg.Dial, "unexpected custom dial")
			}
		})
	}
}

func Test_Unreachable(t *testing.T) {
	probeTime := time.Unix(123456789, 0)
	cases := []struct {
		name                string
		cd                  *hivev1.ClusterDeployment
		expectedUnreachable bool
		expectedLastCheck   time.Time
	}{
		{
			name: "unreachable still unknown",
			cd: testcd.Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Status: corev1.ConditionUnknown,
				Type:   hivev1.UnreachableCondition,
			})),
			expectedUnreachable: true,
		},
		{
			name:                "unreachable true",
			cd:                  testcd.Build(withUnreachableCondition(corev1.ConditionTrue, probeTime)),
			expectedUnreachable: true,
			expectedLastCheck:   probeTime,
		},
		{
			name:                "unreachable false",
			cd:                  testcd.Build(withUnreachableCondition(corev1.ConditionFalse, probeTime)),
			expectedUnreachable: false,
			expectedLastCheck:   probeTime,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actualUnreachable, actualLastCheck := Unreachable(tc.cd)
			assert.Equal(t, tc.expectedUnreachable, actualUnreachable, "unexpected unreachable")
			assert.Equal(t, tc.expectedLastCheck, actualLastCheck, "unexpected last check")
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
				rc, buildErr := builder.Build()
				assert.NoError(t, buildErr, "unexpected error building client")
				namespaced_name := types.NamespacedName{
					Name:      "bad-name",
					Namespace: "bad-namespace",
				}
				err = rc.Get(context.Background(), namespaced_name, &hivev1.ClusterDeployment{})
			} else {
				rc, buildErr := builder.BuildDynamic()
				assert.NoError(t, buildErr, "unexpected error building dynamic client")
				_, err = rc.Resource(hivev1.Resource("ClusterDeployment").WithVersion("v1")).
					Get(context.Background(), "bad-name", metav1.GetOptions{})
			}
			if assert.Error(t, err, "expected error") {
				assert.Contains(t, err.Error(), tc.expectedHost, "expected to find host in error")
				assert.Contains(t, err.Error(), "no such host", "expected to find \"no such host\" in error")
			}
		})
	}
}

func fakeClient(objects ...runtime.Object) client.Client {
	return testfake.NewFakeClientBuilder().WithRuntimeObjects(objects...).Build()
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

func withUnreachableCondition(status corev1.ConditionStatus, probeTime time.Time) testcd.Option {
	return testcd.WithCondition(
		hivev1.ClusterDeploymentCondition{
			Type:          hivev1.UnreachableCondition,
			Status:        status,
			LastProbeTime: metav1.NewTime(probeTime),
		},
	)
}

func testKubeconfigSecret(t *testing.T) *corev1.Secret {
	kubeconfigFile := filepath.Join("testdata", "kubeconfig.sample")
	kubeconfig, err := os.ReadFile(kubeconfigFile)
	if err != nil {
		t.Fatal(err)
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      testKubeconfigSecretName,
		},
		Data: map[string][]byte{constants.KubeconfigSecretKey: kubeconfig},
	}
}
