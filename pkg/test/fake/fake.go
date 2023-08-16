package fake

import (
	"reflect"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivecontractsv1alpha1 "github.com/openshift/hive/apis/hivecontracts/v1alpha1"
	hiveintv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	scheme "github.com/openshift/hive/pkg/util/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Wrapper around fake client which registers all necessary types
// as Status sub-resource and adds the hive scheme to the client.
func NewFakeClientBuilder() *fake.ClientBuilder {

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

	return fake.NewClientBuilder().WithStatusSubresource(subresource_list...).WithScheme(scheme)
}
