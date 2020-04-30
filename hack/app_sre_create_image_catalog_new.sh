#!/bin/bash

set -exv

BRANCH_CHANNEL="production"
QUAY_IMAGE="quay.io/gshereme/hive"

if [[ -z "$OLD_DEPLOYMENT" ]]; then
    echo "OLD_DEPLOYMENT must be defined" 1>&2
    exit 1
fi

if [[ -z "$NEW_DEPLOYMENT" ]]; then
    echo "NEW_DEPLOYMENT must be defined" 1>&2
    exit 1
fi

OLD_BUNDLE_DIR=$(mktemp -d)
NEW_BUNDLE_DIR="$(mktemp -d -p .)/hive"
HIVE_BUNDLE_DIR="bundle"

mkdir -p "${NEW_BUNDLE_DIR}"

# TODO change to correct bundle URL
git clone \
    --branch $BRANCH_CHANNEL \
    git@gitlab.cee.redhat.com:gshereme/saas-hive-operator-bundle.git \
    $OLD_BUNDLE_DIR

# build a new bundle of 2 (old and new). First, grab the old:
pushd "${OLD_BUNDLE_DIR}/hive"
PREV_VERSION=$(find . -name "*${OLD_DEPLOYMENT:0:7}" | xargs -n 1 basename)
popd
cp -r "${OLD_BUNDLE_DIR}/hive/${PREV_VERSION}" "${NEW_BUNDLE_DIR}/"

pushd "${NEW_BUNDLE_DIR}/${PREV_VERSION}"
OLD_CSV_FILE=$(ls -1 hive-operator*clusterserviceversion.yaml)

# remove any replaces field in the old bundle
podman run --rm -v .:/workdir mikefarah/yq yq d -i ${OLD_CSV_FILE} spec.replaces
popd

# generate bundle
./hack/generate-operator-bundle-operatorhub.py osd --previous-version $PREV_VERSION --hive-image ${QUAY_IMAGE}:${NEW_DEPLOYMENT:0:7} --channel $BRANCH_CHANNEL
NEW_VERSION=$((cd ${HIVE_BUNDLE_DIR} && find -type d | xargs -n 1 basename) | sort -V  | tail -n 1)

# copy new csv
cp -ax $HIVE_BUNDLE_DIR/$NEW_VERSION $NEW_BUNDLE_DIR/
# copy package
cp $HIVE_BUNDLE_DIR/hive.package.yaml $NEW_BUNDLE_DIR/

# replace the saas-hive-operator-bundle content with this new bundle content
pushd "${OLD_BUNDLE_DIR}/hive" && rm -rf * && popd
cp -r "${NEW_BUNDLE_DIR}/" "${OLD_BUNDLE_DIR}/"
pushd "${OLD_BUNDLE_DIR}/"
ls -lart
git add -A .
git commit -m "bundle for ${OLD_DEPLOYMENT:0:7} --> ${NEW_DEPLOYMENT:0:7}"
git push
popd

# build the registry image
REGISTRY_IMG="quay.io/gshereme/hive-registry"
DOCKERFILE_REGISTRY="Dockerfile.olm-registry"

cat <<EOF > $DOCKERFILE_REGISTRY
FROM quay.io/openshift/origin-operator-registry:latest

COPY $NEW_BUNDLE_DIR manifests
RUN initializer

CMD ["registry-server", "-t", "/tmp/terminate.log"]
EOF

docker build -f $DOCKERFILE_REGISTRY --tag "${REGISTRY_IMG}:${BRANCH_CHANNEL}-${NEW_DEPLOYMENT:0:7}" .

# TODO uncomment
# # push image
# skopeo copy --dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
#     "docker-daemon:${REGISTRY_IMG}:${BRANCH_CHANNEL}-latest" \
#     "docker://${REGISTRY_IMG}:${BRANCH_CHANNEL}-latest"

# skopeo copy --dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
#     "docker-daemon:${REGISTRY_IMG}:${BRANCH_CHANNEL}-latest" \
#     "docker://${REGISTRY_IMG}:${BRANCH_CHANNEL}-${GIT_HASH}"
