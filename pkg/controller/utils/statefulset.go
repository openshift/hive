package utils

import (
	"crypto/md5"
	"encoding/hex"

	"github.com/openshift/hive/pkg/util/scheme"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

var (
	appsScheme = scheme.GetScheme()
	appsCodecs = serializer.NewCodecFactory(appsScheme)
)

// ReadStatefulsetOrDie converts a statefulset asset into an actual instance of a statefulset.
func ReadStatefulsetOrDie(objBytes []byte) *appsv1.StatefulSet {
	requiredObj, err := runtime.Decode(appsCodecs.UniversalDecoder(appsv1.SchemeGroupVersion), objBytes)
	if err != nil {
		panic(err)
	}
	return requiredObj.(*appsv1.StatefulSet)
}

// CalculateStatefulSetSpecHash returns a hash of the statefulset.Spec.
func CalculateStatefulSetSpecHash(statefulset *appsv1.StatefulSet) (string, error) {

	hasher := md5.New()
	jobSpecBytes, err := statefulset.Spec.Marshal()
	if err != nil {
		return "", err
	}

	_, err = hasher.Write(jobSpecBytes)
	if err != nil {
		return "", err
	}

	sum := hex.EncodeToString(hasher.Sum(nil))

	return sum, nil
}
