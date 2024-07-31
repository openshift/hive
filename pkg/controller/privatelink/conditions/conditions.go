package conditions

import (
	"context"
	"regexp"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

// clusterDeploymentPrivateLinkConditions are the cluster deployment conditions controlled by
// the private link controller
var clusterDeploymentPrivateLinkConditions = []hivev1.ClusterDeploymentConditionType{
	hivev1.PrivateLinkFailedClusterDeploymentCondition,
	hivev1.PrivateLinkReadyClusterDeploymentCondition,
}

func filterErrorMessage(err error) string {
	skipRequestIDRE := regexp.MustCompile(`(request id|Request ID): ([-0-9a-f]+)`)
	return skipRequestIDRE.ReplaceAllString(err.Error(), "${1}: XXXX")
}

func InitializeConditions(cd *hivev1.ClusterDeployment) ([]hivev1.ClusterDeploymentCondition, bool) {
	return controllerutils.InitializeClusterDeploymentConditions(cd.Status.Conditions, clusterDeploymentPrivateLinkConditions)
}

func SetErrCondition(cd *hivev1.ClusterDeployment,
	reason string, err error,
	client client.Client, logger log.FieldLogger) error {
	curr := &hivev1.ClusterDeployment{}
	errGet := client.Get(context.TODO(), types.NamespacedName{Namespace: cd.Namespace, Name: cd.Name}, curr)
	if errGet != nil {
		return errGet
	}
	message := filterErrorMessage(err)
	conditions, failedChanged := controllerutils.SetClusterDeploymentConditionWithChangeCheck(
		curr.Status.Conditions,
		hivev1.PrivateLinkFailedClusterDeploymentCondition,
		corev1.ConditionTrue,
		reason,
		message,
		controllerutils.UpdateConditionIfReasonOrMessageChange)
	conditions, readyChanged := controllerutils.SetClusterDeploymentConditionWithChangeCheck(
		conditions,
		hivev1.PrivateLinkReadyClusterDeploymentCondition,
		corev1.ConditionFalse,
		reason,
		message,
		controllerutils.UpdateConditionIfReasonOrMessageChange)
	if !readyChanged && !failedChanged {
		return nil
	}
	curr.Status.Conditions = conditions
	logger.Debug("setting PrivateLinkFailedClusterDeploymentCondition to true")
	return client.Status().Update(context.TODO(), curr)
}

func SetReadyCondition(cd *hivev1.ClusterDeployment,
	completed corev1.ConditionStatus,
	reason string, message string,
	client client.Client,
	logger log.FieldLogger) error {

	curr := &hivev1.ClusterDeployment{}
	errGet := client.Get(context.TODO(), types.NamespacedName{Namespace: cd.Namespace, Name: cd.Name}, curr)
	if errGet != nil {
		return errGet
	}

	conditions := curr.Status.Conditions

	var failedChanged bool
	if completed == corev1.ConditionTrue {
		conditions, failedChanged = controllerutils.SetClusterDeploymentConditionWithChangeCheck(
			conditions,
			hivev1.PrivateLinkFailedClusterDeploymentCondition,
			corev1.ConditionFalse,
			reason,
			message,
			controllerutils.UpdateConditionIfReasonOrMessageChange)
	}

	var readyChanged bool
	ready := controllerutils.FindCondition(conditions, hivev1.PrivateLinkReadyClusterDeploymentCondition)
	if ready == nil || ready.Status != corev1.ConditionTrue {
		// we want to allow Ready condition to reach Ready level
		conditions, readyChanged = controllerutils.SetClusterDeploymentConditionWithChangeCheck(
			conditions,
			hivev1.PrivateLinkReadyClusterDeploymentCondition,
			completed,
			reason,
			message,
			controllerutils.UpdateConditionIfReasonOrMessageChange)
	} else if completed == corev1.ConditionTrue {
		// allow reinforcing Ready level to track the last Ready probe.
		// we have a higher level control of when to sync an already Ready cluster
		conditions, readyChanged = controllerutils.SetClusterDeploymentConditionWithChangeCheck(
			conditions,
			hivev1.PrivateLinkReadyClusterDeploymentCondition,
			corev1.ConditionTrue,
			reason,
			message,
			controllerutils.UpdateConditionAlways)
	}
	if !readyChanged && !failedChanged {
		return nil
	}
	curr.Status.Conditions = conditions
	logger.Debugf("setting PrivateLinkReadyClusterDeploymentCondition to %s", completed)
	return client.Status().Update(context.TODO(), curr)
}
