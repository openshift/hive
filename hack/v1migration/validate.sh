#!/bin/bash

WORKDIR=${1:?must specify working directory}
echo "Using workdir: $WORKDIR"

if [[ ! -d "$WORKDIR" ]]
then
  echo "Directory $WORKDIR does not exist."
  exit 1
fi

# shellcheck source=hivetypes.sh
source "$(dirname "$0")/hivetypes.sh"

TEMPDIR="$(mktemp -d)"

for t in "${HIVE_TYPES[@]}"
do
  if [[ "$t" == "dnsendpoints" ]]; then continue; fi

  echo "Validating ${t}"

  <"${WORKDIR}/$t.json" jq '.metadata.namespace + "/" + .metadata.name' | sort > "${TEMPDIR}/$t.orig"
  oc get "$t.v1alpha1.hive.openshift.io" --all-namespaces -o json | jq '.items[] | .metadata.namespace + "/" + .metadata.name' | sort > "${TEMPDIR}/$t.restored"

  echo "Diff in resources"
  diff "${TEMPDIR}/$t.orig" "${TEMPDIR}/$t.restored"

  echo "Deleted resources"
  oc get "$t.v1alpha1.hive.openshift.io" --all-namespaces -o json | jq '.items[] | select(.metadata.deletionTimestamp) | .metadata.namespace + "/" + .metadata.name'
done

echo "Validating machinepools"

<"${WORKDIR}/clusterdeployments.json" jq '.metadata.namespace + "/" + .metadata.name + "-" + .spec.compute[]?.name' | sort > "${TEMPDIR}/machinepools.orig"
oc get machinepools --all-namespaces -o json | jq '.items[] | .metadata.namespace + "/" + .metadata.name' | sort > "${TEMPDIR}/machinepools.restored"

echo "Diff in resources"
diff "${TEMPDIR}/machinepools.orig" "${TEMPDIR}/machinepools.restored"

echo "Deleted resources"
oc get machinepools --all-namespaces -o json | jq '.items[] | select(.metadata.deletionTimestamp) | .metadata.namespace + "/" + .metadata.name'
