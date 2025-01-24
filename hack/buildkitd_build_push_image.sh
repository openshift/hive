#!/usr/bin/env bash
HIVE_ROOT="$(git rev-parse --show-toplevel)"
CNI_PATH="$HIVE_ROOT"/.tmp/_output/cni/bin
export HIVE_ROOT
export CNI_PATH
export PATH=$HIVE_ROOT/.tmp/_output/bin:$PATH

touch "$HIVE_ROOT/.tmp/_output/config.json"

buildctl --addr unix:///run/user/$UID/buildkit/buildkitd.sock build \
  --frontend dockerfile.v0 \
  --local context=. \
  --local dockerfile=. \
  --secret id=docker,src="$HIVE_ROOT/.tmp/_output/config.json" \
  --opt build-arg:EL8_BUILD_IMAGE=registry.ci.openshift.org/openshift/release:golang-1.22 \
  --opt build-arg:EL9_BUILD_IMAGE=registry.ci.openshift.org/openshift/release:golang-1.22 \
  --opt build-arg:BASE_IMAGE=quay.io/centos/centos:stream9 \
  --opt build-arg:GO="CGO_ENABLED=0 go" \
  --output type=image,name=localhost:5000/hive:latest,push=true
