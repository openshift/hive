#!/bin/bash

set -ex

TEST_NAME=e2e-pool
source ${0%/*}/e2e-common.sh

# TODO: Use something better here.
# `make test-e2e-postdeploy` could work, but does more than we need.
echo "Waiting for the deployment to settle"
sleep 120

echo "Creating imageset"
IMAGESET_NAME=cis
oc apply -f -<<EOF
apiVersion: hive.openshift.io/v1
kind: ClusterImageSet
metadata:
  name: $IMAGESET_NAME
spec:
  releaseImage: $RELEASE_IMAGE
EOF

# NOTE: This is needed in order for the short form (cd) to work
oc get clusterdeployment > /dev/null

function count_cds() {
  oc get cd -A -o json | jq -r '.items | length'
}

# Verify no CDs exist yet
NUM_CDS=$(count_cds)
if [[ $NUM_CDS != "0" ]]; then
  echo "Got an unexpected number of pre-existing ClusterDeployments." >&2
  echo "Expected 0." >&2
  echo "Got: $NUM_CDS" >&2
  exit 5
fi

# Use the CLUSTER_NAME configured by the test as the pool name. This will result in CD names
# being seeded with that as a prefix, which will make them visible to our leak detector.
POOL_NAME=$CLUSTER_NAME

function cleanup() {
  capture_manifests
  # Let's save the logs now in case any of the following never finish
  echo "Saving hive logs before cleanup"
  save_hive_logs
  oc delete clusterclaim --all
  oc delete clusterpool $POOL_NAME
  # Wait indefinitely for all CDs to disappear. If we exceed the test timeout,
  # we'll get killed, and resources will leak.
  while true; do
    sleep ${sleep_between_tries}
    NUM_CDS=$(count_cds)
    if [[ $NUM_CDS == "0" ]]; then
      break
    fi
    echo "Waiting for $NUM_CDS ClusterDeployment(s) to be cleaned up"
  done
  # And if we get this far, overwrite the logs with the latest
  echo "Saving hive logs after cleanup"
  save_hive_logs
}
trap cleanup EXIT

echo "Creating cluster pool"
# TODO: This can't be changed yet -- see other TODOs (search for 'variable POOL_SIZE')
POOL_SIZE=1
# TODO: This is aws-specific at the moment.
go run "${SRC_ROOT}/contrib/cmd/hiveutil/main.go" clusterpool create-pool \
  -n "${CLUSTER_NAMESPACE}" \
  --cloud="${CLOUD}" \
	--creds-file="${CREDS_FILE}" \
	--pull-secret-file="${PULL_SECRET_FILE}" \
  --image-set "${IMAGESET_NAME}" \
  --region us-east-1 \
  --size "${POOL_SIZE}" \
  ${POOL_NAME}

echo "Waiting for pool to create $POOL_SIZE ClusterDeployment(s)"
i=1
while [[ $i -le ${max_tries} ]]; do
  if [[ $i -gt 1 ]]; then
    # Don't sleep on first loop
    echo "sleeping ${sleep_between_tries} seconds"
    sleep ${sleep_between_tries}
  fi

  NUM_CDS=$(count_cds)
  if [[ $NUM_CDS == "${POOL_SIZE}" ]]; then
    echo "Success"
    break
  else
    echo -n "Failed $(NUM_CDS), "
  fi

  i=$((i + 1))
done

if [[ $i -ge ${max_tries} ]] ; then
  # Failed the maximum amount of times.
  echo "exiting"
  exit 10
fi

# Get the CD name & namespace (which should be the same)
# TODO: Set this up for variable POOL_SIZE
CLUSTER_NAME=$(oc get cd -A -o json | jq -r .items[0].metadata.name)

echo "Waiting for ClusterDeployment $CLUSTER_NAME to finish installing"
# TODO: Set this up for variable POOL_SIZE
i=1
while [[ $i -le ${max_cluster_deployment_status_checks} ]]; do
  CD_JSON=$(oc get cd -n $CLUSTER_NAME $CLUSTER_NAME -o json)
  if [[ $(jq .spec.installed <<<"${CD_JSON}") == "true" ]]; then
    echo "ClusterDeployment is Installed"
    break
  fi
  PF_COND=$(jq -r '.status.conditions[] | select(.type == "ProvisionFailed")' <<<"${CD_JSON}")
  if [[ $(jq -r .status <<<"${PF_COND}") == 'True' ]]; then
    FAILURE_REASON=$(jq -r .reason <<<"${PF_COND}")
    FAILURE_MESSAGE=$(jq -r .message <<<"${PF_COND}")
    echo "ClusterDeployment install failed with reason '$FAILURE_REASON' and message: $FAILURE_MESSAGE" >&2
    capture_cluster_logs $CLUSTER_NAME $CLUSTER_NAME failure
    exit 7
  fi
  sleep ${sleep_between_cluster_deployment_status_checks}
  echo "Still waiting for the ClusterDeployment ${CLUSTER_NAME} to install. Status check #${i}/${max_cluster_deployment_status_checks}... "
  i=$((i + 1))

done

function wait_for_hibernation_state() {
  local CLUSTER_NAME=$1
  local EXPECTED_STATE=$2
  echo "Waiting for ClusterDeployment $CLUSTER_NAME to be $EXPECTED_STATE"
  local i=1
  while [[ $i -le ${max_tries} ]]; do
    if [[ $i -gt 1 ]]; then
      # Don't sleep on first loop
      echo "sleeping ${sleep_between_tries} seconds"
      sleep ${sleep_between_tries}
    fi

    HIB_COND=$(oc get cd -n $CLUSTER_NAME $CLUSTER_NAME -o json | jq -r '.status.conditions[] | select(.type == "Hibernating")')
    if [[ $(jq -r .reason <<<"${HIB_COND}") == $EXPECTED_STATE ]]; then
      echo "Success"
      break
    else
      echo -n "Failed, "
    fi

    i=$((i + 1))
  done

  if [[ $i -ge ${max_tries} ]] ; then
    # Failed the maximum amount of times.
    echo "ClusterDeployment $CLUSTER_NAME still not $EXPECTED_STATE" >&2
    echo "Reason: $(jq -r .reason <<<"${HIB_COND}")" >&2
    echo "Message: $(jq -r .message <<<"${HIB_COND}")" >&2
    exit 9
  fi
}

wait_for_hibernation_state $CLUSTER_NAME Hibernating

echo "Claiming"
CLAIM_NAME=the-claim
go run "${SRC_ROOT}/contrib/cmd/hiveutil/main.go" clusterpool claim -n $CLUSTER_NAMESPACE $POOL_NAME $CLAIM_NAME

wait_for_hibernation_state $CLUSTER_NAME Running

echo "Re-hibernating"
oc patch cd -n $CLUSTER_NAME $CLUSTER_NAME --type=merge -p '{"spec": {"powerState": "Hibernating"}}'

wait_for_hibernation_state $CLUSTER_NAME Hibernating

echo "Re-resuming"
oc patch cd -n $CLUSTER_NAME $CLUSTER_NAME --type=merge -p '{"spec": {"powerState": "Running"}}'

wait_for_hibernation_state $CLUSTER_NAME Running

# Let the cleanup trap do the cleanup.
