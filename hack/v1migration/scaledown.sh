#!/bin/bash

set -e

# shellcheck source=verify_scaledown.sh
source "$(dirname "$0")/verify_scaledown.sh"

scaledown() {
  local deployment_name=${1:?must specify a deployment name}
  echo "Scaling down $deployment_name"
  oc scale -n hive "deployment.v1.apps/$deployment_name" --replicas=0
  verify_scaledown "$deployment_name"
}

scaledown "hive-operator"
scaledown "hive-controllers"
