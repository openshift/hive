#!/usr/bin/env bash

export SVC_NAME="${SVC_NAME:-hiveapi}"
export SVC_NAMESPACE="${SVC_NAMESPACE:-hive}"

oc process --local=true -f "config/apiserver/apiserver.template.yaml" \
  -p CA_CRT="$(base64 -w0 config/apiserver/certificates/apiserver_ca.crt)" \
  -p TLS_CRT="$(base64 -w0 config/apiserver/certificates/apiserver.crt)" \
  -p TLS_KEY="$(base64 -w0 config/apiserver/certificates/apiserver.key)" \
  | oc apply -f -
