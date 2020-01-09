package hiveconversion

import (
	"math/rand"
	"reflect"
	"testing"

	fuzz "github.com/google/gofuzz"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"k8s.io/apimachinery/pkg/api/apitesting"
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	metafuzzer "k8s.io/apimachinery/pkg/apis/meta/fuzzer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/diff"

	hiveapi "github.com/openshift/hive/pkg/hive/apis/hive"
)

func init() {
	_, codecFactory := apitesting.SchemeForOrDie(hiveapi.Install)
	customFuzzer = fuzzer.FuzzerFor(
		fuzzer.MergeFuzzerFuncs(metafuzzer.Funcs),
		rand.NewSource(rand.Int63()),
		codecFactory,
	).Funcs(customFuzzerFuncs...).NilChance(0)

}

// TODO (staebler): Add conversion tests for other types

// make sure v1alpha1 <-> v1 round trip does not lose any data

func TestV1Alpha1ClusterImageSetFidelity(t *testing.T) {
	for i := 0; i < 100; i++ {
		v1a1cis := &hiveapi.ClusterImageSet{}
		v1a1cis2 := &hiveapi.ClusterImageSet{}
		v1cis := &hivev1.ClusterImageSet{}
		customFuzzer.Fuzz(v1a1cis)
		if err := Convert_v1alpha1_ClusterImageSet_To_v1_ClusterImageSet(v1a1cis, v1cis, nil); err != nil {
			t.Fatal(err)
		}
		if err := Convert_v1_ClusterImageSet_To_v1alpha1_ClusterImageSet(v1cis, v1a1cis2, nil); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(v1a1cis, v1a1cis2) {
			t.Errorf("v1alpah1 data not preserved; the diff is %s", diff.ObjectDiff(v1a1cis, v1a1cis2))
		}
	}
}

func TestV1ClusterImageSetFidelity(t *testing.T) {
	for i := 0; i < 100; i++ {
		v1cis := &hivev1.ClusterImageSet{}
		v1cis2 := &hivev1.ClusterImageSet{}
		v1a1cis := &hiveapi.ClusterImageSet{}
		customFuzzer.Fuzz(v1cis)
		if err := Convert_v1_ClusterImageSet_To_v1alpha1_ClusterImageSet(v1cis, v1a1cis, nil); err != nil {
			t.Fatal(err)
		}
		if err := Convert_v1alpha1_ClusterImageSet_To_v1_ClusterImageSet(v1a1cis, v1cis2, nil); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(v1cis, v1cis2) {
			t.Errorf("v1 data not preserved; the diff is %s", diff.ObjectDiff(v1cis, v1cis2))
		}
	}
}

func TestConversionErrors(t *testing.T) {
	for _, test := range []struct {
		name     string
		expected string
		f        func() error
	}{} {
		if err := test.f(); err == nil || test.expected != err.Error() {
			t.Errorf("%s failed: expected %q got %v", test.name, test.expected, err)
		}
	}
}

var customFuzzerFuncs = []interface{}{
	func(*metav1.TypeMeta, fuzz.Continue) {}, // Ignore TypeMeta
	func(*runtime.Object, fuzz.Continue) {},  // Ignore AttributeRestrictions since they are deprecated
	func(v1a1cis *hiveapi.ClusterImageSet, c fuzz.Continue) {
		c.FuzzNoCustom(v1a1cis)
		v1a1cis.Spec.InstallerImage = nil
	},
	func(v1cis *hivev1.ClusterImageSet, c fuzz.Continue) {
		c.FuzzNoCustom(v1cis)
	},
}
var customFuzzer *fuzz.Fuzzer
