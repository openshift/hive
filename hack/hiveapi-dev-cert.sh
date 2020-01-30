#!/bin/bash
set -e

mkdir hiveapi-certs
pushd hiveapi-certs

cat <<EOF | cfssl genkey - | cfssljson -bare server
{
  "hosts": [
    "hiveapi.hive.svc",
    "hiveapi.hive.svc.cluster.local"
  ],
  "CN": "hiveapi.hive.svc",
  "key": {
    "algo": "ecdsa",
    "size": 256
  }
}
EOF


cat <<EOF | kubectl apply -f -
apiVersion: certificates.k8s.io/v1beta1
kind: CertificateSigningRequest
metadata:
  name: hiveapi.hive
spec:
  request: $(cat server.csr | base64 | tr -d '\n')
  usages:
  - digital signature
  - key encipherment
  - server auth
EOF

kubectl certificate approve hiveapi.hive

sleep 5
kubectl get csr hiveapi.hive -o jsonpath='{.status.certificate}' | base64 --decode > server.crt

cat server.crt

cat <<EOF | kubectl apply -f -
kind: Secret
apiVersion: v1
data:
  tls.crt: $(cat server.crt | base64 | tr -d '\n')
  tls.key: $(cat server-key.pem | base64 | tr -d '\n')
metadata:
  name: hiveapi-serving-cert
  namespace: hive
type: kubernetes.io/tls
EOF


popd

