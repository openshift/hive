package api_test

import (
	_ "k8s.io/kubernetes/pkg/apis/core/install"
	// install all APIs
	_ "github.com/openshift/hive/pkg/api/install"
)

// TODO (staebler): Determine if these round-trip serialization tests offer value for Hive's use-case.
/*
func v1alpha1Fuzzer(t *testing.T, seed int64) *fuzz.Fuzzer {
	f := fuzzer.FuzzerFor(kapitesting.FuzzerFuncs, rand.NewSource(seed), legacyscheme.Codecs)
	f.Funcs(
		func(j *hiveapi.ClusterImageSet, c fuzz.Continue) {
			c.FuzzNoCustom(j)
		},

		func(j *runtime.Object, c fuzz.Continue) {
			// runtime.EmbeddedObject causes a panic inside of fuzz because runtime.Object isn't handled.
		},
		func(t *time.Time, c fuzz.Continue) {
			// This is necessary because the standard fuzzed time.Time object is
			// completely nil, but when JSON unmarshals dates it fills in the
			// unexported loc field with the time.UTC object, resulting in
			// reflect.DeepEqual returning false in the round trip tests. We solve it
			// by using a date that will be identical to the one JSON unmarshals.
			*t = time.Date(2000, 1, 1, 1, 1, 1, 0, time.UTC)
		},
		func(u64 *uint64, c fuzz.Continue) {
			// TODO: uint64's are NOT handled right.
			*u64 = c.RandUint64() >> 8
		},
	)
	return f
}

const fuzzIters = 20

// For debugging problems
func TestSpecificKind(t *testing.T) {
	seed := int64(2703387474910584091)
	fuzzer := v1alpha1Fuzzer(t, seed)

	legacyscheme.Scheme.Log(t)
	defer legacyscheme.Scheme.Log(nil)

	gvk := hiveapi.SchemeGroupVersion.WithKind("ClusterImageSet")
	// TODO: make upstream CodecFactory customizable
	codecs := serializer.NewCodecFactory(legacyscheme.Scheme)
	for i := 0; i < fuzzIters; i++ {
		roundtrip.RoundTripSpecificKindWithoutProtobuf(t, gvk, legacyscheme.Scheme, codecs, fuzzer, nil)
	}
}

// TestRoundTripTypes applies the round-trip test to all round-trippable Kinds
// in all of the API groups registered for test in the testapi package.
func TestRoundTripTypes(t *testing.T) {
	seed := rand.Int63()
	fuzzer := v1alpha1Fuzzer(t, seed)

	roundtrip.RoundTripTypes(t, legacyscheme.Scheme, legacyscheme.Codecs, fuzzer, nil)
}
*/
