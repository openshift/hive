package resource

import (
	"context"
	"testing"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/openshift/hive/pkg/resource"
)

func TestPatch(t *testing.T) {
	tests := []struct {
		name      string
		patch     string
		patchType string
		validate  func(t *testing.T, cm *corev1.ConfigMap)
	}{
		// All tests begin with a single configmap with one key { "foo": "bar" }
		{
			name:      "json patch",
			patch:     `[ { "op": "replace", "path": "/data/foo", "value": "baz" } ]`,
			patchType: "json",
			validate: func(t *testing.T, cm *corev1.ConfigMap) {
				if cm.Data["foo"] != "baz" {
					t.Errorf("unexpected value in data: %v", cm.Data)
				}
			},
		},
		{
			name:      "merge patch",
			patch:     `{ "data": { "foo": null, "baz": "bar" } }`,
			patchType: "merge",
			validate: func(t *testing.T, cm *corev1.ConfigMap) {
				if len(cm.Data) != 1 {
					t.Errorf("unexpected length of data: %v", cm.Data)
				}
				if cm.Data["baz"] != "bar" {
					t.Errorf("unexpected value of bar: %v", cm.Data)
				}
			},
		},
		{
			name:      "strategic patch",
			patch:     `{ "data": { "test": "baz" } }`,
			patchType: "strategic",
			validate: func(t *testing.T, cm *corev1.ConfigMap) {
				if len(cm.Data) != 2 {
					t.Errorf("unexpected length of data: %v", cm.Data)
				}
				if cm.Data["test"] != "baz" || cm.Data["foo"] != "bar" {
					t.Errorf("unexpected values in data: %v", cm.Data)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logger := log.WithField("test", test.name)
			namespace := &corev1.Namespace{}
			namespace.GenerateName = "apply-test-"
			err := c.Create(context.TODO(), namespace)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			cm := testConfigMap()
			cm.Namespace = namespace.Name
			err = c.Create(context.TODO(), cm)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			h, err := resource.NewHelper(kubeconfig, logger)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			err = h.Patch(types.NamespacedName{Namespace: namespace.Name, Name: cm.Name}, "ConfigMap", "v1", []byte(test.patch), test.patchType)
			if err != nil {
				t.Errorf("unexpected error calling patch: %v", err)
				return
			}
			resultingCM := &corev1.ConfigMap{}
			err = c.Get(context.TODO(), types.NamespacedName{Namespace: namespace.Name, Name: cm.Name}, resultingCM)
			if err != nil {
				t.Errorf("unexpected error retrieving config map: %v", err)
			}
			test.validate(t, resultingCM)
		})
	}
}
