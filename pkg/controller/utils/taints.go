package utils

import (
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	corev1 "k8s.io/api/core/v1"
)

func IdentifierForTaint(taint *corev1.Taint) hivev1.TaintIdentifier {
	// Let a nil `taint` panic
	return hivev1.TaintIdentifier{
		Key:    taint.Key,
		Effect: taint.Effect,
	}
}

// GetUniqueTaints collapses the duplicates from the list of taints provided before returning them.
// Key+Effect are unique identifiers, and the first Value of the taint entry encountered will be preserved.
func GetUniqueTaints(taints *[]corev1.Taint) *[]corev1.Taint {
	// uniqueTaints would be a map of key + effect to taint
	uniqueTaints := make(map[hivev1.TaintIdentifier]corev1.Taint)
	for _, taint := range *taints {
		// If value is already present (we have encountered a duplicate), skip.
		// This means that when we encounter duplicates, the value of the first entry encountered will be preserved.
		if !TaintExists(uniqueTaints, &taint) {
			uniqueTaints[IdentifierForTaint(&taint)] = taint
		}
	}
	return ListFromTaintMap(&uniqueTaints)
}

// ListFromTaintMap simply compiles a list of taints from the corresponding map
func ListFromTaintMap(taintMap *map[hivev1.TaintIdentifier]corev1.Taint) *[]corev1.Taint {
	returnTaints := make([]corev1.Taint, 0, len(*taintMap))
	for _, taint := range *taintMap {
		returnTaints = append(returnTaints, taint)
	}
	return &returnTaints
}

// TaintExists checks if the TaintIdentifier for given taint is present in the provided taintMap
func TaintExists(taintMap map[hivev1.TaintIdentifier]corev1.Taint, taint *corev1.Taint) bool {
	if _, ok := taintMap[IdentifierForTaint(taint)]; ok {
		return true
	}
	return false
}
