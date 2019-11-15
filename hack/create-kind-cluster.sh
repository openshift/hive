#!/usr/bin/env bash

# Copyright 2018 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# This script creates a kind cluster and configures it to use an insecure registry
# running on the host OS.
#
# USAGE: ./hack/create-kind-cluster.sh [cluster_name]
#
# This script was adapted from the kubefed project:
# https://github.com/kubernetes-sigs/kubefed/blob/master/scripts/create-clusters.sh

set -o errexit
set -o nounset
set -o pipefail

CONTAINER_REGISTRY_HOST="${CONTAINER_REGISTRY_HOST:-172.17.0.1:5000}"
CLUSTER_NAME="${1:-hive}"
OVERWRITE_KUBECONFIG="${OVERWRITE_KUBECONFIG:-}"
KIND_IMAGE="${KIND_IMAGE:-}"
KIND_TAG="${KIND_TAG:-}"
containerd_config="/etc/containerd/config.toml"
kubeconfig="${HOME}/.kube/config"
docker_daemon_config="/etc/docker/daemon.json"
OS=`uname`

function create-cluster() {
  local cluster_name=${1}

  local image_arg=""
  if [[ "${KIND_IMAGE}" ]]; then
    image_arg="--image=${KIND_IMAGE}"
  elif [[ "${KIND_TAG}" ]]; then
    image_arg="--image=kindest/node:${KIND_TAG}"
  fi
  kind create cluster --name "${cluster_name}" ${image_arg}
  # TODO(font): remove once all workarounds are addressed.
  fixup-cluster ${cluster_name}
  echo

  echo "Waiting for clusters to be ready"
  check-cluster-ready ${cluster_name}

  # TODO(font): kind will create separate kubeconfig files for each cluster.
  # Remove once https://github.com/kubernetes-sigs/kind/issues/113 is resolved.
  if [[ "${OVERWRITE_KUBECONFIG}" ]]; then
    kubectl config view --flatten > ${kubeconfig}
    unset KUBECONFIG
  fi

  # TODO(font): Configure insecure registry on kind host cluster. Remove once
  # https://github.com/kubernetes-sigs/kind/issues/110 is resolved.
  echo "Configuring insecure container registry on kind host cluster"
  configure-insecure-registry-on-cluster ${cluster_name}
}

function fixup-cluster() {
  local cluster_name=${1} # cluster num

  local kubeconfig_path="$(kind get kubeconfig-path --name ${cluster_name})"
  export KUBECONFIG="${kubeconfig_path}"

  if [ "$OS" != "Darwin" ];then
    # Set container IP address as kube API endpoint in order for clusters to reach kube API servers in other clusters.
    kind get kubeconfig --name "${cluster_name}" --internal >${kubeconfig_path}
  fi

  # Simplify context name
  kubectl config rename-context "kubernetes-admin@${cluster_name}" "${cluster_name}-context"
  kubectl config use-context "${cluster_name}-context"

  # TODO(font): Need to rename auth user name to avoid conflicts when using
  # multiple cluster kubeconfigs. Remove once
  # https://github.com/kubernetes-sigs/kind/issues/112 is resolved.
  sed -i.bak "s/kubernetes-admin/kubernetes-${cluster_name}-admin/" ${kubeconfig_path} && rm -rf ${kubeconfig_path}.bak
}

function check-cluster-ready() {
  local cluster_name=${1}
  local kubeconfig_path="$(kind get kubeconfig-path --name ${cluster_name})"
  wait-for-condition 'ok' "kubectl --kubeconfig ${kubeconfig_path} --context ${cluster_name}-context get --raw=/healthz &> /dev/null" 120
}

function configure-insecure-registry-on-cluster() {
  cmd_context="docker exec ${1}-control-plane bash -c"
  containerd_id=`${cmd_context} "pgrep -x containerd"`

  configure-insecure-registry-and-reload "${cmd_context}" ${containerd_id} ${containerd_config}
}

function configure-insecure-registry-and-reload() {
  local cmd_context="${1}" # context to run command e.g. sudo, docker exec
  local docker_pid="${2}"
  local config_file="${3}"
  ${cmd_context} "$(insecure-registry-config-cmd ${config_file})"
  ${cmd_context} "$(reload-daemon-cmd "${docker_pid}")"
}

function reload-daemon-cmd() {
  echo "kill -s SIGHUP ${1}"
}

function insecure-registry-config-cmd() {
  local config_file="${1}"
  case $config_file in
    $docker_daemon_config)
      echo "cat <<EOF > ${docker_daemon_config}
{
    \"insecure-registries\": [\"${CONTAINER_REGISTRY_HOST}\"]
}
EOF
"
      ;;
    $containerd_config)
     echo "sed -i '/\[plugins.cri.registry.mirrors\]/a [plugins.cri.registry.mirrors."\"${CONTAINER_REGISTRY_HOST}\""]\nendpoint = ["\"http://${CONTAINER_REGISTRY_HOST}\""]' ${containerd_config}"
     ;;
    *)
     echo "Sorry, config insecure registy is not supported for $config_file"
     ;;
  esac
}


# wait-for-condition blocks until the provided condition becomes true
#
# Globals:
#  None
# Arguments:
#  - 1: message indicating what conditions is being waited for (e.g. 'config to be written')
#  - 2: a string representing an eval'able condition.  When eval'd it should not output
#       anything to stdout or stderr.
#  - 3: optional timeout in seconds.  If not provided, waits forever.
# Returns:
#  1 if the condition is not met before the timeout
function wait-for-condition() {
  local msg=$1
  # condition should be a string that can be eval'd.
  local condition=$2
  local timeout=${3:-}

  local start_msg="Waiting for ${msg}"
  local error_msg="[ERROR] Timeout waiting for ${msg}"

  local counter=0
  while ! eval ${condition}; do
    if [[ "${counter}" = "0" ]]; then
      echo -n "${start_msg}"
    fi

    if [[ -z "${timeout}" || "${counter}" -lt "${timeout}" ]]; then
      counter=$((counter + 1))
      if [[ -n "${timeout}" ]]; then
        echo -n '.'
      fi
      sleep 1
    else
      echo -e "\n${error_msg}"
      return 1
    fi
  done

  if [[ "${counter}" != "0" && -n "${timeout}" ]]; then
    echo ' done'
  fi
}
readonly -f wait-for-condition

echo "Creating cluster $CLUSTER_NAME"
create-cluster $CLUSTER_NAME

echo "Complete"

if [[ ! "${OVERWRITE_KUBECONFIG}" ]]; then
    echo <<EOF "OVERWRITE_KUBECONFIG was not set so ${kubeconfig} was not modified. \
You can access your clusters by setting your KUBECONFIG environment variable using:

export KUBECONFIG=\"${KUBECONFIG}\"

Then you can overwrite ${kubeconfig} if you prefer using:

kubectl config view --flatten > ${kubeconfig}
unset KUBECONFIG
"
EOF
fi
