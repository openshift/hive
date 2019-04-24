/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package resource

import (
	"bytes"
	"io"
	"reflect"
	"unsafe"

	json "github.com/json-iterator/go"
	"github.com/modern-go/reflect2"
	log "github.com/sirupsen/logrus"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions/printers"
)

var (
	jsonAPI json.API
)

func init() {
	jsonAPI = json.Config{EscapeHTML: true}.Froze()
	jsonAPI.RegisterExtension(&metaTimeExtension{})
}

type metaTimeExtension struct {
	json.DummyExtension
}

func (e *metaTimeExtension) CreateEncoder(typ reflect2.Type) json.ValEncoder {
	if typ.Type1() == reflect.TypeOf(metav1.Time{}) {
		return e
	}
	return nil
}

func (e *metaTimeExtension) IsEmpty(ptr unsafe.Pointer) bool {
	metaTime := reflect2.TypeOf(metav1.Time{}).UnsafeIndirect(ptr).(metav1.Time)
	return metaTime.IsZero()
}

func (e *metaTimeExtension) Encode(ptr unsafe.Pointer, stream *json.Stream) {
	metaTime := reflect2.TypeOf(metav1.Time{}).UnsafeIndirect(ptr).(metav1.Time)
	data, err := metaTime.MarshalJSON()
	if err != nil {
		log.Errorf("cannot marshal %#v as meta time: %v", ptr, err)
		return
	}
	_, err = stream.Write(data)
	if err != nil {
		log.Errorf("cannot write serialized time (%s): %v", string(data), err)
	}
}

type jsonPrinter struct{}

func (p *jsonPrinter) PrintObj(obj runtime.Object, w io.Writer) error {
	data, err := jsonAPI.Marshal(obj)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = w.Write(data)
	return err
}

// Serialize uses a custom JSON extension to properly determine whether metav1.Time should
// be serialized or not. In cases where a metav1.Time is labeled as 'omitempty', the default
// json marshaler still outputs a "null" value because it is considered a struct.
// The json-iterator/go marshaler will first check whether a value is empty and if its tag
// says 'omitempty' it will not output it.
// This is needed for us to prevent patching from happening unnecessarily when applying resources
// that don't have a timeCreated timestamp. With the default serializer, they output a
// `timeCreated: null` which always causes a mismatch with whatever's already in the server.
func (r *Helper) Serialize(obj runtime.Object, scheme *runtime.Scheme) ([]byte, error) {
	printer := printers.NewTypeSetter(scheme).ToPrinter(&jsonPrinter{})
	buf := &bytes.Buffer{}
	if err := printer.PrintObj(obj, buf); err != nil {
		r.logger.WithError(err).Errorf("cannot serialize runtime object of type %T", obj)
		return nil, err
	}
	return buf.Bytes(), nil
}
