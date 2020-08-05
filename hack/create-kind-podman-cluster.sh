#!/bin/sh
#
# This script creates a kind cluster and configures it to use an insecure registry
# running on the host OS.
#
# USAGE: ./hack/create-kind-podman-cluster.sh [cluster_name]
#
# Adapted from: https://github.com/kubernetes-sigs/kind/blob/master/site/static/examples/kind-with-registry.sh

set -o errexit

cluster_name="${1:-hive}"

# create registry container unless it already exists
reg_name='kind-podman-registry'
reg_port='5000'
running="$(sudo podman inspect -f '{{.State.Running}}' "${reg_name}" 2>/dev/null || true)"
if [ "${running}" != 'true' ]; then
  sudo podman run \
    -d --restart=always -p "${reg_port}:5000" --name "${reg_name}" \
    registry:2
else
  echo "Registry ${reg_name} already exists."
fi

# Grab the registry IP, between containers we will use this, but dev user can push to localhost, and specify localhost
# in your pod manifests thanks to the endpoint override below
reg_ip="$(sudo podman inspect kind-podman-registry -f '{{.NetworkSettings.IPAddress}}')"

# create a cluster with the local registry enabled in containerd
cat <<EOF | sudo KIND_EXPERIMENTAL_PROVIDER="podman" kind create cluster --name ${cluster_name} --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:${reg_port}"]
    endpoint = ["http://${reg_ip}:${reg_port}"]
EOF

# connect the registry to the cluster network
#podman network disconnect "kind" "${reg_name}" || true
#podman network connect "kind" "${reg_name}"

sudo cp /root/.kube/config ~/.kube/${cluster_name}.kubeconfig
sudo chown $USER ~/.kube/${cluster_name}.kubeconfig
echo "Kubeconfig written to $HOME/.kube/${cluster_name}.kubeconfig"

