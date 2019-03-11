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
	"k8s.io/cli-runtime/pkg/genericclioptions/printers"
	"k8s.io/client-go/kubernetes/scheme"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/resource"
)

func TestApply(t *testing.T) {
	tests := []struct {
		name     string
		existing []runtime.Object
		apply    runtime.Object
		validate func(t *testing.T, info *resource.Info, ns string)
	}{
		{
			name:  "create resource",
			apply: testConfigMap(),
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
			name:     "update resource",
			existing: []runtime.Object{testConfigMap()},
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
			name:  "create crd instance",
			apply: testClusterImageSet(),
			validate: func(t *testing.T, info *resource.Info, ns string) {
				cis := &hivev1.ClusterImageSet{}
				err := c.Get(context.TODO(), types.NamespacedName{Name: info.Name}, cis)
				if err != nil {
					t.Errorf("unexpected error retrieving cluster image set: %v", err)
					return
				}
				if cis.Spec.InstallerImage == nil || *cis.Spec.InstallerImage != "installer-image:v1" {
					t.Errorf("unexpected installer image value: %v", cis.Spec.InstallerImage)
				}
			},
		},
	}

	printer := printers.NewTypeSetter(scheme.Scheme).ToPrinter(&printers.YAMLPrinter{})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logger := log.WithField("test", test.name)
			namespace := &corev1.Namespace{}
			namespace.GenerateName = "apply-test-"
			err := c.Create(context.TODO(), namespace)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			accessor := meta.NewAccessor()
			for _, obj := range test.existing {
				o := obj
				accessor.SetNamespace(o, namespace.Name)
				err := c.Create(context.TODO(), o)
				if err != nil {
					t.Fatalf("unexpected err: %v", err)
				}
			}
			accessor.SetNamespace(test.apply, namespace.Name)
			buf := &bytes.Buffer{}
			if err = printer.PrintObj(test.apply, buf); err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			t.Logf("The serialized resource:\n%s\n", buf.String())
			h := resource.NewHelper(kubeconfig, logger)
			info, err := h.Info(buf.Bytes())
			if err != nil {
				t.Errorf("unexpected error calling info: %v", err)
				return
			}
			err = h.Apply(buf.Bytes())
			if err != nil {
				t.Errorf("unexpected error calling apply: %v", err)
				return
			}
			test.validate(t, info, namespace.Name)
		})
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

func testClusterImageSet() *hivev1.ClusterImageSet {
	cis := &hivev1.ClusterImageSet{}
	cis.Name = "test-cluster-image-set"
	cis.Spec.InstallerImage = strptr("installer-image:v1")
	return cis
}
