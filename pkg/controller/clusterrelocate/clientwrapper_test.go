package clusterrelocate

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type deleteBlockingClientWrapper struct {
	client.Client
}

var _ client.Client = (*deleteBlockingClientWrapper)(nil)

func (c *deleteBlockingClientWrapper) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	a, err := meta.Accessor(obj)
	if err != nil {
		return err
	}
	now := metav1.Now()
	a.SetDeletionTimestamp(&now)
	return c.Update(ctx, obj)
}
