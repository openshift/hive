# Deploy OpenShift Hive for Service Delivery

## Description

The purpose of this doc is to show how to do a deployment of OpenShift Hive for Service Delivery.

## Release an update payload

- Set up some useful variables:
  ```console
  $ RELEASE_VER=$(date +%Y%m%d)
  $ RELEASE_IMAGE=quay.io/twiest/openshift
  $ RELEASE_IMAGE_TAG="${RELEASE_IMAGE}:${RELEASE_VER}"
  ```

- Create the release:
  ```console
  $ oc adm release new --from-release registry.svc.ci.openshift.org/openshift/origin-release:v4.0 --to-image "${RELEASE_IMAGE_TAG}" --mirror "${RELEASE_IMAGE}"
  ```

## Build the hive-controller Image
- Set up some useful variables:
  ```console
  $ HIVE_GOPATH=/path/to/openshift/installer/gopath
  $ HIVE_SRC=${HIVE_GOPATH}/src/github.com/openshift/hive
  $ HIVE_IMAGE_TAG=quay.io/twiest/hive-controller:${RELEASE_VER}
  $ AUTHFILE=~/.docker/config.json
  ```

- Get the latest Hive code:
  ```console
  $ cd "${HIVE_SRC}"
  $ git checkout master
  $ git fetch upstream
  $ git reset --hard upstream/master
  ```

- Build the Hive container:
  ```console
  $ cd "${HIVE_SRC}"
  $ GOPATH="${HIVE_GOPATH}" IMG="${HIVE_IMAGE_TAG}" make buildah-build
  ```

- Push the Hive image to quay:
  ```console
  $ BUILDAH_ISOLATION=chroot sudo buildah push --authfile ${AUTHFILE} ${HIVE_IMAGE_TAG} docker://${HIVE_IMAGE_TAG}
  ```

## Change the OpenShift Hive SD Deploy to point to the new images
- Set up some useful variables:
  ```console
  $ HIVE_SD_RELEASE_BRANCH="sd-release-${RELEASE_VER}"
  ```

- Create branch for PR:
  ```console
  $ cd "${HIVE_SRC}"
  $ git checkout master
  $ git fetch upstream
  $ git reset --hard upstream/master
  $ git checkout -b "${HIVE_SD_RELEASE_BRANCH}"
  $ git push -u origin "${HIVE_SD_RELEASE_BRANCH}"
  ```

- Update image versions in Kustomize and README:
  ```console
  $ cd "${HIVE_SRC}"
  $ sed -r -i -e "s%quay.io/twiest/hive-controller:[0-9]{8}%${HIVE_IMAGE_TAG}%" README.md config/overlays/sd-dev/image_patch.yaml
  $ sed -r -i -e "s%quay.io/twiest/openshift:[0-9]{8}%${RELEASE_IMAGE_TAG}%" README.md config/overlays/sd-dev/image_patch.yaml
  ```

- Commit and push changes
  ```console
  $ git commit -a -m "Update SD image links to latest release."
  $ git push origin
  ```

- Create PR in github for the new release


## Deploy the new OpenShift Hive and Installer builds to the SD cluster
- Login using `oc` to the SD cluster (aka opshive)
- Remove all existing Hive objects (clusterdeployments, dnszones, etc). We're not currently trying to be backwards compatible, so this is necessary.
- Deploy the new OpenShift Hive build:
  ```console
  $ cd "${HIVE_SRC}"
  $ GOPATH="${HIVE_GOPATH}" make deploy-sd-dev
  ```
- Test deploying a cluster using the instructions in the README. Make sure to use the instructions that use remote images and that the remote images are the new ones that were just built.

- Notify the SD guys (@Juan Hernandez @cben @Elad) in the `#service-development` channel that the new OpenShift Hive and Installer release is ready for them to consume.
