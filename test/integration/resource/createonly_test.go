package resource

import (
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

func TestCreateOnly(t *testing.T) {
	tests := []struct {
		name           string
		existing       []runtime.Object
		expectedResult resource.ApplyResult
		apply          runtime.Object
		validate       func(t *testing.T, info *resource.Info, ns string)
	}{
		{
			name:           "create resource",
			apply:          testConfigMap(),
			expectedResult: resource.CreatedApplyResult,
			validate: func(t *testing.T, info *resource.Info, ns string) {
				expected := testConfigMap()
				if info.Name != expected.Name {
					t.Errorf("unexpected info name: %s", info.Name)
				}
				if info.Namespace != ns {
					t.Errorf("unexpected info namespace: %s", info.Namespace)
				}
				if info.Kind != "ConfigMap" {
					t.Errorf("unexpected info kind: %s", info.Kind)
				}

				cm := &corev1.ConfigMap{}
				err := c.Get(context.TODO(), types.NamespacedName{Name: info.Name, Namespace: info.Namespace}, cm)
				if err != nil {
					t.Errorf("unexpected error retrieving configmap: %v", err)
					return
				}
				if !reflect.DeepEqual(expected.Data, cm.Data) {
					t.Errorf("unexpected configmap data: %v", cm.Data)
				}
			},
		},
		{
			name:           "update resource",
			existing:       []runtime.Object{testConfigMap()},
			expectedResult: resource.UnchangedApplyResult,
			apply: func() runtime.Object {
				cm := testConfigMap()
				cm.Data["foo"] = "baz"
				return cm
			}(),
			validate: func(t *testing.T, info *resource.Info, ns string) {
				cm := &corev1.ConfigMap{}
				err := c.Get(context.TODO(), types.NamespacedName{Name: info.Name, Namespace: info.Namespace}, cm)
				if err != nil {
					t.Errorf("unexpected error retrieving configmap: %v", err)
					return
				}
				if cm.Data["foo"] == "baz" {
					t.Errorf("unexpected data: %v", cm.Data)
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
				var h resource.Helper
				if clientConfig == "kubeconfig" {
					h = resource.NewHelper(kubeconfig, logger)
				} else {
					h = resource.NewHelperFromRESTConfig(cfg, logger)
				}
				accessor := meta.NewAccessor()
				for _, obj := range test.existing {
					o := obj.DeepCopyObject()
					accessor.SetNamespace(o, namespace.Name)
					_, err := h.CreateRuntimeObject(o, scheme.Scheme)
					if err != nil {
						t.Fatalf("unexpected err: %v", err)
					}
				}
				accessor.SetNamespace(test.apply, namespace.Name)
				data, err := resource.Serialize(test.apply, scheme.Scheme)
				if err != nil {
					t.Errorf("unexpected error calling serialize: %v", err)
					return
				}

				t.Logf("The serialized resource:\n%s\n", string(data))
				info, err := h.Info(data)
				if err != nil {
					t.Errorf("unexpected error calling info: %v", err)
					return
				}
				applyResult, err := h.Create(data)
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
