package utils

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/hive/pkg/constants"
	log "github.com/sirupsen/logrus"
)

func ExtractLogFields(obj metav1.Object) (map[string]interface{}, error) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return nil, nil
	}

	addl_log_fields, exists := annotations[constants.AdditionalLogFieldsAnnotation]
	if !exists {
		return nil, nil
	}

	kvmap := map[string]interface{}{}
	if err := json.Unmarshal([]byte(addl_log_fields), &kvmap); err != nil {
		return nil, err
	}
	// If the annotation is being used, we assume it is for log aggregation. Add our component name.
	kvmap["component"] = "hive"
	return kvmap, nil
}

func AddLogFields(obj metav1.Object, logger *log.Entry) *log.Entry {
	switch kvmap, err := ExtractLogFields(obj); {
	case err != nil:
		logger.WithError(err).Warning("failed to extract additional log fields -- ignoring")
	case kvmap != nil:
		logger = logger.WithFields(log.Fields(kvmap))
	}
	return logger
}

func CopyLogAnnotation(from, to metav1.Object) bool {
	froma := from.GetAnnotations()
	if froma == nil {
		// Spoof empty so we can delete the annotation if it exists on `to`
		froma = make(map[string]string)
	}

	toa := to.GetAnnotations()
	if toa == nil {
		toa = make(map[string]string)
	}

	changed := false
	key := constants.AdditionalLogFieldsAnnotation
	fromv, fromexists := froma[key]
	tov, toexists := toa[key]
	if fromexists && fromv != tov {
		changed = true
		toa[key] = fromv
	} else if !fromexists && toexists {
		changed = true
		delete(toa, key)
	}

	if changed {
		to.SetAnnotations(toa)
	}

	return changed
}
