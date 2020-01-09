#!/usr/bin/env bash

export SVC_NAME="${SVC_NAME:-hiveapi}"
export SVC_NAMESPACE="${SVC_NAMESPACE:-hive}"

mkdir -p "config/apiserver/certificates"

openssl req -x509 \
  -newkey rsa:2048 \
	-keyout "config/apiserver/certificates/apiserver_ca.key" \
	-out "config/apiserver/certificates/apiserver_ca.crt" \
	-days 365 \
	-nodes \
	-subj "/C=un/ST=st/L=l/O=o/OU=ou/CN=${SVC_NAME}-certificate-authority"

openssl req \
  -out "config/apiserver/certificates/apiserver.csr" \
	-new \
	-newkey rsa:2048 \
	-nodes \
	-keyout "config/apiserver/certificates/apiserver.key" \
	-subj "/C=un/ST=st/L=l/O=o/OU=ou/CN=${SVC_NAME}.${SVC_NAMESPACE}.svc"

openssl x509 -req \
  -days 365 \
  -in "config/apiserver/certificates/apiserver.csr" \
	-CA "config/apiserver/certificates/apiserver_ca.crt" \
	-CAkey "config/apiserver/certificates/apiserver_ca.key" \
	-CAcreateserial \
	-out "config/apiserver/certificates/apiserver.crt"
