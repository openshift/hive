package utils

import (
	"context"
	"encoding/json"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetDuckType uses the client to fetch a type defined by gck and key, and
// marshals it into the duck type defined in obj.
func GetDuckType(ctx context.Context, client client.Client,
	gvk schema.GroupVersionKind, key types.NamespacedName,
	obj client.Object) error {
	uObj := &unstructured.Unstructured{}
	uObj.SetGroupVersionKind(gvk)
	if err := client.Get(ctx, key, uObj); err != nil {
		return err
	}

	// since client can be backed by cache,
	// there is no way to really use the client-go .Into
	// function to marshal only once form api response
	// directly into the ducked type.
	raw, err := uObj.MarshalJSON()
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, &obj)
}
