package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openshift/hive/pkg/constants"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	"github.com/openshift/hive/pkg/test/generic"
)

func TestIsDeleteProtected(t *testing.T) {
	cases := []struct {
		name           string
		absent         bool
		value          string
		expectedResult bool
	}{
		{
			name:           "absent",
			absent:         true,
			expectedResult: false,
		},
		{
			name:           "true",
			value:          "true",
			expectedResult: true,
		},
		{
			name:           "false",
			value:          "false",
			expectedResult: false,
		},
		{
			name:           "empty",
			value:          "",
			expectedResult: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var options []testcd.Option
			if !tc.absent {
				options = append(
					options,
					testcd.Generic(generic.WithAnnotation(constants.ProtectedDeleteAnnotation, tc.value)),
				)
			}
			cd := testcd.Build(options...)
			actualResult := IsDeleteProtected(cd)
			assert.Equal(t, tc.expectedResult, actualResult, "unexpected result")
		})
	}
}
