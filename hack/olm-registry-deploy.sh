#!/bin/bash

# This developer script:
# - builds a hive image from git HEAD
# - publishes it (HIVE_IMAGE)
# - generates a full OLM ClusterServiceVersion, bundle, and package ad builds a registry image
# - publishes it (REGISTRY_IMG)
# - applies the operator using `oc process`
#
# This is run ad-hoc by developers, not by CI.

set -exv

if [ -z "$HIVE_IMAGE" ]
then
      echo "Please specify the operand HIVE_IMAGE to deploy, for example quay.io/<yourname>/hive (no tag)"
      exit 1
fi

if [ -z "$REGISTRY_IMG" ]
then
      echo "Please specify where to push the registry image as REGISTRY_IMG, for example quay.io/<yourname>/hive-registry"
      exit 1
fi

if [ ! -d "bundle" ]
then
      echo "Can't find bundle directory. Please run this script from the hive code directory root."
      exit 1
fi

oc project ${NAMESPACE:-hive}

GIT_HASH=`git rev-parse --short=7 HEAD`
IMG="${HIVE_IMAGE}:${GIT_HASH}"

# build the image
printf "\n\n\n*** NOTE: sometimes buildah takes a while on the copy. Please be patient...\n\n\n\n"
#BUILD_CMD="buildah bud" IMG="${IMG}" make docker-build

# push the image
podman push "${IMG}"

USE_CHANNEL=${CHANNEL:-staging}

./hack/generate-operator-bundle.py osd --previous-version 0.0.1 --hive-image ${IMG} --channel ${USE_CHANNEL}

NEW_VERSION=$((cd bundle && find -type d | xargs -n 1 basename) | sort -V  | tail -n 1)

# grab the bundle we just created -- don't need all of them
TMP_DIR=$(mktemp -d)
mkdir -p "${TMP_DIR}/bundle"
cp -ax "bundle/${NEW_VERSION}" "${TMP_DIR}/bundle"
cp bundle/hive.package.yaml "${TMP_DIR}/bundle"
CSV_FILE=$(ls -1 "${TMP_DIR}/bundle/${NEW_VERSION}" | grep clusterserviceversion.yaml | head -n 1 )
# remove the replaces field -- this is a fresh install in OLM
podman run --rm -v "${TMP_DIR}/bundle/${NEW_VERSION}":/workdir mikefarah/yq yq d -i ${CSV_FILE} spec.replaces

# Build the registry image:
REGISTRY_IMG_PATH="${REGISTRY_IMG}:${USE_CHANNEL}-latest"
buildah bud --file build/olm-registry/Dockerfile --tag "${REGISTRY_IMG_PATH}" "${TMP_DIR}"

# Push the registry image:
podman push "${REGISTRY_IMG_PATH}"

# Create the remaining OLM artifacts to subscribe to the operator:
oc process -f hack/olm-registry/olm-artifacts-template.yaml REGISTRY_IMG=$REGISTRY_IMG CHANNEL=$USE_CHANNEL | kubectl apply -f -
