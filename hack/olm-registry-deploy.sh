#!/bin/bash

# This developer script:
# - builds a hive image from the current hive git checkout
# - publishes it (HIVE_IMAGE)
# - generates a full OLM ClusterServiceVersion, bundle, and package
# - builds a registry image (aka Catalog Source) from that bundle
# - publishes it (REGISTRY_IMG)
# - applies the operator using `oc process`
#
# This is run ad-hoc by developers, not by CI.

set -exv

if [ -z "$HIVE_IMAGE" ]
then
      echo "Please specify the HIVE_IMAGE to deploy, for example quay.io/<yourname>/hive (no tag)"
      exit 1
fi

if [ -z "$REGISTRY_IMG" ]
then
      echo "Please specify where to push the registry image as REGISTRY_IMG, for example quay.io/<yourname>/hive-registry"
      exit 1
fi

BUNDLE_DIR=$(mktemp -d -p .)
GIT_HASH=`git rev-parse --short=7 HEAD`
IMG="${HIVE_IMAGE}:${GIT_HASH}"
USE_CHANNEL=${CHANNEL:-staging}
REGISTRY_IMG_PATH="${REGISTRY_IMG}:${USE_CHANNEL}-latest"
DOCKERFILE_REGISTRY="Dockerfile.olm-registry"

# set the project
oc project ${NAMESPACE:-hive}

# build the image
BUILD_CMD="docker build" IMG="${IMG}" make docker-build

# build the bundle
./hack/generate-operator-bundle-operatorhub.py osd --previous-version 0.0.1 --hive-image ${IMG} --channel ${USE_CHANNEL} --bundle-dir ${BUNDLE_DIR}

# remove the replaces field -- this is a fresh install in OLM, so there must be no 'replaces:' field in the CSV
NEW_VERSION=$((cd ${BUNDLE_DIR} && find -type d | xargs -n 1 basename) | sort -V  | tail -n 1)
CSV_FILE=$(ls -1 "${BUNDLE_DIR}/${NEW_VERSION}" | grep clusterserviceversion.yaml | head -n 1 )
docker run --rm -v "$(pwd)/${BUNDLE_DIR}/${NEW_VERSION}":/workdir mikefarah/yq yq d -i ${CSV_FILE} spec.replaces

# write the registry Dockerfile
cat <<EOF > $DOCKERFILE_REGISTRY
FROM quay.io/openshift/origin-operator-registry:latest

COPY ${BUNDLE_DIR} manifests
RUN initializer

CMD ["registry-server", "-t", "/tmp/terminate.log"]
EOF

# build the registry image:
docker build --file "${DOCKERFILE_REGISTRY}" --tag "${REGISTRY_IMG_PATH}" .

# push the image
docker push "${IMG}"

# push the registry image:
docker push "${REGISTRY_IMG_PATH}"

# create the remaining OLM artifacts to subscribe to the operator:
oc process -f hack/olm-registry/olm-artifacts-template.yaml REGISTRY_IMG=$REGISTRY_IMG CHANNEL=$USE_CHANNEL | kubectl apply -f -
