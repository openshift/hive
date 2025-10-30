#!/usr/bin/env bash

set -e -o pipefail

CMD=${0##*/}

usage() {
    cat -<<EOF>&2
$CMD: Nondisruptively replace the cloud credentials for existing
clusters belonging to a ClusterPool.

Usage: $CMD CLUSTERPOOL_NAMESPACE CLUSTERPOOL_NAME
EOF
    exit -1
}

[[ $# -eq 2 ]] || usage
CLUSTERPOOL_NAMESPACE=$1
CLUSTERPOOL_NAME=$2

# TODO: Support for other platforms
creds_secret_name_jsonpath='.spec.platform.aws.credentialsSecretRef.name'

pool_creds_secret=$(oc get clusterpool -n "$CLUSTERPOOL_NAMESPACE" "$CLUSTERPOOL_NAME" -o jsonpath="{$creds_secret_name_jsonpath}")

creds_data=$(oc get secret -n "$CLUSTERPOOL_NAMESPACE" "$pool_creds_secret" -o jsonpath={.data})

oc get cd -A -l "hive.openshift.io/clusterpool-namespace=$CLUSTERPOOL_NAMESPACE,hive.openshift.io/clusterpool-name=$CLUSTERPOOL_NAME" -o custom-columns=:.metadata.namespace,:.metadata.name --no-headers | while read ns name; do
    echo -n "Refreshing creds for ClusterDeployment $ns/$name... "
    cd_creds_secret=$(oc get cd -n $ns $name -o jsonpath="{$creds_secret_name_jsonpath}")
    # TODO: Use a shm, named pipe, or tempfile so the creds don't show
    # up in the process table in cleartext (b64). Not a risk when
    # running locally. Generally not a risk when running in a pod, as
    # that pod has permission to see the Secret anyway.
    oc patch secret -n $ns $cd_creds_secret -p '{"data": '"$creds_data"'}'
done
