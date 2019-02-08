#!/bin/bash

set -e

component=hive
TEST_IMAGE=$(eval "echo $IMAGE_FORMAT")
component=installer
INSTALLER_IMAGE=$(eval "echo $IMAGE_FORMAT")

ln -s $(which oc) $(pwd)/kubectl
export PATH=$PATH:$(pwd)

# download kustomize so we can use it for deploying
curl -O -L https://github.com/kubernetes-sigs/kustomize/releases/download/v2.0.0/kustomize_2.0.0_linux_amd64
mv kustomize_2.0.0_linux_amd64 kustomize
chmod u+x kustomize

oc new-project cluster-test

# Install Hive
make deploy DEPLOY_IMAGE="${TEST_IMAGE}"

CLOUD_CREDS_DIR="/tmp/cluster"

# Create a new cluster deployment
# TODO: Determine which domain to use to create Hive clusters
export BASE_DOMAIN="hive-ci.openshift.com"
export CLUSTER_NAME="$(oc get cluster.cluster.k8s.io -n openshift-cluster-api -o jsonpath='{ .items[].metadata.name }')-1"
export SSH_PUB_KEY="$(cat ${CLOUD_CREDS_DIR}/ssh-publickey)"
export PULL_SECRET="$(cat ${CLOUD_CREDS_DIR}/pull-secret)"
export AWS_ACCESS_KEY_ID="$(cat ${CLOUD_CREDS_DIR}/.awscred | awk '/aws_access_key_id/ { print $3; exit; }')"
export AWS_SECRET_ACCESS_KEY="$(cat ${CLOUD_CREDS_DIR}/.awscred | awk '/aws_secret_access_key/ { print $3; exit; }')"

function teardown() {
	oc get configmap/${CLUSTER_NAME}-installconfig -o jsonpath='{ .data.install-config\.yaml }' > "${ARTIFACT_DIR}/install-config.yaml" || true
	oc logs -c installer job/${CLUSTER_NAME}-install &> "${ARTIFACT_DIR}/hive_installer_container.log" || true
	oc logs -c hive job/${CLUSTER_NAME}-install &> "${ARTIFACT_DIR}/hive_install_job.log" || true
	cat "${ARTIFACT_DIR}/hive_install_job.log"
	echo "Deleting ClusterDeployment ${CLUSTER_NAME}"
	oc delete clusterdeployment ${CLUSTER_NAME}
}
trap 'teardown' EXIT

# TODO: Determine how to wait for readiness of the validation webhook
sleep 120

echo "Creating ClusterDeployment ${CLUSTER_NAME}"

oc process -f config/templates/cluster-deployment.yaml \
   CLUSTER_NAME="${CLUSTER_NAME}" \
   SSH_KEY="${SSH_PUB_KEY}" \
   PULL_SECRET="${PULL_SECRET}" \
   AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID}" \
   AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY}" \
   BASE_DOMAIN="${BASE_DOMAIN}" \
   INSTALLER_IMAGE="${INSTALLER_IMAGE}" \
   OPENSHIFT_RELEASE_IMAGE="" \
   TRY_INSTALL_ONCE="true" \
   | oc apply -f -

# Wait for the cluster deployment to be installed
SRC_ROOT=$(git rev-parse --show-toplevel)

echo "Waiting for job ${CLUSTER_NAME}-install to start and complete"

go run "${SRC_ROOT}/contrib/cmd/waitforjob/main.go" "${CLUSTER_NAME}-install"

echo "ClusterDeployment ${CLUSTER_NAME} was installed successfully"
