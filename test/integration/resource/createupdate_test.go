package resource

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/openshift/hive/pkg/resource"
)

func TestCreateOrUpdate(t *testing.T) {
	tests := []struct {
		name           string
		existing       []runtime.Object
		expectedResult resource.ApplyResult
		apply          runtime.Object
		validate       func(t *testing.T, info *resource.Info, ns string)
	}{
		{
			name:           "create large resource",
			apply:          largeSecret(),
			expectedResult: resource.CreatedApplyResult,
			validate: func(t *testing.T, info *resource.Info, ns string) {
				expected := largeSecret()
				if info.Name != expected.Name {
					t.Errorf("unexpected info name: %s", info.Name)
				}
				if info.Namespace != ns {
					t.Errorf("unexpected info namespace: %s", info.Namespace)
				}
				if info.Kind != "Secret" {
					t.Errorf("unexpected info kind: %s", info.Kind)
				}

				s := &corev1.Secret{}
				err := c.Get(context.TODO(), types.NamespacedName{Name: info.Name, Namespace: info.Namespace}, s)
				if err != nil {
					t.Errorf("unexpected error retrieving secret: %v", err)
					return
				}
				if !reflect.DeepEqual(expected.Data, s.Data) {
					t.Errorf("unexpected secret data: %v", s.Data)
				}
			},
		},
		{
			name:           "update large resource",
			existing:       []runtime.Object{largeSecret()},
			expectedResult: resource.ConfiguredApplyResult,
			apply: func() runtime.Object {
				secret := largeSecret()
				secret.Data["foo"] = []byte("baz")
				return secret
			}(),
			validate: func(t *testing.T, info *resource.Info, ns string) {
				s := &corev1.Secret{}
				err := c.Get(context.TODO(), types.NamespacedName{Name: info.Name, Namespace: info.Namespace}, s)
				if err != nil {
					t.Errorf("unexpected error retrieving secret: %v", err)
					return
				}
				if string(s.Data["foo"]) != "baz" {
					t.Errorf("unexpected data: %s", string(s.Data["foo"]))
				}
			},
		},
		{
			name:           "unchanged large resource",
			existing:       []runtime.Object{largeSecret()},
			expectedResult: resource.UnchangedApplyResult,
			apply:          largeSecret(),
			validate: func(t *testing.T, info *resource.Info, ns string) {
				s := &corev1.Secret{}
				err := c.Get(context.TODO(), types.NamespacedName{Name: info.Name, Namespace: info.Namespace}, s)
				if err != nil {
					t.Errorf("unexpected error retrieving secret: %v", err)
					return
				}
			},
		},
	}

	configs := []string{"restconfig", "kubeconfig"}

	for _, test := range tests {
		for _, clientConfig := range configs {
			t.Run(test.name, func(t *testing.T) {
				logger := log.WithField("test", test.name)
				namespace := &corev1.Namespace{}
				namespace.GenerateName = "apply-test-"
				err := c.Create(context.TODO(), namespace)
				if err != nil {
					t.Fatalf("unexpected err: %v", err)
				}
				var h *resource.Helper
				if clientConfig == "kubeconfig" {
					h = resource.NewHelper(kubeconfig, logger)
				} else {
					h = resource.NewHelperFromRESTConfig(cfg, logger)
				}
				accessor := meta.NewAccessor()
				for _, obj := range test.existing {
					o := obj.DeepCopyObject()
					accessor.SetNamespace(o, namespace.Name)
					_, err := h.CreateOrUpdateRuntimeObject(o, scheme.Scheme)
					if err != nil {
						t.Fatalf("unexpected err: %v", err)
					}
				}
				accessor.SetNamespace(test.apply, namespace.Name)
				data, err := h.Serialize(test.apply, scheme.Scheme)
				t.Logf("The serialized resource:\n%s\n", string(data))
				info, err := h.Info(data)
				if err != nil {
					t.Errorf("unexpected error calling info: %v", err)
					return
				}
				applyResult, err := h.CreateOrUpdate(data)
				if err != nil {
					t.Errorf("unexpected error calling apply: %v", err)
					return
				}
				if applyResult != test.expectedResult {
					t.Errorf("unexpected apply result: %v", applyResult)
				}
				test.validate(t, info, namespace.Name)
			})
		}
	}
}

func largeSecret() *corev1.Secret {
	s := &corev1.Secret{}
	s.Name = "large-secret"
	s.Data = map[string][]byte{
		"one":   largeData("1"),
		"two":   largeData("2"),
		"three": largeData("3"),
	}
	return s
}

func largeData(value string) []byte {
	return bytes.Repeat([]byte(value), 90000)
}
