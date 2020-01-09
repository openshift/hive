// Generate deepcopy for apis
//go:generate go run ../../../../vendor/k8s.io/code-generator/cmd/deepcopy-gen/main.go -O zz_generated.deepcopy -i ./... -h ../../../../hack/boilerplate.go.txt

// Generate conversion for apis
//go:generate go run ../../../../vendor/k8s.io/code-generator/cmd/conversion-gen/main.go -O zz_generated.conversion -i ./v1alpha1 -h ../../../../hack/boilerplate.go.txt

// Generate defaults for apis
//go:generate go run ../../../../vendor/k8s.io/code-generator/cmd/defaulter-gen/main.go -O zz_generated.defaults -i ./v1alpha1 -h ../../../../hack/boilerplate.go.txt

// +k8s:deepcopy-gen=package,register

// +groupName=hive.openshift.io

// Package hive is the internal version of the API.
package hive
