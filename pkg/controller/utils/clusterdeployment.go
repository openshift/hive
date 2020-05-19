package utils

import (
	"strconv"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
)

func IsDeleteProtected(cd *hivev1.ClusterDeployment) bool {
	protectedDelete, err := strconv.ParseBool(cd.Annotations[constants.ProtectedDeleteAnnotation])
	return protectedDelete && err == nil
}
