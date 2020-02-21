#!/bin/bash

set -e

DEPLOYMENT_NAME=${1:?must specify a deployment name}

echo "Scaling up $DEPLOYMENT_NAME"
oc scale -n hive "deployment.v1.apps/$DEPLOYMENT_NAME" --replicas=1
oc rollout status -n hive "deployment.v1.apps/$DEPLOYMENT_NAME" -w
if [[ "$(oc get -n hive "deployment.v1.apps/$DEPLOYMENT_NAME" -o jsonpath='{.spec.replicas}')" != "1" ]]
then
  echo "$DEPLOYMENT_NAME has not been scaled down to 0"
fi
