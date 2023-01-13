#!/bin/sh
#
# This script creates a kind cluster and configures it to use an insecure registry
# running on the host OS.
#
# USAGE: ./hack/create-kind-cluster.sh [cluster_name]
#
# Adapted from: https://github.com/kubernetes-sigs/kind/blob/master/site/static/examples/kind-with-registry.sh

set -o errexit

cluster_name="${1:-hive}"

# Discover podman and docker binaries, if they exist.
PODMAN="${PODMAN:-$(which podman 2> /dev/null || true)}"
DOCKER="${DOCKER:-$(which docker 2> /dev/null || true)}"

# Fail when neither podman nor docker are found.
if [ "${PODMAN}" == "" -a "${DOCKER}" == "" ]; then
  echo "Unable to find podman or docker."
  exit 1;
fi

# Prefer podman and fallthrough to docker.
container_cmd="${PODMAN:-${DOCKER}}"
echo "Using ${container_cmd}"

reg_name='kind-registry'
reg_port='5000'

# Podman containers should default to the podman network.
if [ "${container_cmd}" == "${PODMAN}" ]; then
  reg_network_param="--network ${reg_network:-podman}"
fi

# Create registry container unless it already exists
running="$(${container_cmd} inspect -f '{{.State.Running}}' "${reg_name}" 2>/dev/null || true)"
if [ "${running}" != 'true' ]; then
  ${container_cmd} run \
    -d --restart=always \
    -p "${reg_port}:5000" \
    --name "${reg_name}" \
    ${reg_network_param} \
    registry:2
else
  echo "Registry ${reg_name} already exists."
fi

# Determine the local registry endpoint parameter
endpoint="http://${reg_name}:${reg_port}"
if [ "${container_cmd}" == "${PODMAN}" ]; then
  # Grab the registry IP, between containers we will use this, but dev user can push to localhost, and specify localhost
  # in your pod manifests thanks to the endpoint override below
  reg_ip="$(${container_cmd} inspect ${reg_name} -f '{{.NetworkSettings.IPAddress}}')"
  endpoint="http://${reg_ip}:${reg_port}"
fi

# Configure kind to use podman experimental provider
if [ "${container_cmd}" == "${PODMAN}" ]; then
  export KIND_EXPERIMENTAL_PROVIDER="podman"
fi

# Create a cluster with the local registry enabled in containerd
cat <<EOF | kind create cluster --name ${cluster_name} \
  --kubeconfig ${HOME}/.kube/kind-${cluster_name}.kubeconfig \
  --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:${reg_port}"]
    endpoint = ["${endpoint}"]
EOF

if [ "${container_cmd}" == "${DOCKER}" ]; then
  # connect the registry to the cluster network
  ${container_cmd} network disconnect "kind" "${reg_name}" 2> /dev/null || true
  ${container_cmd} network connect "kind" "${reg_name}"
fi
