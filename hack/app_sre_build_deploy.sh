#!/bin/bash

# AppSRE team CD

set -exv

CURRENT_DIR=$(dirname $0)

BASE_IMG="hive"
QUAY_IMAGE="quay.io/app-sre/${BASE_IMG}"
IMG="${BASE_IMG}:latest"

GIT_HASH=`git rev-parse --short=7 HEAD`

# build the image
BUILD_CMD="docker build" IMG="$IMG" make docker-build

# push the image
skopeo copy --dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
    "docker-daemon:${IMG}" \
    "docker://${QUAY_IMAGE}:latest"

skopeo copy --dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
    "docker-daemon:${IMG}" \
    "docker://${QUAY_IMAGE}:${GIT_HASH}"

# IMPORTANT: DO NOT MERGE THIS CHANGE BACK TO MASTER
# we need the count to be synced with deploments from master branch.
# since this branch is used temporarily for deploying hotfixes, and we
# use the number of commits from a past commit for deployment ordering -
# set this variable to the merge commit into master to help with ordering
GIT_COMMIT_CHERRY_PICK="ed7dfcb60d719846815e1f7be88a8060cc76949c"

# create and push staging image catalog
$CURRENT_DIR/app_sre_create_image_catalog.sh staging "$QUAY_IMAGE" "$GIT_COMMIT_CHERRY_PICK"

# create and push production image catalog
REMOVE_UNDEPLOYED=true $CURRENT_DIR/app_sre_create_image_catalog.sh production "$QUAY_IMAGE" "$GIT_COMMIT_CHERRY_PICK"
