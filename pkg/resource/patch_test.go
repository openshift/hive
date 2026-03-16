package resource

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	kcmdpatch "k8s.io/kubectl/pkg/cmd/patch"
)

func TestSetPatchOptionsFieldManagerHACK(t *testing.T) {
	o := kcmdpatch.NewPatchOptions(genericiooptions.NewTestIOStreamsDiscard())

	// verify all the reflection assumptions we make:

	v := reflect.ValueOf(o)
	assert.Equal(t, reflect.Ptr, v.Kind(), "PatchOptions must be a pointer")

	elem := v.Elem()
	assert.Equal(t, reflect.Struct, elem.Kind(), "PatchOptions must dereference to a struct")

	f := elem.FieldByName("fieldManager")
	assert.True(t, f.IsValid(), "PatchOptions has no field named 'fieldManager' - was it renamed or removed in a kubectl update?")

	assert.Equal(t, reflect.String, f.Kind(), "fieldManager must be a string")
	assert.True(t, f.CanAddr(), "fieldManager must be addressable")

	sf, ok := elem.Type().FieldByName("fieldManager")
	assert.True(t, ok, "failed to get struct field metadata")
	assert.False(t, sf.IsExported(), "fieldManager is exported - reflection hack is no longer necessary, set it directly")

	assert.Equal(t, "", f.String(), "initial fieldManager should be empty")

	// verify reflect patching functionality:

	setPrivateFieldManagerHACK(o, "hive-test-controller")
	assert.Equal(t, "hive-test-controller", f.String())

	setPrivateFieldManagerHACK(o, "hive-other-controller")
	assert.Equal(t, "hive-other-controller", f.String())
}
