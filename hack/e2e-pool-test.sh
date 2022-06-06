#!/bin/bash

set -ex

TEST_NAME=e2e-pool
source ${0%/*}/e2e-common.sh

# TODO: Use something better here.
# `make test-e2e-postdeploy` could work, but does more than we need.
echo "Waiting for the deployment to settle"
sleep 120

function create_imageset() {
  local is_name=$1
  echo "Creating imageset $is_name"
  oc apply -f -<<EOF
apiVersion: hive.openshift.io/v1
kind: ClusterImageSet
metadata:
  name: $is_name
spec:
  releaseImage: $RELEASE_IMAGE
EOF
}

function count_cds() {
  J=$(oc get cd -A -o json)
  [[ -z "$J" ]] && return
  jq -r '.items | length' <<<"$J"
}

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

    powerState=$(oc get cd -n $CLUSTER_NAME $CLUSTER_NAME -o json | jq -r '.status.powerState')
    if [[ "${powerState}" == $EXPECTED_STATE ]]; then
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
    echo "Actual state: ${powerState}" >&2
    exit 9
  fi
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
REAL_POOL_NAME=$CLUSTER_NAME

function cleanup() {
  echo "!EXIT TRAP!"
  capture_manifests
  # Let's save the logs now in case any of the following never finish
  echo "Saving hive logs before cleanup"
  save_hive_logs
  oc delete clusterclaim --all
  oc delete clusterpool --all
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
trap 'kill %1; cleanup' EXIT

function wait_for_pool_to_be_ready() {
  local poolname=$1
  local i=0
  # NOTE: This will need to change if we add a test with a zero-size pool.
  while [[ $(oc get clusterpool $poolname -o json | jq '.status.size != 0 and .status.ready + .status.standby == .status.size') != "true" ]] || ! expect_all_clusters_current $poolname True; do
    i=$((i+1))
    if [[ $i -gt $max_cluster_deployment_status_checks ]]; then
      echo "Timed out waiting for clusterpool $poolname to be ready."
      return 1
    fi
    echo "Waiting for clusterpool $poolname to be ready: $i of $max_cluster_deployment_status_checks"
    sleep $sleep_between_cluster_deployment_status_checks
  done
}

function get_all_clusters_current_condition() {
  local poolname=$1
  oc get clusterpool $poolname -o json | jq -r '.status.conditions[] | select(.type=="AllClustersCurrent")'
}

function expect_all_clusters_current() {
  local poolname=$1
  local expected_status=$2
  local cond=$(get_all_clusters_current_condition $poolname)
  if [[ $(jq -r .status <<< $cond) != $expected_status ]]; then
    echo "Expected AllClustersCurrent to be $expected_status, but:"
    jq -r .message <<< $cond
    return 1
  fi
}

function verify_pool_cd_imagesets() {
  local poolname=$1
  local expected_cis=$2
  local rc=0
  echo "Validating all ClusterDeployments for pool $poolname have imageSetRef $expected_cis"
  cd_cis=$(oc get cd -A -o json \
      | jq -r '.items[]
        | select(.metadata.deletionTimestamp==null)
        | select(.spec.clusterPoolRef.poolName=="'$poolname'")
        | [.metadata.name, .spec.provisioning.imageSetRef.name]
        | @tsv')
  while read cd cis; do
    if [[ $cis != $expected_cis ]]; then
      echo "FAIL: ClusterDeployment $cd has imageSetRef $cis"
      rc=1
    fi
  done <<< "$cd_cis"
  return $rc
}

function set_power_state() {
  local cd=$1
  local power_state=$2
  oc patch cd -n $cd $cd --type=merge -p '{"spec": {"powerState": "'$power_state'"}}'
}

IMAGESET_NAME=cis
create_imageset $IMAGESET_NAME

echo "Creating real cluster pool"
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
  ${REAL_POOL_NAME}

### INTERLUDE: FAKE POOL
# The real cluster pool is going to take a while to become ready. While that
# happens, create a fake pool and do some more testing. We'll use the real
# pool as a template, just changing its name and size and adding the annotation
# that causes its CDs to be faked.
FAKE_POOL_NAME=fake-pool
oc get clusterpool ${REAL_POOL_NAME} -o json \
  | jq '.spec.annotations["hive.openshift.io/fake-cluster"] = "true" | .metadata.name = "'${FAKE_POOL_NAME}'" | .spec.size = 4' \
  | oc apply -f -
wait_for_pool_to_be_ready $FAKE_POOL_NAME

## Test stale cluster replacement (HIVE-1058)
verify_pool_cd_imagesets $FAKE_POOL_NAME $IMAGESET_NAME
# Create another cluster image set so we can edit a relevant pool field.
# The cis is identical except for the name, but the pool doesn't know that.
create_imageset fake-cis
# Modify the clusterpool and watch CDs get replaced
oc patch clusterpool $FAKE_POOL_NAME --type=merge -p '{"spec":{"imageSetRef": {"name": "fake-cis"}}}'
expect_all_clusters_current $FAKE_POOL_NAME False
oc wait --for=condition=AllClustersCurrent --timeout=10m clusterpool/$FAKE_POOL_NAME
# The wait returns as soon as we delete the last stale cluster. Wait for its replacement to be ready.
wait_for_pool_to_be_ready $FAKE_POOL_NAME
# At this point all CDs should ref the new imageset
verify_pool_cd_imagesets $FAKE_POOL_NAME fake-cis

# Grab the name (which is also the namespace) of a cd from our pool
cd=$(oc get cd -A -o json | jq -r '.items[] | select(.spec.clusterPoolRef.poolName=="'${FAKE_POOL_NAME}'") | .metadata.name' | head -1)
if [[ -z "$cd" ]]; then
  echo "Failed to retrieve a ClusterDeployment from pool $FAKE_POOL_NAME"
  exit 1
fi

## Test hibernating fake cluster
echo "Hibernating fake cluster"
set_power_state $cd Hibernating
wait_for_hibernation_state $cd Hibernating
installed=$(oc get cd -n $cd $cd -o json | jq -r '.status.installedTimestamp')
hibernated=$(oc get cd -n $cd $cd -o json | jq -r '.status.conditions[] | select(.type=="Hibernating") | .lastTransitionTime')
delta_seconds=$((`date -d "$hibernated" +%s`-`date -d "$installed" +%s`))
# Check fresh fake cluster did not get stuck on SyncSetsNotApplied
if [[ $delta_seconds -gt $((10*60)) ]]; then
  echo "Took longer than 10m to hibernate!"
  exit 9
fi

## Test broken cluster replacement (HIVE-1615)
# Patch the CD's ProvisionStopped status condition to make it look "broken".
# This should cause the clusterpool controller to replace it.
i=0
while j=$(oc get cd -n $cd $cd -o json); do
  i=$((i+1))
  if [[ $i -gt $max_tries ]]; then
    echo "Timed out waiting for clusterdeployment $cd to be deleted."
    exit 1
  fi
  # We do this inside the loop because we can hit a timing window where a controller overwrites the
  # ProvisionStopped condition, undoing this change.
  if [[ $(jq -r '.status.conditions[] | select(.type=="ProvisionStopped") | .status' <<<"$j") != "True" ]]; then
    echo "Patching ProvisionStopped to True for CD $cd"
    ${0%/*}/statuspatch cd -n $cd $cd <<< '(.status.conditions[] | select(.type=="ProvisionStopped") | .status) = "True"'
  fi
  echo "Waiting for clusterdeployment $cd to be deleted: $i of $max_tries"
  sleep $sleep_between_tries
done
# Now wait for the controller to replace the CD, bringing the ready count back
wait_for_pool_to_be_ready $FAKE_POOL_NAME

### BACK TO THE REAL POOL

# Wait for the real cluster pool to become ready (if it isn't yet)
wait_for_pool_to_be_ready $REAL_POOL_NAME

# Get the CD name & namespace (which should be the same)
# TODO: Set this up for variable POOL_SIZE -- as written this would put
#       multiple results in CLUSTER_NAME; for >1 pool size we would not only
#       need to grab just one of the results, but we would also need to make
#       sure that's the one that gets claimed for the meat of this test.
CLUSTER_NAME=$(oc get cd -A -o json | jq -r '.items[] | select(.spec.clusterPoolRef.poolName=="'$REAL_POOL_NAME'") | .metadata.name')

wait_for_hibernation_state $CLUSTER_NAME Hibernating

echo "Claiming"
CLAIM_NAME=the-claim
go run "${SRC_ROOT}/contrib/cmd/hiveutil/main.go" clusterpool claim -n $CLUSTER_NAMESPACE $REAL_POOL_NAME $CLAIM_NAME

wait_for_hibernation_state $CLUSTER_NAME Running

echo "Re-hibernating"
set_power_state $CLUSTER_NAME Hibernating

wait_for_hibernation_state $CLUSTER_NAME Hibernating

echo "Re-resuming"
set_power_state $CLUSTER_NAME Running

wait_for_hibernation_state $CLUSTER_NAME Running

# Let the cleanup trap do the cleanup.
