package syncidentityprovider

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	openshiftapiv1 "github.com/openshift/api/config/v1"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/hive/apis"
	hivev1 "github.com/openshift/hive/apis/hive/v1"

	"github.com/openshift/hive/pkg/constants"
)

const (
	ssidpName  = "ssidp-gh"
	ssidpName2 = "ssidp-gh2"
	sidpName   = "sidp-gh"
	sidpName2  = "sidp-gh2"
)

var (
	labelMap = map[string]string{"company": "giantcorp"}

	emptySyncSet = func() hivev1.SyncSet {
		return hivev1.SyncSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "someclusterdeployment-idp",
				Namespace: "default",
				Labels: map[string]string{
					constants.ClusterDeploymentNameLabel: clusterDeploymentWithLabels(labelMap).Name,
					constants.SyncSetTypeLabel:           constants.SyncSetTypeIdentityProvider,
				},
			},
			Spec: hivev1.SyncSetSpec{
				SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
					Patches: []hivev1.SyncObjectPatch{
						{
							APIVersion: "config.openshift.io/v1",
							Kind:       "OAuth",
							Name:       "cluster",
							PatchType:  "merge",
							Patch:      generatePatch([]openshiftapiv1.IdentityProvider{}),
						},
					},
				},
				ClusterDeploymentRefs: []corev1.LocalObjectReference{
					{
						Name: "someclusterdeployment",
					},
				},
			},
		}
	}

	syncSetAsPointer = func(syncSet hivev1.SyncSet) *hivev1.SyncSet {
		return &syncSet
	}

	syncSetWithIdentityProviders = func(idps ...openshiftapiv1.IdentityProvider) hivev1.SyncSet {
		retval := emptySyncSet()
		retval.Spec.Patches[0].Patch = generatePatch(idps)
		return retval
	}

	emptyClusterDeployment = func() *hivev1.ClusterDeployment {
		return &hivev1.ClusterDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "someclusterdeployment",
				Namespace: "default",
			},
		}
	}

	clusterDeploymentWithLabels = func(labelMap map[string]string) *hivev1.ClusterDeployment {
		cd := emptyClusterDeployment()
		cd.ObjectMeta.Labels = labelMap
		return cd
	}

	emptySyncIdentityProvider = func(prefix string) *hivev1.SyncIdentityProvider {
		return &hivev1.SyncIdentityProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      prefix + "somesyncidentityprovider",
				Namespace: "default",
			},
			Spec: hivev1.SyncIdentityProviderSpec{
				ClusterDeploymentRefs: []corev1.LocalObjectReference{},
			},
		}
	}

	emptySelectorSyncIdentityProvider = func(prefix string) *hivev1.SelectorSyncIdentityProvider {
		return &hivev1.SelectorSyncIdentityProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      prefix + "someselectorsyncidentityprovider",
				Namespace: "default",
			},
			Spec: hivev1.SelectorSyncIdentityProviderSpec{},
		}
	}

	syncIdentityProvidersThatReferencesEmptyClusterDeployment = func(prefix string, idps ...openshiftapiv1.IdentityProvider) *hivev1.SyncIdentityProvider {
		retval := emptySyncIdentityProvider(prefix)
		retval.Spec.ClusterDeploymentRefs = append(retval.Spec.ClusterDeploymentRefs, corev1.LocalObjectReference{
			Name: "someclusterdeployment",
		})
		retval.Spec.IdentityProviders = append(retval.Spec.IdentityProviders, idps...)
		return retval
	}

	selectorSyncIdentityProviders = func(prefix string, idps ...openshiftapiv1.IdentityProvider) *hivev1.SelectorSyncIdentityProvider {
		retval := emptySelectorSyncIdentityProvider(prefix)
		retval.Spec.ClusterDeploymentSelector = metav1.LabelSelector{
			MatchLabels: labelMap,
		}
		retval.Spec.IdentityProviders = append(retval.Spec.IdentityProviders, idps...)

		return retval
	}

	githubIdentityProvider = func(name string) openshiftapiv1.IdentityProvider {
		return openshiftapiv1.IdentityProvider{
			Name:          name,
			MappingMethod: "claim",
			IdentityProviderConfig: openshiftapiv1.IdentityProviderConfig{
				Type: openshiftapiv1.IdentityProviderTypeGitHub,
				GitHub: &openshiftapiv1.GitHubIdentityProvider{
					ClientID: "NUNYA",
					ClientSecret: openshiftapiv1.SecretNameReference{
						Name: "foo-github-client-secret",
					},
					Organizations: []string{
						"openshift",
					},
					Teams: []string{
						"openshift/team-cluster-operator",
					},
				},
			},
		}
	}
)

func TestSyncIdentityProviderWatchHandler(t *testing.T) {
	tests := []struct {
		name                 string
		syncIdentityProvider *hivev1.SyncIdentityProvider
		expectedRequestList  []reconcile.Request
	}{
		{
			name:                "Empty",
			expectedRequestList: []reconcile.Request{},
		},
		{
			name: "Happy Path",
			syncIdentityProvider: &hivev1.SyncIdentityProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sync1",
					Namespace: "default",
				},
				Spec: hivev1.SyncIdentityProviderSpec{
					ClusterDeploymentRefs: []corev1.LocalObjectReference{
						{
							Name: "someclusterdeployment",
						},
					},
				},
			},
			expectedRequestList: []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name:      "someclusterdeployment",
						Namespace: "default",
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			reconciler := ReconcileSyncIdentityProviders{
				logger: log.New(),
			}

			// Act
			actualRequestList := reconciler.syncIdentityProviderWatchHandler(context.TODO(), test.syncIdentityProvider)

			// Assert
			assert.True(t, reflect.DeepEqual(test.expectedRequestList, actualRequestList))
		})
	}
}

func TestSelectorSyncIdentityProviderWatchHandler(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name                         string
		selectorSyncIdentityProvider *hivev1.SelectorSyncIdentityProvider
		existing                     []runtime.Object
		expectedRequestList          []reconcile.Request
	}{
		{
			name:                "Empty",
			expectedRequestList: []reconcile.Request{},
		},
		{
			name: "Happy Path",
			selectorSyncIdentityProvider: &hivev1.SelectorSyncIdentityProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sync1",
					Namespace: "default",
				},
				Spec: hivev1.SelectorSyncIdentityProviderSpec{
					ClusterDeploymentSelector: metav1.LabelSelector{
						MatchLabels: labelMap,
					},
				},
			},
			existing: []runtime.Object{
				clusterDeploymentWithLabels(labelMap),
			},
			expectedRequestList: []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name:      "someclusterdeployment",
						Namespace: "default",
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			r := &ReconcileSyncIdentityProviders{
				Client: fake.NewClientBuilder().WithRuntimeObjects(test.existing...).Build(),
				scheme: scheme.Scheme,
				logger: log.WithField("controller", "syncidentityprovider"),
			}

			// Act
			actualRequestList := r.selectorSyncIdentityProviderWatchHandler(test.selectorSyncIdentityProvider)

			// Assert
			assert.True(t, reflect.DeepEqual(test.expectedRequestList, actualRequestList))
		})
	}
}

func TestReconcile(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name                   string
		watchedObjectName      string
		watchedObjectNamespace string
		existing               []runtime.Object
		expectedResult         reconcile.Result
		expectedError          error
		expectedSyncSetList    hivev1.SyncSetList
	}{
		{
			name:                   "Watched Object Not Found",
			existing:               []runtime.Object{},
			watchedObjectName:      "this-will-not-be-found",
			watchedObjectNamespace: "not-real-ns",
			expectedResult: reconcile.Result{
				Requeue:      false,
				RequeueAfter: 0,
			},
			expectedError:       nil,
			expectedSyncSetList: hivev1.SyncSetList{},
		},
		{
			name: "One Empty ClusterDeployment",
			existing: []runtime.Object{
				emptyClusterDeployment(),
			},
			watchedObjectName:      "someclusterdeployment",
			watchedObjectNamespace: "default",
			expectedResult: reconcile.Result{
				Requeue:      false,
				RequeueAfter: 0,
			},
			expectedError: nil,
			expectedSyncSetList: hivev1.SyncSetList{
				Items: []hivev1.SyncSet{},
			},
		},
		{
			name: "One Empty SyncIdentityProvider with existing sync set.",
			existing: []runtime.Object{
				emptyClusterDeployment(),
				emptySyncIdentityProvider(sidpName),
				syncSetAsPointer(syncSetWithIdentityProviders()),
			},
			watchedObjectName:      "someclusterdeployment",
			watchedObjectNamespace: "default",
			expectedResult: reconcile.Result{
				Requeue:      false,
				RequeueAfter: 0,
			},
			expectedError: nil,
			expectedSyncSetList: hivev1.SyncSetList{
				Items: []hivev1.SyncSet{
					emptySyncSet(),
				},
			},
		},
		{
			name: "One Filled SyncIdentityProvider",
			existing: []runtime.Object{
				emptyClusterDeployment(),
				syncIdentityProvidersThatReferencesEmptyClusterDeployment(sidpName, githubIdentityProvider(sidpName)),
			},
			watchedObjectName:      "someclusterdeployment",
			watchedObjectNamespace: "default",
			expectedResult: reconcile.Result{
				Requeue:      false,
				RequeueAfter: 0,
			},
			expectedError: nil,
			expectedSyncSetList: hivev1.SyncSetList{
				Items: []hivev1.SyncSet{
					syncSetWithIdentityProviders(githubIdentityProvider(sidpName)),
				},
			},
		},
		{
			name: "One Filled SelectorSyncIdentityProvider",
			existing: []runtime.Object{
				clusterDeploymentWithLabels(labelMap),
				selectorSyncIdentityProviders(ssidpName, githubIdentityProvider(ssidpName)),
			},
			watchedObjectName:      "someclusterdeployment",
			watchedObjectNamespace: "default",
			expectedResult: reconcile.Result{
				Requeue:      false,
				RequeueAfter: 0,
			},
			expectedError: nil,
			expectedSyncSetList: hivev1.SyncSetList{
				Items: []hivev1.SyncSet{
					syncSetWithIdentityProviders(githubIdentityProvider(ssidpName)),
				},
			},
		},
		{
			name: "One SyncIdentityProvider and One SelectorSyncIdentityProvider",
			existing: []runtime.Object{
				clusterDeploymentWithLabels(labelMap),
				syncIdentityProvidersThatReferencesEmptyClusterDeployment(sidpName, githubIdentityProvider(sidpName)),
				selectorSyncIdentityProviders(ssidpName, githubIdentityProvider(ssidpName)),
			},
			watchedObjectName:      "someclusterdeployment",
			watchedObjectNamespace: "default",
			expectedResult: reconcile.Result{
				Requeue:      false,
				RequeueAfter: 0,
			},
			expectedError: nil,
			expectedSyncSetList: hivev1.SyncSetList{
				Items: []hivev1.SyncSet{
					syncSetWithIdentityProviders(githubIdentityProvider(ssidpName), githubIdentityProvider(sidpName)),
				},
			},
		},
		{
			name: "Multiple SyncIdentityProvider and Multiple SelectorSyncIdentityProvider",
			existing: []runtime.Object{
				clusterDeploymentWithLabels(labelMap),
				syncIdentityProvidersThatReferencesEmptyClusterDeployment(sidpName, githubIdentityProvider(sidpName)),
				syncIdentityProvidersThatReferencesEmptyClusterDeployment(sidpName2, githubIdentityProvider(sidpName2)),
				selectorSyncIdentityProviders(ssidpName, githubIdentityProvider(ssidpName)),
				selectorSyncIdentityProviders(ssidpName2, githubIdentityProvider(ssidpName2)),
			},
			watchedObjectName:      "someclusterdeployment",
			watchedObjectNamespace: "default",
			expectedResult: reconcile.Result{
				Requeue:      false,
				RequeueAfter: 0,
			},
			expectedError: nil,
			expectedSyncSetList: hivev1.SyncSetList{
				Items: []hivev1.SyncSet{
					syncSetWithIdentityProviders(
						githubIdentityProvider(ssidpName),
						githubIdentityProvider(ssidpName2),
						githubIdentityProvider(sidpName),
						githubIdentityProvider(sidpName2),
					),
				},
			},
		},
		{
			name: "Existing SyncSet, no update",
			existing: []runtime.Object{
				clusterDeploymentWithLabels(labelMap),
				selectorSyncIdentityProviders(ssidpName, githubIdentityProvider(ssidpName)),
				syncSetAsPointer(syncSetWithIdentityProviders(githubIdentityProvider(ssidpName))),
			},
			watchedObjectName:      "someclusterdeployment",
			watchedObjectNamespace: "default",
			expectedResult: reconcile.Result{
				Requeue:      false,
				RequeueAfter: 0,
			},
			expectedError: nil,
			expectedSyncSetList: hivev1.SyncSetList{
				Items: []hivev1.SyncSet{
					syncSetWithIdentityProviders(githubIdentityProvider(ssidpName)),
				},
			},
		},
		{
			name: "Existing SyncSet, with update",
			existing: []runtime.Object{
				clusterDeploymentWithLabels(labelMap),
				selectorSyncIdentityProviders(ssidpName, githubIdentityProvider(ssidpName)),
				syncSetAsPointer(emptySyncSet()),
			},
			watchedObjectName:      "someclusterdeployment",
			watchedObjectNamespace: "default",
			expectedResult: reconcile.Result{
				Requeue:      false,
				RequeueAfter: 0,
			},
			expectedError: nil,
			expectedSyncSetList: hivev1.SyncSetList{
				Items: []hivev1.SyncSet{
					syncSetWithIdentityProviders(githubIdentityProvider(ssidpName)),
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			r := &ReconcileSyncIdentityProviders{
				Client: fake.NewClientBuilder().WithRuntimeObjects(test.existing...).Build(),
				scheme: scheme.Scheme,
				logger: log.WithField("controller", "syncidentityprovider"),
			}

			// Act
			result, err := r.Reconcile(context.TODO(), reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      test.watchedObjectName,
					Namespace: test.watchedObjectNamespace,
				},
			})

			ssList := &hivev1.SyncSetList{}
			ssListErr := r.Client.List(context.TODO(), ssList)

			// Assert
			assert.Equal(t, test.expectedResult.Requeue, result.Requeue)
			assert.Equal(t, test.expectedResult.RequeueAfter, result.RequeueAfter)
			assert.Equal(t, test.expectedError, err)
			assert.Nil(t, ssListErr)
			assert.True(t, areSyncSetSpecsEqual(t, &test.expectedSyncSetList, ssList))
			assertSyncSetLabelsCorrect(t, ssList)
		})
	}
}

func assertSyncSetLabelsCorrect(t *testing.T, actual *hivev1.SyncSetList) {
	for ix := range actual.Items {
		labels := actual.Items[ix].Labels
		assert.Equal(t, clusterDeploymentWithLabels(labelMap).Name, labels[constants.ClusterDeploymentNameLabel], "incorrect cluster deployment name label")
		assert.Equal(t, constants.SyncSetTypeIdentityProvider, labels[constants.SyncSetTypeLabel], "incorrect syncset type label")
	}
}

func areSyncSetSpecsEqual(t *testing.T, expected, actual *hivev1.SyncSetList) bool {
	if len(expected.Items) != len(actual.Items) {
		// They aren't the same size, they can't be the same.
		return false
	}

	for ix := range expected.Items {
		// We only compare the spec
		if !reflect.DeepEqual(expected.Items[ix].Spec, actual.Items[ix].Spec) {
			expectedStr := string(expected.Items[ix].Spec.Patches[0].Patch)
			actualStr := string(actual.Items[ix].Spec.Patches[0].Patch)
			log.WithField("Expected", expectedStr).Info("Expected")
			log.WithField("Actual", actualStr).Info("Actual")

			// The spec's don't match
			return false
		}
	}

	return true
}

func generatePatch(identityProviders []openshiftapiv1.IdentityProvider) string {
	idpp := identityProviderPatch{
		Spec: identityProviderPatchSpec{
			IdentityProviders: identityProviders,
		},
	}

	// We're swallowing the error because it should never error since we're controlling the input.
	idppRaw, _ := json.Marshal(idpp)

	return string(idppRaw)
}
