#!/bin/sh
#
# This script generates a full OLM ClusterServiceVersion, bundle and package for your
# current git HEAD.
#
# Requires yq: https://github.com/mikefarah/yq
set -e


if [ -z "$REGISTRY_IMG" ]
then
      echo "Please specify REGISTRY_IMG"
      exit 1
fi

if [ -z "$DEPLOY_IMG" ]
then
      echo "Please specify DEPLOY_IMG"
      exit 1
fi

oc project hive

# Use commits since some revision/tag to get an ever increasing version number:
GIT_COMMIT_COUNT=`git rev-list 9c56c62c6d0180c27e1cc9cf195f4bbfd7a617dd..HEAD --count`

# Git hash is appended to our version:
GIT_HASH=`git rev-parse --short HEAD`

USE_CHANNEL=${CHANNEL:-staging}

# Use a fake replaces version, we will not be using this.
./hack/generate-operator-bundle.py bundle/ 0.1.5-4b53b492 $GIT_COMMIT_COUNT $GIT_HASH $DEPLOY_IMG

CSV_FILE="bundle/0.1.$GIT_COMMIT_COUNT-sha$GIT_HASH/hive-operator.v0.1.$GIT_COMMIT_COUNT-sha$GIT_HASH.clusterserviceversion.yaml"

CSV_NAME=`yq r $CSV_FILE metadata.name`
echo "ClusterServiceVersion name: $CSV_NAME"

# Remove the replaces field, this script is just for dev testing, we don't plan to update a version:
yq d -i $CSV_FILE spec.replaces

cat <<EOF > bundle/hive.package.yaml
packageName: hive-operator
channels:
- name: staging
  currentCSV: $CSV_NAME
EOF

echo "OLM Operator Package written to: bundle/"

REGISTRY_IMG_PATH="${REGISTRY_IMG}:${USE_CHANNEL}-latest"

# Build the registry image:
sudo buildah bud --file build/olm-registry/Dockerfile --tag "${REGISTRY_IMG_PATH}" .

# Push the registry image:
sudo podman push "${REGISTRY_IMG_PATH}"

# Create the remaining OLM artifacts to subscribe to the operator:
oc process -f hack/olm-registry/olm-artifacts-template.yaml REGISTRY_IMG=$REGISTRY_IMG CHANNEL=$USE_CHANNEL | kubectl apply -f -
