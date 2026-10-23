package benchutil

import (
	"fmt"
	"strings"

	"github.com/gobuffalo/flect"
	configv1 "github.com/openshift/api/config/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	hivescheme "github.com/openshift/hive/pkg/util/scheme"
)

// openshiftCRDs returns minimal CRDs for OpenShift platform types.
func openshiftCRDs() []*apiextensionsv1.CustomResourceDefinition {
	return []*apiextensionsv1.CustomResourceDefinition{
		crdForType(&configv1.ClusterVersion{}, apiextensionsv1.ClusterScoped),
		crdForType(&configv1.ClusterOperator{}, apiextensionsv1.ClusterScoped),
		crdForType(&machinev1beta1.Machine{}, apiextensionsv1.NamespaceScoped),
		crdForType(&machinev1beta1.MachineSet{}, apiextensionsv1.NamespaceScoped),
	}
}

// crdForType builds a minimal CRD from a typed empty object.
func crdForType(obj runtime.Object, scope apiextensionsv1.ResourceScope) *apiextensionsv1.CustomResourceDefinition {
	gvks, _, err := hivescheme.GetScheme().ObjectKinds(obj)
	if err != nil || len(gvks) == 0 {
		panic(fmt.Sprintf("crdForType: type not registered in scheme: %T", obj))
	}
	gvk := gvks[0]
	plural := strings.ToLower(flect.Pluralize(gvk.Kind))

	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: plural + "." + gvk.Group},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: gvk.Group,
			Scope: scope,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   plural,
				Singular: strings.ToLower(gvk.Kind),
				Kind:     gvk.Kind,
				ListKind: gvk.Kind + "List",
			},
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
				Name:    gvk.Version,
				Served:  true,
				Storage: true,
				Schema: &apiextensionsv1.CustomResourceValidation{
					OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
						Type:                   "object",
						XPreserveUnknownFields: boolPtr(true),
					},
				},
			}},
		},
	}
}

func boolPtr(b bool) *bool { return &b }
