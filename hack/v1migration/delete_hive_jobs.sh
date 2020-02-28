#!/bin/bash

set -e

# shellcheck source=verify_scaledown.sh
source "$(dirname "$0")/verify_scaledown.sh"
verify_all_scaled_down

delete_jobs() {
  local labels=${1:?must specify a label for the jobs to delete}
  oc get jobs -l "${labels}" --all-namespaces -o json | \
    jq -r '.items[]?.metadata.namespace' | \
    sort -u | \
    xargs -i oc delete jobs -n {} -l "${labels}"
}

echo "Deleting install jobs"
delete_jobs "hive.openshift.io/install=true"

echo "Deleting imageset jobs"
delete_jobs "hive.openshift.io/imageset=true"

echo "Deleting uninstall jobs"
delete_jobs "hive.openshift.io/uninstall=true"