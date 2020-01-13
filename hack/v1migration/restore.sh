#!/bin/bash

set -e

WORKDIR=${1:?must specify working directory}
echo "Using workdir: $WORKDIR"

if [[ ! -d "$WORKDIR" ]]
then
  echo "Directory $WORKDIR does not exist."
  exit 1
fi

source "$(dirname $0)/verify_scaledown.sh"
verify_all_scaled_down

echo "Restoring Hive resources"
find "${WORKDIR}" -maxdepth 1 -type f -name "*.json" -not -empty -exec bin/hiveutil v1migration recreate-objects {} \;

echo "Restoring owner references to Hive resources"
bin/hiveutil v1migration restore-owner-refs --work-dir="${WORKDIR}"
