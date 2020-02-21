#!/bin/bash

set -e

# shellcheck source=verify_scaledown.sh
source "$(dirname "$0")/verify_scaledown.sh"

DEPLOYMENT_NAME=${1:?must specify a deployment name}

echo "Scaling down $DEPLOYMENT_NAME"
oc scale -n hive "deployment.v1.apps/$DEPLOYMENT_NAME" --replicas=0
verify_scaledown "$DEPLOYMENT_NAME"
