#!/bin/bash

set -e

SRC_DIR="$(git rev-parse --show-toplevel)"

if ! which kubefed2; then
	echo "Ensure that kubefed2 is installed and in your path"
	echo "go get -u github.com/kubernetes-sigs/federation-v2/cmd/kubefed2"
	exit 1
fi

kubectl apply --validate=false -f "${SRC_DIR}/hack/federation/deploy_federation.yaml"
kubectl apply --validate=false -f "${SRC_DIR}/hack/federation/cluster-registry-crd.yaml"
for filename in ${SRC_DIR}/hack/federation/federatedirectives/*.yaml; do
  kubefed2 federate enable -f "${filename}" --federation-namespace="federation-system"
done
kubefed2 federate enable clusterrolebindings --federation-namespace="federation-system"
