package manifest

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/scheme"
)

// resourceId uniquely identifies a Kubernetes resource.
// It is used to identify any duplicate resources within
// a given set of manifests.
type resourceId struct {
	// Group identifies a set of API resources exposed together.
	// +optional
	Group string
	// Kind is the name of a particular object schema.
	Kind string
	// Name, sometimes used with the optional Namespace, helps uniquely identify an object.
	Name string
	// Namespace helps uniquely identify an object.
	// +optional
	Namespace string
}

// Manifest stores Kubernetes object in Raw from a file.
// It stores the id and the GroupVersionKind for
// the manifest. Raw and Obj should always be kept in sync
// such that each provides the same data but in different
// formats. To ensure Raw and Obj are always in sync, they
// should not be set directly but rather only be set by
// calling either method ManifestsFromFiles or
// ParseManifests.
type Manifest struct {
	// OriginalFilename is set to the filename this manifest was loaded from.
	// It is not guaranteed to be set or be unique, but will be set when
	// loading from disk to provide a better debug capability.
	OriginalFilename string

	id resourceId

	Raw []byte
	GVK schema.GroupVersionKind

	Obj *unstructured.Unstructured
}

func (r resourceId) String() string {
	if len(r.Namespace) == 0 {
		return fmt.Sprintf("Group: %q Kind: %q Name: %q", r.Group, r.Kind, r.Name)
	} else {
		return fmt.Sprintf("Group: %q Kind: %q Namespace: %q Name: %q", r.Group, r.Kind, r.Namespace, r.Name)
	}
}

// UnmarshalJSON implements the json.Unmarshaler interface for the Manifest
// type. It unmarshals bytes of a single kubernetes object to Manifest.
func (m *Manifest) UnmarshalJSON(in []byte) error {
	if m == nil {
		return errors.New("Manifest: UnmarshalJSON on nil pointer")
	}

	// This happens when marshalling
	// <yaml>
	// ---	(this between two `---`)
	// ---
	// <yaml>
	if bytes.Equal(in, []byte("null")) {
		m.Raw = nil
		return nil
	}

	m.Raw = append(m.Raw[0:0], in...)
	udi, _, err := scheme.Codecs.UniversalDecoder().Decode(in, nil, &unstructured.Unstructured{})
	if err != nil {
		return errors.Wrapf(err, "unable to decode manifest")
	}
	ud, ok := udi.(*unstructured.Unstructured)
	if !ok {
		return fmt.Errorf("expected manifest to decode into *unstructured.Unstructured, got %T", ud)
	}
	m.GVK = ud.GroupVersionKind()
	m.Obj = ud
	m.id = resourceId{
		Group:     m.GVK.Group,
		Kind:      m.GVK.Kind,
		Namespace: m.Obj.GetNamespace(),
		Name:      m.Obj.GetName(),
	}
	return validateResourceId(m.id)
}

// ManifestsFromFiles reads files and returns Manifests in the same order.
// 'files' should be list of absolute paths for the manifests on disk. An
// error is returned for each manifest that defines a duplicate resource
// as compared to other manifests defined within the 'files' list.
// Duplicate resources have the same group, kind, name, and namespace.
func ManifestsFromFiles(files []string) ([]Manifest, error) {
	var manifests []Manifest
	ids := make(map[resourceId]bool)
	var errs []error
	for _, file := range files {
		file, err := os.Open(file)
		if err != nil {
			errs = append(errs, errors.Wrapf(err, "error opening %s", file.Name()))
			continue
		}
		defer file.Close()

		ms, err := ParseManifests(file)
		if err != nil {
			errs = append(errs, errors.Wrapf(err, "error parsing %s", file.Name()))
			continue
		}
		for _, m := range ms {
			m.OriginalFilename = filepath.Base(file.Name())
			err = addIfNotDuplicateResource(m, ids)
			if err != nil {
				errs = append(errs, errors.Wrapf(err, "File %s contains", file.Name()))
			}
		}
		manifests = append(manifests, ms...)
	}

	agg := utilerrors.NewAggregate(errs)
	if agg != nil {
		return nil, fmt.Errorf("error loading manifests: %v", agg.Error())
	}

	return manifests, nil
}

// ParseManifests parses a YAML or JSON document that may contain one or more
// kubernetes resources. An error is returned if the input cannot be parsed
// or contains a duplicate resource.
func ParseManifests(r io.Reader) ([]Manifest, error) {
	theseIds := make(map[resourceId]bool)
	d := yaml.NewYAMLOrJSONDecoder(r, 1024)
	var manifests []Manifest
	for {
		m := Manifest{}
		if err := d.Decode(&m); err != nil {
			if err == io.EOF {
				return manifests, nil
			}
			return manifests, errors.Wrapf(err, "error parsing")
		}
		m.Raw = bytes.TrimSpace(m.Raw)
		if len(m.Raw) == 0 || bytes.Equal(m.Raw, []byte("null")) {
			continue
		}
		if err := addIfNotDuplicateResource(m, theseIds); err != nil {
			return manifests, err
		}
		manifests = append(manifests, m)
	}
}

// validateResourceId ensures the id contains the required fields per
// https://kubernetes.io/docs/concepts/overview/working-with-objects/kubernetes-objects/#required-fields.
func validateResourceId(id resourceId) error {
	if id.Kind == "" || id.Name == "" {
		return fmt.Errorf("Resource with fields %s must contain kubernetes required fields kind and name", id)
	}
	return nil
}

func addIfNotDuplicateResource(manifest Manifest, resourceIds map[resourceId]bool) error {
	if _, ok := resourceIds[manifest.id]; !ok {
		resourceIds[manifest.id] = true
		return nil
	}
	return fmt.Errorf("duplicate resource: (%s)", manifest.id)
}
