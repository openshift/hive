package apis

import (
	hiveintv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
)

func init() {
	// Register the types with the Scheme so the components can map objects to GroupVersionKinds and back
	AddToSchemes = append(AddToSchemes, hiveintv1alpha1.SchemeBuilder.AddToScheme)
}
