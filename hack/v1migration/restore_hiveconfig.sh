#!/bin/bash

set -e

WORKDIR=${1:?must specify working directory}
echo "Using workdir: $WORKDIR"

if [[ ! -d "$WORKDIR" ]]
then
  echo "Directory $WORKDIR does not exist."
  exit 1
fi

JSONFILE="$WORKDIR/hiveconfigs.json"

if [[ ! -s "$JSONFILE" ]]
then
  echo "Directory $WORKDIR does not contain a hiveconfigs.json file or it is empty"
  exit 1
fi

if [[ $(<"$JSONFILE" jq '(.apiVersion=="hive.openshift.io/v1alpha1") and (.kind=="HiveConfig") and (.metadata.name=="hive")') != "true" ]]
then
  echo "$WORKDIR/hiveconfig.json does not contain the HiveConfig"
  exit 1
fi

# shellcheck source=verify_apiserver.sh
source "$(dirname "$0")/verify_apiserver.sh"
verify_apiserver

echo "Deleting existing HiveConfig"
oc delete hiveconfig hive --ignore-not-found

echo "Restoring HiveConfig"
oc create -f "$JSONFILE"

# shellcheck source=deploy_apiserver.sh
"$(dirname "$0")/deploy_apiserver.sh"
