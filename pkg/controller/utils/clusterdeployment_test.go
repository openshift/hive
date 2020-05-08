package utils

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/test/clusterdeployment"
	"github.com/openshift/hive/pkg/test/generic"
)

func TestShouldSyncCluster(t *testing.T) {
	cases := []struct {
		name     string
		cd       *hivev1.ClusterDeployment
		expected bool
	}{
		{
			name:     "no annotation",
			cd:       clusterdeployment.Build(),
			expected: true,
		},
		{
			name: "syncset annotation true",
			cd: clusterdeployment.Build(
				clusterdeployment.Generic(generic.WithAnnotation(constants.SyncsetPauseAnnotation, "true")),
			),
		},
		{
			name: "syncset annotation false",
			cd: clusterdeployment.Build(
				clusterdeployment.Generic(generic.WithAnnotation(constants.SyncsetPauseAnnotation, "false")),
			),
			expected: true,
		},
		{
			name: "syncset annotation not parsable",
			cd: clusterdeployment.Build(
				clusterdeployment.Generic(generic.WithAnnotation(constants.SyncsetPauseAnnotation, "other")),
			),
			expected: true,
		},
		{
			name: "relocating annotation",
			cd: clusterdeployment.Build(
				clusterdeployment.Generic(generic.WithAnnotation(constants.RelocatingAnnotation, "some-relocator")),
			),
			expected: false,
		},
		{
			name: "relocated annotation",
			cd: clusterdeployment.Build(
				clusterdeployment.Generic(generic.WithAnnotation(constants.RelocatedAnnotation, "some-relocator")),
			),
			expected: false,
		},
		{
			name: "empty relocating annotation",
			cd: clusterdeployment.Build(
				clusterdeployment.Generic(generic.WithAnnotation(constants.RelocatingAnnotation, "")),
			),
			expected: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := ShouldSyncCluster(tc.cd, logrus.StandardLogger())
			assert.Equal(t, tc.expected, actual)
		})
	}
}
