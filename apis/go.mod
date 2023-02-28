module github.com/openshift/hive/apis

go 1.19

require (
	github.com/openshift/api v3.9.1-0.20191111211345-a27ff30ebf09+incompatible
	github.com/openshift/custom-resource-status v1.1.3-0.20220503160415-f2fdb4999d87
	k8s.io/api v0.26.1
	k8s.io/apimachinery v0.26.1
)

require (
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	golang.org/x/net v0.7.0 // indirect
	golang.org/x/text v0.7.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	k8s.io/klog/v2 v2.90.0 // indirect
	k8s.io/utils v0.0.0-20230115233650-391b47cb4029 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
)

replace github.com/openshift/api => github.com/openshift/api v0.0.0-20230208155555-942fb8575cc4

// https://github.com/openshift/hive/security/dependabot/11
// https://github.com/openshift/hive/security/dependabot/14
replace (
	golang.org/x/text v0.3.0 => golang.org/x/text v0.3.8
	golang.org/x/text v0.3.2 => golang.org/x/text v0.3.8
	golang.org/x/text v0.3.3 => golang.org/x/text v0.3.8
	golang.org/x/text v0.3.5 => golang.org/x/text v0.3.8
	golang.org/x/text v0.3.6 => golang.org/x/text v0.3.8
	golang.org/x/text v0.3.7 => golang.org/x/text v0.3.8
)

// https://github.com/openshift/hive/security/dependabot/8
// https://github.com/openshift/hive/security/dependabot/9
// https://github.com/openshift/hive/security/dependabot/10
// https://github.com/openshift/hive/security/dependabot/12
// https://github.com/openshift/hive/security/dependabot/13
replace (
	golang.org/x/net v0.0.0-20180724234803-3673e40ba225 => golang.org/x/net v0.7.0
	golang.org/x/net v0.0.0-20180826012351-8a410e7b638d => golang.org/x/net v0.7.0
	golang.org/x/net v0.0.0-20180906233101-161cd47e91fd => golang.org/x/net v0.7.0
	golang.org/x/net v0.0.0-20190213061140-3a22650c66bd => golang.org/x/net v0.7.0
	golang.org/x/net v0.0.0-20190311183353-d8887717615a => golang.org/x/net v0.7.0
	golang.org/x/net v0.0.0-20190404232315-eb5bcb51f2a3 => golang.org/x/net v0.7.0
	golang.org/x/net v0.0.0-20190620200207-3b0461eec859 => golang.org/x/net v0.7.0
	golang.org/x/net v0.0.0-20190827160401-ba9fcec4b297 => golang.org/x/net v0.7.0
	golang.org/x/net v0.0.0-20200226121028-0de0cce0169b => golang.org/x/net v0.7.0
	golang.org/x/net v0.0.0-20200520004742-59133d7f0dd7 => golang.org/x/net v0.7.0
	golang.org/x/net v0.0.0-20201021035429-f5854403a974 => golang.org/x/net v0.7.0
	golang.org/x/net v0.0.0-20210226172049-e18ecbb05110 => golang.org/x/net v0.7.0
	golang.org/x/net v0.0.0-20210405180319-a5a99cb37ef4 => golang.org/x/net v0.7.0
	golang.org/x/net v0.0.0-20210421230115-4e50805a0758 => golang.org/x/net v0.7.0
	golang.org/x/net v0.0.0-20210428140749-89ef3d95e781 => golang.org/x/net v0.7.0
	golang.org/x/net v0.0.0-20210805182204-aaa1db679c0d => golang.org/x/net v0.7.0
	golang.org/x/net v0.0.0-20211015210444-4f30a5c0130f => golang.org/x/net v0.7.0
	golang.org/x/net v0.0.0-20211209124913-491a49abca63 => golang.org/x/net v0.7.0
	golang.org/x/net v0.0.0-20220127200216-cd36cc0744dd => golang.org/x/net v0.7.0
	golang.org/x/net v0.0.0-20220722155237-a158d28d115b => golang.org/x/net v0.7.0
)
