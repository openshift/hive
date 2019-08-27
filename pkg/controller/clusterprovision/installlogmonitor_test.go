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

	"github.com/openshift/hive/pkg/apis"
	"github.com/openshift/hive/pkg/constants"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

const (
	dnsAlreadyExistsLog    = "blahblah\naws_route53_record.api_external: [ERR]: Error building changeset: InvalidChangeBatch: [Tried to create resource record set [name='api.jh-stg-2405-2.n6b3.s1.devshift.org.'type='A'] but it already exists]\n\nblahblah"
	pendingVerificationLog = "blahblah\naws_instance.master.2: Error launching source instance: PendingVerification: Your request for accessing resources in this region is being validated, and you will not be able to launch additional resources in this region until the validation is complete. We will notify you by email once your request has been validated. While normally resolved within minutes, please allow up to 4 hours for this process to complete. If the issue still persists, please let us know by writing to awsa\n\nblahblah"
)

func TestParseInstallLog(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	tests := []struct {
		name           string
		log            *string
		existing       []runtime.Object
		expectedReason string
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
			name:           "no log",
			existing:       []runtime.Object{buildRegexConfigMap()},
			expectedReason: unknownReason,
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
					Namespace: constants.HiveNamespace,
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
					Namespace: constants.HiveNamespace,
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
					Namespace: constants.HiveNamespace,
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
					Namespace: constants.HiveNamespace,
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
			assert.NotEmpty(t, message, "expected message to be not empty")
		})
	}
}

func buildRegexConfigMap() *corev1.ConfigMap {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      regexConfigMapName,
			Namespace: constants.HiveNamespace,
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
`,
		},
	}
	return cm
}
