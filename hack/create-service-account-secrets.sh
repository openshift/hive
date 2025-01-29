#!/bin/sh
#
# Secrets are no longer automatically created for service accounts in Kube 1.24+.
# This script creates service account secrets for the following serviceaccounts.
# - hiveadmission
# - hive-controllers
#
cat <<EOF | oc apply -f -
apiVersion: v1
kind: List
items:
- apiVersion: v1
  kind: Secret
  type: kubernetes.io/service-account-token
  metadata:
    name: hiveadmission
    namespace: hive
    annotations:
      kubernetes.io/service-account.name: "hiveadmission"
- apiVersion: v1
  kind: Secret
  type: kubernetes.io/service-account-token
  metadata:
    name: hive-controllers
    namespace: hive
    annotations:
      kubernetes.io/service-account.name: "hive-controllers"
EOF
