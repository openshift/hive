module github.com/openshift/hive/apis

go 1.24.0

toolchain go1.24.6

require (
	github.com/openshift/api v0.0.0-20250313134101-8a7efbfb5316
	github.com/openshift/installer v1.4.19-ec5
	k8s.io/api v0.33.3
	k8s.io/apimachinery v0.33.3
	sigs.k8s.io/yaml v1.4.0 // indirect
)

require (
	github.com/fxamacker/cbor/v2 v2.7.0 // indirect
	github.com/go-logr/logr v1.4.2
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	golang.org/x/net v0.45.0 // indirect
	golang.org/x/text v0.30.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/utils v0.0.0-20241210054802-24370beab758 // indirect
	sigs.k8s.io/json v0.0.0-20241014173422-cfa47c3a1cc8 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.6.0 // indirect
)

// CVE-2025-22872: Some transitive deps are still using older versions. Safe to remove once go.sum shows only 0.38.0 or higher.
replace golang.org/x/net => golang.org/x/net v0.38.0

replace github.com/openshift/installer => github.com/dlom/installer v0.0.0-20251023182801-c056b7bdd6ca
