package apis

import (
	hivecontractsv1alpha1 "github.com/openshift/hive/apis/hivecontracts/v1alpha1"
)

func init() {
	// Register the types with the Scheme so the components can map objects to GroupVersionKinds and back
	AddToSchemes = append(AddToSchemes, hivecontractsv1alpha1.SchemeBuilder.AddToScheme)
}
