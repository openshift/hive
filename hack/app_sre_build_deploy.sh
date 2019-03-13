#!/bin/bash

# AppSRE team CD

set -exv

GIT_HASH=`git rev-parse --short=7 HEAD`
GIT_COMMIT_COUNT=`git rev-list 9c56c62c6d0180c27e1cc9cf195f4bbfd7a617dd..HEAD --count`

BASE_IMG="hive"
IMG="${BASE_IMG}:latest"

QUAY_IMAGE="quay.io/app-sre/${BASE_IMG}"

# build the image
BUILD_CMD="docker build" IMG="$IMG" make docker-build

# push the image
skopeo copy --dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
    "docker-daemon:${IMG}" \
    "docker://${QUAY_IMAGE}:latest"

skopeo copy --dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
    "docker-daemon:${IMG}" \
    "docker://${QUAY_IMAGE}:${GIT_HASH}"

# clone bundle repo
SAAS_OPERATOR_DIR="saas-hive-operator-bundle"
BUNDLE_DIR="$SAAS_OPERATOR_DIR/hive/"
BRANCH_CHANNEL="staging"

# trap "rm -f $SAAS_OPERATOR_DIR" EXIT TERM INT
rm -rf "$SAAS_OPERATOR_DIR"

git clone \
    --branch $BRANCH_CHANNEL \
    https://app:${APP_SRE_BOT_PUSH_TOKEN}@github.com/app-sre/saas-hive-operator-bundle.git \
    $SAAS_OPERATOR_DIR

# generate bundle
PREV_VERSION=$(ls $BUNDLE_DIR | sort -t . -k 3 -g | tail -n 1)

./hack/generate-operator-bundle.py \
    $BUNDLE_DIR \
    $PREV_VERSION \
    $GIT_COMMIT_COUNT \
    $GIT_HASH \
    $QUAY_IMAGE:$GIT_HASH

# create package yaml
NEW_VERSION=$(ls $BUNDLE_DIR | sort -t . -k 3 -g | tail -n 1)
cat <<EOF > $BUNDLE_DIR/hive.package.yaml
packageName: hive-operator
channels:
- name: $BRANCH_CHANNEL
  currentCSV: hive-operator.v${NEW_VERSION}
EOF

# add, commit & push
pushd $SAAS_OPERATOR_DIR

git add .

MESSAGE="add version $GIT_COMMIT_COUNT-$GIT_HASH

replaces $PREV_VERSION"

git commit -m "$MESSAGE"

git push origin "$BRANCH_CHANNEL"

popd

# build the registry image
REGISTRY_IMG="quay.io/app-sre/hive-registry"
DOCKERFILE_REGISTRY="Dockerfile.olm-registry"

cat <<EOF > $DOCKERFILE_REGISTRY
FROM quay.io/openshift/origin-operator-registry:latest

COPY $SAAS_OPERATOR_DIR manifests
RUN initializer

CMD ["registry-server", "-t", "/tmp/terminate.log"]
EOF

docker build -f $DOCKERFILE_REGISTRY --tag "${REGISTRY_IMG}:${BRANCH_CHANNEL}-latest" .

# push image
skopeo copy --dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
    "docker-daemon:${REGISTRY_IMG}:${BRANCH_CHANNEL}-latest" \
    "docker://${REGISTRY_IMG}:${BRANCH_CHANNEL}-latest"

skopeo copy --dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
    "docker-daemon:${REGISTRY_IMG}:${BRANCH_CHANNEL}-latest" \
    "docker://${REGISTRY_IMG}:${BRANCH_CHANNEL}-${GIT_HASH}"
