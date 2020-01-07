package validation

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	hiveapi "github.com/openshift/hive/pkg/hive/apis/hive"
)

func TestValidateClusterImageSet(t *testing.T) {
	errs := ValidateClusterImageSet(
		&hiveapi.ClusterImageSet{
			ObjectMeta: metav1.ObjectMeta{Name: "test-image-set"},
		},
	)
	if len(errs) != 0 {
		t.Errorf("expected success: %v", errs)
	}

	errorCases := map[string]struct {
		A hiveapi.ClusterImageSet
		T field.ErrorType
		F string
	}{
		"namespace provided": {
			A: hiveapi.ClusterImageSet{
				ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "test-image-set"},
			},
			T: field.ErrorTypeForbidden,
			F: "metadata.namespace",
		},
		"zero-length name": {
			A: hiveapi.ClusterImageSet{
				ObjectMeta: metav1.ObjectMeta{},
			},
			T: field.ErrorTypeRequired,
			F: "metadata.name",
		},
	}
	for k, v := range errorCases {
		errs := ValidateClusterImageSet(&v.A)
		if len(errs) == 0 {
			t.Errorf("expected failure %s for %v", k, v.A)
			continue
		}
		for i := range errs {
			if errs[i].Type != v.T {
				t.Errorf("%s: expected errors to have type %s: %v", k, v.T, errs[i])
			}
			if errs[i].Field != v.F {
				t.Errorf("%s: expected errors to have field %s: %v", k, v.F, errs[i])
			}
		}
	}
}

func TestValidateClusterImageSetUpdate(t *testing.T) {
	old := &hiveapi.ClusterImageSet{
		ObjectMeta: metav1.ObjectMeta{Name: "test-image-set", ResourceVersion: "1"},
	}

	errs := ValidateClusterImageSetUpdate(
		&hiveapi.ClusterImageSet{
			ObjectMeta: metav1.ObjectMeta{Name: "test-image-set", ResourceVersion: "1"},
		},
		old,
	)
	if len(errs) != 0 {
		t.Errorf("expected success: %v", errs)
	}

	errorCases := map[string]struct {
		A hiveapi.ClusterImageSet
		T field.ErrorType
		F string
	}{
		"changed name": {
			A: hiveapi.ClusterImageSet{
				ObjectMeta: metav1.ObjectMeta{Name: "other-image-set", ResourceVersion: "1"},
			},
			T: field.ErrorTypeInvalid,
			F: "metadata.name",
		},
	}
	for k, v := range errorCases {
		errs := ValidateClusterImageSetUpdate(&v.A, old)
		if len(errs) == 0 {
			t.Errorf("expected failure %s for %v", k, v.A)
			continue
		}
		for i := range errs {
			if errs[i].Type != v.T {
				t.Errorf("%s: expected errors to have type %s: %v", k, v.T, errs[i])
			}
			if errs[i].Field != v.F {
				t.Errorf("%s: expected errors to have field %s: %v", k, v.F, errs[i])
			}
		}
	}
}
