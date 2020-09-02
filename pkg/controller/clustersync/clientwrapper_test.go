package clustersync

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hiveintv1alpha1 "github.com/openshift/hive/pkg/apis/hiveinternal/v1alpha1"
)

type clientWrapper struct {
	client.Client
}

var _ client.Client = (*clientWrapper)(nil)

func (c *clientWrapper) Create(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
	switch t := obj.(type) {
	case *hiveintv1alpha1.ClusterSync:
		t.APIVersion = hiveintv1alpha1.SchemeGroupVersion.String()
		t.Kind = "ClusterSync"
		t.UID = testClusterSyncUID
	}
	return c.Client.Create(ctx, obj, opts...)
}
