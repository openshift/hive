#!/bin/bash

set -exv

BRANCH_CHANNEL="$1"
QUAY_IMAGE="$2"

GIT_HASH=`git rev-parse --short=7 HEAD`

SAAS_OPERATOR_DIR="saas-hive-operator-bundle"
SAAS_OPERATOR_BUNDLE_DIR="$SAAS_OPERATOR_DIR/hive/"
HIVE_BUNDLE_DIR="bundle/"

rm -rf "$SAAS_OPERATOR_DIR"

git clone \
    --branch $BRANCH_CHANNEL \
    https://app:${APP_SRE_BOT_PUSH_TOKEN}@gitlab.cee.redhat.com/service/saas-hive-operator-bundle.git \
    $SAAS_OPERATOR_DIR


# generate bundle
PREV_VERSION=$((cd $SAAS_OPERATOR_BUNDLE_DIR && find -type d | xargs -n 1 basename) | sort -V  | tail -n 1)

./hack/generate-operator-bundle.py osd --previous-version $PREV_VERSION --hive-image $QUAY_IMAGE:$GIT_HASH --channel $BRANCH_CHANNEL

NEW_VERSION=$((cd $HIVE_BUNDLE_DIR && find -type d | xargs -n 1 basename) | sort -V  | tail -n 1)

if [ "$NEW_VERSION" = "$PREV_VERSION" ]; then
    # stopping script as that version was already built, so no need to rebuild it
    exit 0
fi

# copy new csv
cp -ax $HIVE_BUNDLE_DIR/$NEW_VERSION $SAAS_OPERATOR_BUNDLE_DIR/

# copy package
cp $HIVE_BUNDLE_DIR/hive.package.yaml $SAAS_OPERATOR_BUNDLE_DIR/

# add, commit & push
pushd $SAAS_OPERATOR_DIR

git add .

MESSAGE="add version $NEW_VERSION

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
