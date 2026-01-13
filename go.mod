module github.com/openshift/hive

go 1.24.0

toolchain go1.24.6

require (
	github.com/Azure/azure-sdk-for-go v68.0.0+incompatible
	github.com/Azure/go-autorest/autorest v0.11.30
	github.com/Azure/go-autorest/autorest/azure/auth v0.5.13
	github.com/Azure/go-autorest/autorest/to v0.4.0
	github.com/IBM/go-sdk-core/v5 v5.21.0
	github.com/IBM/networking-go-sdk v0.51.11
	github.com/IBM/platform-services-go-sdk v0.86.1
	github.com/IBM/vpc-go-sdk v0.71.1
	github.com/aws/aws-sdk-go v1.55.7 // indirect
	github.com/blang/semver/v4 v4.0.0
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc
	github.com/davegardnerisme/deephash v0.0.0-20210406090112-6d072427d830
	github.com/evanphx/json-patch v5.9.11+incompatible
	github.com/ghodss/yaml v1.0.1-0.20190212211648-25d852aebe32
	github.com/go-bindata/go-bindata v3.1.2+incompatible
	github.com/go-logr/logr v1.4.3
	github.com/golang/mock v1.7.0-rc.1
	github.com/golangci/golangci-lint v1.64.8
	github.com/google/go-cmp v0.7.0
	github.com/google/uuid v1.6.0
	github.com/heptio/velero v1.0.0
	github.com/jonboulle/clockwork v0.5.0
	github.com/json-iterator/go v1.1.12
	github.com/miekg/dns v1.1.35
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.38.2
	github.com/openshift/api v0.0.0-20251120220512-cb382c9eaf42
	github.com/openshift/build-machinery-go v0.0.0-20250530140348-dc5b2804eeee
	github.com/openshift/cluster-autoscaler-operator v0.0.1-0.20250219201631-227f7537c3b4
	github.com/openshift/generic-admission-server v1.14.1-0.20250715141119-66c5d0a9c5e6
	github.com/openshift/hive/apis v0.0.0
	github.com/openshift/installer v1.4.21-pre2.0.20260112230456-1c2444827f23
	github.com/openshift/library-go v0.0.0-20251107090138-0de9712313a5
	github.com/openshift/machine-api-operator v0.2.1-0.20251128002018-85c00c0d525f
	github.com/openshift/machine-api-provider-gcp v0.0.1-0.20241021180644-0eca0846914a
	github.com/openshift/machine-api-provider-ibmcloud v0.0.0-20231207164151-6b0b8ea7b16d
	github.com/pkg/errors v0.9.1
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.74.0
	github.com/prometheus/client_golang v1.23.2
	github.com/prometheus/client_model v0.6.2
	github.com/sirupsen/logrus v1.9.3
	github.com/spf13/cobra v1.10.1
	github.com/spf13/pflag v1.0.10
	github.com/stretchr/testify v1.11.1
	github.com/tidwall/gjson v1.18.0
	github.com/vmware/govmomi v0.52.0
	golang.org/x/crypto v0.45.0
	golang.org/x/lint v0.0.0-20210508222113-6edffad5e616
	golang.org/x/mod v0.30.0
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/oauth2 v0.32.0
	golang.org/x/time v0.14.0
	google.golang.org/api v0.252.0
	gopkg.in/ini.v1 v1.67.0
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.34.2
	k8s.io/apiextensions-apiserver v0.34.2
	k8s.io/apimachinery v0.34.2
	k8s.io/cli-runtime v0.34.1
	k8s.io/client-go v0.34.2
	k8s.io/cluster-registry v0.0.6
	k8s.io/code-generator v0.34.2
	k8s.io/klog v1.0.0
	k8s.io/kube-aggregator v0.34.1
	k8s.io/kubectl v0.34.1
	k8s.io/utils v0.0.0-20251002143259-bc988d571ff4
	sigs.k8s.io/cluster-api-provider-azure v1.21.1-0.20250929163617-2c4eaa611a39
	sigs.k8s.io/controller-runtime v0.22.3
	sigs.k8s.io/controller-tools v0.19.0
	sigs.k8s.io/yaml v1.6.0
)

require (
	4d63.com/gocheckcompilerdirectives v1.3.0 // indirect
	github.com/4meepo/tagalign v1.4.2 // indirect
	github.com/Abirdcfly/dupword v0.1.3 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph v0.8.2 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources v1.2.0 // indirect
	github.com/GaijinEntertainment/go-exhaustruct/v3 v3.3.1 // indirect
	github.com/IBM-Cloud/power-go-client v1.12.0 // indirect
	github.com/IBM/keyprotect-go-client v0.12.2 // indirect
	github.com/OpenPeeDeeP/depguard/v2 v2.2.1 // indirect
	github.com/alexkohler/nakedret/v2 v2.0.5 // indirect
	github.com/alingse/asasalint v0.0.11 // indirect
	github.com/blang/semver v3.5.1+incompatible // indirect
	github.com/breml/errchkjson v0.4.0 // indirect
	github.com/butuzov/mirror v1.3.0 // indirect
	github.com/ccojocar/zxcvbn-go v1.0.2 // indirect
	github.com/curioswitch/go-reassign v0.3.0 // indirect
	github.com/denis-tingaikin/go-header v0.5.0 // indirect
	github.com/firefart/nonamedreturns v1.0.5 // indirect
	github.com/form3tech-oss/jwt-go v3.2.3+incompatible // indirect
	github.com/go-openapi/analysis v0.23.0 // indirect
	github.com/go-openapi/loads v0.22.0 // indirect
	github.com/go-openapi/runtime v0.28.0 // indirect
	github.com/go-openapi/spec v0.21.0 // indirect
	github.com/go-openapi/validate v0.24.0 // indirect
	github.com/go-test/deep v1.1.1 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.0 // indirect
	github.com/hashicorp/go-version v1.7.0 // indirect
	github.com/hexops/gotextdiff v1.0.3 // indirect
	github.com/kkHAIKE/contextcheck v1.1.6 // indirect
	github.com/leonklingele/grouper v1.1.2 // indirect
	github.com/maratori/testableexamples v1.0.0 // indirect
	github.com/nunnatsa/ginkgolinter v0.19.1 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/sashamelentyev/interfacebloat v1.1.0 // indirect
	github.com/sashamelentyev/usestdlibvars v1.28.0 // indirect
	github.com/sivchari/containedctx v1.0.3 // indirect
	github.com/stbenjam/no-sprintf-host-port v0.2.0 // indirect
	github.com/timonwong/loggercheck v0.10.1 // indirect
	github.com/xen0n/gosmopolitan v1.2.2 // indirect
	github.com/yagipy/maintidx v1.0.0 // indirect
	github.com/ykadowak/zerologlint v0.1.5 // indirect
	gitlab.com/bosi/decorder v0.4.2 // indirect
	golang.org/x/exp v0.0.0-20250305212735-054e65f0b394 // indirect
	sigs.k8s.io/cluster-api v1.11.3 // indirect
	sigs.k8s.io/cluster-api-provider-aws/v2 v2.9.1-0.20251121222432-04a4f62bf90a // indirect
	sigs.k8s.io/cluster-api-provider-gcp v1.10.1-0.20251030112604-5c2bfd5aadb3 // indirect
)

require (
	github.com/aws/aws-sdk-go-v2 v1.39.2
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.1 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.31.12
	github.com/aws/aws-sdk-go-v2/credentials v1.18.16
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.9 // indirect
	github.com/aws/aws-sdk-go-v2/feature/s3/manager v1.19.12
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.9 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.9 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.3 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.233.0
	github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2 v1.49.0
	github.com/aws/aws-sdk-go-v2/service/iam v1.32.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi v1.29.0
	github.com/aws/aws-sdk-go-v2/service/route53 v1.48.6
	github.com/aws/aws-sdk-go-v2/service/s3 v1.88.4
	github.com/aws/aws-sdk-go-v2/service/sso v1.29.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.35.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.38.6
	github.com/aws/smithy-go v1.23.0
	github.com/gophercloud/utils/v2 v2.0.0-20250212084022-725b94822eeb
	gopkg.in/evanphx/json-patch.v4 v4.12.0
)

require (
	4d63.com/gochecknoglobals v0.2.2 // indirect
	cel.dev/expr v0.24.0 // indirect
	cloud.google.com/go v0.121.6 // indirect
	cloud.google.com/go/auth v0.17.0 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	cloud.google.com/go/iam v1.5.3 // indirect
	cloud.google.com/go/kms v1.22.0 // indirect
	cloud.google.com/go/longrunning v0.6.7 // indirect
	cloud.google.com/go/monitoring v1.24.2 // indirect
	cloud.google.com/go/storage v1.57.0 // indirect
	github.com/AlecAivazis/survey/v2 v2.3.5 // indirect
	github.com/Antonboom/errname v1.0.0 // indirect
	github.com/Antonboom/testifylint v1.5.2 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5 v5.7.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork v1.1.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v2 v2.2.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage v1.8.1 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20230124172434-306776ec8161 // indirect
	github.com/Azure/go-autorest v14.2.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest/adal v0.9.24 // indirect
	github.com/Azure/go-autorest/autorest/azure/cli v0.4.6 // indirect
	github.com/Azure/go-autorest/autorest/date v0.3.0 // indirect
	github.com/Azure/go-autorest/autorest/validation v0.3.1 // indirect
	github.com/Azure/go-autorest/logger v0.2.1 // indirect
	github.com/Azure/go-autorest/tracing v0.6.0 // indirect
	github.com/BurntSushi/toml v1.4.1-0.20240526193622-a339e1f7089c // indirect
	github.com/Crocmagnon/fatcontext v0.7.1 // indirect
	github.com/Djarvur/go-err113 v0.1.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.29.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric v0.53.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/internal/resourcemapping v0.53.0 // indirect
	github.com/IBM-Cloud/bluemix-go v0.0.0-20211102075456-ffc4e11dfb16 // indirect
	github.com/IBM/ibm-cos-sdk-go v1.12.3 // indirect
	github.com/MakeNowJust/heredoc v1.0.0 // indirect
	github.com/Masterminds/semver/v3 v3.4.0 // indirect
	github.com/NYTimes/gziphandler v1.1.1 // indirect
	github.com/PaesslerAG/gval v1.0.0 // indirect
	github.com/PaesslerAG/jsonpath v0.1.1 // indirect
	github.com/alecthomas/go-check-sumtype v0.3.1 // indirect
	github.com/alexkohler/prealloc v1.0.0 // indirect
	github.com/alingse/nilnesserr v0.1.2 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/apparentlymart/go-cidr v1.1.0 // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/asaskevich/govalidator/v11 v11.0.2-0.20250122183457-e11347878e23 // indirect
	github.com/ashanbrown/forbidigo v1.6.0 // indirect
	github.com/ashanbrown/makezero v1.2.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing v1.32.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/servicequotas v1.31.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bkielbasa/cyclop v1.2.3 // indirect
	github.com/bombsimon/wsl/v4 v4.5.0 // indirect
	github.com/catenacyber/perfsprint v0.8.2 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/chai2010/gettext-go v1.0.2 // indirect
	github.com/charithe/durationcheck v0.0.10 // indirect
	github.com/chavacava/garif v0.1.0 // indirect
	github.com/ckaznocha/intrange v0.3.0 // indirect
	github.com/cncf/xds/go v0.0.0-20250501225837-2ac532fd4443 // indirect
	github.com/containers/image/v5 v5.31.0 // indirect
	github.com/containers/storage v1.54.0 // indirect
	github.com/coreos/go-semver v0.3.1 // indirect
	github.com/coreos/go-systemd/v22 v22.6.0 // indirect
	github.com/coreos/ignition/v2 v2.24.0 // indirect
	github.com/coreos/stream-metadata-go v0.4.10-0.20250806142651-4a7d280a6c7b // indirect
	github.com/coreos/vcontext v0.0.0-20230201181013-d72178a18687 // indirect
	github.com/daixiang0/gci v0.13.7 // indirect
	github.com/digitalocean/go-libvirt v0.0.0-20240220204746-fcabe97a6eed // indirect
	github.com/dimchansky/utfbom v1.1.1 // indirect
	github.com/envoyproxy/go-control-plane/envoy v1.32.4 // indirect
	github.com/envoyproxy/protoc-gen-validate v1.2.1 // indirect
	github.com/ettle/strcase v0.2.0 // indirect
	github.com/exponent-io/jsonpath v0.0.0-20210407135951-1de76d718b3f // indirect
	github.com/fatih/camelcase v1.0.0 // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/fatih/structtag v1.2.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.0 // indirect
	github.com/fzipp/gocyclo v0.6.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.9 // indirect
	github.com/ghostiam/protogetter v0.3.9 // indirect
	github.com/go-critic/go-critic v0.12.0 // indirect
	github.com/go-errors/errors v1.4.2 // indirect
	github.com/go-jose/go-jose/v4 v4.1.2 // indirect
	github.com/go-openapi/errors v0.22.1 // indirect
	github.com/go-openapi/jsonpointer v0.22.3 // indirect
	github.com/go-openapi/jsonreference v0.21.3 // indirect
	github.com/go-openapi/strfmt v0.23.0 // indirect
	github.com/go-openapi/swag v0.25.3 // indirect
	github.com/go-openapi/swag/cmdutils v0.25.3 // indirect
	github.com/go-openapi/swag/conv v0.25.3 // indirect
	github.com/go-openapi/swag/fileutils v0.25.3 // indirect
	github.com/go-openapi/swag/jsonname v0.25.3 // indirect
	github.com/go-openapi/swag/jsonutils v0.25.3 // indirect
	github.com/go-openapi/swag/loading v0.25.3 // indirect
	github.com/go-openapi/swag/mangling v0.25.3 // indirect
	github.com/go-openapi/swag/netutils v0.25.3 // indirect
	github.com/go-openapi/swag/stringutils v0.25.3 // indirect
	github.com/go-openapi/swag/typeutils v0.25.3 // indirect
	github.com/go-openapi/swag/yamlutils v0.25.3 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-toolsmith/astcast v1.1.0 // indirect
	github.com/go-toolsmith/astcopy v1.1.0 // indirect
	github.com/go-toolsmith/astequal v1.2.0 // indirect
	github.com/go-toolsmith/astfmt v1.1.0 // indirect
	github.com/go-toolsmith/astp v1.1.0 // indirect
	github.com/go-toolsmith/strparse v1.1.0 // indirect
	github.com/go-toolsmith/typep v1.1.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/go-xmlfmt/xmlfmt v1.1.3 // indirect
	github.com/gobuffalo/flect v1.0.3 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/gofrs/flock v0.12.1 // indirect
	github.com/gofrs/uuid/v5 v5.3.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/golangci/dupl v0.0.0-20250308024227-f665c8d69b32 // indirect
	github.com/golangci/go-printf-func-name v0.1.0 // indirect
	github.com/golangci/gofmt v0.0.0-20250106114630-d62b90e6713d // indirect
	github.com/golangci/misspell v0.6.0 // indirect
	github.com/golangci/plugin-module-register v0.1.1 // indirect
	github.com/golangci/revgrep v0.8.0 // indirect
	github.com/golangci/unconvert v0.0.0-20240309020433-c5143eacb3ed // indirect
	github.com/google/btree v1.1.3 // indirect
	github.com/googleapis/gax-go/v2 v2.15.0 // indirect
	github.com/gophercloud/gophercloud/v2 v2.8.0 // indirect
	github.com/gordonklaus/ineffassign v0.1.0 // indirect
	github.com/gostaticanalysis/analysisutil v0.7.1 // indirect
	github.com/gostaticanalysis/comment v1.5.0 // indirect
	github.com/gostaticanalysis/forcetypeassert v0.2.0 // indirect
	github.com/gostaticanalysis/nilerr v0.1.1 // indirect
	github.com/gregjones/httpcache v0.0.0-20190611155906-901d90724c79 // indirect
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-immutable-radix/v2 v2.1.0 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.8 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jgautheron/goconst v1.7.1 // indirect
	github.com/jingyugao/rowserrcheck v1.1.1 // indirect
	github.com/jjti/go-spancheck v0.6.4 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/julz/importas v0.2.0 // indirect
	github.com/karamaru-alpha/copyloopvar v1.2.1 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/kdomanski/iso9660 v0.2.1 // indirect
	github.com/kisielk/errcheck v1.9.0 // indirect
	github.com/kulti/thelper v0.6.3 // indirect
	github.com/kunwardeep/paralleltest v1.0.10 // indirect
	github.com/lasiar/canonicalheader v1.1.2 // indirect
	github.com/ldez/exptostd v0.4.2 // indirect
	github.com/ldez/gomoddirectives v0.6.1 // indirect
	github.com/ldez/grignotin v0.9.0 // indirect
	github.com/ldez/tagliatelle v0.7.1 // indirect
	github.com/ldez/usetesting v0.4.2 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/liggitt/tabwriter v0.0.0-20181228230101-89fcab3d43de // indirect
	github.com/macabu/inamedparam v0.1.3 // indirect
	github.com/maratori/testpackage v1.1.1 // indirect
	github.com/matoous/godox v1.1.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/metal3-io/baremetal-operator/apis v0.4.0 // indirect
	github.com/metal3-io/baremetal-operator/pkg/hardwareutils v0.4.0 // indirect
	github.com/mgechev/revive v1.7.0 // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/microsoft/kiota-serialization-form-go v0.9.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/moby/spdystream v0.5.0 // indirect
	github.com/moby/term v0.5.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/monochromegane/go-gitignore v0.0.0-20200626010858-205db1a8cc00 // indirect
	github.com/moricho/tparallel v0.3.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/nakabonne/nestif v0.3.1 // indirect
	github.com/nishanths/exhaustive v0.12.0 // indirect
	github.com/nishanths/predeclared v0.2.2 // indirect
	github.com/nutanix-cloud-native/cluster-api-provider-nutanix v1.7.0 // indirect
	github.com/nutanix-cloud-native/prism-go-client v0.5.0 // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/olekukonko/tablewriter v0.0.5 // indirect
	github.com/openshift/client-go v0.0.0-20251015124057-db0dee36e235 // indirect
	github.com/openshift/cloud-credential-operator v0.0.0-20240404165937-5e8812d64187 // indirect
	github.com/ovirt/go-ovirt v0.0.0-20210809163552-d4276e35d3db // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/polyfloyd/go-errorlint v1.7.1 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.17.0 // indirect
	github.com/quasilyte/go-ruleguard v0.4.3-0.20240823090925-0fe6f58b47b1 // indirect
	github.com/quasilyte/go-ruleguard/dsl v0.3.22 // indirect
	github.com/quasilyte/regex/syntax v0.0.0-20210819130434-b3f0c404a727 // indirect
	github.com/raeperd/recvcheck v0.2.0 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/ryancurrah/gomodguard v1.3.5 // indirect
	github.com/ryanrolds/sqlclosecheck v0.5.1 // indirect
	github.com/sagikazarmark/locafero v0.7.0 // indirect
	github.com/sanposhiho/wastedassign/v2 v2.1.0 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.1 // indirect
	github.com/securego/gosec/v2 v2.22.2 // indirect
	github.com/shurcooL/httpfs v0.0.0-20190707220628-8d4bc4ba7749 // indirect
	github.com/shurcooL/vfsgen v0.0.0-20181202132449-6a9ea43bcacd // indirect
	github.com/sonatard/noctx v0.1.0 // indirect
	github.com/sourcegraph/conc v0.3.0 // indirect
	github.com/sourcegraph/go-diff v0.7.0 // indirect
	github.com/spf13/afero v1.12.0 // indirect
	github.com/spf13/cast v1.7.1 // indirect
	github.com/spf13/viper v1.20.1 // indirect
	github.com/spiffe/go-spiffe/v2 v2.5.0 // indirect
	github.com/ssgreg/nlreturn/v2 v2.2.1 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/tdakkota/asciicheck v0.4.1 // indirect
	github.com/tetafro/godot v1.5.0 // indirect
	github.com/timakin/bodyclose v0.0.0-20241017074812-ed6a65f985e3 // indirect
	github.com/tomarrell/wrapcheck/v2 v2.10.0 // indirect
	github.com/tommy-muehle/go-mnd/v2 v2.5.1 // indirect
	github.com/ultraware/funlen v0.2.0 // indirect
	github.com/ultraware/whitespace v0.2.0 // indirect
	github.com/uudashr/gocognit v1.2.0 // indirect
	github.com/uudashr/iface v1.3.1 // indirect
	github.com/vincent-petithory/dataurl v1.0.0 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/xlab/treeprint v1.2.0 // indirect
	github.com/yeya24/promlinter v0.3.0 // indirect
	github.com/zeebo/errs v1.4.0 // indirect
	go-simpler.org/musttag v0.13.0 // indirect
	go-simpler.org/sloglint v0.9.0 // indirect
	go.etcd.io/etcd/api/v3 v3.6.4 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.6.4 // indirect
	go.etcd.io/etcd/client/v3 v3.6.4 // indirect
	go.mongodb.org/mongo-driver v1.17.3 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/contrib/detectors/gcp v1.36.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.61.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.61.0 // indirect
	go.opentelemetry.io/otel v1.38.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.38.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.38.0 // indirect
	go.opentelemetry.io/otel/metric v1.38.0 // indirect
	go.opentelemetry.io/otel/sdk v1.38.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.38.0 // indirect
	go.opentelemetry.io/otel/trace v1.38.0 // indirect
	go.opentelemetry.io/proto/otlp v1.7.1 // indirect
	go.uber.org/automaxprocs v1.6.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.1 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/sync v0.18.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/term v0.37.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	golang.org/x/tools v0.39.0 // indirect
	gomodules.xyz/jsonpatch/v2 v2.5.0 // indirect
	google.golang.org/genproto v0.0.0-20250603155806-513f23925822 // indirect
	google.golang.org/grpc v1.76.0 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
	gopkg.in/gcfg.v1 v1.2.3 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	honnef.co/go/tools v0.6.1 // indirect
	k8s.io/apiserver v0.34.2 // indirect
	k8s.io/cloud-provider-vsphere v1.33.3 // indirect
	k8s.io/component-base v0.34.2 // indirect
	k8s.io/component-helpers v0.34.1 // indirect
	k8s.io/gengo/v2 v2.0.0-20250922181213-ec3ebc5fd46b // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/kube-openapi v0.0.0-20251121143641-b6aabc6c6745 // indirect
	mvdan.cc/gofumpt v0.7.0 // indirect
	mvdan.cc/unparam v0.0.0-20240528143540-8a5130ca722f // indirect
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.32.0 // indirect
	sigs.k8s.io/cluster-api-provider-ibmcloud v0.12.0 // indirect
	sigs.k8s.io/cluster-api-provider-openstack v0.13.0 // indirect
	sigs.k8s.io/cluster-api-provider-vsphere v1.14.0 // indirect
	sigs.k8s.io/json v0.0.0-20250730193827-2d320260d730 // indirect
	sigs.k8s.io/kube-storage-version-migrator v0.0.6-0.20230721195810-5c8923c5ff96 // indirect
	sigs.k8s.io/kustomize/api v0.20.1 // indirect
	sigs.k8s.io/kustomize/kyaml v0.20.1 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v6 v6.3.1 // indirect
)

require (
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	github.com/Antonboom/nilnil v1.0.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.19.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.13.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.11.2 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.5.0 // indirect
	github.com/blizzy78/varnamelen v0.8.0 // indirect
	github.com/breml/bidichk v0.3.2 // indirect
	github.com/butuzov/ireturn v0.3.1 // indirect
	github.com/cjlapao/common-go v0.0.39 // indirect
	github.com/emicklei/go-restful/v3 v3.12.2 // indirect
	github.com/evanphx/json-patch/v5 v5.9.11 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-logr/zapr v1.3.0 // indirect
	github.com/go-playground/validator/v10 v10.26.0 // indirect
	github.com/golang-jwt/jwt/v4 v4.5.2 // indirect
	github.com/google/cel-go v0.26.0 // indirect
	github.com/google/gnostic-models v0.7.1 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.6 // indirect
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.2 // indirect
	github.com/jongio/azidext/go/azidext v0.5.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/microsoft/kiota-abstractions-go v0.18.0 // indirect
	github.com/microsoft/kiota-authentication-azure-go v0.6.0 // indirect
	github.com/microsoft/kiota-http-go v0.16.0 // indirect
	github.com/microsoft/kiota-serialization-json-go v0.9.0 // indirect
	github.com/microsoft/kiota-serialization-text-go v0.7.0 // indirect
	github.com/microsoftgraph/msgraph-sdk-go v0.59.0 // indirect
	github.com/microsoftgraph/msgraph-sdk-go-core v0.34.1 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/openshift/cluster-control-plane-machine-set-operator v0.0.0-20251029084908-344babe6a957
	github.com/pelletier/go-toml/v2 v2.2.3 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/quasilyte/gogrep v0.5.0 // indirect
	github.com/quasilyte/stdinfo v0.0.0-20220114132959-f7386bf02567 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sivchari/tenv v1.12.1 // indirect
	github.com/stoewer/go-strcase v1.3.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/exp/typeparams v0.0.0-20250210185358-939b2ce775ac // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250825161204-c5933d9347a5 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251002232023-7c0ddcbb5797 // indirect
	k8s.io/kms v0.34.2 // indirect
)

// sub modules
replace github.com/openshift/hive/apis => ./apis

// from installer
replace (
	github.com/metal3-io/baremetal-operator => github.com/openshift/baremetal-operator v0.0.0-20231128154154-6736c9b9c6c8
	github.com/metal3-io/baremetal-operator/apis => github.com/openshift/baremetal-operator/apis v0.0.0-20231128154154-6736c9b9c6c8
	github.com/metal3-io/baremetal-operator/pkg/hardwareutils => github.com/openshift/baremetal-operator/pkg/hardwareutils v0.0.0-20231128154154-6736c9b9c6c8
	k8s.io/cloud-provider-vsphere => github.com/openshift/cloud-provider-vsphere v1.19.1-0.20240626105621-6464d0bb4928
)

// CVE-2022-1996 (only affects go.sum; zero actual code is vendored in from this lib)
replace github.com/emicklei/go-restful v2.15.0+incompatible => github.com/emicklei/go-restful v2.16.0+incompatible

// needed for fixing CVE-2018-1099
exclude (
	go.etcd.io/etcd v0.0.0-20191023171146-3cf2f69b5738
	go.etcd.io/etcd v0.5.0-alpha.5.0.20200910180754-dd1b699fc489
)

// CVE-2025-47914: (only affects go.sum; zero actual code is vendored in from this lib)
replace golang.org/x/crypto/ssh/agent => golang.org/x/crypto/ssh/agent v0.45.0

// CVE-2025-30204: Some transitive deps are still using older v4. Safe to remove once go.sum shows only 4.5.2 or higher.
replace github.com/golang-jwt/jwt/v4 => github.com/golang-jwt/jwt/v4 v4.5.2

// CVE-2025-22872: Some transitive deps are still using older versions. Safe to remove once go.sum shows only 0.38.0 or higher.
replace golang.org/x/net => golang.org/x/net v0.38.0

// From installer: This is to force capi back for the older provider version
replace sigs.k8s.io/controller-runtime => sigs.k8s.io/controller-runtime v0.19.3

// From installer:
// Temporary vendoring of upstream providers while they work to update to capi v1.11, which contains breaking changes
// See: https://issues.redhat.com/browse/CORS-4262
replace (
	github.com/nutanix-cloud-native/cluster-api-provider-nutanix => github.com/nutanix-cloud-native/cluster-api-provider-nutanix v1.7.2-0.20251007022949-442bc2ebe286
	sigs.k8s.io/cluster-api-provider-azure => github.com/mboersma/cluster-api-provider-azure v0.3.1-0.20251030205607-3161b9cc8d3e
)

// Pinned for installer:
replace github.com/vmware/govmomi => github.com/vmware/govmomi v0.51.0
