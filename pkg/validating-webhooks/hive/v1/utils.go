package v1

import (
	"strconv"

	"github.com/openshift/hive/pkg/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func creationHooksDisabled(o metav1.Object) bool {
	v, ok := o.GetLabels()[constants.DisableCreationWebHookForDisasterRecovery]
	if !ok {
		return false
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false
	}
	return b
}
