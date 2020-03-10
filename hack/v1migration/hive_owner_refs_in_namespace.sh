#!/bin/bash

set -e

WORKDIR=${1:?must specify working directory}
NAMESPACE=${2:?must specify namespace}

if [[ ! -d "$WORKDIR" ]]
then
  echo "Directory $WORKDIR does not exist."
  exit 1
fi

<"${WORKDIR}/owner-refs.json" jq --arg NAMESPACE "${NAMESPACE}" 'select(.namespace==$NAMESPACE)'
