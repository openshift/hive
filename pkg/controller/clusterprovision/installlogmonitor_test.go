package clusterprovision

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

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
	dnsAlreadyExistsLog     = "blahblah\naws_route53_record.api_external: [ERR]: Error building changeset: InvalidChangeBatch: [Tried to create resource record set [name='api.jh-stg-2405-2.n6b3.s1.devshift.org.'type='A'] but it already exists]\n\nblahblah"
	pendingVerificationLog  = "blahblah\naws_instance.master.2: Error launching source instance: PendingVerification: Your request for accessing resources in this region is being validated, and you will not be able to launch additional resources in this region until the validation is complete. We will notify you by email once your request has been validated. While normally resolved within minutes, please allow up to 4 hours for this process to complete. If the issue still persists, please let us know by writing to awsa\n\nblahblah"
	gcpInvalidProjectIDLog  = "blahblah\ntime=\"2020-11-13T16:05:07Z\" level=fatal msg=\"failed to fetch Master Machines: failed to load asset \"Install Config\": platform.gcp.project: Invalid value: \"o-6b20f250\": invalid project ID\nblahblah"
	gcpSSDQUotaLog          = "blahblah\ntime=\"2021-01-06T03:35:44Z\" level=error msg=\"Error: Error waiting for instance to create: Quota 'SSD_TOTAL_GB' exceeded. Limit: 500.0 in region asia-northeast2.\nblahblah"
	kubeAPIWaitTimeoutLog   = "blahblah\ntime=\"2021-01-03T07:04:44Z\" level=fatal msg=\"waiting for Kubernetes API: context deadline exceeded\""
	natGatewayLimitExceeded = "blahblah\ntime=\"2021-01-06T03:35:44Z\" level=error msg=\"Error creating NAT Gateway: NatGatewayLimitExceeded: The maximum number of NAT Gateways has been reached.\""
	vpcLimitExceeded        = "blahblah\ntime=\"2021-01-06T03:35:44Z\" level=error msg=\"Error: Error creating VPC: VpcLimitExceeded: The maximum number of VPCs has been reached.\""
	genericLimitExceeded    = "blahblah\ntime=\"2021-01-06T03:35:44Z\" level=error msg=\"Error: Error creating Generic: GenericLimitExceeded: The maximum number of Generics has been reached.\""
	invalidCredentials      = "blahblah\ntime=\"2021-01-06T03:35:44Z\" level=error msg=\"Error: error waiting for Route53 Hosted Zone (Z1009177L956IM4ANFHL) creation: InvalidClientTokenId: The security token included in the request is invalid.\""
	noMatchLog              = "an example of something that doesn't match the log regexes"
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
			name:           "Generic ResourceLimitExceeded",
			log:            pointer.StringPtr(genericLimitExceeded),
			existing:       []runtime.Object{buildRegexConfigMap()},
			expectedReason: "ResourceLimitExceeded",
		},
		{
			name:           "Credentials are invalid",
			log:            pointer.StringPtr(invalidCredentials),
			existing:       []runtime.Object{buildRegexConfigMap()},
			expectedReason: "InvalidCredentials",
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

func buildRegexConfigMap() *corev1.ConfigMap {
	cm := &corev1.ConfigMap{
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
- name: PendingVerification
  searchRegexStrings:
  - "PendingVerification: Your request for accessing resources in this region is being validated"
  installFailingReason: PendingVerification
  installFailingMessage: Account pending verification for region
- name: GCPInvalidProjectID
  searchRegexStrings:
  - "platform.gcp.project.* invalid project ID"
  installFailingReason: GCPInvalidProjectID
  installFailingMessage: Invalid GCP project ID
- name: GCPQuotaSSDTotalGBExceeded
  searchRegexStrings:
  - "Quota \'SSD_TOTAL_GB\' exceeded"
  installFailingReason: GCPQuotaSSDTotalGBExceeded
  installFailingMessage: GCP quota SSD_TOTAL_GB exceeded
# AWS Specific
- name: AWSNATGatewayLimitExceeded
  searchRegexStrings:
  - "NatGatewayLimitExceeded"
  installFailingReason: AWSNATGatewayLimitExceeded
  installFailingMessage: AWS NAT gateway limit exceeded
- name: AWSVPCLimitExceeded
  searchRegexStrings:
    - "VpcLimitExceeded"
  installFailingReason: AWSVPCLimitExceeded
  installFailingMessage: AWS VPC limit exceeded
- name: ResourceLimitExceeded
  searchRegexStrings:
  - "LimitExceeded"
  installFailingReason: ResourceLimitExceeded
  installFailingMessage: Resource limit exceeded
- name: InvalidCredentials
  searchRegexStrings:
  - "InvalidClientTokenId: The security token included in the request is invalid."
  installFailingReason: InvalidCredentials
  installFailingMessage: Credentials are invalid
`,
		},
	}
	return cm
}
