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
# This script creates a local insecure registry and configures the host OS to talk to it.
#
# USAGE: ./hack/create-insecure-registry.sh
#
# This script was copied and adapted from the kubefed project:
# https://github.com/kubernetes-sigs/kubefed/blob/master/scripts/create-clusters.sh

set -o errexit
set -o nounset
set -o pipefail

CONTAINER_REGISTRY_HOST="${CONTAINER_REGISTRY_HOST:-172.17.0.1:5000}"
docker_daemon_config="/etc/docker/daemon.json"
containerd_config="/etc/containerd/config.toml"

function create-insecure-registry() {
  # Run insecure registry as container
  docker run -d -p 5000:5000 --restart=always --name registry registry:2
}

function configure-insecure-registry() {
  local err=
  if sudo test -f "${docker_daemon_config}"; then
    if sudo grep -q "\"insecure-registries\": \[\"${CONTAINER_REGISTRY_HOST}\"\]" ${docker_daemon_config}; then
      return 0
    elif sudo grep -q "\"insecure-registries\": " ${docker_daemon_config}; then
      echo <<EOF "Error: ${docker_daemon_config} exists and \
is already configured with an 'insecure-registries' entry but not set to ${CONTAINER_REGISTRY_HOST}. \
Please make sure it is removed and try again."
EOF
      err=true
    fi
  elif pgrep -a dockerd | grep -q 'insecure-registry'; then
    echo <<EOF "Error: about to write ${docker_daemon_config}, but dockerd is already configured with \
an 'insecure-registry' command line option. Please make the necessary changes or disable \
the command line option and try again."
EOF
    err=true
  fi

  if [[ "${err}" ]]; then
    docker kill registry &> /dev/null
    docker rm registry &> /dev/null
    return 1
  fi

  configure-insecure-registry-and-reload "sudo bash -c" $(pgrep dockerd) ${docker_daemon_config}
}

function configure-insecure-registry-and-reload() {
  local cmd_context="${1}" # context to run command e.g. sudo, docker exec
  local docker_pid="${2}"
  local config_file="${3}"
  ${cmd_context} "$(insecure-registry-config-cmd ${config_file})"
  ${cmd_context} "$(reload-daemon-cmd "${docker_pid}")"
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

function reload-daemon-cmd() {
  echo "kill -s SIGHUP ${1}"
}

echo "Creating container registry on host"
create-insecure-registry

echo "Configuring container registry on host"
configure-insecure-registry

