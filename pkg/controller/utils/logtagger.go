package utils

import (
	"encoding/json"

	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/hive/pkg/constants"
	log "github.com/sirupsen/logrus"
)

type AdditionalLogFieldHavinThing interface {
	GetAdditionalLogFieldsJSON() *string
}

type MetaObjectLogTagger struct {
	metav1.Object
}

func (obj MetaObjectLogTagger) GetAdditionalLogFieldsJSON() *string {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return nil
	}

	addl_log_fields, exists := annotations[constants.AdditionalLogFieldsAnnotation]
	if !exists {
		return nil
	}

	return &addl_log_fields
}

var _ AdditionalLogFieldHavinThing = MetaObjectLogTagger{}

type StringLogTagger struct {
	S string
}

func (s StringLogTagger) GetAdditionalLogFieldsJSON() *string {
	if s.S == "" {
		return nil
	}
	return &s.S
}

var _ AdditionalLogFieldHavinThing = StringLogTagger{}

func parseLogFields(jsonMap string) (map[string]any, error) {
	kvmap := map[string]any{}
	if err := json.Unmarshal([]byte(jsonMap), &kvmap); err != nil {
		return nil, err
	}
	// If additional log fields are being used, we assume it is for log aggregation. Add our component name.
	kvmap["component"] = "hive"
	return kvmap, nil

}

// ExtractLogFields knows where to look in an AdditionalLogFieldHavinThing for additional log
// fields. It attempts to extract them and parse them as a JSON string representing a map. If
// no such fields are found, both returns are nil -- this is not considered an error. If
// parsing succeeds, the first return is the unmarshaled map and the second return is nil. If
// parsing fails, the map is nil and the error is bubbled up.
func ExtractLogFields[O AdditionalLogFieldHavinThing](obj O) (map[string]any, error) {
	addl_log_fields := obj.GetAdditionalLogFieldsJSON()
	if addl_log_fields == nil {
		return nil, nil
	}

	return parseLogFields(*addl_log_fields)
}

func AddLogFields[O AdditionalLogFieldHavinThing](obj O, logger *log.Entry) *log.Entry {
	switch kvmap, err := ExtractLogFields(obj); {
	case err != nil:
		logger.WithError(err).Warning("failed to extract additional log fields -- ignoring")
	case kvmap == nil:
		logger.Debug("no additional log fields found")
	default:
		logger = logger.WithFields(log.Fields(kvmap))
	}
	return logger
}

func AddLogFieldsEnvVar(from metav1.Object, to *batchv1.Job) {
	addl_log_fields := MetaObjectLogTagger{Object: from}.GetAdditionalLogFieldsJSON()
	if addl_log_fields == nil {
		return
	}
	for i, c := range to.Spec.Template.Spec.Containers {
	container:
		// Replace if it already exists
		for j, e := range c.Env {
			if e.Name == constants.AdditionalLogFieldsEnvVar {
				c.Env[j].Value = *addl_log_fields
				continue container
			}
		}
		// Doesn't already exist; add it
		c.Env = append(c.Env, v1.EnvVar{Name: constants.AdditionalLogFieldsEnvVar, Value: *addl_log_fields})
		// Copy the container back to the Job
		to.Spec.Template.Spec.Containers[i] = c
	}
}

func CopyAnnotations(from, to metav1.Object, keys ...string) bool {
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

	for _, key := range keys {
		fromv, fromexists := froma[key]
		tov, toexists := toa[key]
		if fromexists && fromv != tov {
			changed = true
			toa[key] = fromv
		} else if !fromexists && toexists {
			changed = true
			delete(toa, key)
		}
	}

	if changed {
		to.SetAnnotations(toa)
	}

	return changed
}

func CopyLogAnnotation(from, to metav1.Object) bool {
	return CopyAnnotations(from, to, constants.AdditionalLogFieldsAnnotation)
}
