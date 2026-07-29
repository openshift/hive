package fake

import (
	"context"
	"reflect"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivecontractsv1alpha1 "github.com/openshift/hive/apis/hivecontracts/v1alpha1"
	hiveintv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	scheme "github.com/openshift/hive/pkg/util/scheme"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// clientWithTypeMeta wraps a fake client to preserve TypeMeta (Kind/APIVersion) on objects.
//
// BACKGROUND: In production, the real Kubernetes API server returns objects over the wire with
// TypeMeta populated (Kind and APIVersion fields). The real client-go preserves these fields
// when unmarshaling objects, even though etcd doesn't store them. Production code throughout
// this codebase relies on this behavior - for example, creating OwnerReferences via
// metav1.NewControllerRef(obj, obj.GroupVersionKind()) requires obj.GroupVersionKind() to
// return the correct GVK, which it gets from TypeMeta.
//
// PROBLEM: Starting in controller-runtime v0.24, the fake client intentionally clears TypeMeta
// on structured objects to match etcd storage behavior rather than wire protocol behavior
// (see https://github.com/kubernetes-sigs/controller-runtime/issues/1735). This causes tests
// to fail when production code calls obj.GroupVersionKind() on objects retrieved from the
// fake client, because it returns an empty GVK.
//
// SOLUTION: This wrapper intercepts fake client operations and re-populates TypeMeta using
// apiutil.GVKForObject(), which looks up the GVK from the scheme based on the object's Go type.
// This makes the fake client accurately simulate what production code sees from the real
// Kubernetes client, without requiring changes to production code that works correctly in
// real clusters.
type clientWithTypeMeta struct {
	client.Client
	scheme *runtime.Scheme
}

func (c *clientWithTypeMeta) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	err := c.Client.Get(ctx, key, obj, opts...)
	if err == nil {
		if gvk, err := apiutil.GVKForObject(obj, c.scheme); err == nil {
			obj.GetObjectKind().SetGroupVersionKind(gvk)
		}
	}
	return err
}

func (c *clientWithTypeMeta) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	err := c.Client.List(ctx, list, opts...)
	if err == nil {
		items := reflect.ValueOf(list).Elem().FieldByName("Items")
		if items.IsValid() {
			for i := 0; i < items.Len(); i++ {
				if obj, ok := items.Index(i).Addr().Interface().(client.Object); ok {
					if gvk, err := apiutil.GVKForObject(obj, c.scheme); err == nil {
						obj.GetObjectKind().SetGroupVersionKind(gvk)
					}
				}
			}
		}
	}
	return err
}

func (c *clientWithTypeMeta) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	err := c.Client.Create(ctx, obj, opts...)
	if err == nil {
		if gvk, err := apiutil.GVKForObject(obj, c.scheme); err == nil {
			obj.GetObjectKind().SetGroupVersionKind(gvk)
		}
	}
	return err
}

func (c *clientWithTypeMeta) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	err := c.Client.Update(ctx, obj, opts...)
	if err == nil {
		if gvk, err := apiutil.GVKForObject(obj, c.scheme); err == nil {
			obj.GetObjectKind().SetGroupVersionKind(gvk)
		}
	}
	return err
}

func (c *clientWithTypeMeta) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	err := c.Client.Patch(ctx, obj, patch, opts...)
	if err == nil {
		if gvk, err := apiutil.GVKForObject(obj, c.scheme); err == nil {
			obj.GetObjectKind().SetGroupVersionKind(gvk)
		}
	}
	return err
}

func (c *clientWithTypeMeta) Status() client.StatusWriter {
	return &statusWriterWithTypeMeta{
		StatusWriter: c.Client.Status(),
		scheme:       c.scheme,
	}
}

type statusWriterWithTypeMeta struct {
	client.StatusWriter
	scheme *runtime.Scheme
}

func (s *statusWriterWithTypeMeta) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	err := s.StatusWriter.Update(ctx, obj, opts...)
	if err == nil {
		if gvk, err := apiutil.GVKForObject(obj, s.scheme); err == nil {
			obj.GetObjectKind().SetGroupVersionKind(gvk)
		}
	}
	return err
}

func (s *statusWriterWithTypeMeta) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	err := s.StatusWriter.Patch(ctx, obj, patch, opts...)
	if err == nil {
		if gvk, err := apiutil.GVKForObject(obj, s.scheme); err == nil {
			obj.GetObjectKind().SetGroupVersionKind(gvk)
		}
	}
	return err
}

// FakeClientBuilder wraps controller-runtime's fake.ClientBuilder to return
// clients that preserve TypeMeta, matching real Kubernetes client behavior.
type FakeClientBuilder struct {
	*fake.ClientBuilder
	scheme *runtime.Scheme
}

// Wrapper around fake client which registers all necessary types
// as Status sub-resource and adds the hive scheme to the client.
func NewFakeClientBuilder() *FakeClientBuilder {
	scheme := scheme.GetScheme()
	types_list := scheme.KnownTypes(hivev1.SchemeGroupVersion)
	types_list2 := scheme.KnownTypes(hivecontractsv1alpha1.SchemeGroupVersion)
	types_list3 := scheme.KnownTypes(hiveintv1alpha1.SchemeGroupVersion)
	combined := make(map[string]reflect.Type)

	for key, value := range types_list {
		combined[key] = value
	}
	for key, value := range types_list2 {
		combined[key] = value
	}
	for key, value := range types_list3 {
		combined[key] = value
	}

	subresource_list := []client.Object{}

	for _, typ := range combined {
		t := reflect.New(typ).Interface()
		cast, ok := t.(client.Object)
		if ok {
			subresource_list = append(subresource_list, cast)
		}
	}

	return &FakeClientBuilder{
		ClientBuilder: fake.NewClientBuilder().WithStatusSubresource(subresource_list...).WithScheme(scheme),
		scheme:        scheme,
	}
}

func (b *FakeClientBuilder) WithRuntimeObjects(objs ...runtime.Object) *FakeClientBuilder {
	// Ensure TypeMeta is set on all objects before adding them to the fake client
	for _, obj := range objs {
		if clientObj, ok := obj.(client.Object); ok {
			if gvk, err := apiutil.GVKForObject(clientObj, b.scheme); err == nil {
				clientObj.GetObjectKind().SetGroupVersionKind(gvk)
			}
		}
	}
	b.ClientBuilder = b.ClientBuilder.WithRuntimeObjects(objs...)
	return b
}

func (b *FakeClientBuilder) WithObjects(objs ...client.Object) *FakeClientBuilder {
	b.ClientBuilder = b.ClientBuilder.WithObjects(objs...)
	return b
}

func (b *FakeClientBuilder) WithLists(lists ...client.ObjectList) *FakeClientBuilder {
	b.ClientBuilder = b.ClientBuilder.WithLists(lists...)
	return b
}

func (b *FakeClientBuilder) WithScheme(scheme *runtime.Scheme) *FakeClientBuilder {
	b.ClientBuilder = b.ClientBuilder.WithScheme(scheme)
	b.scheme = scheme
	return b
}

func (b *FakeClientBuilder) WithStatusSubresource(objs ...client.Object) *FakeClientBuilder {
	b.ClientBuilder = b.ClientBuilder.WithStatusSubresource(objs...)
	return b
}

func (b *FakeClientBuilder) WithIndex(obj client.Object, field string, extractValue client.IndexerFunc) *FakeClientBuilder {
	b.ClientBuilder = b.ClientBuilder.WithIndex(obj, field, extractValue)
	return b
}

func (b *FakeClientBuilder) Build() client.Client {
	baseClient := b.ClientBuilder.Build()
	return &clientWithTypeMeta{
		Client: baseClient,
		scheme: b.scheme,
	}
}
