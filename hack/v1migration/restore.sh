#!/bin/bash

set -e

WORKDIR=${1:?must specify working directory}
echo "Using workdir: $WORKDIR"

if [[ ! -d "$WORKDIR" ]]
then
  echo "Directory $WORKDIR does not exist."
  exit 1
fi

# shellcheck source=verify_scaledown.sh
source "$(dirname "$0")/verify_scaledown.sh"
verify_all_scaled_down

# shellcheck source=verify_apiserver.sh
source "$(dirname "$0")/verify_apiserver.sh"
verify_apiserver

echo "Restoring Hive resources"
find "${WORKDIR}" -maxdepth 1 -type f \
  -name "*.json" \
  -not -name "hiveconfigs.json" \
  -not -name "dnsendpoints.json" \
  -not -empty \
  -exec bin/hiveutil v1migration recreate-objects {} \;

echo "Restoring owner references to Hive resources"
bin/hiveutil v1migration restore-owner-refs --work-dir="${WORKDIR}"
