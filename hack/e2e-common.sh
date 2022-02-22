max_tries=60
sleep_between_tries=10
# Set timeout for the cluster deployment to install
# timeout = sleep_between_cluster_deployment_status_checks * max_cluster_deployment_status_checks
max_cluster_deployment_status_checks=90
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

ORIGINAL_NAMESPACE=$(oc config view -o json | jq -er 'select(.contexts[].name == ."current-context") | .contexts[]?.context.namespace // ""')
echo Original default namespace is ${ORIGINAL_NAMESPACE}
echo Setting default namespace to ${CLUSTER_NAMESPACE}
if ! oc config set-context --current --namespace=${CLUSTER_NAMESPACE}; then
	echo "Failed to set the default namespace"
	exit 1
fi

function restore_default_namespace() {
	echo Restoring default namespace to ${ORIGINAL_NAMESPACE}
	oc config set-context --current --namespace=${ORIGINAL_NAMESPACE}
}
trap 'restore_default_namespace' EXIT

if [ $i -ge ${max_tries} ] ; then
  # Failed the maximum amount of times.
  echo "exiting"
  exit 10
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

# Install Hive
IMG="${HIVE_IMAGE}" make deploy

function save_hive_logs() {
  oc logs -n "${HIVE_NS}" deployment/hive-controllers > "${ARTIFACT_DIR}/hive-controllers.log"
  oc logs -n "${HIVE_NS}" deployment/hiveadmission > "${ARTIFACT_DIR}/hiveadmission.log"
}

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
