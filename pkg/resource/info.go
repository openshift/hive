package resource

import (
	"bytes"
	"fmt"

	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
)

// Info contains information obtained from a resource submitted to the Apply function
type Info struct {
	Name       string
	Namespace  string
	APIVersion string
	Kind       string
	Resource   string
}

// Info determines the name/namespace and type of the passed in resource bytes
func (r *Helper) Info(obj []byte) (*Info, error) {
	factory, err := r.getFactory("")
	if err != nil {
		return nil, err
	}
	resourceInfo, err := r.getResourceInfo(factory, obj)
	if err != nil {
		return nil, err
	}
	return resourceInfo, err
}

func (r *Helper) getResourceInfo(f cmdutil.Factory, obj []byte) (*Info, error) {
	builder := f.NewBuilder()
	infos, err := builder.Unstructured().Stream(bytes.NewBuffer(obj), "object").Flatten().Do().Infos()
	if err != nil {
		r.logger.WithError(err).Error("Failed to obtain resource info")
		return nil, fmt.Errorf("could not get info from passed resource: %v", err)
	}
	if len(infos) != 1 {
		r.logger.WithError(err).WithField("infos", infos).Errorf("Expected to get 1 resource info, got %d", len(infos))
		return nil, fmt.Errorf("unexpected number of resources found: %d", len(infos))
	}
	return &Info{
		Name:       infos[0].Name,
		Namespace:  infos[0].Namespace,
		Kind:       infos[0].ResourceMapping().GroupVersionKind.Kind,
		APIVersion: infos[0].ResourceMapping().GroupVersionKind.GroupVersion().String(),
		Resource:   infos[0].ResourceMapping().Resource.Resource,
	}, nil
}
