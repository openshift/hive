package clustersync

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	hiveintv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
)

type clientWrapper struct {
	client.Client
}

var _ client.Client = (*clientWrapper)(nil)

func (c *clientWrapper) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	switch t := obj.(type) {
	case *hiveintv1alpha1.ClusterSync:
		t.APIVersion = hiveintv1alpha1.SchemeGroupVersion.String()
		t.Kind = "ClusterSync"
		t.UID = testClusterSyncUID
	}
	return c.Client.Create(ctx, obj, opts...)
}
