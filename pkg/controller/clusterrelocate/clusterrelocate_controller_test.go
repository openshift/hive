package clusterrelocate

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/remoteclient"
	remoteclientmock "github.com/openshift/hive/pkg/remoteclient/mock"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	testcr "github.com/openshift/hive/pkg/test/clusterrelocate"
	testcm "github.com/openshift/hive/pkg/test/configmap"
	testdnszone "github.com/openshift/hive/pkg/test/dnszone"
	testfake "github.com/openshift/hive/pkg/test/fake"
	testgeneric "github.com/openshift/hive/pkg/test/generic"
	testjob "github.com/openshift/hive/pkg/test/job"
	testmp "github.com/openshift/hive/pkg/test/machinepool"
	testnamespace "github.com/openshift/hive/pkg/test/namespace"
	testsecret "github.com/openshift/hive/pkg/test/secret"
	testsip "github.com/openshift/hive/pkg/test/syncidentityprovider"
	testss "github.com/openshift/hive/pkg/test/syncset"
	"github.com/openshift/hive/pkg/util/scheme"
)

const (
	namespace = "test-namespace"
	cdName    = "test-cluster-deployment"
	crName    = "test-cluster-relocator"

	kubeconfigNamespace = "test-kubeconfig-namespace"
	kubeconfigName      = "test-kubeconfig"

	labelKey   = "test-key"
	labelValue = "test-value"
)

func TestReconcileClusterRelocate_Reconcile_Movement(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.DebugLevel)

	scheme := scheme.GetScheme()

	cdBuilder := testcd.FullBuilder(namespace, cdName, scheme).GenericOptions(
		testgeneric.WithLabel(labelKey, labelValue),
		testgeneric.WithFinalizer(hivev1.FinalizerDeprovision),
	).Options(
		func(cd *hivev1.ClusterDeployment) { cd.Spec.ManageDNS = true },
	)
	crBuilder := testcr.FullBuilder(crName, scheme).Options(
		testcr.WithKubeconfigSecret(kubeconfigNamespace, kubeconfigName),
		testcr.WithClusterDeploymentSelector(labelKey, labelValue),
	)
	secretBuilder := testsecret.FullBuilder(namespace, "test-secret", scheme)
	cmBuilder := testcm.FullBuilder(namespace, "test-configmap", scheme)
	mpBuilder := testmp.FullBuilder(namespace, "test-pool", cdName, scheme)
	ssBuilder := testss.FullBuilder(namespace, "test-ss", scheme).Options(
		testss.ForClusterDeployments(cdName),
		testss.WithApplyMode(hivev1.SyncResourceApplyMode),
	)
	sipBuilder := testsip.FullBuilder(namespace, "test-sip", scheme).Options(
		testsip.ForClusterDeployments(cdName),
		testsip.ForIdentities("test-user"),
	)
	dnsZoneBuilder := testdnszone.FullBuilder(namespace, controllerutils.DNSZoneName(cdName), scheme).Options(
		testdnszone.WithZone("test-zone"),
	)
	jobBuilder := testjob.FullBuilder(namespace, "test-job", scheme)
	namespaceBuilder := testnamespace.FullBuilder(namespace, scheme)

	cases := []struct {
		name                string
		cd                  *hivev1.ClusterDeployment
		srcResources        []runtime.Object
		destResources       []runtime.Object
		expectedResources   []client.Object
		unexpectedResources []client.Object
	}{
		{
			name: "no relocation",
			cd:   cdBuilder.Build(),
		},
		{
			name: "only clusterdeployment",
			cd: cdBuilder.Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.RelocationFailedCondition,
				Status: corev1.ConditionUnknown,
			})),
			srcResources: []runtime.Object{
				crBuilder.Build(),
			},
			expectedResources: []client.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(
					testcd.Generic(withRelocateAnnotation(crName, hivev1.RelocateIncoming)),
				),
			},
		},
		{
			name: "existing clusterdeployment",
			cd:   cdBuilder.Build(),
			srcResources: []runtime.Object{
				crBuilder.Build(),
			},
			destResources: []runtime.Object{
				cdBuilder.Build(),
			},
			expectedResources: []client.Object{
				cdBuilder.Build(),
			},
		},
		{
			name: "out-of-date clusterdeployment",
			cd:   cdBuilder.Build(),
			srcResources: []runtime.Object{
				crBuilder.Build(),
			},
			destResources: []runtime.Object{
				cdBuilder.Build(
					func(cd *hivev1.ClusterDeployment) {
						cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{}
					},
				),
			},
			expectedResources: []client.Object{
				cdBuilder.Build(
					func(cd *hivev1.ClusterDeployment) {
						cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{}
					},
				),
			},
		},
		{
			name: "existing clusterdeployment with different status",
			cd: cdBuilder.Build(
				func(cd *hivev1.ClusterDeployment) {
					cd.Status.Conditions = []hivev1.ClusterDeploymentCondition{}
				},
			),
			srcResources: []runtime.Object{
				crBuilder.Build(),
			},
			destResources: []runtime.Object{
				cdBuilder.Build(),
			},
			expectedResources: []client.Object{
				cdBuilder.Build(),
			},
		},
		{
			name: "existing clusterdeployment with different instance-specific metadata",
			cd:   cdBuilder.Build(),
			srcResources: []runtime.Object{
				crBuilder.Build(),
			},
			destResources: []runtime.Object{
				cdBuilder.Build(
					testcd.Generic(testgeneric.WithResourceVersion("some-rv")),
				),
			},
			expectedResources: []client.Object{
				cdBuilder.Build(
					testcd.Generic(testgeneric.WithResourceVersion("some-rv")),
				),
			},
		},
		{
			name: "existing namespace",
			cd: cdBuilder.Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.RelocationFailedCondition,
				Status: corev1.ConditionUnknown,
			})),
			srcResources: []runtime.Object{
				crBuilder.Build(),
			},
			destResources: []runtime.Object{
				namespaceBuilder.Build(),
			},
			expectedResources: []client.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(
					testcd.Generic(withRelocateAnnotation(crName, hivev1.RelocateIncoming)),
				),
			},
		},
		{
			name: "single dependent",
			cd: cdBuilder.Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.RelocationFailedCondition,
				Status: corev1.ConditionUnknown,
			})),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				secretBuilder.Build(testsecret.WithDataKeyValue("test-key", []byte("test-data"))),
			},
			expectedResources: []client.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(
					testcd.Generic(withRelocateAnnotation(crName, hivev1.RelocateIncoming)),
				),
				secretBuilder.Build(testsecret.WithDataKeyValue("test-key", []byte("test-data"))),
			},
		},
		{
			name: "multiple dependents",
			cd: cdBuilder.Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.RelocationFailedCondition,
				Status: corev1.ConditionUnknown,
			})),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				secretBuilder.Build(
					testsecret.WithName("test-secret-1"),
					testsecret.WithDataKeyValue("test-key-1", []byte("test-data-1")),
				),
				secretBuilder.Build(
					testsecret.WithName("test-secret-2"),
					testsecret.WithDataKeyValue("test-key-2", []byte("test-data-2")),
				),
			},
			expectedResources: []client.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(
					testcd.Generic(withRelocateAnnotation(crName, hivev1.RelocateIncoming)),
				),
				secretBuilder.Build(
					testsecret.WithName("test-secret-1"),
					testsecret.WithDataKeyValue("test-key-1", []byte("test-data-1")),
				),
				secretBuilder.Build(
					testsecret.WithName("test-secret-2"),
					testsecret.WithDataKeyValue("test-key-2", []byte("test-data-2")),
				),
			},
		},
		{
			name: "dependent in other namespace",
			cd: cdBuilder.Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.RelocationFailedCondition,
				Status: corev1.ConditionUnknown,
			})),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				secretBuilder.Build(
					testsecret.WithName("test-secret-1"),
					testsecret.WithDataKeyValue("test-key-1", []byte("test-data-1")),
				),
				secretBuilder.Build(
					testsecret.WithNamespace("other-namespace"),
					testsecret.WithName("test-secret-2"),
					testsecret.WithDataKeyValue("test-key-2", []byte("test-data-2")),
				),
			},
			expectedResources: []client.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(
					testcd.Generic(withRelocateAnnotation(crName, hivev1.RelocateIncoming)),
				),
				secretBuilder.Build(
					testsecret.WithName("test-secret-1"),
					testsecret.WithDataKeyValue("test-key-1", []byte("test-data-1")),
				),
			},
		},
		{
			name: "existing dependent",
			cd: cdBuilder.Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.RelocationFailedCondition,
				Status: corev1.ConditionUnknown,
			})),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				secretBuilder.Build(testsecret.WithDataKeyValue("test-key", []byte("test-data"))),
			},
			destResources: []runtime.Object{
				secretBuilder.Build(testsecret.WithDataKeyValue("test-key", []byte("test-data"))),
			},
			expectedResources: []client.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(
					testcd.Generic(withRelocateAnnotation(crName, hivev1.RelocateIncoming)),
				),
				secretBuilder.Build(testsecret.WithDataKeyValue("test-key", []byte("test-data"))),
			},
		},
		{
			name: "out-of-date dependent",
			cd: cdBuilder.Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.RelocationFailedCondition,
				Status: corev1.ConditionUnknown,
			})),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				secretBuilder.Build(testsecret.WithDataKeyValue("test-key", []byte("test-data"))),
			},
			destResources: []runtime.Object{
				secretBuilder.Build(testsecret.WithDataKeyValue("test-key", []byte("other-data"))),
			},
			expectedResources: []client.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(
					testcd.Generic(withRelocateAnnotation(crName, hivev1.RelocateIncoming)),
				),
				secretBuilder.Build(testsecret.WithDataKeyValue("test-key", []byte("test-data"))),
			},
		},
		{
			name: "existing dependent with different status",
			cd: cdBuilder.Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.RelocationFailedCondition,
				Status: corev1.ConditionUnknown,
			})),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				mpBuilder.Build(
					func(mp *hivev1.MachinePool) {
						mp.Status.Conditions = []hivev1.MachinePoolCondition{}
					},
				),
			},
			destResources: []runtime.Object{
				mpBuilder.Build(),
			},
			expectedResources: []client.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(
					testcd.Generic(withRelocateAnnotation(crName, hivev1.RelocateIncoming)),
				),
				mpBuilder.Build(),
			},
		},
		{
			name: "existing dependent with different instance-specific metadata",
			cd: cdBuilder.Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.RelocationFailedCondition,
				Status: corev1.ConditionUnknown,
			})),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				secretBuilder.Build(
					testsecret.WithDataKeyValue("test-key", []byte("test-data")),
				),
			},
			destResources: []runtime.Object{
				secretBuilder.Build(
					testsecret.Generic(testgeneric.WithResourceVersion("some-rv")),
					testsecret.WithDataKeyValue("test-key", []byte("test-data")),
				),
			},
			expectedResources: []client.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(
					testcd.Generic(withRelocateAnnotation(crName, hivev1.RelocateIncoming)),
				),
				secretBuilder.Build(
					testsecret.Generic(testgeneric.WithResourceVersion("some-rv")),
					testsecret.WithDataKeyValue("test-key", []byte("test-data")),
				),
			},
		},
		{
			name: "ignore service account token",
			cd: cdBuilder.Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.RelocationFailedCondition,
				Status: corev1.ConditionUnknown,
			})),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				secretBuilder.Build(
					testsecret.WithType(corev1.SecretTypeServiceAccountToken),
					testsecret.WithDataKeyValue("test-key", []byte("test-data")),
				),
			},
			expectedResources: []client.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(
					testcd.Generic(withRelocateAnnotation(crName, hivev1.RelocateIncoming)),
				),
			},
			unexpectedResources: []client.Object{
				secretBuilder.Build(
					testsecret.WithType(corev1.SecretTypeServiceAccountToken),
					testsecret.WithDataKeyValue("test-key", []byte("test-data")),
				),
			},
		},
		{
			name: "ignore secret owned by service account token",
			cd: cdBuilder.Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.RelocationFailedCondition,
				Status: corev1.ConditionUnknown,
			})),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				secretBuilder.Build(
					testsecret.Generic(testgeneric.WithName("test-sa-token")),
					testsecret.WithType(corev1.SecretTypeServiceAccountToken),
					testsecret.WithDataKeyValue("test-key-sa-token", []byte("test-data-sa-token")),
				),
				secretBuilder.Build(
					testsecret.Generic(testgeneric.WithName("test-dockercfg")),
					testsecret.WithDataKeyValue("test-key-dockercfg", []byte("test-data-dockercfg")),
					func(secret *corev1.Secret) {
						secret.OwnerReferences = append(secret.OwnerReferences, metav1.OwnerReference{
							Kind: "Secret",
							Name: "test-sa-token",
						})
					},
				),
			},
			expectedResources: []client.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(
					testcd.Generic(withRelocateAnnotation(crName, hivev1.RelocateIncoming)),
				),
			},
			unexpectedResources: []client.Object{
				secretBuilder.Build(
					testsecret.Generic(testgeneric.WithName("test-sa-token")),
					testsecret.WithType(corev1.SecretTypeServiceAccountToken),
					testsecret.WithDataKeyValue("test-key-sa-token", []byte("test-data-sa-token")),
				),
				secretBuilder.Build(
					testsecret.Generic(testgeneric.WithName("test-dockercfg")),
					testsecret.WithDataKeyValue("test-key-dockercfg", []byte("test-data-dockercfg")),
					func(secret *corev1.Secret) {
						secret.OwnerReferences = append(secret.OwnerReferences, metav1.OwnerReference{
							Kind: "Secret",
							Name: "test-sa-token",
						})
					},
				),
			},
		},
		{
			name: "ignore kube-controller-manager crt configmaps",
			cd: cdBuilder.Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.RelocationFailedCondition,
				Status: corev1.ConditionUnknown,
			})),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				cmBuilder.Build(
					testcm.WithName("kube-root-ca.crt"),
				),
				cmBuilder.Build(
					testcm.WithName("openshift-service-ca.crt"),
				),
				// make sure a normal one is copied
				cmBuilder.Build(),
			},
			expectedResources: []client.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(
					testcd.Generic(withRelocateAnnotation(crName, hivev1.RelocateIncoming)),
				),
				cmBuilder.Build(),
			},
			unexpectedResources: []client.Object{
				cmBuilder.Build(
					testcm.WithName("kube-root-ca.crt"),
				),
				cmBuilder.Build(
					testcm.WithName("openshift-service-ca.crt"),
				),
			},
		},
		{
			name: "configmap",
			cd: cdBuilder.Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.RelocationFailedCondition,
				Status: corev1.ConditionUnknown,
			})),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				cmBuilder.Build(testcm.WithDataKeyValue("test-key", "test-data")),
			},
			expectedResources: []client.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(
					testcd.Generic(withRelocateAnnotation(crName, hivev1.RelocateIncoming)),
				),
				cmBuilder.Build(testcm.WithDataKeyValue("test-key", "test-data")),
			},
		},
		{
			name: "machinepool",
			cd: cdBuilder.Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.RelocationFailedCondition,
				Status: corev1.ConditionUnknown,
			})),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				mpBuilder.Build(),
			},
			expectedResources: []client.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(
					testcd.Generic(withRelocateAnnotation(crName, hivev1.RelocateIncoming)),
				),
				mpBuilder.Build(),
			},
		},
		{
			name: "syncset",
			cd: cdBuilder.Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.RelocationFailedCondition,
				Status: corev1.ConditionUnknown,
			})),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				ssBuilder.Build(),
			},
			expectedResources: []client.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(
					testcd.Generic(withRelocateAnnotation(crName, hivev1.RelocateIncoming)),
				),
				ssBuilder.Build(),
			},
		},
		{
			name: "syncidentityprovider",
			cd: cdBuilder.Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.RelocationFailedCondition,
				Status: corev1.ConditionUnknown,
			})),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				sipBuilder.Build(),
			},
			expectedResources: []client.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(
					testcd.Generic(withRelocateAnnotation(crName, hivev1.RelocateIncoming)),
				),
				sipBuilder.Build(),
			},
		},
		{
			name: "dnszone",
			cd: cdBuilder.Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.RelocationFailedCondition,
				Status: corev1.ConditionUnknown,
			})),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				dnsZoneBuilder.Build(),
			},
			expectedResources: []client.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(
					testcd.Generic(withRelocateAnnotation(crName, hivev1.RelocateIncoming)),
				),
				dnsZoneBuilder.Build(
					testdnszone.Generic(withRelocateAnnotation(crName, hivev1.RelocateIncoming)),
				),
			},
		},
		{
			name: "non-child dnszone",
			cd: cdBuilder.Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.RelocationFailedCondition,
				Status: corev1.ConditionUnknown,
			})),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				dnsZoneBuilder.Build(testdnszone.Generic(testgeneric.WithName("other-dnszone"))),
			},
			expectedResources: []client.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(
					testcd.Generic(withRelocateAnnotation(crName, hivev1.RelocateIncoming)),
				),
			},
		},
		{
			name: "non-dependent",
			cd: cdBuilder.Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.RelocationFailedCondition,
				Status: corev1.ConditionUnknown,
			})),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				jobBuilder.Build(),
			},
			expectedResources: []client.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(
					testcd.Generic(withRelocateAnnotation(crName, hivev1.RelocateIncoming)),
				),
			},
		},
		{
			name: "full",
			cd: cdBuilder.Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.RelocationFailedCondition,
				Status: corev1.ConditionUnknown,
			})),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				cdBuilder.Build(testcd.WithName("other-cluster-deployment")),
				cdBuilder.Build(testcd.WithNamespace("other-namespace")),
				secretBuilder.Build(
					testsecret.WithName("test-secret-new"),
					testsecret.WithDataKeyValue("test-key-new", []byte("test-data-new")),
				),
				secretBuilder.Build(
					testsecret.WithName("test-secret-existing"),
					testsecret.WithDataKeyValue("test-key-existing", []byte("test-data-existing")),
				),
				secretBuilder.Build(
					testsecret.WithName("test-secret-out-of-date"),
					testsecret.WithDataKeyValue("test-key-out-of-date", []byte("test-data-out-of-date")),
				),
				secretBuilder.Build(
					testsecret.WithNamespace("other-namespace"),
					testsecret.WithName("test-secret-other-namespace"),
					testsecret.WithDataKeyValue("test-key-other-namespace", []byte("test-data-other-namespace")),
				),
				secretBuilder.Build(
					testsecret.Generic(testgeneric.WithName("test-sa-token")),
					testsecret.WithType(corev1.SecretTypeServiceAccountToken),
					testsecret.WithDataKeyValue("test-key-sa-token", []byte("test-data-sa-token")),
				),
				secretBuilder.Build(
					testsecret.Generic(testgeneric.WithName("test-dockercfg")),
					testsecret.WithDataKeyValue("test-key-dockercfg", []byte("test-data-dockercfg")),
					func(secret *corev1.Secret) {
						secret.OwnerReferences = append(secret.OwnerReferences, metav1.OwnerReference{
							Kind: "Secret",
							Name: "test-sa-token",
						})
					},
				),
				cmBuilder.Build(
					testcm.WithName("test-configmap-new"),
					testcm.WithDataKeyValue("test-key-new", "test-data-new")),
				cmBuilder.Build(
					testcm.WithName("test-configmap-existing"),
					testcm.WithDataKeyValue("test-key-existing", "test-data-existing")),
				cmBuilder.Build(
					testcm.WithName("test-configmap-out-of-date"),
					testcm.WithDataKeyValue("test-key-out-of-date", "test-data-out-of-date")),
				cmBuilder.Build(
					testcm.WithNamespace("other-namespace"),
					testcm.WithName("test-configmap-other-namespace"),
					testcm.WithDataKeyValue("test-key-other-namespace", "test-data-other-namespace")),
				mpBuilder.Build(
					testmp.WithPoolNameForClusterDeployment("test-pool-new", cdName),
				),
				mpBuilder.Build(
					testmp.WithPoolNameForClusterDeployment("test-pool-existing", cdName),
				),
				mpBuilder.Build(
					testmp.WithPoolNameForClusterDeployment("test-pool-out-of-date", cdName),
				),
				mpBuilder.Build(
					testmp.WithNamespace("other-namespace"),
					testmp.WithPoolNameForClusterDeployment("test-pool-other-namespace", cdName),
				),
				ssBuilder.Build(
					testss.WithName("test-syncset-new"),
				),
				ssBuilder.Build(
					testss.WithName("test-syncset-existing"),
				),
				ssBuilder.Build(
					testss.WithName("test-syncset-out-of-date"),
				),
				ssBuilder.Build(
					testss.WithNamespace("other-namespace"),
					testss.WithName("test-syncset-other-namespace"),
				),
				sipBuilder.Build(
					testsip.WithName("test-sip-new"),
				),
				sipBuilder.Build(
					testsip.WithName("test-sip-existing"),
				),
				sipBuilder.Build(
					testsip.WithName("test-sip-out-of-date"),
				),
				sipBuilder.Build(
					testsip.WithNamespace("other-namespace"),
					testsip.WithName("test-sip-other-namespace"),
				),
				dnsZoneBuilder.Build(),
				dnsZoneBuilder.Build(testdnszone.Generic(testgeneric.WithName("other-dnszone"))),
				jobBuilder.Build(),
				jobBuilder.Build(testjob.WithNamespace("other-namespace")),
			},
			destResources: []runtime.Object{
				secretBuilder.Build(
					testsecret.WithName("test-secret-existing"),
					testsecret.WithDataKeyValue("test-key-existing", []byte("test-data-existing")),
				),
				secretBuilder.Build(
					testsecret.WithName("test-secret-out-of-date"),
					testsecret.WithDataKeyValue("test-key-out-of-date", []byte("other-test-data")),
				),
				cmBuilder.Build(
					testcm.WithName("test-configmap-existing"),
					testcm.WithDataKeyValue("test-key-existing", "test-data-existing")),
				cmBuilder.Build(
					testcm.WithName("test-configmap-out-of-date"),
					testcm.WithDataKeyValue("test-key-out-of-date", "other-test-data")),
				mpBuilder.Build(
					testmp.WithPoolNameForClusterDeployment("test-pool-existing", cdName),
				),
				mpBuilder.Build(
					testmp.WithPoolNameForClusterDeployment("test-pool-out-of-date", cdName),
					func(mp *hivev1.MachinePool) {
						mp.Spec.Autoscaling = &hivev1.MachinePoolAutoscaling{}
					},
				),
				ssBuilder.Build(
					testss.WithName("test-syncset-existing"),
				),
				ssBuilder.Build(
					testss.WithName("test-syncset-out-of-date"),
					testss.WithApplyMode(hivev1.SyncSetResourceApplyMode("other-mode")),
				),
				sipBuilder.Build(
					testsip.WithName("test-sip-existing"),
				),
				sipBuilder.Build(
					testsip.WithName("test-sip-out-of-date"),
					testsip.ForIdentities("other-user"),
				),
			},
			expectedResources: []client.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(
					testcd.Generic(withRelocateAnnotation(crName, hivev1.RelocateIncoming)),
				),
				secretBuilder.Build(
					testsecret.WithName("test-secret-new"),
					testsecret.WithDataKeyValue("test-key-new", []byte("test-data-new")),
				),
				secretBuilder.Build(
					testsecret.WithName("test-secret-existing"),
					testsecret.WithDataKeyValue("test-key-existing", []byte("test-data-existing")),
				),
				secretBuilder.Build(
					testsecret.WithName("test-secret-out-of-date"),
					testsecret.WithDataKeyValue("test-key-out-of-date", []byte("test-data-out-of-date")),
				),
				cmBuilder.Build(
					testcm.WithName("test-configmap-new"),
					testcm.WithDataKeyValue("test-key-new", "test-data-new")),
				cmBuilder.Build(
					testcm.WithName("test-configmap-existing"),
					testcm.WithDataKeyValue("test-key-existing", "test-data-existing")),
				cmBuilder.Build(
					testcm.WithName("test-configmap-out-of-date"),
					testcm.WithDataKeyValue("test-key-out-of-date", "test-data-out-of-date")),
				mpBuilder.Build(
					testmp.WithPoolNameForClusterDeployment("test-pool-new", cdName),
				),
				mpBuilder.Build(
					testmp.WithPoolNameForClusterDeployment("test-pool-existing", cdName),
				),
				mpBuilder.Build(
					testmp.WithPoolNameForClusterDeployment("test-pool-out-of-date", cdName),
				),
				ssBuilder.Build(
					testss.WithName("test-syncset-new"),
				),
				ssBuilder.Build(
					testss.WithName("test-syncset-existing"),
				),
				ssBuilder.Build(
					testss.WithName("test-syncset-out-of-date"),
				),
				sipBuilder.Build(
					testsip.WithName("test-sip-new"),
				),
				sipBuilder.Build(
					testsip.WithName("test-sip-existing"),
				),
				sipBuilder.Build(
					testsip.WithName("test-sip-out-of-date"),
				),
				dnsZoneBuilder.Build(
					testdnszone.Generic(withRelocateAnnotation(crName, hivev1.RelocateIncoming)),
				),
			},
		},
		{
			name: "no match",
			cd:   cdBuilder.Build(),
			srcResources: []runtime.Object{
				crBuilder.Build(testcr.WithClusterDeploymentSelector("other-key", "other-value")),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.cd != nil {
				tc.srcResources = append(tc.srcResources, tc.cd)
			}
			kubeconfigSecret := testsecret.FullBuilder(kubeconfigNamespace, "test-kubeconfig", scheme).Build(
				testsecret.WithDataKeyValue("kubeconfig", []byte("some-kubeconfig-data")),
			)
			tc.srcResources = append(tc.srcResources, kubeconfigSecret)
			srcClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(tc.srcResources...).Build()
			destClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(tc.destResources...).Build()

			mockCtrl := gomock.NewController(t)

			mockRemoteClientBuilder := remoteclientmock.NewMockBuilder(mockCtrl)
			mockRemoteClientBuilder.EXPECT().Build().Return(destClient, nil).AnyTimes()

			reconciler := &ReconcileClusterRelocate{
				Client: srcClient,
				logger: logger,
				remoteClusterAPIClientBuilder: func(secret *corev1.Secret) remoteclient.Builder {
					assert.Equal(t, kubeconfigSecret, secret, "unexpected secret passed to remote client builder")
					return mockRemoteClientBuilder
				},
			}
			_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      cdName,
					Namespace: namespace,
				},
			})
			require.NoError(t, err, "unexpected error during reconcile")

			for _, obj := range tc.expectedResources {
				objKey := client.ObjectKeyFromObject(obj)
				destObj := reflect.New(reflect.TypeOf(obj).Elem()).Interface().(client.Object)
				err = destClient.Get(context.Background(), objKey, destObj)
				if !assert.NoError(t, err, "unexpected error getting destination object") {
					continue
				}
				assert.Equal(t, obj, destObj, "destination object different than expected object")
			}
			for _, obj := range tc.unexpectedResources {
				objKey := client.ObjectKeyFromObject(obj)
				destObj := reflect.New(reflect.TypeOf(obj).Elem()).Interface().(client.Object)
				err = destClient.Get(context.Background(), objKey, destObj)
				if assert.Error(t, err, "expected error getting destination object") {
					assert.True(t, apierrors.IsNotFound(err), "expected error to be 'not found'")
				}
			}
		})
	}
}

func TestReconcileClusterRelocate_Reconcile_RelocateStatus(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.DebugLevel)

	scheme := scheme.GetScheme()

	cdBuilder := testcd.FullBuilder(namespace, cdName, scheme).GenericOptions(
		testgeneric.WithLabel(labelKey, labelValue),
	).Options(
		func(cd *hivev1.ClusterDeployment) { cd.Spec.ManageDNS = true },
	)
	crBuilder := testcr.FullBuilder(crName, scheme).Options(
		testcr.WithKubeconfigSecret(kubeconfigNamespace, kubeconfigName),
		testcr.WithClusterDeploymentSelector(labelKey, labelValue),
	)
	dnsZoneBuilder := testdnszone.FullBuilder(namespace, controllerutils.DNSZoneName(cdName), scheme)

	cases := []struct {
		name                              string
		cd                                *hivev1.ClusterDeployment
		dnsZone                           *hivev1.DNSZone
		missingKubeconfigSecret           bool
		srcResources                      []runtime.Object
		destResources                     []runtime.Object
		expectedError                     bool
		expectedRelocateStatus            hivev1.RelocateStatus
		expectDeleted                     bool
		expectedRelocationFailedCondition *hivev1.ClusterDeploymentCondition
		validate                          func(t *testing.T, cd *hivev1.ClusterDeployment)
	}{
		{
			name: "fresh clusterdeployment",
			cd: cdBuilder.Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.RelocationFailedCondition,
				Status: corev1.ConditionUnknown,
			})),
			dnsZone: dnsZoneBuilder.Build(),
			srcResources: []runtime.Object{
				crBuilder.Build(),
			},
			expectedRelocateStatus: hivev1.RelocateComplete,
			expectDeleted:          true,
		},
		{
			name: "clusterdeployment with dnszone already relocating",
			cd: cdBuilder.Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.RelocationFailedCondition,
				Status: corev1.ConditionUnknown,
			})),
			dnsZone: dnsZoneBuilder.Build(
				testdnszone.Generic(withRelocateAnnotation("other-relocate", hivev1.RelocateOutgoing)),
			),
			srcResources: []runtime.Object{
				crBuilder.Build(),
			},
			expectedRelocateStatus: hivev1.RelocateComplete,
			expectDeleted:          true,
		},
		{
			name: "switch relocates",
			cd: cdBuilder.Build(
				testcd.Generic(withRelocateAnnotation("other-relocate", hivev1.RelocateOutgoing)),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:   hivev1.RelocationFailedCondition,
					Status: corev1.ConditionUnknown,
				}),
			),
			dnsZone: dnsZoneBuilder.Build(
				testdnszone.Generic(withRelocateAnnotation("other-relocate", hivev1.RelocateOutgoing)),
			),
			srcResources: []runtime.Object{
				crBuilder.Build(),
			},
			expectedRelocateStatus: hivev1.RelocateComplete,
			expectDeleted:          true,
		},
		{
			name: "multiple relocates",
			cd: cdBuilder.Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.RelocationFailedCondition,
				Status: corev1.ConditionUnknown,
			})),
			dnsZone: dnsZoneBuilder.Build(),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				crBuilder.Build(
					testcr.Generic(testgeneric.WithName("other-relocate")),
				),
			},
			expectedRelocationFailedCondition: &hivev1.ClusterDeploymentCondition{
				Status: corev1.ConditionTrue,
				Reason: "MultipleMatchingRelocates",
			},
		},
		{
			name: "multiple relocates when already relocating",
			cd: cdBuilder.Build(
				testcd.Generic(withRelocateAnnotation("other-relocate", hivev1.RelocateOutgoing)),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:   hivev1.RelocationFailedCondition,
					Status: corev1.ConditionUnknown,
				}),
			),
			dnsZone: dnsZoneBuilder.Build(
				testdnszone.Generic(withRelocateAnnotation("other-relocate", hivev1.RelocateOutgoing)),
			),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				crBuilder.Build(
					testcr.Generic(testgeneric.WithName("other-relocate")),
				),
			},
			expectedRelocationFailedCondition: &hivev1.ClusterDeploymentCondition{
				Status: corev1.ConditionTrue,
				Reason: "MultipleMatchingRelocates",
			},
		},
		{
			name: "already relocated clusterdeployment",
			cd: cdBuilder.Build(
				testcd.Generic(withRelocateAnnotation(crName, hivev1.RelocateComplete)),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:   hivev1.RelocationFailedCondition,
					Status: corev1.ConditionUnknown,
				}),
			),
			dnsZone: dnsZoneBuilder.Build(
				testdnszone.Generic(withRelocateAnnotation(crName, hivev1.RelocateComplete)),
			),
			srcResources: []runtime.Object{
				crBuilder.Build(),
			},
			expectedRelocateStatus: hivev1.RelocateComplete,
			expectDeleted:          true,
		},
		{
			name: "already moved clusterdeployment",
			cd: cdBuilder.Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.RelocationFailedCondition,
				Status: corev1.ConditionUnknown,
			})),
			dnsZone: dnsZoneBuilder.Build(),
			srcResources: []runtime.Object{
				crBuilder.Build(),
			},
			destResources: []runtime.Object{
				cdBuilder.Build(),
			},
			expectedRelocateStatus: hivev1.RelocateComplete,
			expectDeleted:          true,
		},
		{
			name: "already moved clusterdeployment with dnszone already completed",
			cd: cdBuilder.Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.RelocationFailedCondition,
				Status: corev1.ConditionUnknown,
			})),
			dnsZone: dnsZoneBuilder.Build(
				testdnszone.Generic(withRelocateAnnotation(crName, hivev1.RelocateComplete)),
			),
			srcResources: []runtime.Object{
				crBuilder.Build(),
			},
			destResources: []runtime.Object{
				cdBuilder.Build(),
			},
			expectedRelocateStatus: hivev1.RelocateComplete,
			expectDeleted:          true,
		},
		{
			name: "clusterdeployment mismatch",
			cd: cdBuilder.Build(
				func(cd *hivev1.ClusterDeployment) {
					cd.Spec.BaseDomain = "test-domain"
					cd.Status.Conditions = append(cd.Status.Conditions, hivev1.ClusterDeploymentCondition{
						Status: corev1.ConditionUnknown,
						Type:   hivev1.RelocationFailedCondition,
					})
				},
			),
			dnsZone: dnsZoneBuilder.Build(),
			srcResources: []runtime.Object{
				crBuilder.Build(),
			},
			destResources: []runtime.Object{
				cdBuilder.Build(
					func(cd *hivev1.ClusterDeployment) {
						cd.Spec.BaseDomain = "other-domain"
					},
				),
			},
			expectedRelocationFailedCondition: &hivev1.ClusterDeploymentCondition{
				Status: corev1.ConditionTrue,
				Reason: "ClusterDeploymentMismatch",
			},
		},
		{
			name: "incoming",
			cd: cdBuilder.Build(
				testcd.Generic(withRelocateAnnotation("other-relocate", hivev1.RelocateIncoming)),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:   hivev1.RelocationFailedCondition,
					Status: corev1.ConditionUnknown,
				}),
			),
			dnsZone: dnsZoneBuilder.Build(
				testdnszone.Generic(withRelocateAnnotation("other-relocate", hivev1.RelocateIncoming)),
			),
			expectedRelocationFailedCondition: &hivev1.ClusterDeploymentCondition{
				Status: corev1.ConditionFalse,
				Reason: "NoMatchingRelocates",
			},
		},
		{
			name: "incoming with dnszone already released",
			cd: cdBuilder.Build(
				testcd.Generic(withRelocateAnnotation("other-relocate", hivev1.RelocateIncoming)),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:   hivev1.RelocationFailedCondition,
					Status: corev1.ConditionUnknown,
				}),
			),
			dnsZone: dnsZoneBuilder.Build(),
			expectedRelocationFailedCondition: &hivev1.ClusterDeploymentCondition{
				Status: corev1.ConditionFalse,
				Reason: "NoMatchingRelocates",
			},
		},
		{
			name: "clusterdeployment deleted while outgoing",
			cd: cdBuilder.Build(
				testcd.Generic(withRelocateAnnotation(crName, hivev1.RelocateOutgoing)),
				testcd.Generic(testgeneric.Deleted()),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Status: corev1.ConditionFalse,
					Type:   hivev1.RelocationFailedCondition,
					Reason: "DontUpdateCondition",
				}),
			),
			dnsZone: dnsZoneBuilder.Build(
				testdnszone.Generic(withRelocateAnnotation(crName, hivev1.RelocateOutgoing)),
			),
			srcResources: []runtime.Object{
				crBuilder.Build(),
			},
			expectDeleted: true,
			expectedRelocationFailedCondition: &hivev1.ClusterDeploymentCondition{
				Status: corev1.ConditionFalse,
				Reason: "DontUpdateCondition",
			},
		},
		{
			name: "clusterdeployment deleted while incoming",
			cd: cdBuilder.Build(
				testcd.Generic(withRelocateAnnotation(crName, hivev1.RelocateIncoming)),
				testcd.Generic(testgeneric.Deleted()),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Status: corev1.ConditionFalse,
					Type:   hivev1.RelocationFailedCondition,
					Reason: "DontUpdateCondition",
				}),
			),
			dnsZone: dnsZoneBuilder.Build(
				testdnszone.Generic(withRelocateAnnotation(crName, hivev1.RelocateIncoming)),
			),
			srcResources: []runtime.Object{
				crBuilder.Build(),
			},
			expectDeleted: true,
			expectedRelocationFailedCondition: &hivev1.ClusterDeploymentCondition{
				Status: corev1.ConditionFalse,
				Reason: "DontUpdateCondition",
			},
		},
		{
			name: "clusterdeployment deleted while outgoing",
			cd: cdBuilder.Build(
				testcd.Generic(withRelocateAnnotation(crName, hivev1.RelocateOutgoing)),
				testcd.Generic(testgeneric.Deleted()),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Status: corev1.ConditionFalse,
					Type:   hivev1.RelocationFailedCondition,
					Reason: "DontUpdateCondition",
				}),
			),
			dnsZone: dnsZoneBuilder.Build(
				testdnszone.Generic(withRelocateAnnotation(crName, hivev1.RelocateOutgoing)),
			),
			srcResources: []runtime.Object{
				crBuilder.Build(),
			},
			expectDeleted: true,
			expectedRelocationFailedCondition: &hivev1.ClusterDeploymentCondition{
				Status: corev1.ConditionFalse,
				Reason: "DontUpdateCondition",
			},
		},
	}
	for _, tc := range cases {
		if tc.name != "fresh clusterdeployment" {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			tc.srcResources = append(tc.srcResources, tc.cd)
			if tc.dnsZone != nil {
				tc.srcResources = append(tc.srcResources, tc.dnsZone)
			}
			kubeconfigSecret := testsecret.FullBuilder(kubeconfigNamespace, "test-kubeconfig", scheme).Build(
				testsecret.WithDataKeyValue("kubeconfig", []byte("some-kubeconfig-data")),
			)
			if !tc.missingKubeconfigSecret {
				tc.srcResources = append(tc.srcResources, kubeconfigSecret)
			}
			srcClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(tc.srcResources...).Build()
			destClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(tc.destResources...).Build()

			mockCtrl := gomock.NewController(t)

			mockRemoteClientBuilder := remoteclientmock.NewMockBuilder(mockCtrl)
			mockRemoteClientBuilder.EXPECT().Build().Return(destClient, nil).AnyTimes()

			reconciler := &ReconcileClusterRelocate{
				Client: srcClient,
				logger: logger,
				remoteClusterAPIClientBuilder: func(secret *corev1.Secret) remoteclient.Builder {
					assert.Equal(t, kubeconfigSecret, secret, "unexpected secret passed to remote client builder")
					return mockRemoteClientBuilder
				},
			}
			_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      cdName,
					Namespace: namespace,
				},
			})
			if tc.expectedError {
				require.Error(t, err, "expected error during reconcile")
			} else {
				require.NoError(t, err, "unexpected error during reconcile")
			}

			var dnsZone *hivev1.DNSZone
			if tc.dnsZone != nil {
				dnsZone = &hivev1.DNSZone{}
				err = srcClient.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: tc.dnsZone.Name}, dnsZone)
				require.NoError(t, err, "unexpected error fetching dnszone")
			}

			cd := &hivev1.ClusterDeployment{}
			err = srcClient.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: cdName}, cd)
			if tc.expectDeleted {
				require.True(t, apierrors.IsNotFound(err), "expected clusterdeployment to be deleted")
				// The remainder of the checks rely on the CD existing
				return
			} else {
				require.NoError(t, err, "unexpected error fetching clusterdeployment")
			}

			if tc.expectedRelocateStatus != "" {
				expectedAnnotationValue := fmt.Sprintf("%s/%s", crName, tc.expectedRelocateStatus)
				assert.Equal(t, expectedAnnotationValue, cd.Annotations[constants.RelocateAnnotation], "unexpected relocate annotation on clusterdeployment")
				if dnsZone != nil {
					assert.Equal(t, expectedAnnotationValue, dnsZone.Annotations[constants.RelocateAnnotation], "unexpected relocate annotation on dnszone")
				}
			} else {
				assert.NotContains(t, cd.Annotations, constants.RelocateAnnotation, "unexpected relocate annotation on clusterdeployment")
				if dnsZone != nil {
					assert.NotContains(t, dnsZone.Annotations, constants.RelocateAnnotation, "unexpected relocate annotation on dnszone")
				}
			}

			cond := controllerutils.FindCondition(cd.Status.Conditions, hivev1.RelocationFailedCondition)
			if tc.expectedRelocationFailedCondition != nil {
				if assert.NotNil(t, cond, "missing relocating condition") {
					assert.Equal(t, tc.expectedRelocationFailedCondition.Status, cond.Status, "unexpected condition status")
					assert.Equal(t, tc.expectedRelocationFailedCondition.Reason, cond.Reason, "unexpected condition reason")
				}
			} else {
				assert.Equal(t, corev1.ConditionFalse, cond.Status, "unexpected condition status")
				assert.Equal(t, "MoveSuccessful", cond.Reason, "unexpected condition reason")
			}

			if tc.validate != nil {
				tc.validate(t, cd)
			}
		})
	}
}

func withRelocateAnnotation(clusterRelocateName string, status hivev1.RelocateStatus) testgeneric.Option {
	return testgeneric.WithAnnotation(
		constants.RelocateAnnotation,
		fmt.Sprintf("%s/%s", clusterRelocateName, status),
	)
}
