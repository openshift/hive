#!/usr/bin/env bash

export SVC_NAME="${SVC_NAME:-hiveapi}"
export SVC_NAMESPACE="${SVC_NAMESPACE:-hive}"
export HIVE_IMAGE="${HIVE_IMAGE:-registry.svc.ci.openshift.org/openshift/hivev1:hive}"

oc process --local=true -f "config/apiserver/apiserver.template.yaml" \
  -p CA_CRT="$(base64 -w0 hiveapi-certs/apiserver_ca.crt)" \
  -p TLS_CRT="$(base64 -w0 hiveapi-certs/apiserver.crt)" \
  -p TLS_KEY="$(base64 -w0 hiveapi-certs/apiserver.key)" \
  -p HIVE_IMAGE="${HIVE_IMAGE}" \
  | oc apply -f -
