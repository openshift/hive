package validation

import (
	hivevalidation "github.com/openshift/hive/pkg/hive/apis/hive/validation"
	// required to be loaded before we register
	_ "github.com/openshift/hive/pkg/api/install"
	hiveapi "github.com/openshift/hive/pkg/hive/apis/hive"
)

func init() {
	registerAll()
}

func registerAll() {
	Validator.MustRegister(&hiveapi.ClusterImageSet{}, false, hivevalidation.ValidateClusterImageSet, hivevalidation.ValidateClusterImageSetUpdate)
}
