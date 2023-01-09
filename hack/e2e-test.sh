#!/bin/bash

set -e

TEST_NAME=e2e
DIR=${0%/*}
source ${DIR}/e2e-common.sh


function teardown() {
  echo "!EXIT TRAP!"
  capture_manifests
  # Let's save the logs now in case any of the following never finish
  echo "Saving hive logs before cleanup"
  save_hive_logs
	echo ""
	echo ""
  # Skip tear down if the clusterdeployment is no longer there
  if ! oc get clusterdeployment ${CLUSTER_NAME}; then
    return
  fi

  # This is here for backup. The test-e2e-destroycluster test
  # should normally delete the clusterdeployment. Only if the
  # test fails before then, this will ensure we at least attempt
  # to delete the cluster.
	echo "Deleting ClusterDeployment ${CLUSTER_NAME}"
	oc delete --wait=false clusterdeployment ${CLUSTER_NAME} || :

	if ! go run "${SRC_ROOT}/contrib/cmd/waitforjob/main.go" --log-level=debug "${CLUSTER_NAME}" "uninstall"
	then
		echo "Waiting for uninstall job failed"
		if oc logs job/${CLUSTER_NAME}-uninstall &> "${ARTIFACT_DIR}/hive_uninstall_job_onfailure.log"
		then
			echo "************* UNINSTALL JOB LOG *************"
			cat "${ARTIFACT_DIR}/hive_uninstall_job_onfailure.log"
			echo ""
			echo ""
		fi
		exit 1
	fi
  # And if we get this far, overwrite the logs with the latest
  echo "Saving hive logs after cleanup"
  save_hive_logs
}
trap 'kill %1; teardown' EXIT

echo "Running post-deploy tests in original namespace $HIVE_NS"
make test-e2e-postdeploy

## Test changing the target namespace
ORIG_NS=$HIVE_NS
# 1) Let the rest of the suite know where to look for things
export HIVE_NS=hive-e2e-two
# 2) Patch the hiveconfig
oc patch hiveconfig hive -n $HIVE_OPERATOR_NS --type=merge -p '{"spec":{"targetNamespace": "'$HIVE_NS'"}}'
# Wait for hive-operator to roll out the new namespace
wait_for_namespace $HIVE_NS 60
# 3) If USE_MANAGED_DNS=true, "Move" the managed DNS creds secret to the new namespace. (In real
#    life the user would be responsible for making sure the secret referenced by hiveconfig exists
#    in the new target namespace -- either by moving the secret or creating a new one and updating
#    hiveconfig.)
#    TODO: Or should we try to do that for the user?
if $USE_MANAGED_DNS; then
  J=$(oc get secret -l hive.openshift.io/managed-dns-credentials=true -n $ORIG_NS -o json)
  num_secrets=$(jq -r '.items[] | length' <<<"$J")
  # Our retry loop for `hiveutil adm manage-dns enable` isn't idempotent, so there may be multiple
  # suitable secrets present.
  if [[ $num_secrets -lt 1 ]]; then
    echo "Expected to find at least one secret with the managed-dns-credentials label, but found $num_secrets!"
    exit 1
  fi
  # They should all be the same, so just pick the first one to copy over.
  jq '.items[0].metadata.namespace = "'$HIVE_NS'"' <<<"$J" | oc apply -f -
fi
# 4) Rerun postdeploy tests, which wait for everything to come up
echo "Running post-deploy tests in new namespace $HIVE_NS"
make test-e2e-postdeploy
# 5) Make sure the old namespace is "clean" (modulo the garbage that k8s/openshift leave behind, sad-face)
rc=0
for resource in secret configmap role rolebinding serviceaccount deployment replicaset statefulset pod; do
  echo "Checking for stale $resource resources in original namespace $ORIG_NS"
  if oc get $resource -n $ORIG_NS | grep hive; then
    echo "FAIL: found stale $resource in original namespace $ORIG_NS"
    rc=1
  fi
done
if [[ $rc -ne 0 ]]; then
  exit 1
fi

function install_check() {
  i=1
  cluster_name=$1
  while [ $i -le ${max_cluster_deployment_status_checks} ]; do
    CD_JSON=$(oc get cd ${cluster_name} -n ${CLUSTER_NAMESPACE} -o json)
    if [[ $(jq .spec.installed <<<"${CD_JSON}") == "true" ]] ; then
      INSTALL_RESULT="success"
      break
    fi
    PF_COND=$(jq -r '.status.conditions[] | select(.type == "ProvisionFailed")' <<<"${CD_JSON}")
    if [[ $(jq -r .status <<<"${PF_COND}") == 'True' ]]; then
      INSTALL_RESULT="failure"
      FAILURE_TYPE=ProvisionFailed
      FAILURE_REASON=$(jq -r .reason <<<"${PF_COND}")
      FAILURE_MESSAGE=$(jq -r .message <<<"${PF_COND}")
      break
    fi
    PF_COND=$(jq -r '.status.conditions[] | select(.type == "ProvisionStopped")' <<<"${CD_JSON}")
    if [[ $(jq -r .status <<<"${PF_COND}") == 'True' ]]; then
      INSTALL_RESULT="failure"
      FAILURE_TYPE=ProvisionStopped
      FAILURE_REASON=$(jq -r .reason <<<"${PF_COND}")
      FAILURE_MESSAGE=$(jq -r .message <<<"${PF_COND}")
      break
    fi
    # HACK: We've seen flakes where the dnszone controller can't instantiate the AWS actuator because
    # the *-aws-creds secret hasn't come to life yet. This causes the dnszone controller to ignore it
    # for 2h, which is too long for this test; and the CD is stuck during that time. So here we
    # detect whether that condition has happened, and then kick the DNSZone object in such a way that
    # the controller stops ignoring it and tries to resync it.
    DNS_COND=$(jq -r '.status.conditions[] | select(.type == "DNSNotReady")' <<<"${CD_JSON}")
    if [[ $(jq -r .status <<<"${DNS_COND}") == 'True' ]] && [[ $(jq -r .reason <<<"${DNS_COND}") == 'ActuatorNotInitialized' ]]; then
      echo "Found DNSNotReady=>ActuatorNotInitialized condition. Forcing DNSZone to resync..."
      # The DNSZone is in the CD's namespace. Its name is the CD name suffixed with '-zone'. Resetting
      # its lastSyncGeneration should trigger a resync.
      ${DIR}/statuspatch dnszone -n ${CLUSTER_NAMESPACE} ${cluster_name}-zone <<< '.status.lastSyncGeneration = 0'
    fi
    sleep ${sleep_between_cluster_deployment_status_checks}
    echo "Still waiting for the ClusterDeployment ${cluster_name} to install. Status check #${i}/${max_cluster_deployment_status_checks}... "
    i=$((i + 1))
  done

  case "${INSTALL_RESULT}" in
      success)
          echo "ClusterDeployment ${cluster_name} was installed successfully"
          ;;
      failure)
          echo "ClusterDeployment ${cluster_name} provision failed" >&2
          echo "Type: $FAILURE_TYPE" >&2
          echo "Reason: $FAILURE_REASON" >&2
          echo "Message: $FAILURE_MESSAGE" >&2
          ;;
      *)
          echo "Timed out waiting for the ClusterDeployment ${cluster_name} to install" >&2
          echo "You may be interested in its status conditions:" >&2
          jq -r .status.conditions <<<"${CD_JSON}" >&2
          ;;
  esac

}

export CLUSTER_NAME="${CLUSTER_NAME:-hive-$(uuidgen | tr '[:upper:]' '[:lower:]')}"

HIBERNATE_AFTER="2h"

echo "Creating cluster deployment"
go run "${SRC_ROOT}/contrib/cmd/hiveutil/main.go" create-cluster "${CLUSTER_NAME}" \
	--cloud="${CLOUD}" \
	--creds-file="${CREDS_FILE}" \
	--ssh-public-key-file="${SSH_PUBLIC_KEY_FILE}" \
	--pull-secret-file="${PULL_SECRET_FILE}" \
	--base-domain="${CLUSTER_DOMAIN}" \
	--release-image="${RELEASE_IMAGE}" \
	--install-once=true \
	--uninstall-once=true \
	${MANAGED_DNS_ARG} \
	${EXTRA_CREATE_CLUSTER_ARGS} \
  --hibernate-after=${HIBERNATE_AFTER}

# Sanity check the cluster deployment printer
i=1
while [ $i -le ${max_tries} ]; do
  if [ $i -gt 1 ]; then
    # Don't sleep on first loop
    echo "sleeping ${sleep_between_tries} seconds"
    sleep ${sleep_between_tries}
  fi

  echo "Getting ClusterDeployment ${CLUSTER_NAME}. Try #${i}/${max_tries}:"

  GET_BY_SHORT_NAME=$(oc get cd)

  if echo "${GET_BY_SHORT_NAME}" | grep 'INFRAID' ; then
    echo "Success"
    break
  else
    echo -n "Failed, "
  fi

  i=$((i + 1))
done

if [ $i -ge ${max_tries} ] ; then
  # Failed the maximum amount of times.
  echo "exiting"
  exit 10
fi

sleep 120

echo "Deployments in hive namespace"
oc get deployments -n ${HIVE_NS}
echo ""
echo "Pods in hive namespace"
oc get pods -n ${HIVE_NS}
echo ""
echo "Pods in cluster namespace"
oc get pods -n ${CLUSTER_NAMESPACE}
echo ""
echo "Events in hive namespace"
oc get events -n ${HIVE_NS}
echo ""
echo "Events in cluster namespace"
oc get events -n ${CLUSTER_NAMESPACE}

echo "Waiting for the ClusterDeployment ${CLUSTER_NAME} to install"
INSTALL_RESULT=""
install_check $CLUSTER_NAME

capture_cluster_logs $CLUSTER_NAME $CLUSTER_NAMESPACE $INSTALL_RESULT
capture_manifests
echo "Running post-install tests"
make test-e2e-postinstall

# poll status.powerState Running
if [[ ${INSTALL_RESULT} == "success" ]]; then

  EXPECTED_STATE=$(oc get cd -n ${CLUSTER_NAMESPACE} ${CLUSTER_NAME} -o json | jq -r '.spec.powerState')
  
  # Expected state will be null prior to hibernation if not otherwise specified
  if [[ ${EXPECTED_STATE} == "null" ]]; then
    EXPECTED_STATE="Running" # Is this correct?
  fi

  wait_for_hibernation_state $CLUSTER_NAME $EXPECTED_STATE
  POWER_STATE_RESULT=$(echo $?)
  # TODO: Do somthing with POWER_STATE_RESULT or is printed error message sufficient?

  # Patch ClusterDeployment to hibernate now
  if [[ ${EXPECTED_STATE} == "Running" ]]; then

    EXPECTED_STATE="Hibernating"

    CD_JSON=$(oc get cd $CLUSTER_NAME -n $CLUSTER_NAMESPACE -o json)
    jq '.spec += {"hibernateAfter": "1s"}' <<< ${CD_JSON} | oc apply -f -

    # poll status.powerState Hibernating
    wait_for_hibernation_state $CLUSTER_NAME $EXPECTED_STATE
    POWER_STATE_RESULT=$(echo $?)
    # TODO: Do something with POWER_STATE_RESULT or is printed error message sufficient?

  fi

fi

echo "Running destroy test"
make test-e2e-destroycluster

echo "Saving hive logs"
save_hive_logs

echo "Uninstalling hive and validating cleanup"
make test-e2e-uninstallhive
