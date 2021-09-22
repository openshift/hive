#!/bin/bash

set -e

TEST_NAME=e2e
source ${0%/*}/e2e-common.sh


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

echo "Running post-deploy tests"
make test-e2e-postdeploy

export CLUSTER_NAME="${CLUSTER_NAME:-hive-$(uuidgen | tr '[:upper:]' '[:lower:]')}"

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
	${EXTRA_CREATE_CLUSTER_ARGS}

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
  CD_JSON=$(oc get cd ${CLUSTER_NAME} -n ${CLUSTER_NAMESPACE} -o json)
  if [[ $(jq .spec.installed <<<"${CD_JSON}") == "true" ]] ; then
    INSTALL_RESULT="success"
    break
  fi
  PF_COND=$(jq -r '.status.conditions[] | select(.type == "ProvisionFailed")' <<<"${CD_JSON}")
  if [[ $(jq -r .status <<<"${PF_COND}") == 'True' ]]; then
    INSTALL_RESULT="failure"
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
    ${0%/*}/statuspatch dnszone -n ${CLUSTER_NAMESPACE} ${CLUSTER_NAME}-zone <<< '.status.lastSyncGeneration = 0'
  fi
  sleep ${sleep_between_cluster_deployment_status_checks}
  echo "Still waiting for the ClusterDeployment ${CLUSTER_NAME} to install. Status check #${i}/${max_cluster_deployment_status_checks}... "
  i=$((i + 1))
done

case "${INSTALL_RESULT}" in
    success)
        echo "ClusterDeployment ${CLUSTER_NAME} was installed successfully"
        ;;
    failure)
        echo "ClusterDeployment ${CLUSTER_NAME} provision failed" >&2
        echo "Reason: $FAILURE_REASON" >&2
        echo "Message: $FAILURE_MESSAGE" >&2
        ;;
    *)
        echo "Timed out waiting for the ClusterDeployment ${CLUSTER_NAME} to install" >&2
        echo "You may be interested in its status conditions:" >&2
        jq -r .status.conditions <<<"${CD_JSON}" >&2
        ;;
esac

capture_manifests
capture_cluster_logs $CLUSTER_NAME $CLUSTER_NAMESPACE $INSTALL_RESULT

echo "Running post-install tests"
make test-e2e-postinstall

echo "Running destroy test"
make test-e2e-destroycluster

echo "Saving hive logs"
save_hive_logs

echo "Uninstalling hive and validating cleanup"
make test-e2e-uninstallhive
