#!/bin/bash

set -e

WORKDIR=${1:?must specify working directory}
echo "Using workdir: $WORKDIR"

if [[ -n "$OWNED_RESOURCES" ]]
then
  RESOURCES_ARG="--resources $OWNED_RESOURCES"
fi

if [[ -z "$NO_SCALEDOWN_VERIFY" ]]
then
  # shellcheck source=verify_scaledown.sh
  source "$(dirname "$0")/verify_scaledown.sh"
  verify_all_scaled_down
fi

mkdir -p "$WORKDIR"

if [[ "$(ls "$WORKDIR" 2>/dev/null)" ]]
then
  echo "WARNING: workdir ($WORKDIR) is not empty."
  echo "Files in workdir may be overwritten."
  read -r -p "Do you wish to proceed? [y/N] " response
  if [[ ! "$response" =~ ^([yY]([eE][sS])?)$ ]]
  then
    exit 1
  fi
fi

# shellcheck source=hivetypes.sh
source "$(dirname "$0")/hivetypes.sh"

for t in "${HIVE_TYPES[@]}"
do
	echo "Storing all ${t} in ${WORKDIR}/${t}.json"
	oc get "${t}.hive.openshift.io" --all-namespaces -o json | jq .items[] > "${WORKDIR}/${t}.json"
done

bin/hiveutil v1migration save-owner-refs --work-dir "${WORKDIR}" $RESOURCES_ARG
