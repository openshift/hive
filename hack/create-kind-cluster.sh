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

# create registry container unless it already exists
reg_name='kind-registry'
reg_port='5000'
running="$(docker inspect -f '{{.State.Running}}' "${reg_name}" 2>/dev/null || true)"
if [ "${running}" != 'true' ]; then
  docker run \
    -d --restart=always -p "${reg_port}:5000" --name "${reg_name}" \
    registry:2
fi

# create a cluster with the local registry enabled in containerd
cat <<EOF | kind create cluster --name ${cluster_name} --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:${reg_port}"]
    endpoint = ["http://${reg_name}:${reg_port}"]
EOF

# connect the registry to the cluster network
docker network disconnect "kind" "${reg_name}" || true
docker network connect "kind" "${reg_name}"

