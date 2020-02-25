#!/bin/bash

set -e

if [[ "$#" -ne 1 ]]; then
	echo "Usage: $0 CAFILENAME - where CAFILENAME is a file that contains a ca.crt"
	exit
fi

if oc get secret -n hive additional-ca &> /dev/null; then
	oc delete secret -n hive additional-ca
fi

oc create secret generic --from-file "ca.crt=${1}" -n hive additional-ca

oc patch hiveconfig/hive --type=json -p '[{"op":"replace", "path":"/spec/additionalCertificateAuthoritiesSecretRef", "value":[{"name": "additional-ca"}]}]'
