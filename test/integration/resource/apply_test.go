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

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/resource"
)

func TestApply(t *testing.T) {
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
				if info.Name != testConfigMap().Name {
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
				if !reflect.DeepEqual(testConfigMap().Data, cm.Data) {
					t.Errorf("unexpected configmap data: %v", cm.Data)
				}
			},
		},
		{
			name:           "update resource",
			existing:       []runtime.Object{testConfigMap()},
			expectedResult: resource.ConfiguredApplyResult,
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
				if cm.Data["foo"] != "baz" {
					t.Errorf("unexpected data: %v", cm.Data)
				}
			},
		},
		{
			name:           "unchanged resource",
			existing:       []runtime.Object{testConfigMap()},
			expectedResult: resource.UnchangedApplyResult,
			apply:          testConfigMap(),
			validate: func(t *testing.T, info *resource.Info, ns string) {
				cm := &corev1.ConfigMap{}
				err := c.Get(context.TODO(), types.NamespacedName{Name: info.Name, Namespace: info.Namespace}, cm)
				if err != nil {
					t.Errorf("unexpected error retrieving configmap: %v", err)
					return
				}
			},
		},
		{
			name:           "create crd instance",
			apply:          testDNSZone(),
			expectedResult: resource.CreatedApplyResult,
			validate: func(t *testing.T, info *resource.Info, ns string) {
				zone := &hivev1.DNSZone{}
				err := c.Get(context.TODO(), types.NamespacedName{Name: info.Name, Namespace: info.Namespace}, zone)
				if err != nil {
					t.Errorf("unexpected error retrieving dns zone: %v", err)
					return
				}
				if zone.Spec.Zone != "foo.example.com" {
					t.Errorf("unexpected zone: %s", zone.Spec.Zone)
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
					h, err = resource.NewHelper(kubeconfig, logger)
				} else {
					h, err = resource.NewHelperFromRESTConfig(cfg, logger)
				}
				if err != nil {
					t.Fatalf("unexpected err: %v", err)
				}
				accessor := meta.NewAccessor()
				for _, obj := range test.existing {
					o := obj.DeepCopyObject()
					accessor.SetNamespace(o, namespace.Name)
					_, err := h.ApplyRuntimeObject(o, scheme.Scheme)
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
				applyResult, err := h.Apply(data)
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

func testConfigMap() *corev1.ConfigMap {
	cm := &corev1.ConfigMap{}
	cm.Name = "test-config-map"
	cm.Data = map[string]string{"foo": "bar"}
	return cm
}

func strptr(s string) *string {
	return &s
}

func testDNSZone() *hivev1.DNSZone {
	z := &hivev1.DNSZone{}
	z.Name = "test-dns-zone"
	z.Spec.Zone = "foo.example.com"
	return z
}
