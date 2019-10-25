#!/bin/bash

set -e

max_tries=60
sleep_between_tries=10
component=hive
local_hive_image=$(eval "echo $IMAGE_FORMAT")
export HIVE_IMAGE="${HIVE_IMAGE:-$local_hive_image}"
export RELEASE_IMAGE="${RELEASE_IMAGE:-registry.svc.ci.openshift.org/${OPENSHIFT_BUILD_NAMESPACE}/release:latest}"
export CLUSTER_NAMESPACE="${CLUSTER_NAMESPACE:-cluster-test}"

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

i=1
while [ $i -le ${max_tries} ]; do
  if [ $i -gt 1 ]; then
    # Don't sleep on first loop
    echo "sleeping ${sleep_between_tries} seconds"
    sleep ${sleep_between_tries}
  fi

  echo -n "Creating project ${CLUSTER_NAMESPACE}. Try #${i}/${max_tries}... "
  if oc new-project "${CLUSTER_NAMESPACE}"; then
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


# Install Hive
make deploy DEPLOY_IMAGE="${HIVE_IMAGE}"

CLOUD_CREDS_DIR="${CLOUD_CREDS_DIR:-/tmp/cluster}"

# Create a new cluster deployment
export CLOUD="${CLOUD:-aws}"
export CLUSTER_NAME="${CLUSTER_NAME:-hive-$(uuidgen | tr '[:upper:]' '[:lower:]')}"
export ARTIFACT_DIR="${ARTIFACT_DIR:-/tmp}"
export SSH_PUBLIC_KEY_FILE="${SSH_PUBLIC_KEY_FILE:-${CLOUD_CREDS_DIR}/ssh-publickey}"
export PULL_SECRET_FILE="${PULL_SECRET_FILE:-${CLOUD_CREDS_DIR}/pull-secret}"

function teardown() {
	echo ""
	echo ""
	echo "Deleting ClusterDeployment ${CLUSTER_NAME}"
	oc delete --wait=false clusterdeployment ${CLUSTER_NAME} || :

	if ! go run "${SRC_ROOT}/contrib/cmd/waitforjob/main.go" --log-level=debug "${CLUSTER_NAME}" "uninstall"
	then
		echo "Waiting for uninstall job failed"
		if oc logs job/${CLUSTER_NAME}-uninstall &> "${ARTIFACT_DIR}/hive_uninstall_job.log"
		then
			echo "************* UNINSTALL JOB LOG *************"
			cat "${ARTIFACT_DIR}/hive_uninstall_job.log"
			echo ""
			echo ""
		fi
		exit 1
	fi
}
trap 'teardown' EXIT

echo "Running post-deploy tests"
make test-e2e-postdeploy

SRC_ROOT=$(git rev-parse --show-toplevel)

case "${CLOUD}" in
"aws")
	CREDS_FILE="${CLOUD_CREDS_DIR}/.awscred"
	BASE_DOMAIN="${BASE_DOMAIN:-hive-ci.openshift.com}"
	# Generate a short random shard string for this cluster similar to OSD prod.
	# This is to prevent name conflicts across customer clusters.
	CLUSTER_SHARD=$(cat /dev/urandom | tr -dc 'a-z' | fold -w 8 | head -n 1)
	CLUSTER_DOMAIN="${CLUSTER_SHARD}.${BASE_DOMAIN}"
	echo "Using cluster base domain: ${CLUSTER_DOMAIN}"
	go run "${SRC_ROOT}/contrib/cmd/hiveutil/main.go" adm manage-dns enable ${BASE_DOMAIN} \
		--creds-file="${CREDS_FILE}"
	EXTRA_CREATE_CLUSTER_ARGS=" --manage-dns"
	;;
"azure")
	CREDS_FILE="${CLOUD_CREDS_DIR}/osServicePrincipal.json"
	BASE_DOMAIN="${BASE_DOMAIN:-ci.azure.devcluster.openshift.com}"
	# NOTE: No plans to implement DNS management for Azure at this time, so the cluster
	# will use the root base domain.
	CLUSTER_DOMAIN="${BASE_DOMAIN}"
	;;
"gcp")
	CREDS_FILE="${CLOUD_CREDS_DIR}/gce.json"
	BASE_DOMAIN="${BASE_DOMAIN:-origin-ci-int-gce.dev.openshift.com}"
	# TODO: Use a sharded base domain and --manage-dns as we do for AWS above
	CLUSTER_DOMAIN="${BASE_DOMAIN}"
	EXTRA_CREATE_CLUSTER_ARGS=" --gcp-project-id=openshift-gce-devel-ci"
	;;
*)
	echo "unknown cloud: ${CLOUD}"
	exit 1
	;;
esac

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

  if echo "${GET_BY_SHORT_NAME}" | grep 'BASEDOMAIN' ; then
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
oc get deployments -n hive
echo ""
echo "Pods in hive namespace"
oc get pods -n hive
echo ""
echo "Events in hive namespace"
oc get events -n hive

echo "Waiting for install job to start and complete"

INSTALL_RESULT=""
if go run "${SRC_ROOT}/contrib/cmd/waitforjob/main.go" "${CLUSTER_NAME}" "install"
then
	echo "ClusterDeployment ${CLUSTER_NAME} was installed successfully"
	INSTALL_RESULT="success"
fi

# Capture install logs
if INSTALL_JOB_NAME=$(oc get job -l "hive.openshift.io/cluster-deployment-name=${CLUSTER_NAME},hive.openshift.io/install=true" -o name) && [ "${INSTALL_JOB_NAME}" ]
then
	oc logs -c hive ${INSTALL_JOB_NAME} &> "${ARTIFACT_DIR}/hive_install_job.log" || true
	oc get ${INSTALL_JOB_NAME} -o yaml &> "${ARTIFACT_DIR}/hive_install_job.yaml" || true
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
