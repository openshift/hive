package validation

import (
	hiveapi "github.com/openshift/hive/v1alpha1apiserver/pkg/hive/apis/hive"
	hivevalidation "github.com/openshift/hive/v1alpha1apiserver/pkg/hive/apis/hive/validation"
	// required to be loaded before we register
	_ "github.com/openshift/hive/v1alpha1apiserver/pkg/api/install"
)

func init() {
	registerAll()
}

func registerAll() {
	Validator.MustRegister(&hiveapi.ClusterImageSet{}, false, hivevalidation.ValidateClusterImageSet, hivevalidation.ValidateClusterImageSetUpdate)
}
