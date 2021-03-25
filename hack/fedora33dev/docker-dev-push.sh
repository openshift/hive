#!/bin/sh
set -e

# exit if IMG is not set, should be a ref to a registry you can push to (i.e. quay.io/user/hive:latest)
set -u
: "$IMG"

# TODO: update not strictly required here unless dealing with manifest changes
make update build
docker build -t ${IMG} -f hack/fedora33dev/Dockerfile.dev .
docker push ${IMG}
kubectl delete pod -n hive -l control-plane=clustersync --wait=false
kubectl delete pod -n hive -l control-plane=controller-manager --wait=false
kubectl delete pod -n hive -l control-plane=hive-operator --wait=false
kubectl delete pod -n hive -l app=hiveadmission --wait=false
