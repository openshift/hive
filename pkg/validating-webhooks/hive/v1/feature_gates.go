package v1

import (
	"fmt"
	"os"
	"strings"

	"github.com/stretchr/testify/assert"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
)

type featureSet struct {
	*hivev1.FeatureGatesEnabled
}

func (fs *featureSet) IsEnabled(featureGate string) bool {
	s := sets.NewString(fs.FeatureGatesEnabled.Enabled...)
	return s.Has(featureGate)
}

func newFeatureSet() *featureSet {
	return &featureSet{
		FeatureGatesEnabled: &hivev1.FeatureGatesEnabled{
			Enabled: strings.Split(os.Getenv(constants.HiveFeatureGatesEnabledEnvVar), ","),
		},
	}
}

// existsOnlyWhenFeatureGate ensures that the fieldPath specified in the obj is only set when the featureGate is enabled.
// NOTE: the path to the field cannot include array / slice.
func existsOnlyWhenFeatureGate(fs *featureSet, obj *unstructured.Unstructured, fieldPath string, featureGate string) field.ErrorList {
	allErrs := field.ErrorList{}

	p := strings.Split(fieldPath, ".")
	_, found, err := unstructured.NestedFieldNoCopy(obj.Object, p...)
	if err == nil && found && !fs.IsEnabled(featureGate) {
		return append(allErrs, field.Forbidden(field.NewPath(fieldPath), fmt.Sprintf("should only be set when feature gate %s is enabled", featureGate)))
	}
	return allErrs
}

// equalOnlyWhenFeatureGate ensures that the fieldPath specified in the obj is equal to the expected value when
// the featureGate is enabled.
// NOTE: the path to the field cannot include array / slice.
func equalOnlyWhenFeatureGate(fs *featureSet, obj *unstructured.Unstructured, fieldPath string, featureGate string, expected interface{}) field.ErrorList {
	allErrs := field.ErrorList{}

	p := strings.Split(fieldPath, ".")
	v, found, err := unstructured.NestedFieldNoCopy(obj.Object, p...)
	if err == nil && found && assert.ObjectsAreEqualValues(expected, v) && !fs.IsEnabled(featureGate) {
		return append(allErrs, field.NotSupported(field.NewPath(fieldPath), v, []string{}))
	}
	return allErrs
}
