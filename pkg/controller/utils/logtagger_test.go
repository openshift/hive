package utils

import (
	"testing"

	"github.com/openshift/hive/pkg/constants"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	testjob "github.com/openshift/hive/pkg/test/job"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
)

const (
	// save some typing
	alfa = constants.AdditionalLogFieldsAnnotation
)

func TestAddLogFields(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        map[string]any
	}{
		{
			name: "no annotations",
		},
		{
			name:        "annotations, but not the one we care about",
			annotations: map[string]string{"foo": "bar"},
		},
		{
			name:        "empty string (unparseable json)",
			annotations: map[string]string{alfa: ""},
		},
		{
			name:        "unparseable json",
			annotations: map[string]string{alfa: "foo: bar"},
		},
		{
			name:        "empty json",
			annotations: map[string]string{alfa: `{}`},
			want:        map[string]any{"component": "hive"},
		},
		{
			name:        "one field",
			annotations: map[string]string{alfa: `{"foo": "bar"}`},
			want: map[string]any{
				"foo":       "bar",
				"component": "hive",
			},
		},
		{
			name:        "multiple fields",
			annotations: map[string]string{alfa: `{"foo": "bar", "HELLO": "WORLD"}`},
			want: map[string]any{
				"foo":       "bar",
				"HELLO":     "WORLD",
				"component": "hive",
			},
		},
		{
			name: "complex value",
			// logrus.WithFields happily accepts this (and marshals to a string when emitting)
			annotations: map[string]string{alfa: `{"foo": {"bar": "baz"}}`},
			want: map[string]any{
				"foo":       map[string]any{"bar": "baz"},
				"component": "hive",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origLogger := log.NewEntry(&log.Logger{})
			cd := testcd.BasicBuilder().Build()
			var s string
			if tt.annotations != nil {
				cd.SetAnnotations(tt.annotations)
				// May be "" if not using alfa
				s = tt.annotations[alfa]
			}
			for typstr, obj := range map[string]AdditionalLogFieldHavinThing{
				"meta":   MetaObjectLogTagger{Object: cd},
				"string": StringLogTagger{S: s},
			} {
				if got := AddLogFields(obj, origLogger); assert.NotNil(t, got, "%s: expected AddLogFields to return non-nil", typstr) {
					// If no fields are expected, `component` should be absent
					if tt.want == nil {
						gotv, exists := got.Data["component"]
						assert.False(t, exists, "%s: unexpected component %q", typstr, gotv)
					}

					for wantk, wantv := range tt.want {
						if gotv, exists := got.Data[wantk]; assert.True(t, exists, "%s: key %q missing", typstr, wantk) {
							assert.Equal(t, wantv, gotv, "%s: bad value for key %q", typstr, wantk)
						}
					}
				}

			}
		})
	}
}

func TestAddLogFieldsEnvVar(t *testing.T) {
	mkJob := func(containers []v1.Container) *batchv1.Job {
		if containers == nil {
			containers = make([]v1.Container, 0)
		}
		job := testjob.BasicBuilder().Build()
		job.Spec.Template.Spec.Containers = containers
		return job
	}
	tests := []struct {
		name           string
		annotations    map[string]string
		origContainers []v1.Container
	}{
		{
			name: "no op: source has no annotations",
			origContainers: []v1.Container{
				{},
			},
		},
		{
			name:        "no op: source has annotations, but not the one we care about",
			annotations: map[string]string{"foo": "bar"},
			origContainers: []v1.Container{
				{},
			},
		},
		{
			name:        "no op: target has no containers",
			annotations: map[string]string{alfa: `{"HELLO": "WORLD}`},
		},
		{
			name:        "add, replace, no-op",
			annotations: map[string]string{alfa: `{"HELLO": "WORLD}`},
			origContainers: []v1.Container{
				// Empty container gets the var added
				{},
				{
					Env: []v1.EnvVar{
						{
							Name:  "UNRELATED",
							Value: "unchanged",
						},
						{
							Name:  constants.AdditionalLogFieldsEnvVar,
							Value: "will be replaced",
						},
					},
				},
				{
					Env: []v1.EnvVar{
						{
							Name:  constants.AdditionalLogFieldsEnvVar,
							Value: `{"HELLO": "WORLD}`,
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			from := testcd.BasicBuilder().Build()
			if tt.annotations != nil {
				from.SetAnnotations(tt.annotations)
			}
			job := mkJob(tt.origContainers)
			origJob := job.DeepCopy()
			AddLogFieldsEnvVar(from, job)
			var alfaVal *string
			if tt.annotations != nil {
				if annotation, exists := tt.annotations[constants.AdditionalLogFieldsAnnotation]; exists {
					alfaVal = &annotation
				}
			}
			if alfaVal == nil {
				assert.Equal(t, origJob, job, "expected job to be unchanged")
			} else {
				// Env var should be present on every container
				for i, c := range job.Spec.Template.Spec.Containers {
					found := false
					for _, e := range c.Env {
						if e.Name != constants.AdditionalLogFieldsEnvVar {
							continue
						}
						found = true
						assert.Equal(t, *alfaVal, e.Value, "incorrect env var value")
					}
					assert.True(t, found, "didn't find env var in container %d", i)
				}
			}
		})
	}
}

func TestCopyLogAnnotation(t *testing.T) {
	strPtr := func(s string) *string { return &s }
	tests := []struct {
		name                   string
		froma                  map[string]string
		toa                    map[string]string
		expectChanged          bool
		expectTargetAnnotation *string
	}{
		{
			name: "no op: none, none",
		},
		{
			name: "no op: none, empty",
			toa:  map[string]string{},
		},
		{
			name: "no op: none, unrelated",
			toa:  map[string]string{"foo": "bar"},
		},
		{
			name: "delete: none, present",
			toa: map[string]string{
				"foo": "bar",
				alfa:  `{"HELLO": "WORLD"}`,
			},
			expectChanged: true,
		},
		{
			name:  "no op: empty, none",
			froma: map[string]string{},
		},
		{
			name:  "no op: empty, empty",
			froma: map[string]string{},
			toa:   map[string]string{},
		},
		{
			name:  "no op: empty, unrelated",
			froma: map[string]string{},
			toa:   map[string]string{"foo": "bar"},
		},
		{
			name:  "delete: empty, present",
			froma: map[string]string{},
			toa: map[string]string{
				"foo": "bar",
				alfa:  `{"HELLO": "WORLD"}`,
			},
			expectChanged: true,
		},
		{
			name:  "no op: unrelated, none",
			froma: map[string]string{"foo": "bar"},
		},
		{
			name:  "no op: unrelated, empty",
			froma: map[string]string{"foo": "bar"},
			toa:   map[string]string{},
		},
		{
			name:  "no op: unrelated, unrelated",
			froma: map[string]string{"foo": "bar"},
			toa:   map[string]string{"foo": "baz"},
		},
		{
			name:  "delete: unrelated, present",
			froma: map[string]string{"foo": "bar"},
			toa: map[string]string{
				"foo": "bar",
				alfa:  `{"HELLO": "WORLD"}`,
			},
			expectChanged: true,
		},
		{
			name: "create: present, none",
			froma: map[string]string{
				"foo": "bar",
				alfa:  `{"HELLO": "WORLD"}`,
			},
			expectChanged:          true,
			expectTargetAnnotation: strPtr(`{"HELLO": "WORLD"}`),
		},
		{
			name: "add: present, empty",
			froma: map[string]string{
				"foo": "bar",
				alfa:  `{"HELLO": "WORLD"}`,
			},
			toa:                    map[string]string{},
			expectChanged:          true,
			expectTargetAnnotation: strPtr(`{"HELLO": "WORLD"}`),
		},
		{
			name: "add: present, unrelated",
			froma: map[string]string{
				"foo": "bar",
				alfa:  `{"HELLO": "WORLD"}`,
			},
			toa:                    map[string]string{"foo": "baz"},
			expectChanged:          true,
			expectTargetAnnotation: strPtr(`{"HELLO": "WORLD"}`),
		},
		{
			name: "replace: present, different",
			froma: map[string]string{
				"foo": "bar",
				alfa:  `{"HELLO": "WORLD"}`,
			},
			toa: map[string]string{
				alfa: `{"GOODBYE": "CRUEL WORLD"}`,
			},
			expectChanged:          true,
			expectTargetAnnotation: strPtr(`{"HELLO": "WORLD"}`),
		},
		{
			name: "no op: present, same",
			froma: map[string]string{
				"foo": "bar",
				alfa:  `{"HELLO": "WORLD"}`,
			},
			toa: map[string]string{
				alfa: `{"HELLO": "WORLD"}`,
			},
			expectTargetAnnotation: strPtr(`{"HELLO": "WORLD"}`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			from := testcd.BasicBuilder().Build()
			if tt.froma != nil {
				from.Annotations = tt.froma
			}
			to := testcd.BasicBuilder().Build()
			if tt.toa != nil {
				to.Annotations = tt.toa
			}
			if got := CopyLogAnnotation(from, to); got != tt.expectChanged {
				t.Errorf("CopyLogAnnotations() = %v, want %v", got, tt.expectChanged)
			}
			if tt.expectTargetAnnotation != nil {
				if assert.NotNil(t, to.Annotations, "expected target's annotations not to be nil") {
					value, exists := to.Annotations[alfa]
					if assert.True(t, exists, "expected %s annotation to exist", alfa) {
						assert.Equal(t, *tt.expectTargetAnnotation, value, "wrong value for %s annotation", alfa)
					}
				}
			}
		})
	}
}
