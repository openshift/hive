#!/bin/bash

set -e

max_tries=60
sleep_between_tries=10
# Set timeout for the cluster deployment to install
# timeout = sleep_between_cluster_deployment_status_checks * max_cluster_deployment_status_checks
max_cluster_deployment_status_checks=90
sleep_between_cluster_deployment_status_checks="1m"

component=hive
local_hive_image=$(eval "echo $IMAGE_FORMAT")
export HIVE_IMAGE="${HIVE_IMAGE:-$local_hive_image}"

# Replace stable:hive with release:latest in the IMAGE_FORMAT we're given by the ci-operator:
local_release_image="${HIVE_IMAGE/stable:hive/release:latest}"
export RELEASE_IMAGE="${RELEASE_IMAGE:-$local_release_image}"

export CLUSTER_NAMESPACE="${CLUSTER_NAMESPACE:-cluster-test}"

echo "Running e2e with HIVE_IMAGE ${HIVE_IMAGE}"
echo "Running e2e with RELEASE_IMAGE ${RELEASE_IMAGE}"

if ! which kustomize > /dev/null; then
  kustomize_dir="$(mktemp -d)"
  export PATH="$PATH:${kustomize_dir}"
  # download kustomize so we can use it for deploying
  pushd "${kustomize_dir}"
  curl -O -L https://github.com/kubernetes-sigs/kustomize/releases/download/v2.0.0/kustomize_2.0.0_linux_amd64
  mv kustomize_2.0.0_linux_amd64 kustomize
  chmod u+x kustomize
  popd
fi

if ! which certbot > /dev/null; then
  certbot_dir="$(mktemp -d)"
  export PATH="$PATH:${certbot_dir}"
  pushd "${certbot_dir}"
  wget https://dl.eff.org/certbot-auto
  # Renaming to just certbot, certbot-auto is a wrapper around certbot and accepts same args, but will setup the
  # the tool and always use latest version.
  mv certbot-auto certbot
  chown root certbot
  chmod 0755 certbot
  #/usr/local/bin/certbot --help # will configure certbot on the system
  popd
fi

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


CLOUD_CREDS_DIR="${CLOUD_CREDS_DIR:-/tmp/cluster}"

# Create a new cluster deployment
export CLOUD="${CLOUD:-aws}"
export CLUSTER_NAME="${CLUSTER_NAME:-hive-$(uuidgen | tr '[:upper:]' '[:lower:]')}"
export ARTIFACT_DIR="${ARTIFACT_DIR:-/tmp}"
export SSH_PUBLIC_KEY_FILE="${SSH_PUBLIC_KEY_FILE:-${CLOUD_CREDS_DIR}/ssh-publickey}"
export PULL_SECRET_FILE="${PULL_SECRET_FILE:-${CLOUD_CREDS_DIR}/pull-secret}"
export HIVE_NS="hive-e2e"
export HIVE_OPERATOR_NS="hive-operator"

# Install Hive
DEPLOY_IMAGE="${HIVE_IMAGE}" make deploy


function teardown() {
	echo ""
	echo ""
        # Skip tear down if the clusterdeployment is no longer there
        if ! oc get clusterdeployment ${CLUSTER_NAME}; then
          return
        fi

        # This is here for backup. The test-e2e-destroycluster test
        # should normally delete the clusterdeployemnt. Only if the
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
}
trap 'teardown' EXIT

function save_hive_logs() {
  oc logs -n "${HIVE_NS}" deployment/hive-controllers > "${ARTIFACT_DIR}/hive-controllers.log"
  oc logs -n "${HIVE_NS}" deployment/hiveadmission > "${ARTIFACT_DIR}/hiveadmission.log"
}

echo "Running post-deploy tests"
make test-e2e-postdeploy

SRC_ROOT=$(git rev-parse --show-toplevel)

USE_MANAGED_DNS=true

case "${CLOUD}" in
"aws")
	CREDS_FILE="${CLOUD_CREDS_DIR}/.awscred"
	BASE_DOMAIN="${BASE_DOMAIN:-hive-ci.openshift.com}"
	EXTRA_CREATE_CLUSTER_ARGS="--aws-user-tags expirationDate=$(date -d '4 hours' --iso=minutes --utc)"
	;;
"azure")
	CREDS_FILE="${CLOUD_CREDS_DIR}/osServicePrincipal.json"
	BASE_DOMAIN="${BASE_DOMAIN:-ci.azure.devcluster.openshift.com}"
	;;
"gcp")
	CREDS_FILE="${CLOUD_CREDS_DIR}/gce.json"
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
	go run "${SRC_ROOT}/contrib/cmd/hiveutil/main.go" adm manage-dns enable ${BASE_DOMAIN} \
		--creds-file="${CREDS_FILE}" --cloud="${CLOUD}"
	MANAGED_DNS_ARG=" --manage-dns"
else
	CLUSTER_DOMAIN="${BASE_DOMAIN}"
fi

echo "Generating letsencrypt certificates for cluster"
go run "${SRC_ROOT}/contrib/cmd/hiveutil/main.go" certificate create "${CLUSTER_NAME}" --base-domain "${CLUSTER_DOMAIN}" --output-dir "${SRC_DIR}"
# This will cause a hive-controllers pod restart:
${SRC_ROOT}/hack/set-additional-ca.sh "${SRC_DIR}/${CLUSTER_NAME}.ca"

echo "Using cluster base domain: ${CLUSTER_DOMAIN}"
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
	--serving-cert="${SRC_ROOT}/${CLUSTER_NAME}.crt" \
	--serving-cert-key="${SRC_ROOT}/${CLUSTER_NAME}.key" \
	${MANAGED_DNS_ARG} \
	${EXTRA_CREATE_CLUSTER_ARGS}

# NOTE: This is needed in order for the short form (cd) to work
oc get clusterdeployment > /dev/null

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

i=1
while [ $i -le ${max_cluster_deployment_status_checks} ]; do
  IS_CLUSTER_DEPLOYMENT_INSTALLED=$(oc get cd ${CLUSTER_NAME} -n ${CLUSTER_NAMESPACE} -o json | jq .spec.installed )
  if [[ "${IS_CLUSTER_DEPLOYMENT_INSTALLED}" == "true" ]] ; then
    INSTALL_RESULT="success"
    break
  fi
  sleep ${sleep_between_cluster_deployment_status_checks}
  echo "Still waiting for the ClusterDeployment ${CLUSTER_NAME} to install. Status check #${i}/${max_cluster_deployment_status_checks}... "
  i=$((i + 1))
done

if [[ "${INSTALL_RESULT}" == "success" ]]
then
  echo "ClusterDeployment ${CLUSTER_NAME} was installed successfully"
else
  echo "Timed out waiting for the ClusterDeployment ${CLUSTER_NAME} to install"
fi

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
oc get clusterdeployment -A -o yaml &> "${ARTIFACT_DIR}/hive_clusterdeployment.yaml" || true
oc get clusterimageset -o yaml &> "${ARTIFACT_DIR}/hive_clusterimagesets.yaml" || true
oc get clusterprovision -A -o yaml &> "${ARTIFACT_DIR}/hive_clusterprovision.yaml" || true
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

echo "Running post-install tests"
make test-e2e-postinstall

echo "Running destroy test"
make test-e2e-destroycluster

echo "Saving hive logs"
save_hive_logs

echo "Uninstalling hive and validating cleanup"
make test-e2e-uninstallhive
