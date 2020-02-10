// Generate deepcopy for apis
//go:generate go run ../../vendor/k8s.io/code-generator/cmd/deepcopy-gen/main.go -O zz_generated.deepcopy -i ./... -h ./../../hack/boilerplate.go.txt

// Generate clientset for apis
//go:generate go run ../../vendor/k8s.io/code-generator/cmd/client-gen/main.go --clientset-name "clientset" --input "github.com/openshift/hive/pkg/apis/hive/v1" --input-base "" --output-package "github.com/openshift/hive/pkg/client/clientset-generated" -h ./../../hack/boilerplate.go.txt

// Package apis contains Kubernetes API groups.
package apis

import (
	"k8s.io/apimachinery/pkg/runtime"
)

// AddToSchemes may be used to add all resources defined in the project to a Scheme
var AddToSchemes runtime.SchemeBuilder

// AddToScheme adds all Resources to the Scheme
func AddToScheme(s *runtime.Scheme) error {
	return AddToSchemes.AddToScheme(s)
}
