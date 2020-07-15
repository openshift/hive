package resource

import (
	"context"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"

	"github.com/openshift/hive/pkg/resource"
)

func TestInfo(t *testing.T) {
	logger := log.WithFields(nil)
	namespace := &corev1.Namespace{}
	namespace.GenerateName = "info-test-"
	err := c.Create(context.TODO(), namespace)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	h := resource.NewHelper(kubeconfig, logger)
	i, err := h.Info([]byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: test-configmap
data:
  test-key: test-value`))
	require.NoError(t, err)
	require.NotNil(t, i.Object)
	assert.Equal(t, "test-configmap", i.Object.GetName())
}
