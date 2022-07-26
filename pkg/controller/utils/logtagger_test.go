package utils

import (
	"reflect"
	"testing"

	log "github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAddLogFields(t *testing.T) {
	type args struct {
		obj    AdditionalLogFieldHavinThing
		logger *log.Entry
	}
	tests := []struct {
		name string
		args args
		want *log.Entry
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AddLogFields(tt.args.obj, tt.args.logger); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AddLogFields() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddLogFieldsEnvVar(t *testing.T) {
	type args struct {
		from metav1.Object
		to   *batchv1.Job
	}
	tests := []struct {
		name string
		args args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			AddLogFieldsEnvVar(tt.args.from, tt.args.to)
		})
	}
}

func TestCopyLogAnnotations(t *testing.T) {
	type args struct {
		from metav1.Object
		to   metav1.Object
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CopyLogAnnotations(tt.args.from, tt.args.to); got != tt.want {
				t.Errorf("CopyLogAnnotations() = %v, want %v", got, tt.want)
			}
		})
	}
}
