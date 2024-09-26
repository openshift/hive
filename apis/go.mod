module github.com/openshift/hive/apis

go 1.22.0

toolchain go1.22.5

require (
	github.com/openshift/api v0.0.0-20240904015708-69df64132c91
	github.com/openshift/custom-resource-status v1.1.3-0.20220503160415-f2fdb4999d87
	// !WARNING! We intend this to pull in *only* pkg/types. Bringing in anything
	// else could cause a dependency explosion, which we must avoid in this apis/
	// package for the sake of our downstream consumers.
	github.com/openshift/installer v0.91.0
	k8s.io/api v0.31.0-alpha.2
	k8s.io/apimachinery v0.31.0-alpha.2
)

require (
	github.com/fxamacker/cbor/v2 v2.7.0-beta // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	golang.org/x/net v0.29.0 // indirect
	golang.org/x/text v0.18.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	k8s.io/klog/v2 v2.120.1 // indirect
	k8s.io/utils v0.0.0-20240310230437-4693a0247e57 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.4.1 // indirect
)
