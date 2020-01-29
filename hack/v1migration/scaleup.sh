#!/bin/bash

set -e

scaleup() {
  local deployment_name=${1:?must specify a deployment name}
  echo "Scaling up $deployment_name"
  oc scale -n hive "deployment.v1.apps/$deployment_name" --replicas=1
  oc rollout status -n hive "deployment.v1.apps/$deployment_name" -w
  if [[ "$(oc get -n hive "deployment.v1.apps/$deployment_name" -o jsonpath='{.spec.replicas}')" != "1" ]]
  then
    echo "$deployment_name has not been scaled down to 0"
  fi
}

scaleup "hive-operator"
scaleup "hive-controllers"
