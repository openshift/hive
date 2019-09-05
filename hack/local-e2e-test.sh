#!/bin/bash

set -e

# Example script to launch a Hive e2e test on your own cluster

# Image to use for hive operator and controllers
export HIVE_IMAGE="registry.svc.ci.openshift.org/openshift/hive-v4.0:hive"

# Release image to use. If using the one from CI, your pull secret must include valid creds for api.ci
export RELEASE_IMAGE="registry.svc.ci.openshift.org/origin/release:4.2"

# Namespace where e2e cluster will be created
export CLUSTER_NAMESPACE="hive-e2e"

# Base domain for e2e cluster
export BASE_DOMAIN="new-installer.openshift.com"

# Where artifacts from the test will be placed
export ARTIFACT_DIR="/tmp"

# Public SSH key for the cluster
export SSH_PUBLIC_KEY_FILE="${HOME}/.ssh/id_rsa.pub"

# Pull secret to use for installing the cluster
export PULL_SECRET_FILE="${HOME}/.pull-secret"

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
${DIR}/e2e-test.sh
