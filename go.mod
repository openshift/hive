module github.com/openshift/hive

go 1.13

require (
	github.com/Azure/azure-sdk-for-go v38.1.0+incompatible
	github.com/Azure/go-autorest/autorest v0.9.3
	github.com/Azure/go-autorest/autorest/azure/auth v0.4.1
	github.com/Azure/go-autorest/autorest/to v0.3.1-0.20191028180845-3492b2aff503
	github.com/aws/aws-sdk-go v1.29.24
	github.com/docker/go-healthcheck v0.1.0
	github.com/evanphx/json-patch v4.5.0+incompatible
	github.com/ghodss/yaml v1.0.1-0.20190212211648-25d852aebe32
	github.com/golang/mock v1.3.1
	github.com/golangci/golangci-lint v1.23.8
	github.com/google/gofuzz v1.1.0 // indirect
	github.com/google/uuid v1.1.1
	github.com/heptio/velero v1.0.0
	github.com/json-iterator/go v1.1.8
	github.com/jteeuwen/go-bindata v3.0.8-0.20151023091102-a0ff2567cfb7+incompatible
	github.com/miekg/dns v1.1.15
	github.com/modern-go/reflect2 v1.0.1
	github.com/onsi/gomega v1.8.1
	github.com/openshift/api v3.9.1-0.20191111211345-a27ff30ebf09+incompatible
	github.com/openshift/cluster-api v0.0.0-20191129101638-b09907ac6668
	github.com/openshift/cluster-api-provider-gcp v0.0.1-0.20200120152131-1b09fd9e7156
	github.com/openshift/cluster-autoscaler-operator v0.0.0-20190521201101-62768a6ba480
	github.com/openshift/generic-admission-server v1.14.0
	github.com/openshift/installer v0.9.0-master.0.20200415072451-8ba1754a3f54
	github.com/openshift/library-go v0.0.0-20200210105614-4bf528465627
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.2.1
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.4.0
	golang.org/x/lint v0.0.0-20200302205851-738671d3881b
	golang.org/x/net v0.0.0-20200226121028-0de0cce0169b
	golang.org/x/oauth2 v0.0.0-20191202225959-858c2ad4c8b6
	golang.org/x/tools v0.0.0-20200331202046-9d5940d49312 // indirect
	google.golang.org/api v0.14.0
	gopkg.in/ini.v1 v1.51.0
	gopkg.in/yaml.v2 v2.2.8
	k8s.io/api v0.17.2
	k8s.io/apiextensions-apiserver v0.17.1
	k8s.io/apimachinery v0.17.3
	k8s.io/apiserver v0.17.1 // indirect
	k8s.io/cli-runtime v0.17.1
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/cluster-registry v0.0.6
	k8s.io/code-generator v0.17.2
	k8s.io/component-base v0.17.1 // indirect
	k8s.io/klog v1.0.0
	k8s.io/kube-aggregator v0.17.1
	k8s.io/kubectl v0.17.1
	k8s.io/utils v0.0.0-20191217005138-9e5e9d854fcc
	sigs.k8s.io/cluster-api-provider-aws v0.0.0
	sigs.k8s.io/cluster-api-provider-azure v0.0.0
	sigs.k8s.io/controller-runtime v0.4.0
	sigs.k8s.io/controller-tools v0.2.4
	sigs.k8s.io/yaml v1.1.0
)

replace (
	github.com/coreos/go-systemd => github.com/coreos/go-systemd/v22 v22.0.0 // Pin non-versioned import to v22.0.0
	github.com/go-log/log => github.com/go-log/log v0.1.1-0.20181211034820-a514cf01a3eb // Pinned by MCO
	github.com/hashicorp/consul => github.com/hashicorp/consul v1.6.2 // Pin to version required by terraform fork
	github.com/hashicorp/terraform => github.com/openshift/hashicorp-terraform v0.12.20-openshift-2 // Pin to fork with deduplicated rpc types
	github.com/hashicorp/terraform-plugin-sdk => github.com/openshift/hashicorp-terraform-plugin-sdk v1.6.0-openshift // Pin to fork with public rpc types
	github.com/metal3-io/baremetal-operator => github.com/openshift/baremetal-operator v0.0.0-20200206190020-71b826cc0f0a // Use OpenShift fork
	github.com/metal3-io/cluster-api-provider-baremetal => github.com/openshift/cluster-api-provider-baremetal v0.0.0-20190821174549-a2a477909c1d // Pin OpenShift fork
	github.com/openshift/api => github.com/openshift/api v0.0.0-20190627141250-de5ca909c732 // random commit that we happen to be using
	github.com/openshift/library-go => github.com/openshift/library-go v0.0.0-20200106191802-9821002633e8 // random commit that we happen to be using
	github.com/openshift/machine-config-operator => github.com/openshift/machine-config-operator v0.0.1-0.20200130220348-e5685c0cf530 // Pin MCO so it doesn't get downgraded
	github.com/prometheus => github.com/prometheus v0.9.2 // v0.9.2
	github.com/prometheus/client_golang => github.com/prometheus/client_golang v0.9.2 // v0.9.2
	github.com/terraform-providers/terraform-provider-azurerm => github.com/openshift/terraform-provider-azurerm v1.41.1-openshift-3 // Pin to openshift fork with IPv6 fixes
	google.golang.org/api => google.golang.org/api v0.13.0 // Pin to version required by tf-provider-google
	k8s.io/api => k8s.io/api v0.16.4 // v0.16.4
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20190409022649-727a075fdec8 // kubernetes-1.14.1
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190404173353-6a84e37a896d // kubernetes-1.14.1
	k8s.io/apiserver => k8s.io/apiserver v0.0.0-20190409021813-1ec86e4da56c // kubernetes-1.14.1
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.0.0-20190409023024-d644b00f3b79 // kubernetes-1.14.1
	k8s.io/client-go => k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible // kubernetes-1.14.1
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.0.0-20190409023720-1bc0c81fa51d // kubernetes-1.14.1
	k8s.io/code-generator => k8s.io/code-generator v0.0.0-20190311093542-50b561225d70 // kubernetes-1.14.1
	k8s.io/cri-api => k8s.io/cri-api v0.0.0-20190326082326-5c2568eea0b8 // kubernetes-1.14.0-alpha.3
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.0.0-20190409022021-00b8e31abe9d // kubernetes-1.14.1
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20190425185145-07b897206552 // commit corresponding to kube 1.14.1
	k8s.io/kubectl => k8s.io/kubectl v0.16.4 // v0.16.4
	k8s.io/kubernetes => k8s.io/kubernetes v1.14.1 // v1.14.1
	k8s.io/metrics => k8s.io/metrics v0.0.0-20190409022812-850dadb8b49c // kubernetes-1.14.1
	k8s.io/utils => k8s.io/utils v0.0.0-20190607212802-c55fbcfc754a // commit corresponding to kube 1.14.1
	sigs.k8s.io/cluster-api-provider-aws => github.com/openshift/cluster-api-provider-aws v0.2.1-0.20200316201703-923caeb1d0d8 // Pin OpenShift fork
	sigs.k8s.io/cluster-api-provider-azure => github.com/openshift/cluster-api-provider-azure v0.1.0-alpha.3.0.20200120114645-8a9592f1f87b // Pin OpenShift fork
	sigs.k8s.io/cluster-api-provider-openstack => github.com/openshift/cluster-api-provider-openstack v0.0.0-20200130125124-ef82ce374112 // Pin OpenShift fork
	sigs.k8s.io/controller-runtime => sigs.k8s.io/controller-runtime v0.2.0-beta.3 // v0.2.0-beta.3
	sigs.k8s.io/controller-tools => github.com/openshift/kubernetes-sigs-controller-tools v0.1.10-0.20190430113700-72ae52c08b9d // origin-4.1-kubernetes-1.13.4
	sigs.k8s.io/structured-merge-diff => sigs.k8s.io/structured-merge-diff v0.0.0-20190302045857-e85c7b244fd2 // commit corresponding to kube 1.14.1
)
