package utils

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	testfake "github.com/openshift/hive/pkg/test/fake"
	teststatefulset "github.com/openshift/hive/pkg/test/statefulset"
	"github.com/openshift/hive/pkg/util/scheme"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

const stsName = "some-controller"

func TestIsUIDAssignedToMe(t *testing.T) {
	scheme := scheme.GetScheme()

	cases := []struct {
		name         string
		statefulSet  runtime.Object
		uid          types.UID
		myOrdinalID  int64
		expectedIsMe bool
		expectedErr  bool
	}{
		{
			name:        "missing sts",
			expectedErr: true,
		},
		{
			name:        "sts replicas not set",
			expectedErr: true,
			statefulSet: teststatefulset.FullBuilder("hive", stsName, scheme).Build(),
		},
		{
			name:        "sts replicas not same as currentReplicas",
			expectedErr: true,
			statefulSet: teststatefulset.FullBuilder("hive", stsName, scheme).Build(
				teststatefulset.WithCurrentReplicas(1),
				teststatefulset.WithReplicas(3),
			),
		},
		{
			name:        "assigned to me - ordinal 0",
			myOrdinalID: 0,
			statefulSet: teststatefulset.FullBuilder("hive", stsName, scheme).Build(
				teststatefulset.WithCurrentReplicas(3),
				teststatefulset.WithReplicas(3),
			),
			uid:          types.UID("1138528c-c36e-11e9-a1a7-42010a800195"),
			expectedIsMe: true,
		},
		{
			name:        "not assigned to me - ordinal 0",
			myOrdinalID: 0,
			statefulSet: teststatefulset.FullBuilder("hive", stsName, scheme).Build(
				teststatefulset.WithCurrentReplicas(3),
				teststatefulset.WithReplicas(3),
			),
			uid: types.UID("1138528c-c36e-11e9-a1a7-42010a800196"),
		},
		{
			name:         "assigned to me - ordinal 1",
			myOrdinalID:  1,
			expectedIsMe: true,
			statefulSet: teststatefulset.FullBuilder("hive", stsName, scheme).Build(
				teststatefulset.WithCurrentReplicas(3),
				teststatefulset.WithReplicas(3),
			),
			uid: types.UID("1138528c-c36e-11e9-a1a7-42010a800196"),
		},
		{
			name:        "not assigned to me - ordinal 1",
			myOrdinalID: 1,
			statefulSet: teststatefulset.FullBuilder("hive", stsName, scheme).Build(
				teststatefulset.WithCurrentReplicas(3),
				teststatefulset.WithReplicas(3),
			),
			uid: types.UID("1138528c-c36e-11e9-a1a7-42010a800197"),
		},
		{
			name:         "assigned to me - ordinal 2",
			myOrdinalID:  2,
			expectedIsMe: true,
			statefulSet: teststatefulset.FullBuilder("hive", stsName, scheme).Build(
				teststatefulset.WithCurrentReplicas(3),
				teststatefulset.WithReplicas(3),
			),
			uid: types.UID("1138528c-c36e-11e9-a1a7-42010a800197"),
		},
		{
			name:        "not assigned to me - ordinal 2",
			myOrdinalID: 2,
			statefulSet: teststatefulset.FullBuilder("hive", stsName, scheme).Build(
				teststatefulset.WithCurrentReplicas(3),
				teststatefulset.WithReplicas(3),
			),
			uid:         types.UID("1138528c-c36e-11e9-a1a7-42010a800198"),
			expectedErr: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logger := log.New()
			logger.SetLevel(log.DebugLevel)

			existing := []runtime.Object{}
			if tc.statefulSet != nil {
				existing = append(existing, tc.statefulSet)
			}
			c := testfake.NewFakeClientBuilder().WithRuntimeObjects(existing...).Build()

			actualIsMe, actualErr := IsUIDAssignedToMe(c, stsName, tc.uid, tc.myOrdinalID, logger)

			assert.Equal(t, tc.expectedIsMe, actualIsMe)
			if tc.expectedErr {
				assert.Error(t, actualErr, "Didn't error when was expected to.")
			} else {
				assert.NoError(t, actualErr, "Got error when not expected.")
			}
		})
	}
}
