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

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericclioptions/printers"
	kcmd "k8s.io/kubernetes/pkg/kubectl/cmd"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
)

// Apply applies the given resource bytes to the target cluster specified by kubeconfig
func (r *Helper) Apply(obj []byte) error {

	fileName, err := r.createTempFile("apply-", obj)
	if err != nil {
		return err
	}
	defer r.deleteTempFile(fileName)
	factory, err := r.getFactory(r.kubeconfig, "")
	if err != nil {
		return err
	}
	ioStreams := genericclioptions.IOStreams{
		In:     &bytes.Buffer{},
		Out:    &bytes.Buffer{},
		ErrOut: &bytes.Buffer{},
	}
	applyOptions, err := r.setupApplyCommand(factory, fileName, ioStreams)
	if err != nil {
		r.logger.WithError(err).Error("failed to setup apply command")
		return err
	}
	err = applyOptions.Run()
	if err != nil {
		r.logger.WithError(err).
			WithField("stdout", ioStreams.Out.(*bytes.Buffer).String()).
			WithField("stderr", ioStreams.ErrOut.(*bytes.Buffer).String()).Error("running the apply command failed")
		return err
	}
	return nil
}

// ApplyRuntimeObject serializes an object and applies it to the target cluster specified by the kubeconfig.
func (r *Helper) ApplyRuntimeObject(obj runtime.Object, s *json.Serializer) error {
	buf := bytes.NewBuffer([]byte{})
	err := s.Encode(obj, buf)
	if err != nil {
		return err
	}
	err = r.Apply(buf.Bytes())
	if err != nil {
		return err
	}
	return nil
}

func (r *Helper) setupApplyCommand(f cmdutil.Factory, fileName string, ioStreams genericclioptions.IOStreams) (*kcmd.ApplyOptions, error) {
	r.logger.Debug("setting up apply command")
	o := kcmd.NewApplyOptions(ioStreams)
	o.ToPrinter = func(string) (printers.ResourcePrinter, error) { return o.PrintFlags.ToPrinter() }
	dynamicClient, err := f.DynamicClient()
	if err != nil {
		r.logger.WithError(err).Error("cannot obtain dynamic client from factory")
		return nil, err
	}
	o.DeleteOptions = o.DeleteFlags.ToOptions(dynamicClient, o.IOStreams)
	o.OpenAPISchema, _ = f.OpenAPISchema()
	o.Validator, err = f.Validator(false)
	o.Builder = f.NewBuilder()
	o.Mapper, err = f.ToRESTMapper()
	if err != nil {
		r.logger.WithError(err).Error("cannot obtain RESTMapper from factory")
		return nil, err
	}

	o.DynamicClient = dynamicClient
	o.Namespace, o.EnforceNamespace, err = f.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		r.logger.WithError(err).Error("cannot obtain namespace from factory")
		return nil, err
	}

	o.DeleteOptions.FilenameOptions.Filenames = []string{fileName}
	return o, nil
}
