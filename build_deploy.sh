#!/bin/bash

# AppSRE team CD

set -exv

GIT_HASH=`git rev-parse --short=7 HEAD`
GIT_COMMIT_COUNT=`git rev-list 9c56c62c6d0180c27e1cc9cf195f4bbfd7a617dd..HEAD --count`

BASE_IMG="hive"
IMG="${BASE_IMG}:latest"
QUAY_IMAGE="quay.io/app-sre/${BASE_IMG}"

BUNDLE_DIR="bundle"

# Build the image
BUILD_CMD="docker build" IMG="$IMG" make docker-build

# Push the image
skopeo copy --dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
    "docker-daemon:${IMG}" \
    "docker://${QUAY_IMAGE}:latest"

skopeo copy --dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
    "docker-daemon:${IMG}" \
    "docker://${QUAY_IMAGE}:${GIT_HASH}"

# Clone bundle repo
SAAS_OPERATOR_DIR="saas-hive-operator-bundle"
BUNDLE_DIR="$SAAS_OPERATOR_DIR/hive/"
BRANCH_CHANNEL="staging"

trap "rm -f $SAAS_OPERATOR_DIR" EXIT TERM INT
rm -rf "$SAAS_OPERATOR_DIR"

git clone \
    --branch $BRANCH_CHANNEL \
    https://app:${APP_SRE_BOT_PUSH_TOKEN}@github.com/app-sre/saas-hive-operator-bundle.git \
    $SAAS_OPERATOR_DIR

# generate bundle
LAST_BUNDLE=$(ls $BUNDLE_DIR | sort -t . -k 3 -g | tail -n 1)

./hack/generate-operator-bundle.py \
    $BUNDLE_DIR \
    $LAST_BUNDLE \
    $GIT_COMMIT_COUNT \
    $GIT_HASH \
    $QUAY_IMAGE

(
    cd $SAAS_OPERATOR_DIR

    # add, commit & push
    git add .

    MESSAGE=$'add version $GIT_COMMIT_COUNT-$GIT_HASH\n\nreplaces $LAST_BUNDLE'
    git commit -m "$MESSAGE"

    git push origin "$BRANCH_CHANNEL"
)

# TODO: create and push operator catalog container
