#!/bin/bash
set -e

HIVE_NS="${HIVE_NS:-hive}"

mkdir hiveadmission-certs
pushd hiveadmission-certs
cat <<EOF | cfssl genkey - | cfssljson -bare server
{
  "hosts": [
    "hiveadmission.${HIVE_NS}.svc",
    "hiveadmission.${HIVE_NS}.svc.cluster.local"
  ],
  "CN": "system:node:hiveadmission.${HIVE_NS}.svc",
  "names": [
    {
      "O": "system:nodes"
    }
  ],
  "key": {
    "algo": "ecdsa",
    "size": 256
  }
}
EOF

cat <<EOF | oc apply -f -
apiVersion: certificates.k8s.io/v1
kind: CertificateSigningRequest
metadata:
  name: hiveadmission.${HIVE_NS}
spec:
  request: $(cat server.csr | base64 | tr -d '\n')
  signerName: kubernetes.io/kubelet-serving
  usages:
  - digital signature
  - key encipherment
  - server auth
EOF

oc adm certificate approve hiveadmission.${HIVE_NS}

sleep 5
oc get csr hiveadmission.${HIVE_NS} -o jsonpath='{.status.certificate}' | base64 --decode > server.crt

cat server.crt

cat <<EOF | oc apply -f -
kind: Secret
apiVersion: v1
data:
  tls.crt: $(cat server.crt | base64 | tr -d '\n')
  tls.key: $(cat server-key.pem | base64 | tr -d '\n')
metadata:
  name: hiveadmission-serving-cert
  namespace: ${HIVE_NS}
type: kubernetes.io/tls
EOF


popd

