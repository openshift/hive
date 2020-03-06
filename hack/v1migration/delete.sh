#!/bin/bash

set -e

WORKDIR=${1:?must specify working directory}
echo "Using workdir: $WORKDIR"

# shellcheck source=verify_scaledown.sh
source "$(dirname "$0")/verify_scaledown.sh"
verify_all_scaled_down

echo "WARNING: this script will delete all Hive custom resources and their definitions"
echo "It should only be used during migrations to the v1 API."
read -r -p "Do you wish to proceed? [y/N] " response
if [[ ! "$response" =~ ^([yY]([eE][sS])?)$ ]]
then
  exit 1
fi

# shellcheck source=hivetypes.sh
source "$(dirname "$0")/hivetypes.sh"

for t in "${HIVE_TYPES[@]}"
do
  echo "Deleting ${t} objects"
  bin/hiveutil v1migration delete-objects "${WORKDIR}/${t}.json"

  for i in 1 2 3 4 5
  do
    if [[ "$(oc get "${t}.hive.openshift.io" --all-namespaces -o name | wc -l)" -eq 0 ]]; then break; fi
    if (( i == 5 ))
    then
      echo "There are still remaining ${t}"
      exit 1
    fi
    echo "Waiting for delete of ${t} to complete..."
    sleep 1
  done

	echo "Deleting ${t}.hive.openshift.io CRD"
  oc delete "customresourcedefinition.apiextensions.k8s.io/${t}.hive.openshift.io"
done
