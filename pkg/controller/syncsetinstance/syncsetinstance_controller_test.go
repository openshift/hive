/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package syncsetinstance

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/hive/pkg/apis"
	"github.com/openshift/hive/pkg/apis/helpers"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/remoteclient"
	remoteclientmock "github.com/openshift/hive/pkg/remoteclient/mock"
	"github.com/openshift/hive/pkg/resource"
)

const (
	testName                 = "foo"
	testNamespace            = "default"
	adminKubeconfigSecret    = "foo-admin-kubeconfig"
	adminKubeconfigSecretKey = "kubeconfig"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestSyncSetReconcile(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	tenMinutesAgo := time.Unix(metav1.NewTime(time.Now().Add(-10*time.Minute)).Unix(), 0)

	tests := []struct {
		name                   string
		status                 hivev1.SyncSetInstanceStatus
		syncSet                *hivev1.SyncSet
		deletedSyncSet         *hivev1.SyncSet
		selectorSyncSet        *hivev1.SelectorSyncSet
		deletedSelectorSyncSet *hivev1.SelectorSyncSet
		existingObjs           []runtime.Object
		clusterDeployment      *hivev1.ClusterDeployment
		validate               func(*testing.T, *hivev1.SyncSetInstance)
		isDeleted              bool
		expectDeleted          []deletedItemInfo
		expectSSIDeleted       bool
		expectRequeue          bool
		expectApplied          bool
	}{
		{
			name:    "Create single resource successfully",
			syncSet: testSyncSetWithResources("ss1", testCM("cm1", "foo", "bar")),
			validate: func(t *testing.T, ssi *hivev1.SyncSetInstance) {
				validateSyncSetInstanceStatus(t, ssi.Status,
					successfulResourceStatus(testCM("cm1", "foo", "bar")))
			},
			expectApplied: true,
		},
		{
			name:    "Create multiple resources successfully",
			syncSet: testSyncSetWithResources("foo", testCMs("bar", 5)...),
			validate: func(t *testing.T, ssi *hivev1.SyncSetInstance) {
				validateSyncSetInstanceStatus(t, ssi.Status,
					successfulResourceStatus(testCMs("bar", 5)...))
			},
			expectApplied: true,
		},
		{
			name:    "Update single resource",
			status:  successfulResourceStatus(testCM("cm1", "key1", "value1")),
			syncSet: testSyncSetWithResources("ss1", testCM("cm1", "key1", "value***changed")),
			validate: func(t *testing.T, ssi *hivev1.SyncSetInstance) {
				validateSyncSetInstanceStatus(t, ssi.Status,
					successfulResourceStatus(testCM("cm1", "key1", "value***changed")))
			},
			expectApplied: true,
		},
		{
			name: "Update only resources that have changed",
			status: successfulResourceStatusWithTime(
				[]runtime.Object{
					testCM("cm1", "key1", "value1"),
					testCM("cm2", "key2", "value2"),
					testCM("cm3", "key3", "value3"),
				},
				metav1.NewTime(tenMinutesAgo)),
			syncSet: testSyncSetWithResources("aaa",
				testCM("cm1", "key1", "value1"),
				testCM("cm2", "key2", "value***changed"),
				testCM("cm3", "key3", "value3"),
			),
			validate: func(t *testing.T, ssi *hivev1.SyncSetInstance) {
				validateSyncSetInstanceStatus(t, ssi.Status, successfulResourceStatus(
					testCM("cm1", "key1", "value1"),
					testCM("cm2", "key2", "value***changed"),
					testCM("cm3", "key3", "value3"),
				))
				unchanged := []hivev1.SyncStatus{
					ssi.Status.Resources[0],
					ssi.Status.Resources[2],
				}
				for _, ss := range unchanged {
					if ss.Conditions[0].LastProbeTime.Time.Unix() != tenMinutesAgo.Unix() {
						t.Errorf("unexpected condition last probe time for resource %s/%s. Got: %v, Expected: %v", ss.Namespace, ss.Name, ss.Conditions[0].LastProbeTime.Time, tenMinutesAgo)
					}
					if ss.Conditions[0].LastTransitionTime.Time.Unix() != tenMinutesAgo.Unix() {
						t.Errorf("unexpected condition last transition time for resource %s/%s. Got: %v, Expected: %v", ss.Namespace, ss.Name, ss.Conditions[0].LastTransitionTime.Time, tenMinutesAgo)
					}
				}
				changed := ssi.Status.Resources[1]
				if changed.Conditions[0].LastProbeTime.Time.Unix() <= tenMinutesAgo.Unix() {
					t.Errorf("unexpected condition last probe time for resource %s/%s. Got: %v, Expected a more recent time", changed.Namespace, changed.Name, changed.Conditions[0].LastProbeTime.Time)
				}
				// The last transition time should not have changed because we went from successful to successful
				if changed.Conditions[0].LastTransitionTime.Time.Unix() != tenMinutesAgo.Unix() {
					t.Errorf("unexpected condition last transition time for resource %s/%s. Got: %v, Expected: %v", changed.Namespace, changed.Name, changed.Conditions[0].LastTransitionTime.Time, tenMinutesAgo)
				}
			},
			expectApplied: true,
		},
		{
			name: "Check for failed info call, set condition",
			syncSet: testSyncSetWithResources("foo",
				testCM("cm1", "key1", "value1"),
				testCM("info-error", "key2", "value2"),
			),
			validate: func(t *testing.T, ssi *hivev1.SyncSetInstance) {
				validateUnknownObjectCondition(t, ssi.Status)
			},
			expectRequeue: true,
		},
		{
			name: "Stop applying resources when error occurs",
			syncSet: testSyncSetWithResources("foo",
				testCM("cm1", "key1", "value1"),
				testCM("cm2", "key2", "value2"),
				testCM("apply-error", "key3", "value3"),
				testCM("cm4", "key4", "value4"),
			),
			validate: func(t *testing.T, ssi *hivev1.SyncSetInstance) {
				status := successfulResourceStatus(
					testCM("cm1", "key1", "value1"),
					testCM("cm2", "key2", "value2"),
				)
				status.Resources = append(status.Resources, applyFailedResourceStatus("foo",
					testCM("apply-error", "key3", "value3"),
				).Resources...)
				validateSyncSetInstanceStatus(t, ssi.Status, status)
			},
			expectRequeue: true,
		},
		{
			name: "Stop applying resources when have annotation: hive.openshift.io/syncset-pause=true",
			clusterDeployment: func() *hivev1.ClusterDeployment {
				cd := testClusterDeployment()
				cd.Annotations = map[string]string{}
				cd.Annotations[constants.SyncsetPauseAnnotation] = "true"
				return cd
			}(),
			syncSet: testSyncSetWithResources("foo",
				testCM("cm1", "key1", "value1"),
			),
			validate: func(t *testing.T, ssi *hivev1.SyncSetInstance) {
				if ssi.Status.Conditions != nil {
					t.Fatalf("conditions should be nil")
				}
			},
		},
		{
			name: "selectorsyncset: apply single resource",
			selectorSyncSet: testSelectorSyncSetWithResources("foo",
				testCM("cm1", "key1", "value1"),
			),
			validate: func(t *testing.T, ssi *hivev1.SyncSetInstance) {
				validateSyncSetInstanceStatus(t, ssi.Status, successfulResourceStatus(
					testCM("cm1", "key1", "value1"),
				))
			},
			expectApplied: true,
		},
		{
			name:    "Apply single patch successfully",
			syncSet: testSyncSetWithPatches("ss1", testSyncObjectPatch("foo", "bar", "baz", "v1", "value1")),
			validate: func(t *testing.T, ssi *hivev1.SyncSetInstance) {
				validateSyncSetInstanceStatus(t, ssi.Status, successfulPatchStatus(
					[]hivev1.SyncObjectPatch{
						testSyncObjectPatch("foo", "bar", "baz", "v1", "value1"),
					}))
			},
			expectApplied: true,
		},
		{
			name: "Apply multiple patches successfully",
			syncSet: testSyncSetWithPatches("ss1",
				testSyncObjectPatch("foo", "bar", "baz", "v1", "value1"),
				testSyncObjectPatch("chicken", "potato", "stew", "v1", "value2"),
			),
			validate: func(t *testing.T, ssi *hivev1.SyncSetInstance) {
				validateSyncSetInstanceStatus(t, ssi.Status, successfulPatchStatus(
					[]hivev1.SyncObjectPatch{
						testSyncObjectPatch("foo", "bar", "baz", "v1", "value1"),
						testSyncObjectPatch("chicken", "potato", "stew", "v1", "value2"),
					},
				))
			},
			expectApplied: true,
		},
		{
			name: "Reapply single patch",
			status: successfulPatchStatus([]hivev1.SyncObjectPatch{
				testSyncObjectPatch("foo", "bar", "baz", "v1", "value1"),
			}),
			syncSet: testSyncSetWithPatches("ss1",
				testSyncObjectPatch("foo", "bar", "baz", "v1", "value1***changed"),
			),
			validate: func(t *testing.T, ssi *hivev1.SyncSetInstance) {
				validateSyncSetInstanceStatus(t, ssi.Status, successfulPatchStatus(
					[]hivev1.SyncObjectPatch{
						testSyncObjectPatch("foo", "bar", "baz", "v1", "value1***changed"),
					},
				))
			},
			expectApplied: true,
		},
		{
			name: "Stop applying patches when error occurs",
			syncSet: testSyncSetWithPatches("ss1",
				testSyncObjectPatch("thing1", "bar", "baz", "v1", "value1"),
				testSyncObjectPatch("thing2", "bar", "baz", "v1", "value1"),
				testSyncObjectPatch("thing3", "bar", "baz", "v1", "patch-error"),
				testSyncObjectPatch("thing4", "bar", "baz", "v1", "value1"),
			),
			validate: func(t *testing.T, ssi *hivev1.SyncSetInstance) {
				status := successfulPatchStatus([]hivev1.SyncObjectPatch{
					testSyncObjectPatch("thing1", "bar", "baz", "v1", "value1"),
					testSyncObjectPatch("thing2", "bar", "baz", "v1", "value1"),
				})
				status.Patches = append(status.Patches, failedPatchStatus("ss1", []hivev1.SyncObjectPatch{
					testSyncObjectPatch("thing3", "bar", "baz", "v1", "patch-error"),
				}).Patches...)
				validateSyncSetInstanceStatus(t, ssi.Status, status)
			},
			expectRequeue: true,
		},
		{
			name: "No patches applied when resource error occurs",
			syncSet: testSyncSet("ss1",
				[]runtime.Object{
					testCM("cm1", "key1", "value1"),
					testCM("cm2", "key2", "value2"),
					testCM("apply-error", "key3", "value3"),
					testCM("cm4", "key4", "value4"),
				},
				[]hivev1.SyncObjectPatch{
					testSyncObjectPatch("thing1", "bar", "baz", "v1", "value1"),
					testSyncObjectPatch("thing2", "bar", "baz", "v1", "value2"),
				}),
			validate: func(t *testing.T, ssi *hivev1.SyncSetInstance) {
				status := successfulResourceStatus(
					testCM("cm1", "key1", "value1"),
					testCM("cm2", "key2", "value2"),
				)
				status.Resources = append(status.Resources, applyFailedResourceStatus("ss1",
					testCM("apply-error", "key3", "value3"),
				).Resources...)
				validateSyncSetInstanceStatus(t, ssi.Status, status)
			},
			expectRequeue: true,
		},
		{
			name: "resource sync mode, remove resources",
			status: successfulResourceStatus(
				testCM("cm1", "key1", "value1"),
				testCM("cm2", "key2", "value2"),
				testCM("cm3", "key3", "value3"),
				testCM("cm4", "key4", "value4"),
			),
			syncSet: func() *hivev1.SyncSet {
				ss := testSyncSetWithResources("aaa",
					testCM("cm1", "key1", "value1"),
					testCM("cm3", "key3", "value3"),
				)
				ss.Spec.ResourceApplyMode = hivev1.SyncResourceApplyMode
				return ss
			}(),
			expectDeleted: []deletedItemInfo{
				deletedItem("cm2", "ConfigMap"),
				deletedItem("cm4", "ConfigMap"),
			},
			expectApplied: true,
		},
		{
			name: "delete syncset instance when syncset doesn't exist",
			deletedSyncSet: testSyncSetWithResources("aaa",
				testCM("cm1", "key1", "value1"),
			),
			expectSSIDeleted: true,
		},
		{
			name: "delete syncset instance when selectorsyncset doesn't exist",
			deletedSelectorSyncSet: testSelectorSyncSetWithResources("aaa",
				testCM("cm1", "key1", "value1"),
			),
			expectSSIDeleted: true,
		},
		{
			name: "cleanup deleted syncset resources",
			deletedSyncSet: func() *hivev1.SyncSet {
				ss := testSyncSetWithResources("aaa",
					testCM("cm1", "key1", "value1"),
					testCM("cm1", "key1", "value1"),
				)
				ss.Spec.ResourceApplyMode = hivev1.SyncResourceApplyMode
				return ss
			}(),
			isDeleted: true,
			status: successfulResourceStatus(
				testCM("cm1", "key1", "value1"),
				testCM("cm2", "key2", "value2"),
			),
			expectDeleted: []deletedItemInfo{
				deletedItem("cm1", "ConfigMap"),
				deletedItem("cm2", "ConfigMap"),
			},
		},
		{
			name: "Apply single secret successfully",
			existingObjs: []runtime.Object{
				testSecret("foo", "bar"),
			},
			syncSet: testSyncSetWithSecretMappings("ss1", testSecretMapping("foo")),
			validate: func(t *testing.T, ssi *hivev1.SyncSetInstance) {
				validateSyncSetInstanceStatus(t, ssi.Status,
					successfulSecretStatus(testSecret("foo", "bar")))
			},
			expectApplied: true,
		},
		{
			name: "Skip syncset with secret references in another namespace",
			existingObjs: []runtime.Object{
				func() *corev1.Secret {
					s := testSecret("foo", "bar")
					s.Namespace = "anotherns"
					return s
				}(),
			},
			syncSet: func() *hivev1.SyncSet {
				secretMapping := testSecretMapping("foo")
				secretMapping.SourceRef.Namespace = "anotherns"
				return testSyncSetWithSecretMappings("ss1", secretMapping)
			}(),
			expectApplied: false,
		},
		{
			name:    "Local secret does not exist",
			syncSet: testSyncSetWithSecretMappings("ss1", testSecretMapping("foo")),
			validate: func(t *testing.T, ssi *hivev1.SyncSetInstance) {
				status := hivev1.SyncSetInstanceStatus{}
				status.Secrets = append(status.Secrets, applyFailedSecretStatus("ss1",
					testSecret("foo", "bar"),
				).Secrets...)
				validateSyncSetInstanceStatus(t, ssi.Status, status)
			},
			expectRequeue: true,
		},
		{
			name: "Local secret has OwnerReference",
			existingObjs: []runtime.Object{
				testSecretWithOwner("foo", "bar"),
			},
			syncSet: testSyncSetWithSecretMappings("ss1", testSecretMapping("foo")),
			validate: func(t *testing.T, ssi *hivev1.SyncSetInstance) {
				validateSyncSetInstanceStatus(t, ssi.Status,
					successfulSecretStatus(
						testSecret("foo", "bar"),
					))
			},
			expectApplied: true,
		},
		{
			name: "Apply multiple secrets successfully",
			existingObjs: []runtime.Object{
				testSecret("foo", "bar"),
				testSecret("baz", "bar"),
			},
			syncSet: testSyncSetWithSecretMappings("ss1", testSecretMapping("foo"), testSecretMapping("baz")),
			validate: func(t *testing.T, ssi *hivev1.SyncSetInstance) {
				validateSyncSetInstanceStatus(t, ssi.Status,
					successfulSecretStatus(
						testSecret("foo", "bar"),
						testSecret("baz", "bar"),
					))
			},
			expectApplied: true,
		},
		{
			name: "Update single secret",
			existingObjs: []runtime.Object{
				testSecret("foo", "data_value***changed"),
			},
			status:  successfulSecretStatus(testSecret("key", "data_value")),
			syncSet: testSyncSetWithSecretMappings("ss1", testSecretMapping("foo")),
			validate: func(t *testing.T, ssi *hivev1.SyncSetInstance) {
				validateSyncSetInstanceStatus(t, ssi.Status,
					successfulSecretStatus(testSecret("foo", "data_value***changed")))
			},
			expectApplied: true,
		},
		{
			name: "Update only secrets that have changed",
			existingObjs: []runtime.Object{
				testSecret("s1", "value1"),
				testSecret("s2", "value2***changed"),
				testSecret("s3", "value3"),
			},
			status: successfulSecretStatusWithTime(
				[]runtime.Object{
					testSecret("s1", "value1"),
					testSecret("s2", "value2"),
					testSecret("s3", "value3"),
				},
				metav1.NewTime(tenMinutesAgo)),
			syncSet: testSyncSetWithSecretMappings("aaa",
				testSecretMapping("s1"),
				testSecretMapping("s2"),
				testSecretMapping("s3"),
			),
			validate: func(t *testing.T, ssi *hivev1.SyncSetInstance) {
				validateSyncSetInstanceStatus(t, ssi.Status, successfulSecretStatus(
					testSecret("s1", "value1"),
					testSecret("s2", "value2***changed"),
					testSecret("s3", "value3"),
				))
				unchanged := []hivev1.SyncStatus{
					ssi.Status.Secrets[0],
					ssi.Status.Secrets[2],
				}
				for _, ss := range unchanged {
					if ss.Conditions[0].LastProbeTime.Time.Unix() != tenMinutesAgo.Unix() {
						t.Errorf("unexpected condition last probe time for resource %s/%s. Got: %v, Expected: %v", ss.Namespace, ss.Name, ss.Conditions[0].LastProbeTime.Time, tenMinutesAgo)
					}
					if ss.Conditions[0].LastTransitionTime.Time.Unix() != tenMinutesAgo.Unix() {
						t.Errorf("unexpected condition last transition time for resource %s/%s. Got: %v, Expected: %v", ss.Namespace, ss.Name, ss.Conditions[0].LastTransitionTime.Time, tenMinutesAgo)
					}
				}
				changed := ssi.Status.Secrets[1]
				if changed.Conditions[0].LastProbeTime.Time.Unix() <= tenMinutesAgo.Unix() {
					t.Errorf("unexpected condition last probe time for resource %s/%s. Got: %v, Expected a more recent time", changed.Namespace, changed.Name, changed.Conditions[0].LastProbeTime.Time)
				}
				// The last transition time should not have changed because we went from successful to successful
				if changed.Conditions[0].LastTransitionTime.Time.Unix() != tenMinutesAgo.Unix() {
					t.Errorf("unexpected condition last transition time for resource %s/%s. Got: %v, Expected: %v", changed.Namespace, changed.Name, changed.Conditions[0].LastTransitionTime.Time, tenMinutesAgo)
				}
			},
			expectApplied: true,
		},
		{
			name: "resource sync mode, remove secrets",
			existingObjs: []runtime.Object{
				testSecret("foo1", "bar"),
				testSecret("foo2", "bar"),
				testSecret("foo3", "bar"),
				testSecret("foo4", "bar"),
			},
			status: successfulSecretStatus(
				testSecret("foo1", "bar"),
				testSecret("foo2", "bar"),
				testSecret("foo3", "bar"),
				testSecret("foo4", "bar"),
			),
			syncSet: func() *hivev1.SyncSet {
				ss := testSyncSetWithSecretMappings("aaa",
					testSecretMapping("foo1"),
					testSecretMapping("foo3"),
				)
				ss.Spec.ResourceApplyMode = hivev1.SyncResourceApplyMode
				return ss
			}(),
			expectDeleted: []deletedItemInfo{
				deletedItem("foo2", secretsResource),
				deletedItem("foo4", secretsResource),
			},
			expectApplied: true,
		},
		{
			name: "selectorsyncset: apply single secret",
			existingObjs: []runtime.Object{
				testSecret("s1", "value1"),
			},
			selectorSyncSet: testMatchingSelectorSyncSetWithSecrets("foo",
				testSecretMapping("s1"),
			),
			validate: func(t *testing.T, ssi *hivev1.SyncSetInstance) {
				validateSyncSetInstanceStatus(t, ssi.Status, successfulSecretStatus(
					testSecret("s1", "value1"),
				))
			},
			expectApplied: true,
		},
		{
			name: "syncset: fail to delete resource",
			status: successfulResourceStatus(
				testCM("cm1", "key1", "value1"),
				testCM("delete-error", "key2", "value2"),
			),
			syncSet: func() *hivev1.SyncSet {
				ss := testSyncSetWithResources("aaa", testCM("cm1", "key1", "value1"))
				ss.Spec.ResourceApplyMode = hivev1.SyncResourceApplyMode
				return ss
			}(),
			validate: func(t *testing.T, ssi *hivev1.SyncSetInstance) {
				status := successfulResourceStatus(
					testCM("cm1", "key1", "value1"),
				)
				status.Resources = append(status.Resources, deleteFailedResourceStatus("aaa",
					testCM("delete-error", "key2", "value2"),
				).Resources...)
				validateSyncSetInstanceStatus(t, ssi.Status, status)
			},
			expectApplied: true,
		},
		{
			name: "syncset: fail to delete secret reference secret",
			existingObjs: []runtime.Object{
				testSecret("foo", "bar"),
			},
			status: successfulSecretStatus(
				testSecret("foo", "bar"),
				testSecret("delete-error", "baz"),
			),
			syncSet: func() *hivev1.SyncSet {
				ss := testSyncSetWithSecretMappings("aaa", testSecretMapping("foo"))
				ss.Spec.ResourceApplyMode = hivev1.SyncResourceApplyMode
				return ss
			}(),
			validate: func(t *testing.T, ssi *hivev1.SyncSetInstance) {
				status := successfulSecretStatus(testSecret("foo", "bar"))
				status.Secrets = append(status.Secrets, deleteFailedSecretStatus("aaa",
					testSecret("delete-error", "baz"),
				).Secrets...)
				validateSyncSetInstanceStatus(t, ssi.Status, status)
			},
			expectApplied: true,
		},
		{
			name: "cleanup deleted syncset secrets",
			deletedSyncSet: func() *hivev1.SyncSet {
				ss := testSyncSetWithSecretMappings("aaa",
					testSecretMapping("foo"),
					testSecretMapping("bar"),
				)
				ss.Spec.ResourceApplyMode = hivev1.SyncResourceApplyMode
				return ss
			}(),
			isDeleted: true,
			status: successfulSecretStatus(
				testSecret("foo", "bar"),
				testSecret("bar", "baz"),
			),
			expectDeleted: []deletedItemInfo{
				deletedItem("foo", secretsResource),
				deletedItem("bar", secretsResource),
			},
		},
	}

	for _, test := range tests {
		apis.AddToScheme(scheme.Scheme)
		t.Run(test.name, func(t *testing.T) {
			var ssi *hivev1.SyncSetInstance
			cd := testClusterDeployment()
			if test.clusterDeployment != nil {
				cd = test.clusterDeployment
			}
			runtimeObjs := []runtime.Object{cd, kubeconfigSecret()}
			runtimeObjs = append(runtimeObjs, test.existingObjs...)

			switch {
			case test.deletedSyncSet != nil:
				ssi = syncSetInstanceForSyncSet(cd, test.deletedSyncSet)
			case test.deletedSelectorSyncSet != nil:
				ssi = syncSetInstanceForSelectorSyncSet(cd, test.deletedSelectorSyncSet)
			case test.syncSet != nil:
				ssi = syncSetInstanceForSyncSet(cd, test.syncSet)
				runtimeObjs = append(runtimeObjs, test.syncSet)
			case test.selectorSyncSet != nil:
				ssi = syncSetInstanceForSelectorSyncSet(cd, test.selectorSyncSet)
				runtimeObjs = append(runtimeObjs, test.selectorSyncSet)
			}
			controllerutils.AddFinalizer(ssi, hivev1.FinalizerSyncSetInstance)
			ssi.Status = test.status
			if test.isDeleted {
				now := metav1.Now()
				ssi.DeletionTimestamp = &now
			}

			runtimeObjs = append(runtimeObjs, ssi)
			fakeClient := fake.NewFakeClient(runtimeObjs...)
			dynamicClient := &fakeDynamicClient{}

			helper := &fakeHelper{t: t}

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockRemoteClientBuilder := remoteclientmock.NewMockBuilder(mockCtrl)
			mockRemoteClientBuilder.EXPECT().RESTConfig().Return(nil, nil).AnyTimes()
			mockRemoteClientBuilder.EXPECT().BuildDynamic().Return(dynamicClient, nil).AnyTimes()

			r := &ReconcileSyncSetInstance{
				Client:                        fakeClient,
				scheme:                        scheme.Scheme,
				logger:                        log.WithField("controller", "syncset"),
				applierBuilder:                helper.newHelper,
				hash:                          fakeHashFunc(t),
				remoteClusterAPIClientBuilder: func(*hivev1.ClusterDeployment) remoteclient.Builder { return mockRemoteClientBuilder },
				reapplyInterval:               2 * time.Hour,
			}
			rec, err := r.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      ssi.Name,
					Namespace: ssi.Namespace,
				},
			})

			assert.Equal(t, err, nil)

			assert.Equal(t, test.expectRequeue, rec.Requeue)

			validateDeletedItems(t, dynamicClient.deletedItems, test.expectDeleted)
			if test.expectSSIDeleted {
				result := &hivev1.SyncSetInstance{}
				err := fakeClient.Get(context.TODO(), types.NamespacedName{Name: ssi.Name, Namespace: ssi.Namespace}, result)
				if !errors.IsNotFound(err) {
					t.Errorf("expected syncsetinstance to be deleted")
				}
				return
			}

			result := &hivev1.SyncSetInstance{}
			err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: ssi.Name, Namespace: ssi.Namespace}, result)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if test.validate != nil {
				test.validate(t, result)
			}

			assert.Equal(t, test.expectApplied, result.Status.Applied)
		})
	}
}

func sameDeletedItem(a, b deletedItemInfo) bool {
	return a.name == b.name &&
		a.namespace == b.namespace &&
		a.group == b.group &&
		a.version == b.version &&
		a.resource == b.resource
}

func validateDeletedItems(t *testing.T, actual, expected []deletedItemInfo) {
	if len(actual) != len(expected) {
		t.Errorf("unexpected number of deleted items, actual: %d, expected: %d", len(actual), len(expected))
		return
	}
	for _, item := range actual {
		index := -1
		for i, expectedItem := range expected {
			if sameDeletedItem(item, expectedItem) {
				index = i
				break
			}
		}
		if index == -1 {
			t.Errorf("unexpected deleted item: %#v", item)
			return
		}
		// remove the item from the expected array
		expected = append(expected[0:index], expected[index+1:]...)
	}
}

func testClusterDeployment() *hivev1.ClusterDeployment {
	cd := hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testName,
			Namespace: testNamespace,
			Labels: map[string]string{
				"region": "us-east-1",
			},
		},
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterMetadata: &hivev1.ClusterMetadata{
				AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: adminKubeconfigSecret},
			},
			Installed: true,
		},
		Status: hivev1.ClusterDeploymentStatus{
			Conditions: []hivev1.ClusterDeploymentCondition{{
				Type:   hivev1.UnreachableCondition,
				Status: corev1.ConditionFalse,
			}},
		},
	}

	return &cd
}

func testSyncSet(name string, resources []runtime.Object, patches []hivev1.SyncObjectPatch) *hivev1.SyncSet {
	ss := &hivev1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: hivev1.SyncSetSpec{
			ClusterDeploymentRefs: []corev1.LocalObjectReference{
				{
					Name: testName,
				},
			},
		},
	}
	for _, r := range resources {
		ss.Spec.Resources = append(ss.Spec.Resources, runtime.RawExtension{
			Object: r,
		})
	}
	for _, p := range patches {
		ss.Spec.Patches = append(ss.Spec.Patches, p)
	}
	return ss
}

func testSyncSetWithResources(name string, resources ...runtime.Object) *hivev1.SyncSet {
	ss := &hivev1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: hivev1.SyncSetSpec{
			ClusterDeploymentRefs: []corev1.LocalObjectReference{
				{
					Name: testName,
				},
			},
		},
	}
	for _, r := range resources {
		ss.Spec.Resources = append(ss.Spec.Resources, runtime.RawExtension{
			Object: r,
		})
	}
	return ss
}

func testSyncSetWithPatches(name string, patches ...hivev1.SyncObjectPatch) *hivev1.SyncSet {
	ss := &hivev1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: hivev1.SyncSetSpec{
			ClusterDeploymentRefs: []corev1.LocalObjectReference{
				{
					Name: testName,
				},
			},
		},
	}
	for _, p := range patches {
		ss.Spec.Patches = append(ss.Spec.Patches, p)
	}
	return ss
}

func testSyncSetWithSecretMappings(name string, mappings ...hivev1.SecretMapping) *hivev1.SyncSet {
	ss := &hivev1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: hivev1.SyncSetSpec{
			ClusterDeploymentRefs: []corev1.LocalObjectReference{
				{
					Name: testName,
				},
			},
		},
	}
	ss.Spec.Secrets = append(ss.Spec.Secrets, mappings...)
	return ss
}

func testSyncObjectPatch(name, namespace, kind, apiVersion string, value string) hivev1.SyncObjectPatch {
	patch := fmt.Sprintf("{'spec': {'key: '%v'}}", value)
	return hivev1.SyncObjectPatch{
		Name:       name,
		Namespace:  namespace,
		Kind:       kind,
		APIVersion: apiVersion,
		Patch:      patch,
		PatchType:  "merge",
	}
}

func testSelectorSyncSetWithResources(name string, resources ...runtime.Object) *hivev1.SelectorSyncSet {
	ss := &hivev1.SelectorSyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: hivev1.SelectorSyncSetSpec{
			ClusterDeploymentSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"region": "us-east-1"},
			},
		},
	}
	for _, r := range resources {
		ss.Spec.Resources = append(ss.Spec.Resources, runtime.RawExtension{
			Object: r,
		})
	}
	return ss
}

func testMatchingSelectorSyncSetWithSecrets(name string, mappings ...hivev1.SecretMapping) *hivev1.SelectorSyncSet {
	return testSelectorSyncSetWithSecrets(name, map[string]string{"region": "us-east-1"}, mappings...)
}

func testSelectorSyncSetWithSecrets(name string, matchLabels map[string]string, mappings ...hivev1.SecretMapping) *hivev1.SelectorSyncSet {
	ss := &hivev1.SelectorSyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: hivev1.SelectorSyncSetSpec{
			ClusterDeploymentSelector: metav1.LabelSelector{
				MatchLabels: matchLabels,
			},
		},
	}
	ss.Spec.Secrets = append(ss.Spec.Secrets, mappings...)
	return ss
}

func testCM(name, key, value string) runtime.Object {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			Annotations: map[string]string{
				"hash": fmt.Sprintf("%s=%s", key, value),
			},
		},
		Data: map[string]string{
			key: value,
		},
	}
}

func deletedItem(name, resource string) deletedItemInfo {
	return deletedItemInfo{
		name:      name,
		namespace: testNamespace,
		group:     "",
		version:   "v1",
		resource:  resource,
	}
}

func testCMs(prefix string, count int) []runtime.Object {
	result := []runtime.Object{}
	for i := 0; i < count; i++ {
		key := fmt.Sprintf("%s-key-%d", prefix, i)
		value := fmt.Sprintf("%s-value-%d", prefix, i)
		result = append(result, testCM(fmt.Sprintf("%s-%d", prefix, i), key, value))
	}
	return result
}

func testSecretMapping(name string) hivev1.SecretMapping {
	return hivev1.SecretMapping{
		SourceRef: hivev1.SecretReference{
			Name:      name,
			Namespace: testNamespace,
		},
		TargetRef: hivev1.SecretReference{
			Name:      name,
			Namespace: testNamespace,
		},
	}
}

func testSecret(name, data string) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			testName: []byte(data),
		},
	}
}

func testSecretWithOwner(name, data string) *corev1.Secret {
	secret := testSecret(name, data)
	secret.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: "foo.io/v1",
			Kind:       "Foo",
		},
	}
	return secret
}

func kubeconfigSecret() *corev1.Secret {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      adminKubeconfigSecret,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			adminKubeconfigSecretKey: []byte("foo"),
		},
	}
	return s
}

func applyFailedResourceStatus(name string, resources ...runtime.Object) hivev1.SyncSetObjectStatus {
	conditionTime := metav1.Now()
	status := hivev1.SyncSetObjectStatus{
		Name: name,
	}
	for _, r := range resources {
		obj, _ := meta.Accessor(r)
		status.Resources = append(status.Resources, hivev1.SyncStatus{
			APIVersion: r.GetObjectKind().GroupVersionKind().GroupVersion().String(),
			Kind:       r.GetObjectKind().GroupVersionKind().Kind,
			Resource:   r.GetObjectKind().GroupVersionKind().Kind,
			Name:       obj.GetName(),
			Namespace:  obj.GetNamespace(),
			Hash:       objectHash(obj),
			Conditions: []hivev1.SyncCondition{
				{
					Type:               hivev1.ApplyFailureSyncCondition,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: conditionTime,
					LastProbeTime:      conditionTime,
				},
			},
		})
	}
	return status
}

func applyFailedSecretStatus(name string, secrets ...runtime.Object) hivev1.SyncSetObjectStatus {
	conditionTime := metav1.Now()
	status := hivev1.SyncSetObjectStatus{
		Name: name,
	}
	for _, s := range secrets {
		obj, _ := meta.Accessor(s)
		status.Secrets = append(status.Secrets, hivev1.SyncStatus{
			APIVersion: s.GetObjectKind().GroupVersionKind().GroupVersion().String(),
			Kind:       s.GetObjectKind().GroupVersionKind().Kind,
			Resource:   secretsResource,
			Name:       obj.GetName(),
			Namespace:  obj.GetNamespace(),
			Hash:       "",
			Conditions: []hivev1.SyncCondition{
				{
					Type:               hivev1.ApplyFailureSyncCondition,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: conditionTime,
					LastProbeTime:      conditionTime,
				},
			},
		})
	}
	return status
}

func deleteFailedResourceStatus(name string, resources ...runtime.Object) hivev1.SyncSetObjectStatus {
	conditionTime := metav1.Now()
	status := hivev1.SyncSetObjectStatus{
		Name: name,
	}
	for _, r := range resources {
		obj, _ := meta.Accessor(r)
		status.Resources = append(status.Resources, hivev1.SyncStatus{
			APIVersion: r.GetObjectKind().GroupVersionKind().GroupVersion().String(),
			Kind:       r.GetObjectKind().GroupVersionKind().Kind,
			Resource:   r.GetObjectKind().GroupVersionKind().Kind,
			Name:       obj.GetName(),
			Namespace:  obj.GetNamespace(),
			Hash:       objectHash(obj),
			Conditions: []hivev1.SyncCondition{
				{
					Type:               hivev1.ApplySuccessSyncCondition,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: conditionTime,
					LastProbeTime:      conditionTime,
				},
				{
					Type:               hivev1.DeletionFailedSyncCondition,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: conditionTime,
					LastProbeTime:      conditionTime,
				},
			},
		})
	}
	return status
}

func deleteFailedSecretStatus(name string, secrets ...runtime.Object) hivev1.SyncSetObjectStatus {
	conditionTime := metav1.Now()
	status := hivev1.SyncSetObjectStatus{
		Name: name,
	}
	for _, s := range secrets {
		obj, _ := meta.Accessor(s)
		hash, _ := controllerutils.GetChecksumOfObject(s)
		status.Secrets = append(status.Secrets, hivev1.SyncStatus{
			APIVersion: s.GetObjectKind().GroupVersionKind().GroupVersion().String(),
			Kind:       s.GetObjectKind().GroupVersionKind().Kind,
			Resource:   secretsResource,
			Name:       obj.GetName(),
			Namespace:  obj.GetNamespace(),
			Hash:       hash,
			Conditions: []hivev1.SyncCondition{
				{
					Type:               hivev1.ApplySuccessSyncCondition,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: conditionTime,
					LastProbeTime:      conditionTime,
				},
				{
					Type:               hivev1.DeletionFailedSyncCondition,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: conditionTime,
					LastProbeTime:      conditionTime,
				},
			},
		})
	}
	return status
}

func failedPatchStatus(name string, patches []hivev1.SyncObjectPatch) hivev1.SyncSetObjectStatus {
	conditionTime := metav1.Now()
	status := hivev1.SyncSetObjectStatus{
		Name: name,
	}
	for _, p := range patches {
		b, _ := json.Marshal(p)
		status.Patches = append(status.Patches, hivev1.SyncStatus{
			APIVersion: p.APIVersion,
			Kind:       p.Kind,
			Name:       p.Name,
			Namespace:  p.Namespace,
			Hash:       fmt.Sprintf("%x", md5.Sum(b)),
			Conditions: []hivev1.SyncCondition{
				{
					Type:               hivev1.ApplyFailureSyncCondition,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: conditionTime,
					LastProbeTime:      conditionTime,
				},
			},
		})
	}
	return status
}

func successfulResourceStatus(resources ...runtime.Object) hivev1.SyncSetInstanceStatus {
	return successfulResourceStatusWithTime(resources, metav1.Now())
}

func successfulResourceStatusWithTime(resources []runtime.Object, conditionTime metav1.Time) hivev1.SyncSetInstanceStatus {
	status := hivev1.SyncSetInstanceStatus{}
	for _, r := range resources {
		obj, _ := meta.Accessor(r)
		status.Resources = append(status.Resources, hivev1.SyncStatus{
			APIVersion: r.GetObjectKind().GroupVersionKind().GroupVersion().String(),
			Kind:       r.GetObjectKind().GroupVersionKind().Kind,
			Resource:   r.GetObjectKind().GroupVersionKind().Kind,
			Name:       obj.GetName(),
			Namespace:  obj.GetNamespace(),
			Hash:       objectHash(obj),
			Conditions: []hivev1.SyncCondition{
				{
					Type:               hivev1.ApplySuccessSyncCondition,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: conditionTime,
					LastProbeTime:      conditionTime,
				},
			},
		})
	}
	return status
}

func successfulSecretStatus(secrets ...runtime.Object) hivev1.SyncSetInstanceStatus {
	return successfulSecretStatusWithTime(secrets, metav1.Now())
}

func successfulSecretStatusWithTime(secrets []runtime.Object, conditionTime metav1.Time) hivev1.SyncSetInstanceStatus {
	status := hivev1.SyncSetInstanceStatus{}
	for _, s := range secrets {
		obj, _ := meta.Accessor(s)
		hash, _ := controllerutils.GetChecksumOfObject(s)
		status.Secrets = append(status.Secrets, hivev1.SyncStatus{
			APIVersion: s.GetObjectKind().GroupVersionKind().GroupVersion().String(),
			Kind:       s.GetObjectKind().GroupVersionKind().Kind,
			Resource:   secretsResource,
			Name:       obj.GetName(),
			Namespace:  obj.GetNamespace(),
			Hash:       hash,
			Conditions: []hivev1.SyncCondition{
				{
					Type:               hivev1.ApplySuccessSyncCondition,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: conditionTime,
					LastProbeTime:      conditionTime,
				},
			},
		})
	}
	return status
}

func successfulPatchStatus(patches []hivev1.SyncObjectPatch) hivev1.SyncSetInstanceStatus {
	return successfulPatchStatusWithTime(patches, metav1.Now())
}

func successfulPatchStatusWithTime(patches []hivev1.SyncObjectPatch, conditionTime metav1.Time) hivev1.SyncSetInstanceStatus {
	status := hivev1.SyncSetInstanceStatus{}
	for _, p := range patches {
		b, _ := json.Marshal(p)
		status.Patches = append(status.Patches, hivev1.SyncStatus{
			APIVersion: p.APIVersion,
			Kind:       p.Kind,
			Name:       p.Name,
			Namespace:  p.Namespace,
			Hash:       fmt.Sprintf("%x", md5.Sum(b)),
			Conditions: []hivev1.SyncCondition{
				{
					Type:               hivev1.ApplySuccessSyncCondition,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: conditionTime,
					LastProbeTime:      conditionTime,
				},
			},
		})
	}
	return status
}

type fakeHelper struct {
	t *testing.T
}

func (f *fakeHelper) newHelper(*rest.Config, log.FieldLogger) Applier {
	return f
}

func (f *fakeHelper) ApplyRuntimeObject(object runtime.Object, scheme *runtime.Scheme) (resource.ApplyResult, error) {
	data, err := json.Marshal(object)
	if err != nil {
		return "", fmt.Errorf("cannot serialize runtime object: %s", err)
	}
	return f.Apply(data)
}

func (f *fakeHelper) Apply(data []byte) (resource.ApplyResult, error) {
	info, err := f.Info(data)
	if err != nil {
		return "", fmt.Errorf("cannot gather resource info: %s", err)
	}
	if info.Name == "apply-error" {
		return "", fmt.Errorf("cannot apply resource")
	}
	return resource.UnknownApplyResult, nil
}

func (f *fakeHelper) Info(data []byte) (*resource.Info, error) {
	r, obj, err := decode(f.t, data)
	if err != nil {
		return nil, fmt.Errorf("cannot decode object: %s", err)
	}
	// Special case when the object's name is info-error
	if obj.GetName() == "info-error" {
		return nil, fmt.Errorf("cannot determine info")
	}

	return &resource.Info{
		Name:       obj.GetName(),
		Namespace:  obj.GetNamespace(),
		Kind:       r.GetObjectKind().GroupVersionKind().Kind,
		APIVersion: r.GetObjectKind().GroupVersionKind().GroupVersion().String(),
		Resource:   r.GetObjectKind().GroupVersionKind().Kind,
	}, nil
}

func (f *fakeHelper) Patch(name types.NamespacedName, kind, apiVersion string, patch []byte, patchType string) error {
	p := string(patch)
	if strings.Contains(p, "patch-error") {
		return fmt.Errorf("cannot apply patch")
	}
	return nil
}

func validateSyncSetInstanceStatus(t *testing.T, actual, expected hivev1.SyncSetInstanceStatus) {
	if len(actual.Resources) != len(expected.Resources) {
		t.Errorf("number of resource statuses does not match, actual: %d, expected: %d", len(actual.Resources), len(expected.Resources))
		return
	}
	if len(actual.Patches) != len(expected.Patches) {
		t.Errorf("number of patch statuses does not match, actual %d, expected: %d", len(actual.Patches), len(expected.Patches))
	}
	if len(actual.Secrets) != len(expected.Secrets) {
		t.Errorf("number of secret reference statuses does not match, actual %d, expected: %d", len(actual.Secrets), len(expected.Secrets))
	}

	for _, actualResource := range actual.Resources {
		found := false
		for _, expectedResource := range expected.Resources {
			if matchesResourceStatus(actualResource, expectedResource) {
				found = true
				validateSyncStatus(t, actualResource, expectedResource)
				break
			}
		}
		if !found {
			t.Errorf("got unexpected resource status: %s/%s (kind: %s, apiVersion: %s)",
				actualResource.Namespace, actualResource.Name, actualResource.Kind, actualResource.APIVersion)
		}
	}

	for _, actualPatch := range actual.Patches {
		found := false
		for _, expectedPatch := range expected.Patches {
			if matchesPatchStatus(actualPatch, expectedPatch) {
				found = true
				validateSyncStatus(t, actualPatch, expectedPatch)
				break
			}
		}
		if !found {
			t.Errorf("got unexpected syncset patch status: %s/%s (kind: %s, apiVersion: %s)",
				actualPatch.Namespace, actualPatch.Name, actualPatch.Kind, actualPatch.APIVersion)
		}
	}

	for _, actualSecret := range actual.Secrets {
		found := false
		for _, expectedSecret := range expected.Secrets {
			if matchesSecretStatus(actualSecret, expectedSecret) {
				found = true
				validateSyncStatus(t, actualSecret, expectedSecret)
				break
			}
		}
		if !found {
			t.Errorf("got unexpected secret reference status: %s/%s (kind: %s, apiVersion: %s)",
				actualSecret.Namespace, actualSecret.Name, actualSecret.Kind, actualSecret.APIVersion)
		}
	}
}

func matchesResourceStatus(a, b hivev1.SyncStatus) bool {
	return a.Name == b.Name &&
		a.Namespace == b.Namespace &&
		a.Kind == b.Kind &&
		a.APIVersion == b.APIVersion
}

func matchesPatchStatus(a, b hivev1.SyncStatus) bool {
	return a.Name == b.Name &&
		a.Namespace == b.Namespace &&
		a.Kind == b.Kind &&
		a.APIVersion == b.APIVersion
}

func matchesSecretStatus(a, b hivev1.SyncStatus) bool {
	return a.Name == b.Name &&
		a.Namespace == b.Namespace &&
		a.Kind == b.Kind &&
		a.APIVersion == b.APIVersion
}

func validateSyncStatus(t *testing.T, actual, expected hivev1.SyncStatus) {
	if len(actual.Conditions) != len(expected.Conditions) {
		t.Errorf("number of conditions do not match for resource %s/%s (kind: %s, apiVersion: %s). Expected: %d, Actual: %d",
			actual.Namespace, actual.Name, actual.Kind, actual.APIVersion, len(expected.Conditions), len(actual.Conditions))
		return
	}
	if actual.Hash != expected.Hash {
		t.Errorf("hashes don't match for resource %s/%s (kind: %s, apiVersion: %s). Expected: %s, Actual: %s",
			actual.Namespace, actual.Name, actual.Kind, actual.APIVersion, expected.Hash, actual.Hash)
		return
	}
	for _, actualCondition := range actual.Conditions {
		found := false
		for _, expectedCondition := range expected.Conditions {
			if actualCondition.Type == expectedCondition.Type {
				found = true
				if actualCondition.Status != expectedCondition.Status {
					t.Errorf("condition does not match, resource %s/%s (kind: %s, apiVersion: %s), condition %s. Expected: %s, Actual: %s",
						actual.Namespace, actual.Name, actual.Kind, actual.APIVersion, actualCondition.Type, expectedCondition.Status, actualCondition.Status)
				}
			}
		}
		if !found {
			t.Errorf("got unexpected condition %s in resource %s/%s (kind: %s, apiVersion: %s)",
				actualCondition.Type, actual.Namespace, actual.Name, actual.Kind, actual.APIVersion)
		}
	}
}

func validateUnknownObjectCondition(t *testing.T, status hivev1.SyncSetInstanceStatus) {
	if len(status.Conditions) != 1 {
		t.Errorf("did not get the expected number of syncset level conditions (1)")
		return
	}
	condition := status.Conditions[0]
	if condition.Type != hivev1.UnknownObjectSyncCondition {
		t.Errorf("Unexpected type for syncset level condition: %s", condition.Type)
	}
	if condition.Status != corev1.ConditionTrue {
		t.Errorf("Unexpected condition status: %s", condition.Status)
	}
}

func decode(t *testing.T, data []byte) (runtime.Object, metav1.Object, error) {
	decoder := scheme.Codecs.UniversalDecoder(corev1.SchemeGroupVersion)
	r, _, err := decoder.Decode(data, nil, nil)
	if err != nil {
		return nil, nil, err
	}

	obj, err := meta.Accessor(r)
	if err != nil {
		return nil, nil, err
	}
	return r, obj, nil
}

func fakeHashFunc(t *testing.T) func([]byte) string {
	return func(data []byte) string {
		_, obj, err := decode(t, data)
		if err != nil {
			return fmt.Sprintf("%x", md5.Sum(data))
		}
		return objectHash(obj)
	}
}

func objectHash(obj metav1.Object) string {
	if annotations := obj.GetAnnotations(); annotations != nil {
		if hash, ok := annotations["hash"]; ok {
			return hash
		}
	}
	return fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName())
}

type deletedItemInfo struct {
	name      string
	namespace string
	resource  string
	group     string
	version   string
}

type fakeDynamicClient struct {
	deletedItems []deletedItemInfo
}

type fakeNamespaceableClient struct {
	client    *fakeDynamicClient
	resource  schema.GroupVersionResource
	namespace string
}

func (c *fakeDynamicClient) Resource(resource schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	return &fakeNamespaceableClient{
		client:   c,
		resource: resource,
	}
}

func (c *fakeNamespaceableClient) Namespace(ns string) dynamic.ResourceInterface {
	return &fakeNamespaceableClient{
		client:    c.client,
		resource:  c.resource,
		namespace: ns,
	}
}

func (c *fakeNamespaceableClient) Create(_ context.Context, obj *unstructured.Unstructured, options metav1.CreateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}

func (c *fakeNamespaceableClient) Update(_ context.Context, obj *unstructured.Unstructured, options metav1.UpdateOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}

func (c *fakeNamespaceableClient) UpdateStatus(_ context.Context, obj *unstructured.Unstructured, options metav1.UpdateOptions) (*unstructured.Unstructured, error) {
	return nil, nil
}

func (c *fakeNamespaceableClient) Delete(_ context.Context, name string, options metav1.DeleteOptions, subresources ...string) error {
	if name == "delete-error" {
		return fmt.Errorf("cannot delete resource")
	}

	c.client.deletedItems = append(c.client.deletedItems, deletedItemInfo{
		name:      name,
		namespace: c.namespace,
		resource:  c.resource.Resource,
		group:     c.resource.Group,
		version:   c.resource.Version,
	})
	return nil
}

func (c *fakeNamespaceableClient) DeleteCollection(_ context.Context, options metav1.DeleteOptions, listOptions metav1.ListOptions) error {
	return nil
}

func (c *fakeNamespaceableClient) Get(_ context.Context, name string, options metav1.GetOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}

func (c *fakeNamespaceableClient) List(_ context.Context, opts metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	return nil, nil
}

func (c *fakeNamespaceableClient) Watch(_ context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (c *fakeNamespaceableClient) Patch(_ context.Context, name string, pt types.PatchType, data []byte, options metav1.PatchOptions, subresources ...string) (*unstructured.Unstructured, error) {
	return nil, nil
}

func syncSetInstanceForSyncSet(cd *hivev1.ClusterDeployment, syncSet *hivev1.SyncSet) *hivev1.SyncSetInstance {
	ownerRef := metav1.NewControllerRef(cd, hivev1.SchemeGroupVersion.WithKind("ClusterDeployment"))
	hash := computeHash(syncSet.Spec)
	return &hivev1.SyncSetInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:            syncSetInstanceNameForSyncSet(cd, syncSet),
			Namespace:       cd.Namespace,
			OwnerReferences: []metav1.OwnerReference{*ownerRef},
		},
		Spec: hivev1.SyncSetInstanceSpec{
			ClusterDeploymentRef: corev1.LocalObjectReference{
				Name: cd.Name,
			},
			SyncSetRef: &corev1.LocalObjectReference{
				Name: syncSet.Name,
			},
			ResourceApplyMode: syncSet.Spec.ResourceApplyMode,
			SyncSetHash:       hash,
		},
	}
}

func syncSetInstanceForSelectorSyncSet(cd *hivev1.ClusterDeployment, selectorSyncSet *hivev1.SelectorSyncSet) *hivev1.SyncSetInstance {
	ownerRef := metav1.NewControllerRef(cd, hivev1.SchemeGroupVersion.WithKind("ClusterDeployment"))
	hash := computeHash(selectorSyncSet.Spec)
	return &hivev1.SyncSetInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:            syncSetInstanceNameForSelectorSyncSet(cd, selectorSyncSet),
			Namespace:       cd.Namespace,
			OwnerReferences: []metav1.OwnerReference{*ownerRef},
		},
		Spec: hivev1.SyncSetInstanceSpec{
			ClusterDeploymentRef: corev1.LocalObjectReference{
				Name: cd.Name,
			},
			SelectorSyncSetRef: &hivev1.SelectorSyncSetReference{
				Name: selectorSyncSet.Name,
			},
			ResourceApplyMode: selectorSyncSet.Spec.ResourceApplyMode,
			SyncSetHash:       hash,
		},
	}
}

func syncSetInstanceNameForSyncSet(cd *hivev1.ClusterDeployment, syncSet *hivev1.SyncSet) string {
	syncSetPart := helpers.GetName(syncSet.Name, "syncset", validation.DNS1123SubdomainMaxLength-validation.DNS1123LabelMaxLength)
	return fmt.Sprintf("%s-%s", cd.Name, syncSetPart)
}

func syncSetInstanceNameForSelectorSyncSet(cd *hivev1.ClusterDeployment, selectorSyncSet *hivev1.SelectorSyncSet) string {
	syncSetPart := helpers.GetName(selectorSyncSet.Name, "selector-syncset", validation.DNS1123SubdomainMaxLength-validation.DNS1123LabelMaxLength)
	return fmt.Sprintf("%s-%s", cd.Name, syncSetPart)
}

func computeHash(data interface{}) string {
	b, err := json.Marshal(data)
	if err != nil {
		log.Fatalf("error marshaling json: %v", err)
	}
	return fmt.Sprintf("%x", md5.Sum(b))
}

func TestSSIReApplyDuration(t *testing.T) {
	startProbe := time.Now()

	resources := []runtime.Object{
		testCM("foo1", "bar", "baz"),
		testCM("foo2", "bar", "baz"),
		testCM("foo3", "bar", "baz"),
		testCM("foo4", "bar", "baz"),
	}

	cd := testClusterDeployment()
	ss := testSyncSetWithResources("aaa", resources...)
	ssi := syncSetInstanceForSyncSet(cd, ss)
	ssi.Status = successfulResourceStatus(resources...)

	staleDurations := []time.Duration{
		10 * time.Minute,
		45 * time.Minute,
		90 * time.Minute,
		3 * time.Hour,
	}

	for i, staleDuration := range staleDurations {
		t.Run(fmt.Sprintf("%v", staleDuration), func(t *testing.T) {
			ssi.Status.Resources[i].Conditions[0].LastProbeTime = metav1.NewTime(startProbe.Add(-staleDuration))
			requeueAfter := (&ReconcileSyncSetInstance{reapplyInterval: defaultReapplyInterval}).ssiReapplyDuration(ssi)
			endProbe := time.Now()

			if staleDuration < defaultReapplyInterval {
				assert.LessOrEqual(t, requeueAfter.Seconds(), (defaultReapplyInterval-staleDuration).Seconds()+0.1*defaultReapplyInterval.Seconds())
				assert.GreaterOrEqual(t, requeueAfter.Seconds(), (defaultReapplyInterval - staleDuration - endProbe.Sub(startProbe)).Seconds())
			} else {
				assert.Zero(t, requeueAfter.Seconds())
			}
		})
	}
}

func TestFilterApplyError(t *testing.T) {
	err1 := "error when creating \"/tmp/apply-475927931\": namespaces \"openshift-am-config\" not found"
	expectedFilteredErr1 := "namespaces \"openshift-am-config\" not found"
	if filterApplyError(err1) != expectedFilteredErr1 {
		t.Errorf("expected temporary file to be trimmed from error message")
	}

	err2 := "error when creating \"/var/cache/kubectl/apply-475927931\": namespaces \"openshift-am-config\" not found"
	expectedFilteredErr2 := "namespaces \"openshift-am-config\" not found"
	if filterApplyError(err2) != expectedFilteredErr2 {
		t.Errorf("expected temporary file to be trimmed from error message")
	}

	err3 := "unable to parse"
	if filterApplyError(err3) != err3 {
		t.Errorf("expected error message to be unchanged")
	}
}
