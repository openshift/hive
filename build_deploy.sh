#!/bin/bash

# AppSRE team CD

set -exv

BASE_IMG="hive"
IMG="${BASE_IMG}:latest"

BUILD_CMD="docker build" IMG="$IMG" make docker-build

DEST_IMG="quay.io/app-sre/${BASE_IMG}"

skopeo copy --dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
    "docker-daemon:${IMG}" \
    "docker://${DEST_IMG}:latest"

skopeo copy --dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
    "docker-daemon:${IMG}" \
    "docker://${DEST_IMG}:$(git rev-parse --short=7 HEAD)"
