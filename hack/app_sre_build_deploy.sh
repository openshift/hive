#!/bin/bash

# AppSRE team CD

set -exv

CURRENT_DIR=$(dirname $0)

BASE_IMG="hive"
QUAY_IMAGE="quay.io/app-sre/${BASE_IMG}"
IMG="${BASE_IMG}:latest"

GIT_HASH=`git rev-parse --short=7 HEAD`

## image_exits_in_repo IMAGE_URI
#
# Checks whether IMAGE_URI -- e.g. quay.io/app-sre/osd-metrics-exporter:abcd123
# -- exists in the remote repository.
# If so, returns success.
# If the image does not exist, but the query was otherwise successful, returns
# failure.
# If the query fails for any reason, prints an error and *exits* nonzero.
image_exists_in_repo() {
    local image_uri=$1
    local output
    local rc

    local skopeo_stderr=$(mktemp)

    output=$(skopeo inspect docker://${image_uri} 2>$skopeo_stderr)
    rc=$?
    # So we can delete the temp file right away...
    stderr=$(cat $skopeo_stderr)
    rm -f $skopeo_stderr
    if [[ $rc -eq 0 ]]; then
        # The image exists. Sanity check the output.
        local digest=$(echo $output | jq -r .Digest)
        if [[ -z "$digest" ]]; then
            echo "Unexpected error: skopeo inspect succeeded, but output contained no .Digest"
            echo "Here's the output:"
            echo "$output"
            echo "...and stderr:"
            echo "$stderr"
            exit 1
        fi
        echo "Image ${image_uri} exists with digest $digest."
        return 0
    elif [[ "$stderr" == *"manifest unknown"* ]]; then
        # We were able to talk to the repository, but the tag doesn't exist.
        # This is the normal "green field" case.
        echo "Image ${image_uri} does not exist in the repository."
        return 1
    elif [[ "$stderr" == *"was deleted or has expired"* ]]; then
        # This should be rare, but accounts for cases where we had to
        # manually delete an image.
        echo "Image ${image_uri} was deleted from the repository."
        echo "Proceeding as if it never existed."
        return 1
    else
        # Any other error. For example:
        #   - "unauthorized: access to the requested resource is not
        #     authorized". This happens not just on auth errors, but if we
        #     reference a repository that doesn't exist.
        #   - "no such host".
        #   - Network or other infrastructure failures.
        # In all these cases, we want to bail, because we don't know whether
        # the image exists (and we'd likely fail to push it anyway).
        echo "Error querying the repository for ${image_uri}:"
        echo "stdout: $output"
        echo "stderr: $stderr"
        exit 1
    fi
}

if image_exists_in_repo "${QUAY_IMAGE}:${GIT_HASH}"; then
    echo "Skipping image build/push for ${QUAY_IMAGE}:${GIT_HASH}"
    exit 0
fi

# build the image
BUILD_CMD="docker build" IMG="$IMG" make GO_REQUIRED_MIN_VERSION:= docker-build

# push the image
skopeo copy --dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
    "docker-daemon:${IMG}" \
    "docker://${QUAY_IMAGE}:latest"

skopeo copy --dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
    "docker-daemon:${IMG}" \
    "docker://${QUAY_IMAGE}:${GIT_HASH}"
