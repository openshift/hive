package clustersync

import (
	"bytes"
	"reflect"
	"text/template"

	"github.com/pkg/errors"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

// processParameters modifies `u`, appling text/template parameters found in string values therein.
func processParameters(u *unstructured.Unstructured, cd *hivev1.ClusterDeployment) error {
	resourceParamTemplate := template.New("resourceParams").Funcs(
		template.FuncMap{
			"fromCDLabel": fromCDLabel(cd),
		},
	)
	for k, v := range u.Object {
		newVal, err := applyTemplate(resourceParamTemplate, v)
		if err != nil {
			return errors.Wrapf(err, "Failed to apply template to value %#v", v)
		}
		u.Object[k] = newVal
	}
	return nil
}

// fromCDLabel produces a text/template-suitable func accepting a single parameter which will be
// interpreted as a key for a label on `cd`. If the label exists, its value is returned by the
// func. If `cd` has no labels, or if no label with the specified key exists, the empty string is
// returned.
func fromCDLabel(cd *hivev1.ClusterDeployment) func(string) string {
	return func(labelKey string) string {
		if cd.Labels == nil {
			return ""
		}
		return cd.Labels[labelKey]
	}
}

// applyTemplate recursively parses and executes `t` against the string values found within `v`.
// The template is executed with a `nil` data object -- i.e. templates referring to `.` ("dot")
// will not work. This really exists only to support invoking functions in the template's FuncMap.
// We expect `v` to be a descendant of an Unstructured.Object, and thus limited to types
// string, float, int, bool, []interface{}, or map[string]interface{} (where the list/map
// interface{} values are similarly limited, recursively).
func applyTemplate(t *template.Template, v interface{}) (interface{}, error) {
	ival := reflect.ValueOf(v)
	switch ival.Kind() {
	case reflect.String:
		sval := ival.String()
		// Special case: Parse ignores the empty string so hard that it keeps whatever was
		// previously in the template, resulting in reusing the string from the most recent
		// "successful" iteration. We could rebuild the parser every time, but this is more
		// efficient (and correct because the empty string can't contain any parameters).
		if sval == "" {
			return "", nil
		}
		parsed, err := t.Parse(sval)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse template string %q", sval)
		}
		buf := new(bytes.Buffer)
		err = parsed.Execute(buf, nil)
		return buf.String(), errors.Wrapf(err, "failed to execute template on string %q", sval)
	case reflect.Array, reflect.Slice:
		for i := 0; i < ival.Len(); i++ {
			newVal, err := applyTemplate(t, ival.Index(i).Interface())
			if err != nil {
				return nil, err
			}
			ival.Index(i).Set(reflect.ValueOf(newVal))
		}
	case reflect.Map:
		for _, k := range ival.MapKeys() {
			newVal, err := applyTemplate(t, ival.MapIndex(k).Interface())
			if err != nil {
				return nil, err
			}
			// Special case: If the elem is nil, SetMapIndex deletes the key (why??)
			// We know the template won't mutate `nil` anyway, so skip "setting" it.
			if newVal != nil {
				ival.SetMapIndex(k, reflect.ValueOf(newVal))
			}
		}
	}
	return v, nil
}
