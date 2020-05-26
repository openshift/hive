package clusterrelocate

import (
	"context"
	"reflect"
	"testing"

	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/remoteclient"
	remoteclientmock "github.com/openshift/hive/pkg/remoteclient/mock"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	testcr "github.com/openshift/hive/pkg/test/clusterrelocate"
	testcm "github.com/openshift/hive/pkg/test/configmap"
	testdnszone "github.com/openshift/hive/pkg/test/dnszone"
	testgeneric "github.com/openshift/hive/pkg/test/generic"
	testjob "github.com/openshift/hive/pkg/test/job"
	testmp "github.com/openshift/hive/pkg/test/machinepool"
	testnamespace "github.com/openshift/hive/pkg/test/namespace"
	testsecret "github.com/openshift/hive/pkg/test/secret"
	testsip "github.com/openshift/hive/pkg/test/syncidentityprovider"
	testss "github.com/openshift/hive/pkg/test/syncset"
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

	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	batchv1.AddToScheme(scheme)
	hivev1.AddToScheme(scheme)

	cdBuilder := testcd.FullBuilder(namespace, cdName, scheme).GenericOptions(
		testgeneric.WithLabel(labelKey, labelValue),
		testgeneric.WithFinalizer(hivev1.FinalizerDeprovision),
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
		name              string
		cd                *hivev1.ClusterDeployment
		srcResources      []runtime.Object
		destResources     []runtime.Object
		expectedResources []runtime.Object
	}{
		{
			name: "no relocation",
			cd:   cdBuilder.Build(),
		},
		{
			name: "only clusterdeployment",
			cd:   cdBuilder.Build(),
			srcResources: []runtime.Object{
				crBuilder.Build(),
			},
			expectedResources: []runtime.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(),
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
			expectedResources: []runtime.Object{
				namespaceBuilder.Build(),
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
			expectedResources: []runtime.Object{
				namespaceBuilder.Build(),
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
			expectedResources: []runtime.Object{
				namespaceBuilder.Build(),
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
			expectedResources: []runtime.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(
					testcd.Generic(testgeneric.WithResourceVersion("some-rv")),
				),
			},
		},
		{
			name: "existing namespace",
			cd:   cdBuilder.Build(),
			srcResources: []runtime.Object{
				crBuilder.Build(),
			},
			destResources: []runtime.Object{
				namespaceBuilder.Build(),
			},
			expectedResources: []runtime.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(),
			},
		},
		{
			name: "single dependent",
			cd:   cdBuilder.Build(),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				secretBuilder.Build(testsecret.WithDataKeyValue("test-key", []byte("test-data"))),
			},
			expectedResources: []runtime.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(),
				secretBuilder.Build(testsecret.WithDataKeyValue("test-key", []byte("test-data"))),
			},
		},
		{
			name: "multiple dependents",
			cd:   cdBuilder.Build(),
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
			expectedResources: []runtime.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(),
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
			cd:   cdBuilder.Build(),
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
			expectedResources: []runtime.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(),
				secretBuilder.Build(
					testsecret.WithName("test-secret-1"),
					testsecret.WithDataKeyValue("test-key-1", []byte("test-data-1")),
				),
			},
		},
		{
			name: "existing dependent",
			cd:   cdBuilder.Build(),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				secretBuilder.Build(testsecret.WithDataKeyValue("test-key", []byte("test-data"))),
			},
			destResources: []runtime.Object{
				secretBuilder.Build(testsecret.WithDataKeyValue("test-key", []byte("test-data"))),
			},
			expectedResources: []runtime.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(),
				secretBuilder.Build(testsecret.WithDataKeyValue("test-key", []byte("test-data"))),
			},
		},
		{
			name: "out-of-date dependent",
			cd:   cdBuilder.Build(),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				secretBuilder.Build(testsecret.WithDataKeyValue("test-key", []byte("test-data"))),
			},
			destResources: []runtime.Object{
				secretBuilder.Build(testsecret.WithDataKeyValue("test-key", []byte("other-data"))),
			},
			expectedResources: []runtime.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(),
				secretBuilder.Build(testsecret.WithDataKeyValue("test-key", []byte("test-data"))),
			},
		},
		{
			name: "existing dependent with different status",
			cd:   cdBuilder.Build(),
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
			expectedResources: []runtime.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(),
				mpBuilder.Build(),
			},
		},
		{
			name: "existing dependent with different instance-specific metadata",
			cd:   cdBuilder.Build(),
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
			expectedResources: []runtime.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(),
				secretBuilder.Build(
					testsecret.Generic(testgeneric.WithResourceVersion("some-rv")),
					testsecret.WithDataKeyValue("test-key", []byte("test-data")),
				),
			},
		},
		{
			name: "ignore service account token",
			cd:   cdBuilder.Build(),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				secretBuilder.Build(
					testsecret.WithType(corev1.SecretTypeServiceAccountToken),
					testsecret.WithDataKeyValue("test-key", []byte("test-data")),
				),
			},
			expectedResources: []runtime.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(),
			},
		},
		{
			name: "ignore secret owned by service account token",
			cd:   cdBuilder.Build(),
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
			expectedResources: []runtime.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(),
			},
		},
		{
			name: "configmap",
			cd:   cdBuilder.Build(),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				cmBuilder.Build(testcm.WithDataKeyValue("test-key", "test-data")),
			},
			expectedResources: []runtime.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(),
				cmBuilder.Build(testcm.WithDataKeyValue("test-key", "test-data")),
			},
		},
		{
			name: "machinepool",
			cd:   cdBuilder.Build(),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				mpBuilder.Build(),
			},
			expectedResources: []runtime.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(),
				mpBuilder.Build(),
			},
		},
		{
			name: "syncset",
			cd:   cdBuilder.Build(),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				ssBuilder.Build(),
			},
			expectedResources: []runtime.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(),
				ssBuilder.Build(),
			},
		},
		{
			name: "syncidentityprovider",
			cd:   cdBuilder.Build(),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				sipBuilder.Build(),
			},
			expectedResources: []runtime.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(),
				sipBuilder.Build(),
			},
		},
		{
			name: "dnszone",
			cd:   cdBuilder.Build(),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				dnsZoneBuilder.Build(),
			},
			expectedResources: []runtime.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(),
				dnsZoneBuilder.Build(),
			},
		},
		{
			name: "non-child dnszone",
			cd:   cdBuilder.Build(),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				dnsZoneBuilder.Build(testdnszone.Generic(testgeneric.WithName("other-dnszone"))),
			},
			expectedResources: []runtime.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(),
			},
		},
		{
			name: "non-dependent",
			cd:   cdBuilder.Build(),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				jobBuilder.Build(),
			},
			expectedResources: []runtime.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(),
			},
		},
		{
			name: "full",
			cd:   cdBuilder.Build(),
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
			expectedResources: []runtime.Object{
				namespaceBuilder.Build(),
				cdBuilder.Build(),
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
			},
		},
		{
			name: "no match",
			cd:   cdBuilder.Build(),
			srcResources: []runtime.Object{
				crBuilder.Build(testcr.WithClusterDeploymentSelector("other-key", "other-value")),
			},
		},
		{
			name: "managed DNS",
			cd: cdBuilder.Build(
				func(cd *hivev1.ClusterDeployment) {
					cd.Spec.ManageDNS = true
				},
			),
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
			srcClient := fake.NewFakeClientWithScheme(scheme, tc.srcResources...)
			destClient := fake.NewFakeClientWithScheme(scheme, tc.destResources...)

			cds := &hivev1.ClusterDeploymentList{}
			if err := srcClient.List(context.Background(), cds); err != nil {
				require.NoError(t, err, "failed to list clusterdeployments")
			}

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockRemoteClientBuilder := remoteclientmock.NewMockBuilder(mockCtrl)
			mockRemoteClientBuilder.EXPECT().Build().Return(destClient, nil).AnyTimes()

			reconciler := &ReconcileClusterRelocate{
				Client: srcClient,
				scheme: scheme,
				logger: logger,
				remoteClusterAPIClientBuilder: func(secret *corev1.Secret) remoteclient.Builder {
					assert.Equal(t, kubeconfigSecret, secret, "unexpected secret passed to remote client builder")
					return mockRemoteClientBuilder
				},
			}
			_, err := reconciler.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      cdName,
					Namespace: namespace,
				},
			})
			require.NoError(t, err, "unexpected error during reconcile")

			for _, obj := range tc.expectedResources {
				objKey, err := client.ObjectKeyFromObject(obj)
				if !assert.NoError(t, err, "unexpected error getting object key") {
					continue
				}
				destObj := reflect.New(reflect.TypeOf(obj).Elem()).Interface().(runtime.Object)
				err = destClient.Get(context.Background(), objKey, destObj)
				if !assert.NoError(t, err, "unexpected error getting destination object") {
					continue
				}
				assert.Equal(t, obj, destObj, "destination object different than expected object")
			}
		})
	}
}

func TestReconcileClusterRelocate_Reconcile_CDState(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.DebugLevel)

	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	hivev1.AddToScheme(scheme)

	cdBuilder := testcd.FullBuilder(namespace, cdName, scheme).GenericOptions(
		testgeneric.WithLabel(labelKey, labelValue),
	)
	crBuilder := testcr.FullBuilder(crName, scheme).Options(
		testcr.WithKubeconfigSecret(kubeconfigNamespace, kubeconfigName),
		testcr.WithClusterDeploymentSelector(labelKey, labelValue),
	)

	cases := []struct {
		name                              string
		cd                                *hivev1.ClusterDeployment
		missingKubeconfigSecret           bool
		srcResources                      []runtime.Object
		destResources                     []runtime.Object
		expectedError                     bool
		expectedRelocatingAnnotation      bool
		expectedRelocatedAnnotation       bool
		expectedDeletionTimestamp         bool
		expectedRelocationFailedCondition *hivev1.ClusterDeploymentCondition
		validate                          func(t *testing.T, cd *hivev1.ClusterDeployment)
	}{
		{
			name: "fresh clusterdeployment",
			cd:   cdBuilder.Build(),
			srcResources: []runtime.Object{
				crBuilder.Build(),
			},
			expectedRelocatedAnnotation: true,
			expectedDeletionTimestamp:   true,
		},
		{
			name: "switch relocators",
			cd: cdBuilder.Build(
				testcd.Generic(testgeneric.WithAnnotation(constants.RelocatingAnnotation, "other-relocator")),
			),
			srcResources: []runtime.Object{
				crBuilder.Build(),
			},
			missingKubeconfigSecret:      true,
			expectedError:                true,
			expectedRelocatingAnnotation: true,
			expectedRelocationFailedCondition: &hivev1.ClusterDeploymentCondition{
				Status: corev1.ConditionTrue,
				Reason: "MoveFailed",
			},
		},
		{
			name: "multiple relocators",
			cd:   cdBuilder.Build(),
			srcResources: []runtime.Object{
				crBuilder.Build(),
				crBuilder.Build(
					testcr.Generic(testgeneric.WithName("other-relocator")),
				),
			},
			expectedRelocationFailedCondition: &hivev1.ClusterDeploymentCondition{
				Status: corev1.ConditionTrue,
				Reason: "MultipleMatchingRelocators",
			},
		},
		{
			name: "already relocated clusterdeployment",
			cd: cdBuilder.Build(
				testcd.Generic(testgeneric.WithAnnotation(constants.RelocatedAnnotation, crName)),
			),
			srcResources: []runtime.Object{
				crBuilder.Build(),
			},
			expectedRelocatedAnnotation: true,
			expectedDeletionTimestamp:   true,
		},
		{
			name: "already moved clusterdeployment",
			cd:   cdBuilder.Build(),
			srcResources: []runtime.Object{
				crBuilder.Build(),
			},
			destResources: []runtime.Object{
				cdBuilder.Build(),
			},
			expectedRelocatedAnnotation: true,
			expectedDeletionTimestamp:   true,
		},
		{
			name: "clusterdeployment mismatch",
			cd: cdBuilder.Build(
				func(cd *hivev1.ClusterDeployment) {
					cd.Spec.BaseDomain = "test-domain"
				},
			),
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
			expectedRelocatingAnnotation: true,
			expectedRelocationFailedCondition: &hivev1.ClusterDeploymentCondition{
				Status: corev1.ConditionTrue,
				Reason: "ClusterDeploymentMismatch",
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
			if !tc.missingKubeconfigSecret {
				tc.srcResources = append(tc.srcResources, kubeconfigSecret)
			}
			srcClient := &deleteBlockingClientWrapper{fake.NewFakeClientWithScheme(scheme, tc.srcResources...)}
			destClient := fake.NewFakeClientWithScheme(scheme, tc.destResources...)

			cds := &hivev1.ClusterDeploymentList{}
			if err := srcClient.List(context.Background(), cds); err != nil {
				require.NoError(t, err, "failed to list clusterdeployments")
			}

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockRemoteClientBuilder := remoteclientmock.NewMockBuilder(mockCtrl)
			mockRemoteClientBuilder.EXPECT().Build().Return(destClient, nil).AnyTimes()

			reconciler := &ReconcileClusterRelocate{
				Client: srcClient,
				scheme: scheme,
				logger: logger,
				remoteClusterAPIClientBuilder: func(secret *corev1.Secret) remoteclient.Builder {
					assert.Equal(t, kubeconfigSecret, secret, "unexpected secret passed to remote client builder")
					return mockRemoteClientBuilder
				},
			}
			_, err := reconciler.Reconcile(reconcile.Request{
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

			cd := &hivev1.ClusterDeployment{}
			err = srcClient.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: cdName}, cd)
			require.NoError(t, err, "unexpected error fetching clusterdeployment")

			if tc.expectedRelocatingAnnotation {
				assert.Equal(t, crName, cd.Annotations[constants.RelocatingAnnotation], "unexpected relocating annotation")
			} else {
				assert.NotContains(t, cd.Annotations, constants.RelocatingAnnotation, "unexpected relocating annotation")
			}

			if tc.expectedRelocatedAnnotation {
				assert.Equal(t, crName, cd.Annotations[constants.RelocatedAnnotation], "unexpected relocated annotation")
			} else {
				assert.NotContains(t, cd.Annotations, constants.RelocatedAnnotation, "unexpected relocated annotation")
			}

			if tc.expectedDeletionTimestamp {
				assert.NotNil(t, cd.DeletionTimestamp, "expected ClusterDeployment to be deleted")
			} else {
				assert.Nil(t, cd.DeletionTimestamp, "expected ClusterDeployment to not be deleted")
			}

			cond := controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.RelocationFailedCondition)
			if tc.expectedRelocationFailedCondition != nil {
				if assert.NotNil(t, cond, "missing relocating condition") {
					assert.Equal(t, tc.expectedRelocationFailedCondition.Status, cond.Status, "unexpected condition status")
					assert.Equal(t, tc.expectedRelocationFailedCondition.Reason, cond.Reason, "unexpected condition reason")
				}
			} else {
				assert.Nil(t, cond, "unexpected relocation failed condition")
			}

			if tc.validate != nil {
				tc.validate(t, cd)
			}
		})
	}
}
