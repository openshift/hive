#!/bin/sh
set -e
IMG=quay.io/dgoodwin/hive:latest

# TODO: update not strictly required here unless dealing with manifest changes
make update build
docker build -t ${IMG} -f hack/fedora33dev/Dockerfile.dev .
docker push ${IMG}
kubectl delete pod -n hive -l control-plane=clustersync --wait=false
kubectl delete pod -n hive -l control-plane=controller-manager --wait=false
kubectl delete pod -n hive -l control-plane=hive-operator --wait=false
kubectl delete pod -n hive -l app=hiveadmission --wait=false
