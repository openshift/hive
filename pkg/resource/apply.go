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
	"fmt"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericclioptions/printers"
	kcmd "k8s.io/kubernetes/pkg/kubectl/cmd"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
)

// Info contains information obtained from a resource submitted to the Apply function
type Info struct {
	Name       string
	Namespace  string
	APIVersion string
	Kind       string
}

// Apply applies the given resource bytes to the target cluster specified by kubeconfig
func (r *Helper) Apply(obj []byte) (*Info, error) {

	fileName, err := r.createTempFile("apply-", obj)
	if err != nil {
		return nil, err
	}
	defer r.deleteTempFile(fileName)
	factory, err := r.getFactory(r.kubeconfig, "")
	if err != nil {
		return nil, err
	}
	resourceInfo, err := r.getResourceInfo(factory, obj)
	if err != nil {
		return nil, err
	}
	ioStreams := genericclioptions.IOStreams{
		In:     &bytes.Buffer{},
		Out:    &bytes.Buffer{},
		ErrOut: &bytes.Buffer{},
	}
	applyOptions, err := r.setupApplyCommand(factory, fileName, ioStreams)
	if err != nil {
		r.logger.WithError(err).Error("failed to setup apply command")
		return nil, err
	}
	err = applyOptions.Run()
	if err != nil {
		r.logger.WithError(err).
			WithField("stdout", ioStreams.Out.(*bytes.Buffer).String()).
			WithField("stderr", ioStreams.ErrOut.(*bytes.Buffer).String()).Error("running the apply command failed")
		return nil, err
	}
	return resourceInfo, nil
}

func (r *Helper) getResourceInfo(f cmdutil.Factory, obj []byte) (*Info, error) {
	builder := f.NewBuilder()
	r.logger.Debug("Obtaining resource info for object")
	infos, err := builder.Unstructured().Stream(bytes.NewBuffer(obj), "object").Flatten().Do().Infos()
	if err != nil {
		r.logger.WithError(err).Error("Failed to obtain resource info")
		return nil, fmt.Errorf("could not get info from passed resource: %v", err)
	}
	if len(infos) != 1 {
		r.logger.WithError(err).WithField("infos", infos).Errorf("Expected to get 1 resource info, got %d", len(infos))
		return nil, fmt.Errorf("unexpected number of resources found: %d", len(infos))
	}
	r.logger.WithField("info", infos[0]).Debug("obtained resource information")
	return &Info{
		Name:       infos[0].Name,
		Namespace:  infos[0].Namespace,
		Kind:       infos[0].ResourceMapping().GroupVersionKind.Kind,
		APIVersion: infos[0].ResourceMapping().GroupVersionKind.GroupVersion().String(),
	}, nil
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
