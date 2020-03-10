#!/bin/bash

set -e

NAMESPACE=${1:?must specify namespace}
echo "Using namespace: $NAMESPACE"

# shellcheck source=hivetypes.sh
source "$(dirname "$0")/hivetypes.sh"

INSTALL_CONFIG_SECRETS=$(oc get clusterdeployments -n "${NAMESPACE}" -ojson | \
  jq -r '.items[].spec.provisioning.installConfigSecretRef.name')


for t in "${NAMESPACE_SCOPED_HIVE_TYPES[@]}"
do
  if [[ "$t" == "dnsendpoints" ]]; then continue; fi
  echo "Deleting $t"
  oc get "$t".v1alpha1.hive.openshift.io -n "${NAMESPACE}" -oname | \
    xargs -i oc patch {} -n "${NAMESPACE}" --type='merge' -p $'metadata:\n finalizers: []'
  oc delete "$t".v1alpha1.hive.openshift.io -n "${NAMESPACE}" --all --cascade=false
done

echo "Deleting machinepools"
oc delete machinepools -n "${NAMESPACE}" --all --cascade=false

echo "Deleting installconfig secrets"
for name in $INSTALL_CONFIG_SECRETS
do
  oc delete secret "$name" -n "${NAMESPACE}"
done
