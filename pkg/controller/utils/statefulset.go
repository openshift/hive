package utils

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/util/scheme"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	appsScheme = scheme.GetScheme()
	appsCodecs = serializer.NewCodecFactory(appsScheme)
)

// ReadStatefulsetOrDie converts a statefulset asset into an actual instance of a statefulset.
func ReadStatefulsetOrDie(objBytes []byte) *appsv1.StatefulSet {
	requiredObj, err := runtime.Decode(appsCodecs.UniversalDecoder(appsv1.SchemeGroupVersion), objBytes)
	if err != nil {
		panic(err)
	}
	return requiredObj.(*appsv1.StatefulSet)
}

// CalculateStatefulSetSpecHash returns a hash of the statefulset.Spec.
func CalculateStatefulSetSpecHash(statefulset *appsv1.StatefulSet) (string, error) {

	hasher := md5.New()
	jobSpecBytes, err := statefulset.Spec.Marshal()
	if err != nil {
		return "", err
	}

	_, err = hasher.Write(jobSpecBytes)
	if err != nil {
		return "", err
	}

	sum := hex.EncodeToString(hasher.Sum(nil))

	return sum, nil
}

// GetMyOrdinalID expects to be executed in the context of a pod which is a replica of a
// StatefulSet. We thus expect the pod name to have an ordinal suffix ("-0", "-1", etc)
// by which we can identify the replica. To discover this, we expect the pod name to be
// present in an env var named HIVE_${controllerName}_POD_NAME.
// The first return is the ordinal number.
// The error return is non-nil if:
// - we can't find the pod name env var.
// - the last hyphen-delimited component of the pod name can't be parsed as an int.
func GetMyOrdinalID(controllerName hivev1.ControllerName, logger log.FieldLogger) (int64, error) {
	// For searchability, we expect to produce one of
	// HIVE_CLUSTERSYNC_POD_NAME
	// HIVE_MACHINEPOOL_POD_NAME
	envKey := fmt.Sprintf("HIVE_%s_POD_NAME", strings.ToUpper(string(controllerName)))
	logger.Debugf("Getting %s", envKey)
	podname, found := os.LookupEnv(envKey)

	if !found {
		return -1, fmt.Errorf("environment variable %s not set", envKey)
	}

	logger.Debug("Setting ordinalID")
	parts := strings.Split(podname, "-")
	ordinalIDStr := parts[len(parts)-1]
	ordinalID32, err := strconv.Atoi(ordinalIDStr)
	if err != nil {
		return -1, errors.Wrapf(err, "error converting ordinalID %q to int", ordinalIDStr)
	}

	logger.WithField("ordinalID", ordinalID32).Debug("ordinalID discovered")
	return int64(ordinalID32), nil
}

func IsUIDAssignedToMe(c client.Client, deploymentName hivev1.DeploymentName, uid types.UID, myOrdinalID int64, logger log.FieldLogger) (bool, error) {
	l := logger.WithField("deploymentName", deploymentName).WithField("myOrdinalID", myOrdinalID)
	hiveNS := GetHiveNamespace()

	sts := &appsv1.StatefulSet{}
	err := c.Get(context.Background(), types.NamespacedName{Namespace: hiveNS, Name: string(deploymentName)}, sts)
	if err != nil {
		l.WithError(err).WithField("hiveNS", hiveNS).Error("error getting statefulset.")
		return false, err
	}

	if sts.Spec.Replicas == nil {
		return false, errors.New("sts.Spec.Replicas not set")
	}

	if sts.Status.CurrentReplicas != *sts.Spec.Replicas {
		// This ensures that we don't have partial syncing which may make it seem like things are working.
		// TODO: Remove this once https://issues.redhat.com/browse/CO-1268 is completed as this should no longer be needed.
		return false, fmt.Errorf("statefulset replica count is off. current: %v  expected: %v", sts.Status.CurrentReplicas, *sts.Spec.Replicas)
	}

	// There are a couple of assumptions here:
	// * the uid has 4 sections separated by hyphens
	// * the 4 sections are hex numbers
	// These assumptions are based on the fact that Kubernetes says UIDs are actually
	// ISO/IEC 9834-8 UUIDs. If this changes, this code may fail and may need to be updated.
	// https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#uids
	hexUID := strings.Replace(string(uid), "-", "", 4)
	l.Debugf("hexUID: %+v", hexUID)
	var uidAsBigInt big.Int
	uidAsBigInt.SetString(hexUID, 16)

	// For test purposes, if we've scaled down the controller so we can run locally, this will be zero; spoof it to one:
	replicas := int64(*sts.Spec.Replicas)
	if replicas == 0 {
		l.Warningf("%s StatefulSet has zero replicas! Hope you're running locally!", deploymentName)
		replicas = 1
	}

	l.Debug("determining who is assigned to sync this cluster")
	ordinalIDOfAssignee := uidAsBigInt.Mod(&uidAsBigInt, big.NewInt(replicas)).Int64()
	assignedToMe := ordinalIDOfAssignee == myOrdinalID

	l.WithFields(log.Fields{
		"replicas":            replicas,
		"ordinalIDOfAssignee": ordinalIDOfAssignee,
		"assignedToMe":        assignedToMe,
	}).Debug("computed values")

	return assignedToMe, nil
}
