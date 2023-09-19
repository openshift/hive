package utils

import (
	"context"
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/stretchr/testify/require"
)

func TestRateLimitedEventHandler(t *testing.T) {
	o := &hivev1.DNSZone{ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns", Name: "test-name"}}

	// always not rate limited
	q := &trackedQueue{RateLimitingInterface: workqueue.NewRateLimitingQueue(workqueue.DefaultItemBasedRateLimiter())}
	h := NewRateLimitedUpdateEventHandler(&handler.EnqueueRequestForObject{}, func(_ event.UpdateEvent) bool { return false })
	h.Update(context.TODO(), event.UpdateEvent{ObjectOld: o, ObjectNew: o}, q)

	require.Equal(t, 1, len(q.added))
	require.Equal(t, 0, len(q.ratelimitAdded))

	// always rate limited
	q = &trackedQueue{RateLimitingInterface: workqueue.NewRateLimitingQueue(workqueue.DefaultItemBasedRateLimiter())}
	h = NewRateLimitedUpdateEventHandler(&handler.EnqueueRequestForObject{}, func(_ event.UpdateEvent) bool { return true })
	h.Update(context.TODO(), event.UpdateEvent{ObjectOld: o, ObjectNew: o}, q)

	require.Equal(t, 0, len(q.added))
	require.Equal(t, 1, len(q.ratelimitAdded))

	// always rate limited not UPDATE
	q = &trackedQueue{RateLimitingInterface: workqueue.NewRateLimitingQueue(workqueue.DefaultItemBasedRateLimiter())}
	h = NewRateLimitedUpdateEventHandler(&handler.EnqueueRequestForObject{}, func(_ event.UpdateEvent) bool { return true })
	h.Generic(context.TODO(), event.GenericEvent{Object: o}, q)

	require.Equal(t, 1, len(q.added))
	require.Equal(t, 0, len(q.ratelimitAdded))

	// always rate limited with complex handler
	q = &trackedQueue{RateLimitingInterface: workqueue.NewRateLimitingQueue(workqueue.DefaultItemBasedRateLimiter())}
	h = NewRateLimitedUpdateEventHandler(handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, _ client.Object) []reconcile.Request {
		return []reconcile.Request{{
			NamespacedName: types.NamespacedName{
				Namespace: "test-ns",
				Name:      "test-name-1",
			},
		}, {
			NamespacedName: types.NamespacedName{
				Namespace: "test-ns",
				Name:      "test-name-2",
			},
		}, {
			NamespacedName: types.NamespacedName{
				Namespace: "test-ns",
				Name:      "test-name-3",
			},
		}}
	}), func(_ event.UpdateEvent) bool { return true })
	h.Update(context.TODO(), event.UpdateEvent{ObjectOld: o, ObjectNew: o}, q)

	require.Equal(t, 0, len(q.added))
	require.Equal(t, 3, len(q.ratelimitAdded))

}

type trackedQueue struct {
	workqueue.RateLimitingInterface

	added          []string
	ratelimitAdded []string
}

var _ workqueue.RateLimitingInterface = &trackedQueue{}

// Add implements workqueue.Interface
func (q *trackedQueue) Add(item interface{}) {
	q.added = append(q.added, fmt.Sprintf("%s", item))
	q.RateLimitingInterface.Add(item)
}

// AddRateLimited implements workqueue.RateLimitingInterface
func (q *trackedQueue) AddRateLimited(item interface{}) {
	q.ratelimitAdded = append(q.ratelimitAdded, fmt.Sprintf("%s", item))
	q.RateLimitingInterface.AddRateLimited(item)
}
