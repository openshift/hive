package clusterprovision

import (
	"io/ioutil"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openshift/hive/apis"
	"github.com/openshift/hive/pkg/constants"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

const (
	dnsAlreadyExistsLog       = "blahblah\naws_route53_record.api_external: [ERR]: Error building changeset: InvalidChangeBatch: [Tried to create resource record set [name='api.jh-stg-2405-2.n6b3.s1.devshift.org.'type='A'] but it already exists]\n\nblahblah"
	pendingVerificationLog    = "blahblah\naws_instance.master.2: Error launching source instance: PendingVerification: Your request for accessing resources in this region is being validated, and you will not be able to launch additional resources in this region until the validation is complete. We will notify you by email once your request has been validated. While normally resolved within minutes, please allow up to 4 hours for this process to complete. If the issue still persists, please let us know by writing to awsa\n\nblahblah"
	gcpInvalidProjectIDLog    = "blahblah\ntime=\"2020-11-13T16:05:07Z\" level=fatal msg=\"failed to fetch Master Machines: failed to load asset \"Install Config\": platform.gcp.project: Invalid value: \"o-6b20f250\": invalid project ID\nblahblah"
	gcpSSDQUotaLog            = "blahblah\ntime=\"2021-01-06T03:35:44Z\" level=error msg=\"Error: Error waiting for instance to create: Quota 'SSD_TOTAL_GB' exceeded. Limit: 500.0 in region asia-northeast2.\nblahblah"
	gcpCPUQuotaLog            = "level=fatal msg=failed to fetch Cluster: failed to fetch dependency of \"Cluster\": failed to generate asset \"Platform Quota Check\": error(MissingQuota): compute.googleapis.com/cpus is not available in us-east1 because the required number of resources (20) is more than remaining quota of 16"
	gcpServiceAccountQuotaLog = "level=fatal msg=failed to fetch Cluster: failed to fetch dependency of \"Cluster\": failed to generate asset \"Platform Quota Check\": error(MissingQuota): iam.googleapis.com/quota/service-account-count is not available in global because the required number of resources (5) is more than remaining quota of 0"
	kubeAPIWaitTimeoutLog     = "blahblah\ntime=\"2021-01-03T07:04:44Z\" level=fatal msg=\"waiting for Kubernetes API: context deadline exceeded\""
	natGatewayLimitExceeded   = "blahblah\ntime=\"2021-01-06T03:35:44Z\" level=error msg=\"Error creating NAT Gateway: NatGatewayLimitExceeded: The maximum number of NAT Gateways has been reached.\""
	vpcLimitExceeded          = "blahblah\ntime=\"2021-01-06T03:35:44Z\" level=error msg=\"Error: Error creating VPC: VpcLimitExceeded: The maximum number of VPCs has been reached.\""
	route53LimitExceeded      = "blahblah\nlevel=error msg=\"Error: error creating Route53 Hosted Zone: TooManyHostedZones: Limits Exceeded: MAX_HOSTED_ZONES_BY_OWNER - Cannot create more hosted zones.\\nlevel=error msg=\\tstatus code: 400,\""
	genericLimitExceeded      = "blahblah\ntime=\"2021-01-06T03:35:44Z\" level=error msg=\"Error: Error creating Generic: GenericLimitExceeded: The maximum number of Generics has been reached.\""
	invalidCredentials        = "blahblah\ntime=\"2021-01-06T03:35:44Z\" level=error msg=\"Error: error waiting for Route53 Hosted Zone (Z1009177L956IM4ANFHL) creation: InvalidClientTokenId: The security token included in the request is invalid.\""
	kubeAPIWaitFailedLog      = "blahblah\ntime=\"2021-01-06T03:35:44Z\" level=error msg=\"Failed waiting for Kubernetes API. This error usually happens when there is a problem on the bootstrap host that prevents creating a temporary control plane.\""
	awsDeleteRoleFailed       = "time=\"2021-09-22T12:25:40Z\" level=error msg=\"Error: Error deleting IAM Role (my-fake-cluster-hashn0s-bootstrap-role): DeleteConflict: Cannot delete entity, must detach all policies first.\""
	subnetDoesNotExist        = "blahblah\nlevel=fatal msg=\"failed to fetch Master Machines: failed to load asset \"Install Config\": [platform.aws.subnets: Invalid value: []string{\"subnet-whatever\", \"subnet-whatever2\"}: describing subnets: InvalidSubnetID.NotFound: The subnet ID 'subnet-whatever' does not exist"
	insufficientPermissions   = "level=fatal msg=failed to fetch Cluster: failed to fetch dependency of \"Cluster\": failed to generate asset \"Platform Permissions Check\": validate AWS credentials: current credentials insufficient for performing cluster installation"
	accessDeniedSLR           = "blahblah\nError: Error creating network Load Balancer: AccessDenied: User: arn:aws:sts::123456789:assumed-role/ManagedOpenShift-Installer-Role/123456789 is not authorized to perform: iam:CreateServiceLinkedRole on resource: arn:aws:iam::123456789:role/aws-service-role/elasticloadbalancing.amazonaws.com/AWSServiceRoleForElasticLoadBalancing"
	loadBalancerLimitExceeded = "blahblah\ntime=\"2021-01-06T03:35:44Z\" level=info msg=\"Cluster operator ingress Available is False with IngressUnavailable: The \"default\" ingress controller reports Available=False: IngressControllerUnavailable: One or more status	conditions indicate unavailable: LoadBalancerReady=False (SyncLoadBalancerFailed: The service-controller component is reporting SyncLoadBalancerFailed events like: Error syncing load balancer: failed to ensure load balancer: TooManyLoadBalancers: Exceeded quota of account 1234567890\n\tstatus code: 400, request id: f0cb17ec-68b6-4f32-8997-cce5049a6a1e\nThe kube-controller-manager logs may contain more details.)"
	proxyTimeoutLog           = "time=\"2021-11-17T03:56:25Z\" level=info msg=\"[pull.q1w2.quay.rhcloud.com/openshift-release-dev/ocp-release@sha256:53576e4df71a5f00f77718f25aec6ac7946eaaab998d99d3e3f03fcb403364db: error pinging docker registry pull.q1w2.quay.rhcloud.com: Get \\\"https://pull.q1w2.quay.rhcloud.com/v2/\\\": proxyconnect tcp: dial tcp 10.0.125.189:8080: i/o timeout]): quay.io/openshift-release-dev/ocp-release@sha256:53576e4df71a5f00f77718f25aec6ac7946eaaab998d99d3e3f03fcb403364db: error pinging docker registry quay.io: Get \\\"https://quay.io/v2/\\\": proxyconnect tcp: dial tcp 10.0.125.189:8080: i/o timeout"
	proxyInvalidCABundleLog   = "time=\"2021-08-27T05:56:50Z\" level=info msg=\"[pull.q1w2.quay.rhcloud.com/openshift-release-dev/ocp-release@sha256:7047acb946649cc1f54d98a1c28dd7b487fe91479aa52c13c971ea014a66c8a8: error pinging docker registry pull.q1w2.quay.rhcloud.com: Get \\\"https://pull.q1w2.quay.rhcloud.com/v2/\\\": proxyconnect tcp: x509: certificate signed by unknown authority]): quay.io/openshift-release-dev/ocp-release@sha256:7047acb946649cc1f54d98a1c28dd7b487fe91479aa52c13c971ea014a66c8a8: error pinging docker registry quay.io: Get \\\"https://quay.io/v2/\\\": proxyconnect tcp: x509: certificate signed by unknown authority"
	multilineLog              = "this is a gcp cluster\nand we ran into a quota issue here"
	noMatchLog                = "an example of something that doesn't match the log regexes"
)

func TestParseInstallLog(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	tests := []struct {
		name            string
		log             *string
		existing        []runtime.Object
		expectedReason  string
		expectedMessage *string
	}{
		{
			name:           "validate multiline regex",
			log:            pointer.StringPtr(multilineLog),
			existing:       []runtime.Object{buildRegexConfigMap()},
			expectedReason: "MultiLineWorks",
		},
		{
			name:           "load balancer service linked role prereq",
			log:            pointer.StringPtr(accessDeniedSLR),
			existing:       []runtime.Object{buildRegexConfigMap()},
			expectedReason: "AWSAccessDeniedSLR",
		},
		{
			name:           "DNS already exists",
			log:            pointer.StringPtr(dnsAlreadyExistsLog),
			existing:       []runtime.Object{buildRegexConfigMap()},
			expectedReason: "DNSAlreadyExists",
		},
		{
			name:           "PendingVerification",
			log:            pointer.StringPtr(pendingVerificationLog),
			existing:       []runtime.Object{buildRegexConfigMap()},
			expectedReason: "PendingVerification",
		},
		{
			name:           "Wildcard",
			log:            pointer.StringPtr(gcpInvalidProjectIDLog),
			existing:       []runtime.Object{buildRegexConfigMap()},
			expectedReason: "GCPInvalidProjectID",
		},
		{
			name:           "Escaped single quotes",
			log:            pointer.StringPtr(gcpSSDQUotaLog),
			existing:       []runtime.Object{buildRegexConfigMap()},
			expectedReason: "GCPQuotaSSDTotalGBExceeded",
		},
		{
			name:           "AWSNATGatewayLimitExceeded",
			log:            pointer.StringPtr(natGatewayLimitExceeded),
			existing:       []runtime.Object{buildRegexConfigMap()},
			expectedReason: "AWSNATGatewayLimitExceeded",
		},
		{
			name:           "AWSVPCLimitExceeded",
			log:            pointer.StringPtr(vpcLimitExceeded),
			existing:       []runtime.Object{buildRegexConfigMap()},
			expectedReason: "AWSVPCLimitExceeded",
		},
		{
			name:           "AWSRoute53LimitExceeded",
			log:            pointer.StringPtr(route53LimitExceeded),
			existing:       []runtime.Object{buildRegexConfigMap()},
			expectedReason: "TooManyRoute53Zones",
		},
		{
			name:           "Generic ResourceLimitExceeded",
			log:            pointer.StringPtr(genericLimitExceeded),
			existing:       []runtime.Object{buildRegexConfigMap()},
			expectedReason: "FallbackResourceLimitExceeded",
		},
		{
			name:           "Credentials are invalid",
			log:            pointer.StringPtr(invalidCredentials),
			existing:       []runtime.Object{buildRegexConfigMap()},
			expectedReason: "InvalidCredentials",
		},
		{
			name:           "Failed waiting for Kubernetes API",
			log:            pointer.StringPtr(kubeAPIWaitFailedLog),
			existing:       []runtime.Object{buildRegexConfigMap()},
			expectedReason: "KubeAPIWaitFailed",
		},
		{
			name:           "ProxyTimeout",
			log:            pointer.StringPtr(proxyTimeoutLog),
			existing:       []runtime.Object{buildRegexConfigMap()},
			expectedReason: "ProxyTimeout",
		},
		{
			name:           "ProxyInvalidCABundle",
			log:            pointer.StringPtr(proxyInvalidCABundleLog),
			existing:       []runtime.Object{buildRegexConfigMap()},
			expectedReason: "ProxyInvalidCABundle",
		},
		{
			name: "KubeAPIWaitTimeout from additional regex entries",
			log:  pointer.StringPtr(kubeAPIWaitTimeoutLog),
			existing: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      regexConfigMapName,
						Namespace: constants.DefaultHiveNamespace,
					},
					Data: map[string]string{
						"regexes": `
- name: DNSAlreadyExists
  searchRegexStrings:
  - "aws_route53_record.*Error building changeset:.*Tried to create resource record set.*but it already exists"
  installFailingReason: DNSAlreadyExists
  installFailingMessage: DNS record already exists
`,
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      additionalRegexConfigMapName,
						Namespace: constants.DefaultHiveNamespace,
					},
					Data: map[string]string{
						"regexes": `
- name: KubeAPIWaitTimeout
  searchRegexStrings:
  - "waiting for Kubernetes API: context deadline exceeded"
  installFailingReason: KubeAPIWaitTimeout
  installFailingMessage: Timeout waiting for the Kubernetes API to begin responding
`,
					},
				},
			},
			expectedReason: "KubeAPIWaitTimeout",
		},
		{
			name: "regexes take precedence over additionalRegexes",
			log:  pointer.StringPtr(kubeAPIWaitTimeoutLog),
			existing: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      regexConfigMapName,
						Namespace: constants.DefaultHiveNamespace,
					},
					Data: map[string]string{
						"regexes": `
- name: KubeAPIWaitTimeout
  searchRegexStrings:
  - "waiting for Kubernetes API: context deadline exceeded"
  installFailingReason: KubeAPIWaitTimeoutRegexes
  installFailingMessage: Timeout waiting for the Kubernetes API to begin responding
`,
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      additionalRegexConfigMapName,
						Namespace: constants.DefaultHiveNamespace,
					},
					Data: map[string]string{
						"regexes": `
- name: KubeAPIWaitTimeout
  searchRegexStrings:
  - "waiting for Kubernetes API: context deadline exceeded"
  installFailingReason: KubeAPIWaitTimeoutAdditional
  installFailingMessage: Timeout waiting for the Kubernetes API to begin responding
`,
					},
				},
			},
			expectedReason: "KubeAPIWaitTimeoutRegexes",
		},
		{
			name:           "no log",
			existing:       []runtime.Object{buildRegexConfigMap()},
			expectedReason: unknownReason,
		},
		{
			name:            "no matching log",
			log:             pointer.StringPtr(noMatchLog),
			existing:        []runtime.Object{buildRegexConfigMap()},
			expectedReason:  unknownReason,
			expectedMessage: pointer.StringPtr(noMatchLog),
		},
		{
			name:           "missing regex configmap",
			log:            pointer.StringPtr(dnsAlreadyExistsLog),
			expectedReason: unknownReason,
		},
		{
			name: "missing regexes data entry",
			log:  pointer.StringPtr(dnsAlreadyExistsLog),
			existing: []runtime.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      regexConfigMapName,
					Namespace: constants.DefaultHiveNamespace,
				},
			}},
			expectedReason: unknownReason,
		},
		{
			name: "malformed regex",
			log:  pointer.StringPtr(dnsAlreadyExistsLog),
			existing: []runtime.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      regexConfigMapName,
					Namespace: constants.DefaultHiveNamespace,
				},
				BinaryData: map[string][]byte{
					"regexes": []byte("malformed"),
				},
			}},
			expectedReason: unknownReason,
		},
		{
			name: "skip bad regex entry",
			log:  pointer.StringPtr(dnsAlreadyExistsLog),
			existing: []runtime.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      regexConfigMapName,
					Namespace: constants.DefaultHiveNamespace,
				},
				Data: map[string]string{
					"regexes": `
- name: BadEntry
  searchRegexStrings:
  - "*"
  installFailingReason: BadEntry
  installFailingMessage: Bad entry
- name: DNSAlreadyExists
  searchRegexStrings:
  - "aws_route53_record.*Error building changeset:.*Tried to create resource record set.*but it already exists"
  installFailingReason: DNSAlreadyExists
  installFailingMessage: DNS record already exists
`,
				},
			}},
			expectedReason: "DNSAlreadyExists",
		},
		{
			name: "skip bad search string",
			log:  pointer.StringPtr(dnsAlreadyExistsLog),
			existing: []runtime.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      regexConfigMapName,
					Namespace: constants.DefaultHiveNamespace,
				},
				Data: map[string]string{
					"regexes": `
- name: DNSAlreadyExists
  searchRegexStrings:
  - "*"
  - "aws_route53_record.*Error building changeset:.*Tried to create resource record set.*but it already exists"
  installFailingReason: DNSAlreadyExists
  installFailingMessage: DNS record already exists
`,
				},
			}},
			expectedReason: "DNSAlreadyExists",
		},
		{
			name:           "GCP compute quota",
			log:            pointer.StringPtr(gcpCPUQuotaLog),
			existing:       []runtime.Object{buildRegexConfigMap()},
			expectedReason: "GCPComputeQuotaExceeded",
		},
		{
			name:           "GCP service account quota",
			log:            pointer.StringPtr(gcpServiceAccountQuotaLog),
			existing:       []runtime.Object{buildRegexConfigMap()},
			expectedReason: "GCPServiceAccountQuotaExceeded",
		},
		{
			name: "Can't delete IAM role",
			log:  pointer.StringPtr(awsDeleteRoleFailed),
			existing: []runtime.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      regexConfigMapName,
					Namespace: constants.DefaultHiveNamespace,
				},
				Data: map[string]string{
					"regexes": `
    - name: ErrorDeletingIAMRole
      searchRegexStrings:
        - "Error deleting IAM Role .* DeleteConflict: Cannot delete entity, must detach all policies first."
      installFailingReason: ErrorDeletingIAMRole
      installFailingMessage: The cluster installer was not able to delete the roles it used during the installation. Ensure that no policies are added to new roles by default and try again.
`,
				},
			}},
			expectedReason: "ErrorDeletingIAMRole",
		},
		{
			name: "AWSSubnetDoesNotExist",
			log:  pointer.StringPtr(subnetDoesNotExist),
			existing: []runtime.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      regexConfigMapName,
					Namespace: constants.DefaultHiveNamespace,
				},
				Data: map[string]string{
					"regexes": `
- name: AWSSubnetDoesNotExist
  searchRegexStrings:
  - "The subnet ID .* does not exist"
  installFailingReason: AWSSubnetDoesNotExist
  installFailingMessage: AWS Subnet Does Not Exist
`,
				},
			}},
			expectedReason: "AWSSubnetDoesNotExist",
		},
		{
			name: "AWSInsufficientPermissions",
			log:  pointer.StringPtr(insufficientPermissions),
			existing: []runtime.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      regexConfigMapName,
					Namespace: constants.DefaultHiveNamespace,
				},
				Data: map[string]string{
					"regexes": `
- name: InsufficientPermissions
  searchRegexStrings:
  - "current credentials insufficient for performing cluster installation"
  installFailingReason: AWSInsufficientPermissions
  installFailingMessage: AWS credentials are insufficient for performing cluster installation
`,
				},
			}},
			expectedReason: "AWSInsufficientPermissions",
		},
		{
			name:           "LoadBalancerLimitExceeded",
			log:            pointer.StringPtr(loadBalancerLimitExceeded),
			existing:       []runtime.Object{buildRegexConfigMap()},
			expectedReason: "LoadBalancerLimitExceeded",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClient(test.existing...)
			r := &ReconcileClusterProvision{
				Client: fakeClient,
				scheme: scheme.Scheme,
			}
			reason, message := r.parseInstallLog(test.log, log.WithFields(log.Fields{}))
			assert.Equal(t, test.expectedReason, reason, "unexpected reason")
			if test.expectedMessage != nil {
				assert.Equal(t, *test.expectedMessage, message)
			} else {
				assert.NotEmpty(t, message, "expected message to be not empty")
			}
		})
	}
}

// buildRegexConfigMap reads the install log regexes configmap from within config/configmaps/install-log-regexes-configmap.yaml
func buildRegexConfigMap() runtime.Object {
	decode := serializer.NewCodecFactory(scheme.Scheme).UniversalDeserializer().Decode
	stream, err := ioutil.ReadFile("../../../config/configmaps/install-log-regexes-configmap.yaml")
	if err != nil {
		log.Fatal(err)
	}
	obj, _, err := decode(stream, nil, nil)
	if err != nil {
		log.Fatal(err)
	}
	return obj
}
