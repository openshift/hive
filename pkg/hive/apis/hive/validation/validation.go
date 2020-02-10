package validation

import (
	"k8s.io/apimachinery/pkg/api/validation/path"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/kubernetes/pkg/apis/core/validation"

	hiveapi "github.com/openshift/hive/pkg/hive/apis/hive"
)

// TODO (staebler): Do we need validation? The v1 validating webhooks should handle the validation

// ValidateClusterImageSet validates that a create of a ClusterImageSet is valid.
func ValidateClusterImageSet(clusterImageSet *hiveapi.ClusterImageSet) field.ErrorList {
	allErrs := validation.ValidateObjectMeta(&clusterImageSet.ObjectMeta, false, path.ValidatePathSegmentName, field.NewPath("metadata"))
	return allErrs
}

// ValidateClusterImageSetUpdate validates that an update of a ClusterImageSet is valid.
func ValidateClusterImageSetUpdate(clusterImageSet *hiveapi.ClusterImageSet, oldClusterImageSet *hiveapi.ClusterImageSet) field.ErrorList {
	allErrs := ValidateClusterImageSet(clusterImageSet)
	allErrs = append(allErrs, validation.ValidateObjectMetaUpdate(&clusterImageSet.ObjectMeta, &oldClusterImageSet.ObjectMeta, field.NewPath("metadata"))...)
	return allErrs
}
