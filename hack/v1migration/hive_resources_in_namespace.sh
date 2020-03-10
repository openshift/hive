#!/bin/bash

set -e

WORKDIR=${1:?must specify working directory}
NAMESPACE=${2:?must specify namespace}

if [[ ! -d "$WORKDIR" ]]
then
  echo "Directory $WORKDIR does not exist."
  exit 1
fi

for f in "${WORKDIR}"/*.json
do
  if [[ "$f" =~ (hiveconfigs)|(dnsendpoints)|(owner-refs).json ]]; then continue; fi
  <"$f" jq --arg NAMESPACE "$NAMESPACE" 'select(.metadata.namespace==$NAMESPACE)'
done
