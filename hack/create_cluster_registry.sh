#!/usr/bin/env bash

HIVE_ROOT="$(git rev-parse --show-toplevel)"
CNI_PATH="$HIVE_ROOT"/.tmp/_output/cni/bin
export HIVE_ROOT
export CNI_PATH
export PATH=$HIVE_ROOT/.tmp/_output/bin:$PATH

set -o errexit

cluster_name="dev-hive"
reg_name='kind-nerdctl-registry'
reg_port='5000'

sleep 3

# create cluster
cat <<EOF | KIND_EXPERIMENTAL_PROVIDER="nerdctl" kind create cluster --name "${cluster_name}" --kubeconfig "${HIVE_ROOT}"/.tmp/_output/"${cluster_name}".kubeconfig --config=-

kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
 [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:${reg_port}"]
   endpoint = ["http://${reg_name}:${reg_port}"]
EOF

sleep 3

# create registry
nerdctl run -d --restart=always -p "5000:5000" --name "kind-nerdctl-registry" --network "kind" registry:2
