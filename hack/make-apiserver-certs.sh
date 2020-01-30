#!/usr/bin/env bash
set -e

export SVC_NAME="${SVC_NAME:-hiveapi}"
export SVC_NAMESPACE="${SVC_NAMESPACE:-hive}"

mkdir "hiveapi-certs"

openssl req -x509 \
  -newkey rsa:2048 \
	-keyout "hiveapi-certs/apiserver_ca.key" \
	-out "hiveapi-certs/apiserver_ca.crt" \
	-days 365 \
	-nodes \
	-subj "/C=un/ST=st/L=l/O=o/OU=ou/CN=${SVC_NAME}-certificate-authority"

openssl req \
  -out "hiveapi-certs/apiserver.csr" \
	-new \
	-newkey rsa:2048 \
	-nodes \
	-keyout "hiveapi-certs/apiserver.key" \
	-subj "/C=un/ST=st/L=l/O=o/OU=ou/CN=${SVC_NAME}.${SVC_NAMESPACE}.svc"

openssl x509 -req \
  -days 365 \
  -in "hiveapi-certs/apiserver.csr" \
	-CA "hiveapi-certs/apiserver_ca.crt" \
	-CAkey "hiveapi-certs/apiserver_ca.key" \
	-CAcreateserial \
	-out "hiveapi-certs/apiserver.crt"
