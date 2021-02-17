package annotations

// AddAnnotation returns a map with the given key and value added to the given map.
func AddAnnotation(annotations map[string]string, annotationKey, annotationValue string) map[string]string {
	if annotationKey == "" {
		// Don't need to add an annotation.
		return annotations
	}
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[annotationKey] = annotationValue
	return annotations
}
