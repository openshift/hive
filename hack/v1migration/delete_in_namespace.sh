#!/bin/bash

set -e

NAMESPACE=${1:?must specificy namespace}
echo "Using namespace: $NAMESPACE"

# shellcheck source=hivetypes.sh
source "$(dirname "$0")/hivetypes.sh"

echo "Deleting machinepools"
oc delete machinepools -n "${NAMESPACE}" --all --cascade=false

echo "Deleting installconfig secrets"
oc get clusterdeployments -n "${NAMESPACE}" -ojson | \
  jq -r '.items[].spec.provisioning.installConfigSecretRef.name' | \
  xargs -i oc delete secret {} -n "${NAMESPACE}"

for t in "${NAMESPACE_SCOPED_HIVE_TYPES[@]}"
do
  if [[ "$t" == "dnsendpoints" ]]; then continue; fi
  echo "Deleting $t"
  oc get "$t".v1alpha1.hive.openshift.io -n "${NAMESPACE}" -oname | \
    xargs -i oc patch {} -n "${NAMESPACE}" --type='merge' -p $'metadata:\n finalizers: []'
  oc delete "$t".v1alpha1.hive.openshift.io -n "${NAMESPACE}" --all --cascade=false
done
