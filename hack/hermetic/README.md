# Hermetic builds for hive

- [Overview](#overview)
- [Updating](#updating)
- [Manual Process](#manual-process)

## Overview
"Hermetic" in this context means the pieces that went into the build must provably be exactly what we expect.
This is in contrast to building in a container and accepting whatever operating system libraries happen to be there.
The idea is to minimize ambiguities/variances, and thus the incidence of attack vectors, in (this part of) the supply chain.

The mechanism is to maintain a list ("lock file") of all the RPMs we require/expect in the build container, pinned at specific versions, accompanied by metadata (e.g. checksums) to validate that the contents have not been altered. Our [konflux configs](../../.tekton/) are wired to use the contents of this directory to enforce these package versions during the build.

## Updating
MintMaker is [configured](../../renovate.json) to propose PRs to keep our lockfile up to date.

If that fails or lags behind, we have a never-run [periodic](https://github.com/openshift/release/blob/c36cb7de5b6c2f66cc54e27802d128bbee678bb0/ci-operator/jobs/infra-periodics.yaml#L3479) that can be triggered on demand.

And as a last resort, the process to do it manually is described [below](#manual-process) (though you may be better off using the script embedded in the periodic).

## Manual Process

https://konflux.pages.redhat.com/docs/users/building/activation-keys-subscription.html#configuring-an-rpm-lockfile-for-hermetic-builds

* Basically just follow the instructions
* Files referenced in the instructions live in the directory containing this README. Path accordingly.
* Step 1: As of this writing, the ubi-minimal base image from the hive root dockerfile currently does *not* have subscription-manager in it. You can copy it from your system into your container's mount point *or* use a different image (of the same RHEL version) e.g.: `podman run -v ${PWD}:/source/:z -it registry.access.redhat.com/ubi9/ubi:latest bash`
* Step 2: A viable activation key can be found in our bitwarden account. (I think you can instead create a fresh one following the instructions, but I have not tried it.)
* Steps 3, 5, and 6 can be skippped unless `redhat.repo` is out of date and needs to be updated
* Step 7:
  * I logged into skopeo outside of the base image running container, and saved the auth file in this directory (which is mounted inside the container)
  * Instead of logging in from within the container, `export REGISTRY_AUTH_FILE=<path to auth file mounted inside container>`
* Step 8 can be skipped unless `rpms.in.yaml` needs to be updated
* In step 9, I ran into the SSL errors and had to fix the path under `/etc/pki/entitlement` in the `redhat.repo` file. YMMV. (The directory name may or may not be unique to the container image from which you're running this procedure.)
