package resource

import (
	"bytes"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/resource"
)

// Info contains information obtained from a resource submitted to the Apply function
type Info struct {
	Name       string
	Namespace  string
	APIVersion string
	Kind       string
	Resource   string
	Object     *unstructured.Unstructured
}

// Info determines the name/namespace and type of the passed in resource bytes
func (r *Helper) Info(obj []byte) (*Info, error) {
	resourceInfo, err := r.getResourceInfo(obj)
	if err != nil {
		return nil, err
	}
	return resourceInfo, err
}

func (r *Helper) getResourceInternalInfo(obj []byte) (*resource.Info, error) {
	builder := r.factory.NewBuilder()
	infos, err := builder.Unstructured().Stream(bytes.NewBuffer(obj), "object").Flatten().Do().Infos()
	if err != nil {
		r.logger.WithError(err).Error("Failed to obtain resource info")
		return nil, fmt.Errorf("could not get info from passed resource: %v", err)
	}
	if len(infos) != 1 {
		r.logger.WithError(err).WithField("infos", infos).Errorf("Expected to get 1 resource info, got %d", len(infos))
		return nil, fmt.Errorf("unexpected number of resources found: %d", len(infos))
	}
	return infos[0], nil
}

func (r *Helper) getResourceInfo(obj []byte) (*Info, error) {
	info, err := r.getResourceInternalInfo(obj)
	if err != nil {
		return nil, err
	}
	return &Info{
		Name:       info.Name,
		Namespace:  info.Namespace,
		Kind:       info.ResourceMapping().GroupVersionKind.Kind,
		APIVersion: info.ResourceMapping().GroupVersionKind.GroupVersion().String(),
		Resource:   info.ResourceMapping().Resource.Resource,
		Object:     info.Object.(*unstructured.Unstructured),
	}, nil
}
