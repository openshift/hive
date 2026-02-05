#!/bin/bash

set -e

TEST_NAME=e2e-pool
source ${0%/*}/e2e-common.sh

echo "Waiting for the operator deployment to settle"
oc wait --for=condition=Available --timeout=2m deploy/hive-operator -n $HIVE_OPERATOR_NS
echo "Waiting for operator to deploy controllers"
oc wait --for=condition=Ready --timeout=2m hiveconfig/hive
echo "Waiting for the controllers and admission deployments to settle"
for deployment in hive-controllers hiveadmission; do
  oc wait --for=condition=Available --timeout=2m deploy/$deployment -n $HIVE_NS
done

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

function create_customization() {
  local is_name=$1
  local ns=$2
  local cname=$3
  echo "Creating ClusterDeploymentCustomization $is_name"
  oc apply -f -<<EOF
apiVersion: hive.openshift.io/v1
kind: ClusterDeploymentCustomization
metadata:
  name: $is_name
  namespace: $ns
spec:
  installConfigPatches:
    - op: replace
      path: /metadata/name
      value: $cname
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
  capture_manifests CLEANUP_000
  # Let's save the logs now in case any of the following fail
  echo "Saving hive logs before cleanup"
  save_hive_logs
  # Do these asynchronously so we can keep polling the logs
  oc delete clusterclaim --all &
  oc delete clusterpool --all &
  # Wait indefinitely for all CDs to disappear. If we exceed the test timeout,
  # we'll get killed, and resources will leak.
  i=0
  while true; do
    sleep ${sleep_between_tries}
    i=$((i+1))
    # re-capture logs so we can debug if things aren't deleting properly
    echo "Re-capturing hive logs during cleanup"
    save_hive_logs
    # re-capture manifests, likewise, but not *too* often as these get added, not overwritten.
    # This will fire every 100s.
    [[ $((i%10)) -eq 0 ]] && capture_manifests CLEANUP_$(printf "%03d" $i)
    NUM_CDS=$(count_cds)
    if [[ $NUM_CDS == "0" ]]; then
      break
    fi
    echo "Waiting for $NUM_CDS ClusterDeployment(s) to be cleaned up"
  done
  echo "Cleanup complete"
}
trap 'set +e; kill %1; cleanup' EXIT

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

function verify_cluster_name() {
  local poolname=$1
  local cdcname=$2
  local expected_name=$3
  local rc=0
 echo "Validating customization $cdcname succefully changed cluster name to $expected_name"
  cd_cdc=$(oc get cd -A -o json \
      | jq -r '.items[]
        | select(.metadata.deletionTimestamp==null)
        | select(.spec.clusterPoolRef.poolName=="'$poolname'")
        | [.metadata.name, .metadata.namespace, .spec.clusterPoolRef.customizationRef.name, .spec.provisioning.installConfigSecretRef.name]
        | @tsv')
  while read cd ns cdc iref; do
    if [[ $cdc == $cdcname ]]; then
      name="$(oc -n  $ns extract secret/$iref --to=- | yq .metadata.name -r)"
      if [[ $name != $expected_name ]]; then
        echo "FAIL: ClusterDeployment $cd has ClusterDeploymentCustomization $cdcname but cluster $name is not $expected_name"
        rc=1
      fi
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
# NOTE: We start with a zero-size pool and scale it up after we add the inventory.
# Otherwise, adding the inventory immediately makes the already-provisioning cluster
# stale, BUT it doesn't get purged until it's done provisioning. (Intentional
# architectural decision to prefer presenting a stale claimable cluster early vs.
# delaying until a non-stale one is available.)
go run "${SRC_ROOT}/contrib/cmd/hiveutil/main.go" clusterpool create-pool \
  -n "${CLUSTER_NAMESPACE}" \
  --cloud="${CLOUD}" \
  ${REGION_ARG} \
	${CREDS_FILE_ARG} \
	--pull-secret-file="${PULL_SECRET_FILE}" \
	--base-domain="${CLUSTER_DOMAIN}" \
  --image-set "${IMAGESET_NAME}" \
  --region us-east-1 \
  --size 0 \
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

# Add customization to REAL POOL
# NOTE: Use the cdcci- prefix for this cluster name so it gets caught by our leak detector.
# NOTE: We do this after the fake pool creation so the fake pool doesn't pick up the same inventory.
NEW_CLUSTER_NAME=cdcci-${CLUSTER_NAME#*-}
create_customization "cdc-test" "${CLUSTER_NAMESPACE}" "${NEW_CLUSTER_NAME}"
oc patch cp -n $CLUSTER_NAMESPACE $REAL_POOL_NAME --type=merge -p '{"spec": {"inventory": [{"name": "cdc-test"}]}}'
# Set the legacy deprovision annotation to true. This annotation will be propagated to the created cluster.
# When the deletion of that cluster explodes, it will signal us to fix the legacy destroy path post AWK SDK v2 migration.
# TODO: Undo this as a part of HIVE-2944
oc patch cp -n $CLUSTER_NAMESPACE $REAL_POOL_NAME --type=merge -p '{"spec": {"annotations": {"hive.openshift.io/legacy-deprovision": "true"}}}'
# Now we can scale up the pool so it starts creating clusters
oc scale cp -n $CLUSTER_NAMESPACE $REAL_POOL_NAME --replicas=$POOL_SIZE

wait_for_pool_to_be_ready $FAKE_POOL_NAME

## Test stale cluster replacement (HIVE-1058)
verify_pool_cd_imagesets $FAKE_POOL_NAME $IMAGESET_NAME
# Create another cluster image set so we can edit a relevant pool field.
# The cis is identical except for the name, but the pool doesn't know that.
create_imageset fake-cis
# Modify the clusterpool and watch CDs get replaced
oc patch clusterpool $FAKE_POOL_NAME --type=merge -p '{"spec":{"imageSetRef": {"name": "fake-cis"}}}'
oc wait --for=condition=AllClustersCurrent=false --timeout=1m clusterpool/$FAKE_POOL_NAME
oc wait --for=condition=AllClustersCurrent --timeout=10m clusterpool/$FAKE_POOL_NAME
# The wait returns as soon as we delete the last stale cluster. Wait for its replacement to be ready.
wait_for_pool_to_be_ready $FAKE_POOL_NAME
# At this point all CDs should ref the new imageset
verify_pool_cd_imagesets $FAKE_POOL_NAME fake-cis

echo "Claiming fake cluster"
CLAIM_NAME=fake-claim
go run "${SRC_ROOT}/contrib/cmd/hiveutil/main.go" clusterpool claim -n $CLUSTER_NAMESPACE $FAKE_POOL_NAME $CLAIM_NAME
# Wait for the claim to be fulfilled
oc wait --for=condition=Pending=false --timeout=2m clusterclaim/$CLAIM_NAME

# Grab the namespace (which is also the name) of the claimed cd
cd=$(oc get clusterclaim -n $CLUSTER_NAMESPACE $CLAIM_NAME -o jsonpath={.spec.namespace})
if [[ -z "$cd" ]]; then
  echo "Failed to glean ClusterDeployment name(space) from claim $CLAIM_NAME"
  exit 1
fi

# Cluster should be Running when claim is fulfilled
pState=$(oc get cd -n $cd $cd -o jsonpath={.status.powerState})
if [[ "$pState" != "Running" ]]; then
  echo "Expected claimed cluster to be Running, but it was '$pState'"
  exit 1
fi

## Test hibernating/resuming fake cluster
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
echo "Resuming fake cluster"
set_power_state $cd Running
wait_for_hibernation_state $cd Running

## Deleting the claim should delete the CD
oc delete cd -n $cd $cd
oc wait --for=delete cd/$cd -n $cd --timeout=5m

## Test broken cluster replacement (HIVE-1615)
# Grab the name (which is also the namespace) of a cd from our pool
cd=$(oc get cd -A -o json | jq -r '.items[] | select(.spec.clusterPoolRef.poolName=="'${FAKE_POOL_NAME}'") | .metadata.name' | head -1)
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

# Test customization
verify_cluster_name $REAL_POOL_NAME "cdc-test" $NEW_CLUSTER_NAME

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
