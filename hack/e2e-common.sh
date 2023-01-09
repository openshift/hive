###
# TEMPORARY workaround for https://issues.redhat.com/browse/DPTP-2871
# The configured job timeout after isn't signaling the test script like it
# should, so our exit trap isn't being activated, so we're not cleaning up
# spoke clusters, so we're leaking cloud resources. Inject a "manual" timeout
# that actually does signal us.
###
# This is necessary so our child process can signal us.
set -o monitor
# Background a sleep followed by sending SIGINT to our PID.
# Exit traps need to kill this, or it'll hold the test env until the timeout is
# reached :(
# Compute the timeout as 2m less than what's configured for the job in prow --
# keep these up to date with the job config!
if [[ $0 == */e2e-pool-test.sh ]]; then
  # TODO: set this back to 148 when we figure out how to make the *test script*
  # timeout something other than 2h.
  timeout_minutes=118
else
  timeout_minutes=118
fi
/usr/bin/bash -c "sleep $(($timeout_minutes*60)) && echo 'Timed out!' && kill -n 2 $$" &
###

max_tries=120
sleep_between_tries=10
# Set timeout for the cluster deployment to install
# timeout = sleep_between_cluster_deployment_status_checks * max_cluster_deployment_status_checks
max_cluster_deployment_status_checks=180
sleep_between_cluster_deployment_status_checks="1m"

export CLUSTER_NAMESPACE="${CLUSTER_NAMESPACE:-cluster-test}"

# In CI, HIVE_IMAGE and RELEASE_IMAGE are set via the job's `dependencies`.
if [[ -z "$HIVE_IMAGE" ]]; then
    echo "The HIVE_IMAGE environment variable was not found." >&2
    echo "It must be set to the fully-qualified pull spec of a hive container image." >&2
    echo "E.g. quay.io/my-user/hive:latest" >&2
    exit 1
fi
if [[ -z "$RELEASE_IMAGE" ]]; then
    echo "The RELEASE_IMAGE environment variable was not found." >&2
    echo "It must be set to the fully-qualified pull spec of an OCP release container image." >&2
    echo "E.g. quay.io/openshift-release-dev/ocp-release:4.7.0-x86_64" >&2
    exit 1
fi

echo "Running ${TEST_NAME} with HIVE_IMAGE ${HIVE_IMAGE}"
echo "Running ${TEST_NAME} with RELEASE_IMAGE ${RELEASE_IMAGE}"

i=1
while [ $i -le ${max_tries} ]; do
  if [ $i -gt 1 ]; then
    # Don't sleep on first loop
    echo "sleeping ${sleep_between_tries} seconds"
    sleep ${sleep_between_tries}
  fi

  echo -n "Creating namespace ${CLUSTER_NAMESPACE}. Try #${i}/${max_tries}... "
  if oc create namespace "${CLUSTER_NAMESPACE}"; then
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

echo Setting default namespace to ${CLUSTER_NAMESPACE}
if ! oc config set-context --current --namespace=${CLUSTER_NAMESPACE}; then
	echo "Failed to set the default namespace"
	exit 1
fi

CLUSTER_PROFILE_DIR="${CLUSTER_PROFILE_DIR:-/tmp/cluster}"

CLOUD="${CLOUD:-aws}"
export ARTIFACT_DIR="${ARTIFACT_DIR:-/tmp}"

SSH_PUBLIC_KEY_FILE="${SSH_PUBLIC_KEY_FILE:-${CLUSTER_PROFILE_DIR}/ssh-publickey}"
# If not specified or nonexistent, generate a keypair to use
if ! [[ -s "${SSH_PUBLIC_KEY_FILE}" ]]; then
    echo "Specified SSH public key file '${SSH_PUBLIC_KEY_FILE}' is invalid or nonexistent. Generating a single-use keypair."
    WHERE=${SSH_PUBLIC_KEY_FILE%/*}
    mkdir -p ${WHERE}
    # Tell the installmanager where to find the private key
    export SSH_PRIV_KEY_PATH=$(mktemp -p ${WHERE})
    # ssh-keygen will put the public key here
    TMP_PUB=${SSH_PRIV_KEY_PATH}.pub
    # Answer 'y' to the overwrite prompt, since we touched the file
    yes y | ssh-keygen -q -t rsa -N '' -f ${SSH_PRIV_KEY_PATH}
    # Now put the pubkey where we expected it
    mv ${TMP_PUB} ${SSH_PUBLIC_KEY_FILE}
fi

PULL_SECRET_FILE="${PULL_SECRET_FILE:-${CLUSTER_PROFILE_DIR}/pull-secret}"
export HIVE_NS="hive-e2e"
export HIVE_OPERATOR_NS="hive-operator"

# wait_for_namespace polls the namespace named by $1 and succeeds when it appears.
# Otherwise it eventually times out after $2 seconds and hard exits.
function wait_for_namespace {
  echo -n "Waiting $2 seconds for namespace $1 to appear"
  i=0
  while [[ $i -lt $2 ]]; do
    oc get namespace $1 >/dev/null 2>&1 && return 0
    i=$((i+1))
    echo -n .
    sleep 1
  done
  echo " Timed out!"
  exit 1
}

function save_hive_logs() {
  tmpf=$(mktemp)
  for x in "hive-controllers ${HIVE_NS}" "hiveadmission ${HIVE_NS}" "hive-operator ${HIVE_OPERATOR_NS}"; do
    read d n <<<$x
    # Don't save/overwrite the file unless log extraction succeeds
    if oc logs -n $n deployment/$d > $tmpf; then
      mv $tmpf "${ARTIFACT_DIR}/${d}.log"
    fi
    # If deployments' pods didn't start, the above won't produce logs; but these statuses may hint why.
    for n in ${HIVE_NS} ${HIVE_OPERATOR_NS}; do
      if oc describe po -n $n > $tmpf; then
        mv $tmpf "${ARTIFACT_DIR}/describe-po-in-ns-${n}.txt"
      fi
    done
  done
  # Let's try to save any prov/deprov pod logs
  oc get po -A -l hive.openshift.io/install=true -o custom-columns=:.metadata.namespace,:.metadata.name --no-headers | while read ns po; do
    oc logs -n $ns $po -c hive > ${ARTIFACT_DIR}/${ns}-${po}.log
  done
  oc get po -A -l hive.openshift.io/uninstall=true -o custom-columns=:.metadata.namespace,:.metadata.name --no-headers | while read ns po; do
    oc logs -n $ns $po > ${ARTIFACT_DIR}/${ns}-${po}.log
  done
  # Grab the HiveConfig. We may be running this after cleanup, so make sure it exists first.
  if oc get hiveconfig hive -o yaml > $tmpf; then
    mv $tmpf "${ARTIFACT_DIR}/hiveconfig-hive.yaml"
  fi
  # Save webhook manifests. This loop will iterate zero times if there aren't any.
  oc get validatingwebhookconfiguration -o name | awk -F/ '/hive/ {print $2}' | while read vwc; do
    oc get validatingwebhookconfiguration $vwc -o yaml > ${ARTIFACT_DIR}/${vwc}.yaml
  done
  # Save service manifests
  oc get service -n $HIVE_NS -o yaml > $ARTIFACT_DIR/services.yaml
  # Save the hiveadmission APIService
  if oc get apiservice v1.admission.hive.openshift.io -o yaml > $tmpf; then
    mv $tmpf $ARTIFACT_DIR/apiservice-hiveadmission.yaml
  fi
}
# The consumer of this lib can set up its own exit trap, but this basic one will at least help
# debug e.g. problems from `make deploy` and managed DNS setup.
trap 'kill %1; save_hive_logs' EXIT

# Install Hive
IMG="${HIVE_IMAGE}" make deploy

# Wait for $HIVE_NS to appear, as subsequent test steps rely on it.
# Timeout needs to account for Deployment=>ReplicaSet=>Pod=>Container as well
# as the operator starting up and reconciling HiveConfig to create the
# targetNamespace.
wait_for_namespace $HIVE_NS 180

echo

SRC_ROOT=$(git rev-parse --show-toplevel)

USE_MANAGED_DNS=${USE_MANAGED_DNS:-true}

case "${CLOUD}" in
"aws")
	CREDS_FILE="${CLUSTER_PROFILE_DIR}/.awscred"
        # Accept creds from the env if the file doesn't exist.
        if ! [[ -f $CREDS_FILE ]] && [[ -n "${AWS_ACCESS_KEY_ID}" ]] && [[ -n "${AWS_SECRET_ACCESS_KEY}" ]]; then
            # TODO: Refactor contrib/pkg/adm/managedns/enable::generateAWSCredentialsSecret to
            # use contrib/pkg/utils/aws/aws::GetAWSCreds, which knows how to look for the env
            # vars if the file isn't specified; and use this condition to generate (or not)
            # the whole CREDS_FILE_ARG="--creds-file=${CREDS_FILE}".
            printf '[default]\naws_access_key_id=%s\naws_secret_access_key=%s\n' "$AWS_ACCESS_KEY_ID" "$AWS_SECRET_ACCESS_KEY" > $CREDS_FILE
        fi
	BASE_DOMAIN="${BASE_DOMAIN:-hive-ci.openshift.com}"
	EXTRA_CREATE_CLUSTER_ARGS="--aws-user-tags expirationDate=$(date -d '4 hours' --iso=minutes --utc)"
	;;
"azure")
	CREDS_FILE="${CLUSTER_PROFILE_DIR}/osServicePrincipal.json"
	BASE_DOMAIN="${BASE_DOMAIN:-ci.azure.devcluster.openshift.com}"
	# For azure we set managedDNS=false as we are facing issues with this feature currently.
	# This is a temporary workaround to fix the e2e
	USE_MANAGED_DNS=false
	;;
"gcp")
	CREDS_FILE="${CLUSTER_PROFILE_DIR}/gce.json"
	BASE_DOMAIN="${BASE_DOMAIN:-origin-ci-int-gce.dev.openshift.com}"
	;;
*)
	echo "unknown cloud: ${CLOUD}"
	exit 1
	;;
esac

if $USE_MANAGED_DNS; then
	# Generate a short random shard string for this cluster similar to OSD prod.
	# This is to prevent name conflicts across customer clusters.
	CLUSTER_SHARD=$(cat /dev/urandom | tr -dc 'a-z' | fold -w 8 | head -n 1)
	CLUSTER_DOMAIN="${CLUSTER_SHARD}.${BASE_DOMAIN}"
	# We've seen this failing 409 (concurrent updates) recently. So try a couple times.
	# TODO: Check output for "the object has been modified" and only retry if it matches.
	echo "enabling managed DNS"
	for i in 1 2 3; do
	  go run "${SRC_ROOT}/contrib/cmd/hiveutil/main.go" adm manage-dns enable ${BASE_DOMAIN} \
	    --creds-file="${CREDS_FILE}" --cloud="${CLOUD}" && break
	  if [[ $i -eq 3 ]]; then
	    echo "Failed after $i attempts!"
	    exit 1
	  fi
	  echo "Retrying..."
	done
	MANAGED_DNS_ARG=" --manage-dns"
else
	CLUSTER_DOMAIN="${BASE_DOMAIN}"
fi

echo "Using cluster base domain: ${CLUSTER_DOMAIN}"

# NOTE: This is needed in order for the short form (cd) to work
oc get clusterdeployment > /dev/null

function capture_manifests() {
    oc get clusterdeployment -A -o yaml &> "${ARTIFACT_DIR}/hive_clusterdeployment.yaml" || true
    oc get clusterimageset -o yaml &> "${ARTIFACT_DIR}/hive_clusterimagesets.yaml" || true
    oc get clusterprovision -A -o yaml &> "${ARTIFACT_DIR}/hive_clusterprovision.yaml" || true
    oc get clusterstate -A -o yaml &> "${ARTIFACT_DIR}/hive_clusterstate.yaml" || true
    oc get dnszone -A -o yaml &> "${ARTIFACT_DIR}/hive_dnszones.yaml" || true
    oc get machinepool -A -o yaml &> "${ARTIFACT_DIR}/hive_machinepools.yaml" || true
    oc get clusterdeploymentcustomization -A -o yaml &> "${ARTIFACT_DIR}/hive_clusterdeploymentcustomization.yaml" || true
    oc get clusterpool -A -o yaml &> "${ARTIFACT_DIR}/hive_clusterpool.yaml" || true
    # Don't get the contents of the secrets, since they're sensitive; hopefully just listing them will be helpful.
    oc get secrets -A &> "${ARTIFACT_DIR}/secret_list.txt" || true
}

function capture_cluster_logs() {
    local CLUSTER_NAME=$1
    local CLUSTER_NAMESPACE=$2
    local INSTALL_RESULT=$3

    # Capture install logs
    if IMAGESET_JOB_NAME=$(oc get job -l "hive.openshift.io/cluster-deployment-name=${CLUSTER_NAME},hive.openshift.io/imageset=true" -o name -n ${CLUSTER_NAMESPACE}) && [ "${IMAGESET_JOB_NAME}" ]
    then
        oc logs -c hive -n ${CLUSTER_NAMESPACE} ${IMAGESET_JOB_NAME} &> "${ARTIFACT_DIR}/hive_imageset_job.log" || true
        oc get ${IMAGESET_JOB_NAME} -n ${CLUSTER_NAMESPACE} -o yaml &> "${ARTIFACT_DIR}/hive_imageset_job.yaml" || true
    fi
    if INSTALL_JOB_NAME=$(oc get job -l "hive.openshift.io/cluster-deployment-name=${CLUSTER_NAME},hive.openshift.io/install=true" -o name -n ${CLUSTER_NAMESPACE}) && [ "${INSTALL_JOB_NAME}" ]
    then
        oc logs -c hive -n ${CLUSTER_NAMESPACE} ${INSTALL_JOB_NAME} &> "${ARTIFACT_DIR}/hive_install_job.log" || true
        oc get ${INSTALL_JOB_NAME} -n ${CLUSTER_NAMESPACE} -o yaml &> "${ARTIFACT_DIR}/hive_install_job.yaml" || true
    fi
    echo "************* INSTALL JOB LOG *************"
    if oc get clusterprovision -l "hive.openshift.io/cluster-deployment-name=${CLUSTER_NAME}" -o jsonpath='{.items[0].spec.installLog}' &> "${ARTIFACT_DIR}/hive_install_console.log"; then
        cat "${ARTIFACT_DIR}/hive_install_console.log"
    else
        cat "${ARTIFACT_DIR}/hive_install_job.log"
    fi

    if [[ "${INSTALL_RESULT}" != "success" ]]
    then
        mkdir "${ARTIFACT_DIR}/hive"
        ${SRC_ROOT}/hack/logextractor.sh ${CLUSTER_NAME} "${ARTIFACT_DIR}/hive"
        exit 1
    fi
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
    return 9
  fi
}

function wait_for_hibernation_state_or_exit() {
  rc=$(wait_for_hibernation_state $cd Hibernating; echo $?)
  if [[ $rc != 0 ]]; then
    exit $rc
  fi
}
