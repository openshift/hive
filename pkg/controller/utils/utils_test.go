package utils

import (
	"context"
	goerrors "errors"
	"fmt"
	"os"
	"testing"
	"time"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hiveassert "github.com/openshift/hive/pkg/test/assert"
	testfake "github.com/openshift/hive/pkg/test/fake"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"golang.org/x/time/rate"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	testControllerName = "foo"
)

func TestMergeJsons(t *testing.T) {
	tests := []struct {
		name        string
		jsonStr1    string
		jsonStr2    string
		expectedStr string
		expectedErr bool
	}{
		{
			name:        "Merge pull secrets 01",
			jsonStr1:    `{"auths": {"registry.svc.ci.okd.org": {"auth": "dXNljlfjldsfSDD"}}}`,
			jsonStr2:    `{"auths":{"cloud.okd.com":{"auth":"b34xVjWERckjfUyV1pMQTc=","email":"abc@xyz.com"},"quay.io":{"auth":"b3BlbnNoVkc=","email":"abc@xyz.com"},"registry.connect.redhat.com":{"auth":"NjQ4ODeDZ3d1pN","email":"abc@xyz.com"},"registry.redhat.io":{"auth":"NjQ4ODX1pN","email":"abc@xyz.com"}}}`,
			expectedStr: `{"auths":{"cloud.okd.com":{"auth":"b34xVjWERckjfUyV1pMQTc=","email":"abc@xyz.com"},"quay.io":{"auth":"b3BlbnNoVkc=","email":"abc@xyz.com"},"registry.connect.redhat.com":{"auth":"NjQ4ODeDZ3d1pN","email":"abc@xyz.com"},"registry.redhat.io":{"auth":"NjQ4ODX1pN","email":"abc@xyz.com"},"registry.svc.ci.okd.org":{"auth":"dXNljlfjldsfSDD"}}}`,
		},
		{
			name:        "Merge pull secrets 02",
			jsonStr1:    `{"auths": {"registry.svc.ci.okd.org": {"auth": "dXNljlfjldsfSDD"}}}`,
			jsonStr2:    `{"auths":{"cloud.okd.com":{"auth":"b34xVjWERckjfUyV1pMQTc=","email":"abc@xyz.com"}}}`,
			expectedStr: `{"auths":{"cloud.okd.com":{"auth":"b34xVjWERckjfUyV1pMQTc=","email":"abc@xyz.com"},"registry.svc.ci.okd.org":{"auth":"dXNljlfjldsfSDD"}}}`,
		},
		{
			name:        "Merege global and local same auth key but different secret",
			jsonStr1:    `{"auths":{"cloud.okd.com":{"auth":"b34xVjWERckjfUyV1pMQTc=","email":"abc@xyz.com"}}}`,
			jsonStr2:    `{"auths":{"cloud.okd.com":{"auth":"b34xVjWERckjfUyV1pMQTlocal=","email":"abc@xyz.com"}}}`,
			expectedStr: `{"auths":{"cloud.okd.com":{"auth":"b34xVjWERckjfUyV1pMQTlocal=","email":"abc@xyz.com"}}}`,
		},
		{
			name:        "Merge of pull secrets should fail",
			jsonStr1:    `{"auths": {"registry.svc.ci.okd.org": {"auth": "dXNljlfjldsfSDD"}}}`,
			jsonStr2:    `{"auths":{"cloud.okd.com":{"auth":"b34xVjWERckjfUyV1pMQTc=","email":"abc@xyz.com"}}`,
			expectedErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tLogger := log.New()
			resultStr, err := MergeJsons(test.jsonStr1, test.jsonStr2, tLogger)
			if test.expectedErr {
				assert.Error(t, err)
			} else {
				assert.Equal(t, test.expectedStr, resultStr)
			}
		})
	}

}

func TestLogLevel(t *testing.T) {
	cases := []struct {
		name          string
		err           error
		expectedLevel log.Level
	}{
		{
			name:          "nil",
			err:           nil,
			expectedLevel: log.ErrorLevel,
		},
		{
			name:          "already exists",
			err:           apierrors.NewAlreadyExists(schema.GroupResource{}, ""),
			expectedLevel: log.InfoLevel,
		},
		{
			name:          "conflict",
			err:           apierrors.NewConflict(schema.GroupResource{}, "", nil),
			expectedLevel: log.InfoLevel,
		},
		{
			name:          "not found",
			err:           apierrors.NewNotFound(schema.GroupResource{}, ""),
			expectedLevel: log.InfoLevel,
		},
		{
			name:          "other api error",
			err:           apierrors.NewUnauthorized(""),
			expectedLevel: log.ErrorLevel,
		},
		{
			name:          "wrapped info-level error",
			err:           errors.Wrap(apierrors.NewAlreadyExists(schema.GroupResource{}, ""), "wrapper"),
			expectedLevel: log.InfoLevel,
		},
		{
			name:          "wrapped error-level error",
			err:           errors.Wrap(apierrors.NewUnauthorized(""), "wrapper"),
			expectedLevel: log.ErrorLevel,
		},
		{
			name:          "double-wrapped error",
			err:           errors.Wrap(errors.Wrap(apierrors.NewAlreadyExists(schema.GroupResource{}, ""), "inner wrapper"), "outer wrapper"),
			expectedLevel: log.InfoLevel,
		},
	}
	for _, tc := range cases {
		actualLevel := LogLevel(tc.err)
		assert.Equal(t, tc.expectedLevel, actualLevel)
	}
}

func TestGetConcurrentReconciles(t *testing.T) {
	cases := []struct {
		name                         string
		environmentVariables         map[string]string
		expectedConcurrentReconciles int
		expectedError                bool
	}{
		{
			name: "Only default goroutines is set",
			environmentVariables: map[string]string{
				fmt.Sprintf(ConcurrentReconcilesEnvVariableFormat, "default"): "10",
			},
			expectedConcurrentReconciles: 10,
		},
		{
			name: "Only controller goroutines is set",
			environmentVariables: map[string]string{
				fmt.Sprintf(ConcurrentReconcilesEnvVariableFormat, testControllerName): "10",
			},
			expectedConcurrentReconciles: 10,
		},
		{
			name: "Both default and controller goroutines are set",
			environmentVariables: map[string]string{
				fmt.Sprintf(ConcurrentReconcilesEnvVariableFormat, "default"):          "10",
				fmt.Sprintf(ConcurrentReconcilesEnvVariableFormat, testControllerName): "20",
			},
			expectedConcurrentReconciles: 20,
		},
		{
			name:                         "Neither default nor controller goroutines are set",
			environmentVariables:         map[string]string{},
			expectedConcurrentReconciles: defaultConcurrentReconciles,
		},
		{
			name: "default goroutines are set incorrectly",
			environmentVariables: map[string]string{
				fmt.Sprintf(ConcurrentReconcilesEnvVariableFormat, "default"): "not-a-int",
			},
			expectedError: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// set environment variables
			for k, v := range tc.environmentVariables {
				os.Setenv(k, v)
				defer os.Unsetenv(k)
			}

			concurrentReconciles, err := getConcurrentReconciles(testControllerName)
			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equalf(t, tc.expectedConcurrentReconciles, concurrentReconciles, "unexpected concurrent reconciles")
		})
	}
}

func TestGetClientRateLimiter(t *testing.T) {
	cases := []struct {
		name                 string
		environmentVariables map[string]string
		expectedRateLimiter  flowcontrol.RateLimiter
		expectedError        bool
	}{
		{
			name:                 "No qps or burst is set",
			environmentVariables: map[string]string{},
			expectedRateLimiter:  flowcontrol.NewTokenBucketRateLimiter(rest.DefaultQPS, rest.DefaultBurst),
		},
		{
			name: "default qps and default burst is set",
			environmentVariables: map[string]string{
				fmt.Sprintf(ClientQPSEnvVariableFormat, "default"):   "500",
				fmt.Sprintf(ClientBurstEnvVariableFormat, "default"): "1000",
			},
			expectedRateLimiter: flowcontrol.NewTokenBucketRateLimiter(500, 1000),
		},
		{
			name: "controller qps and burst are set",
			environmentVariables: map[string]string{
				fmt.Sprintf(ClientQPSEnvVariableFormat, testControllerName):   "500",
				fmt.Sprintf(ClientBurstEnvVariableFormat, testControllerName): "1000",
			},
			expectedRateLimiter: flowcontrol.NewTokenBucketRateLimiter(500, 1000),
		},
		{
			name: "Both default as well as controller qps and burst are set",
			environmentVariables: map[string]string{
				fmt.Sprintf(ClientQPSEnvVariableFormat, "default"):            "500",
				fmt.Sprintf(ClientQPSEnvVariableFormat, testControllerName):   "501",
				fmt.Sprintf(ClientBurstEnvVariableFormat, "default"):          "1000",
				fmt.Sprintf(ClientBurstEnvVariableFormat, testControllerName): "1001",
			},
			expectedRateLimiter: flowcontrol.NewTokenBucketRateLimiter(501, 1001),
		},
		{
			name: "default qps is set incorrectly",
			environmentVariables: map[string]string{
				fmt.Sprintf(ClientQPSEnvVariableFormat, "default"): "not-a-int",
			},
			expectedError: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// set environment variables
			for k, v := range tc.environmentVariables {
				os.Setenv(k, v)
				defer os.Unsetenv(k)
			}
			rateLimiter, err := getClientRateLimiter(testControllerName)
			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equalf(t, tc.expectedRateLimiter, rateLimiter, "unexpected rate limiter")
		})
	}
}

func TestGetQueueRateLimiter(t *testing.T) {
	cases := []struct {
		name                 string
		environmentVariables map[string]string
		expectedRateLimiter  workqueue.RateLimiter
		expectedError        bool
	}{
		{
			name:                 "No qps or burst is set",
			environmentVariables: map[string]string{},
			expectedRateLimiter: workqueue.NewMaxOfRateLimiter(
				workqueue.NewItemExponentialFailureRateLimiter(5*time.Millisecond, 1000*time.Second),
				&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(defaultQueueQPS), defaultQueueBurst)},
			),
		},
		{
			name: "default qps and default burst is set",
			environmentVariables: map[string]string{
				fmt.Sprintf(QueueQPSEnvVariableFormat, "default"):   "500",
				fmt.Sprintf(QueueBurstEnvVariableFormat, "default"): "1000",
			},
			expectedRateLimiter: workqueue.NewMaxOfRateLimiter(
				workqueue.NewItemExponentialFailureRateLimiter(5*time.Millisecond, 1000*time.Second),
				&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(500), 1000)},
			),
		},
		{
			name: "controller qps and burst are set",
			environmentVariables: map[string]string{
				fmt.Sprintf(QueueQPSEnvVariableFormat, testControllerName):   "500",
				fmt.Sprintf(QueueBurstEnvVariableFormat, testControllerName): "1000",
			},
			expectedRateLimiter: workqueue.NewMaxOfRateLimiter(
				workqueue.NewItemExponentialFailureRateLimiter(5*time.Millisecond, 1000*time.Second),
				&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(500), 1000)},
			),
		},
		{
			name: "Both default as well as controller qps and burst are set",
			environmentVariables: map[string]string{
				fmt.Sprintf(QueueQPSEnvVariableFormat, "default"):            "500",
				fmt.Sprintf(QueueQPSEnvVariableFormat, testControllerName):   "501",
				fmt.Sprintf(QueueBurstEnvVariableFormat, "default"):          "1000",
				fmt.Sprintf(QueueBurstEnvVariableFormat, testControllerName): "1001",
			},
			expectedRateLimiter: workqueue.NewMaxOfRateLimiter(
				workqueue.NewItemExponentialFailureRateLimiter(5*time.Millisecond, 1000*time.Second),
				&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(501), 1001)},
			),
		},
		{
			name: "default qps is set incorrectly",
			environmentVariables: map[string]string{
				fmt.Sprintf(QueueQPSEnvVariableFormat, "default"): "not-a-int",
			},
			expectedError: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// set environment variables
			for k, v := range tc.environmentVariables {
				os.Setenv(k, v)
				defer os.Unsetenv(k)
			}
			rateLimiter, err := getQueueRateLimiter(testControllerName)
			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equalf(t, tc.expectedRateLimiter, rateLimiter, "unexpected rate limiter")
		})
	}
}

func TestEnsureRequeueAtLeastWithin(t *testing.T) {
	cases := []struct {
		name           string
		duration       time.Duration
		result         reconcile.Result
		err            error
		expectedResult reconcile.Result
		expectedErr    error
	}{
		{
			name:           "no error or requeue",
			duration:       time.Hour,
			expectedResult: reconcile.Result{RequeueAfter: time.Hour, Requeue: true},
		},
		{
			name:        "error",
			duration:    time.Hour,
			err:         goerrors.New("test error"),
			expectedErr: goerrors.New("test error"),
		},
		{
			name:           "requeue",
			duration:       time.Hour,
			result:         reconcile.Result{Requeue: true},
			expectedResult: reconcile.Result{Requeue: true},
		},
		{
			name:           "shorter requeue after",
			duration:       time.Hour,
			result:         reconcile.Result{RequeueAfter: time.Minute},
			expectedResult: reconcile.Result{RequeueAfter: time.Minute},
		},
		{
			name:           "longer requeue after",
			duration:       time.Hour,
			result:         reconcile.Result{RequeueAfter: 2 * time.Hour},
			expectedResult: reconcile.Result{RequeueAfter: time.Hour, Requeue: true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actualResult, actualErr := EnsureRequeueAtLeastWithin(tc.duration, tc.result, tc.err)
			assert.Equal(t, tc.expectedResult, actualResult, "unexpected reconcile result")
			assert.Equal(t, tc.expectedErr, actualErr, "unexpected error")
		})
	}
}

const (
	testHttpProxy  = "localhost:3112"
	testHttpsProxy = "localhost:4432"
	testNoProxy    = "example.com,foo.com,bar.org"
)

func TestSetProxyEnvVars(t *testing.T) {
	createPodSpec := func(containers int, origEnvVars map[string]string) *corev1.PodSpec {
		podSpec := &corev1.PodSpec{}
		envVars := []corev1.EnvVar{}
		for k, v := range origEnvVars {
			envVars = append(envVars, corev1.EnvVar{Name: k, Value: v})
		}
		for i := 0; i < containers; i++ {
			c := corev1.Container{
				Name: fmt.Sprintf("container%d", i),
				Env:  envVars,
			}
			podSpec.Containers = append(podSpec.Containers, c)

		}
		return podSpec
	}
	cases := []struct {
		name                           string
		podSpec                        *corev1.PodSpec
		httpProxy, httpsProxy, noProxy string
		expectedEnvVars                map[string]string
		expectedErr                    bool
	}{
		{
			name: "add all proxy env vars",
			podSpec: createPodSpec(1, map[string]string{
				"foo": "bar",
			}),
			httpProxy:  testHttpProxy,
			httpsProxy: testHttpsProxy,
			noProxy:    testNoProxy,
			expectedEnvVars: map[string]string{
				"foo":         "bar",
				"HTTP_PROXY":  testHttpProxy,
				"HTTPS_PROXY": testHttpsProxy,
				"NO_PROXY":    testNoProxy,
			},
		},
		{
			name: "add some proxy env vars",
			podSpec: createPodSpec(1, map[string]string{
				"foo": "bar",
			}),
			httpProxy: testHttpProxy,
			expectedEnvVars: map[string]string{
				"foo":        "bar",
				"HTTP_PROXY": testHttpProxy,
			},
		},
		{
			name: "add no proxy env vars",
			podSpec: createPodSpec(1, map[string]string{
				"foo": "bar",
			}),
			expectedEnvVars: map[string]string{
				"foo": "bar",
			},
		},
		{
			name: "container already has proxy env vars",
			podSpec: createPodSpec(1, map[string]string{
				"foo":         "bar",
				"HTTP_PROXY":  testHttpProxy,
				"HTTPS_PROXY": testHttpsProxy,
				"NO_PROXY":    testNoProxy,
			}),
			httpProxy:  testHttpProxy,
			httpsProxy: testHttpsProxy,
			noProxy:    testNoProxy,
			expectedEnvVars: map[string]string{
				"foo":         "bar",
				"HTTP_PROXY":  testHttpProxy,
				"HTTPS_PROXY": testHttpsProxy,
				"NO_PROXY":    testNoProxy,
			},
		},
		{
			name: "container already has proxy env vars diff values",
			podSpec: createPodSpec(1, map[string]string{
				"foo":         "bar",
				"HTTP_PROXY":  "foo",
				"HTTPS_PROXY": "bar",
				"NO_PROXY":    "no",
			}),
			httpProxy:  testHttpProxy,
			httpsProxy: testHttpsProxy,
			noProxy:    testNoProxy,
			expectedEnvVars: map[string]string{
				"foo":         "bar",
				"HTTP_PROXY":  testHttpProxy,
				"HTTPS_PROXY": testHttpsProxy,
				"NO_PROXY":    testNoProxy,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			SetProxyEnvVars(tc.podSpec, tc.httpProxy, tc.httpsProxy, tc.noProxy)
			for _, c := range tc.podSpec.Containers {
				assert.Equal(t, len(tc.expectedEnvVars), len(c.Env), "unexpected env var cound on container %s", c.Name)
			}
			for k, v := range tc.expectedEnvVars {
				hiveassert.AssertAllContainersHaveEnvVar(t, tc.podSpec, k, v)
			}
		})
	}
}

func TestSafeDelete(t *testing.T) {
	fakeClient := testfake.NewFakeClientBuilder().Build()

	cp := &hivev1.ClusterPool{
		ObjectMeta: v1.ObjectMeta{
			Name:      "foo",
			Namespace: "bar",
		},
	}

	cp1 := cp.DeepCopy()
	t.Log("1) Deletion with matching ResourceVersion")
	if err := fakeClient.Create(context.Background(), cp1); err != nil {
		t.Errorf("Failed initial creation: %v", err)
	}
	t.Logf("ResourceVersion after Creation: %s", cp1.ResourceVersion)

	// SafeDelete works if we don't muck with the ResourceVersion
	if err := SafeDelete(fakeClient, context.Background(), cp1); err != nil {
		t.Errorf("SafeDelete() error: %v", err)
	}

	t.Log("2) Deletion with stale ResourceVersion")
	cp2 := cp.DeepCopy()
	if err := fakeClient.Create(context.Background(), cp2); err != nil {
		t.Errorf("Failed creation: %v", err)
	}

	cp3 := cp2.DeepCopy()
	// Make a change to the created object
	cp2.Spec.BaseDomain = "foo.example.com"
	if err := fakeClient.Update(context.Background(), cp2); err != nil {
		t.Errorf("Failed update: %v", err)
	}
	assert.NotEqual(t, cp3.ResourceVersion, cp2.ResourceVersion)
	t.Logf("ResourceVersion after update: %s", cp2.ResourceVersion)

	// Now try to delete the object using the stale copy
	err := SafeDelete(fakeClient, context.Background(), cp3)
	assert.True(
		t, apierrors.IsConflict(err),
		"Expected conflict error deleting stale copy with ResourceVersion %s, but got %s", cp3.ResourceVersion, err)
}
